# Event Ticketing System

A full-stack event ticketing platform built as a distributed-systems study: four Go microservices
behind an API gateway, PostgreSQL for durable state, Stripe for payments, and Kafka for asynchronous
fan-out of purchase and cancellation events.

The interesting problem here is **selling a fixed pool of tickets to concurrent buyers without ever
overselling**, while keeping payment charges idempotent and downstream consumers eventually
consistent. Most of this document is about how that is done and why.

---

## Status

| Phase | Scope | State |
|---|---|---|
| **0** | Reproducible local stack (`docker compose up` works end to end) | ✅ Complete |
| **1** | Concurrency correctness + k6 load validation | ✅ Complete |
| **2** | Stripe charge/refund with idempotency keys | ✅ Complete |
| **3** | Kafka async flows via transactional outbox | ✅ Complete |
| **4** | Frontend purchase/cancel flows + observability | ⬜ Not started |

Phase detail, including acceptance criteria, is in [Delivery Phases](#delivery-phases).

---

## Architecture

```text
                        ┌──────────────────────────┐
                        │   Next.js Frontend       │
                        │   localhost:3000         │
                        └────────────┬─────────────┘
                                     │ HTTPS/JSON (gateway only)
                                     ▼
                        ┌──────────────────────────┐
                        │      API Gateway :8000   │
                        │  CORS · JWT · logging    │
                        │  prefix-based routing    │
                        └────┬────────┬────────┬───┘
                             │        │        │
              ┌──────────────┘        │        └──────────────┐
              ▼                       ▼                       ▼
     ┌─────────────────┐    ┌─────────────────┐    ┌─────────────────────┐
     │  User Service   │    │  Event Service  │    │   Ticket Service    │
     │      :8081      │    │      :8082      │    │        :8083        │
     │  register/login │    │  catalog CRUD   │    │  inventory·purchase │
     │  JWT issuance   │    │                 │    │  cancel·refund·QR   │
     └────────┬────────┘    └────────┬────────┘    └──────┬───────┬──────┘
              │                      │                    │       │
              └──────────────────────┴────────────────────┘       │ charge/refund
                                     │                            ▼
                                     ▼                   ┌──────────────────┐
                          ┌────────────────────┐         │   Stripe API     │
                          │    PostgreSQL      │         │   (test mode)    │
                          │    :5433 → 5432    │         └──────────────────┘
                          │  users · events    │
                          │  tickets · outbox  │
                          │  payments · idem   │
                          └─────────┬──────────┘
                                    │ polled by relay
                                    ▼
                          ┌────────────────────┐        ┌─────────────────────┐
                          │   Outbox Relay     │───────▶│   Kafka (KRaft)     │
                          │  (ticket-service)  │        │  ticket.purchased   │
                          └────────────────────┘        │  ticket.cancelled   │
                                                        └──────────┬──────────┘
                                                                   ▼
                                                        ┌─────────────────────┐
                                                        │ Notification Worker │
                                                        │  consumer group     │
                                                        └─────────────────────┘
```

### Service responsibilities

| Service | Owns | Notes |
|---|---|---|
| **api-gateway** | Edge concerns | Sole entry point for the browser. CORS, JWT verification, request logging, prefix routing (`/users`, `/events`, `/tickets`). Forwards the full path unchanged. |
| **user-service** | Identity | Registration, login, JWT issuance (`user_id`, `email`, `exp`), `is_admin` flag. |
| **event-service** | Catalog | Event creation and listing. Owns the definition of "upcoming" (`start_time >= now`). |
| **ticket-service** | Inventory + money | The critical path: minting inventory, the purchase transaction, cancellation/refund, QR receipts, and the outbox relay. |

Services communicate over HTTP through the gateway; the ticket service talks to Stripe directly and
publishes to Kafka via the outbox relay. This is a single shared database in dev — see
[Known simplifications](#known-simplifications).

---

## Design decisions

This section is the point of the project. Each decision below has a defensible alternative that was
considered and rejected.

### 1. Two isolation levels, chosen per workload

A single blanket isolation level is the wrong answer here, because the two write paths have opposite
characteristics. The system deliberately uses different strategies for each.

**Purchase — `READ COMMITTED` + `SELECT … FOR UPDATE SKIP LOCKED`**

Claiming a ticket is a high-contention operation on one hot table. Every concurrent buyer for a given
event targets the same set of rows. The claim is:

```sql
SELECT id FROM tickets
 WHERE event_id = $1 AND status = 'available'
 ORDER BY id
 LIMIT 1
 FOR UPDATE SKIP LOCKED;
```

`SKIP LOCKED` is what makes this scale. Without it, 100 concurrent buyers serialize into a single
queue behind one row lock, and — worse — PostgreSQL re-evaluates the `WHERE` clause after each lock
is released. Combined with `LIMIT 1`, a waiting transaction wakes up, finds its candidate row now
`purchased`, and returns **zero rows** even though inventory remains. That surfaces as a spurious
"sold out" error under exactly the load you would want to demo. `SKIP LOCKED` sidesteps both problems:
each transaction immediately claims a *different* unlocked row.

`SERIALIZABLE` would be actively wrong here. Under 100 concurrent buyers it produces a storm of
serialization failures and retries for an invariant that a row lock already enforces perfectly.

**Cancel / refund — `SERIALIZABLE` with a retry loop**

Cancellation is low-volume but spans three tables: it must verify the ticket is `purchased` and owned
by the caller, verify no refund has already been issued against the `payments` row, release the
ticket back to the pool, write the audit log, and enqueue the outbox event. The invariant
*"a ticket is refunded at most once"* spans a read-then-write across tables, which is precisely the
shape that row locks do not protect and `SERIALIZABLE` does.

The cost is that transactions can abort with SQLSTATE `40001` (`serialization_failure`) or `40P01`
(`deadlock_detected`). Both are retried with exponential backoff and jitter, capped at 3 attempts.
Because the whole operation is wrapped in an idempotency key, a retry is always safe.

> **The rule of thumb:** row-level locks where you need throughput on a single hot table;
> `SERIALIZABLE` where the invariant spans tables and volume is low enough to absorb retries.

### 2. Idempotency in two layers

Double-charging a customer is the worst failure this system can produce, and it is easy to trigger:
a user double-clicks, a mobile client retries on a flaky connection, a load balancer replays a
request. Both layers are necessary.

**Layer 1 — application-level deduplication.** Clients send an `Idempotency-Key` header on
`POST /tickets/purchase` and `POST /tickets/cancel`. The service records the key, the caller, and a
hash of the request body in an `idempotency_keys` table with a unique constraint on the key. On a
replay:

- Same key, same request hash, work already complete → the stored response is replayed verbatim.
- Same key, same request hash, work still in flight → `409 Conflict`, telling the client to retry.
- Same key, **different** request hash → `422 Unprocessable Entity`. The key is being reused for a
  different operation, which is a client bug worth surfacing loudly rather than silently guessing.

The key is inserted in the *same transaction* as the ticket claim, so the dedup record and the state
change commit or roll back together.

**Layer 2 — Stripe's own idempotency.** The same key is forwarded to Stripe via
`params.SetIdempotencyKey(...)`. Layer 1 cannot protect the window between "we called Stripe" and
"we committed our transaction" — if the process dies there, we have a charge with no local record.
Stripe's key ensures the retry returns the *original* PaymentIntent instead of creating a second one,
letting us reconcile rather than double-charge.

### 3. Transactional outbox instead of dual writes

The obvious way to publish a purchase event is to commit to Postgres and then produce to Kafka. This
is broken: the two systems have no shared transaction. A crash between them either loses the event
(consumers never learn about the purchase) or, if published first, announces a purchase that then
rolls back.

Instead, the purchase transaction writes its event to an `outbox` table **in the same transaction**
as the ticket update. A relay goroutine polls for unpublished rows, produces to Kafka, and marks them
sent. The state change and the intent-to-publish are now atomic.

This gives **at-least-once** delivery: the relay may crash after producing but before marking the row
sent, republishing on restart. Consumers are therefore written to be idempotent, keyed on `ticket_id`
plus event type. Exactly-once across a database and a broker is not achievable without distributed
transactions, and at-least-once plus idempotent consumers is the standard, honest tradeoff.

### 4. Why Kafka, and not Redpanda / SQS / Postgres LISTEN-NOTIFY

Genuinely evaluated, and worth being able to defend:

- **Redpanda** — Kafka-API-compatible, single binary, no JVM, boots in about a second versus Kafka's
  ~20. The Go client code is *identical*, so it is a drop-in. Rejected because with KRaft mode
  (no ZooKeeper) Kafka's operational overhead is now a handful of compose environment variables, and
  matching what the majority of production deployments actually run is worth more than faster local
  boots. Redpanda would be the right call for a resource-constrained CI environment.
- **AWS SQS** — simpler, fully managed, but a queue rather than a log. No replay, no independent
  consumer groups reading the same stream at different offsets. Ticketing wants a durable, replayable
  log: an analytics consumer and a notification consumer should read the same events independently.
- **Postgres `LISTEN`/`NOTIFY`** — appealing given a database is already present, and genuinely fine
  at small scale. Rejected because notifications are fire-and-forget: a disconnected listener misses
  them permanently, with no offsets and no replay. That is a poor fit for payment-adjacent events.

Kafka is chosen for the durable replayable log and independent consumer groups. Running it in
**KRaft mode** drops the ZooKeeper dependency entirely.

The Go client is **`segmentio/kafka-go`**. The obvious alternative, `confluent-kafka-go`, is ruled out
by an existing constraint rather than preference: it wraps librdkafka and requires cgo, while these
images build with `CGO_ENABLED=0` onto `distroless/static`. `twmb/franz-go` is the faster pure-Go
option and was the runner-up; `kafka-go` wins on readability for a producer and a single consumer
group at this scale.

### 5. Layered structure in one service, not all four

`ticket-service` is split into three layers; the other services are not.

| Layer | Responsibility | Never touches |
|---|---|---|
| `handlers` | Decode the request, call the service, encode the response, map errors to status codes | SQL, business rules |
| `service` | Owns transactions; orchestrates store, payments, and outbox | HTTP types |
| `store` | SQL only, against a `pgx.Tx` supplied by the caller | Transaction boundaries |

The governing rule is that **the service layer opens the transaction and the store layer receives
it**. That is what allows a single purchase transaction to atomically span `tickets`, `payments`,
`idempotency_keys`, and `outbox` — four stores, one commit.

This split was not adopted as a general principle. It is the minimum structure required to write one
specific test:

> *Stripe's charge succeeds, then the local commit fails. Does the ticket return to the pool, and
> does the retry avoid double-charging?*

That test needs a payment provider that can be made to fail on demand (hence an interface rather than
a concrete Stripe client), business logic invokable without an HTTP server (hence service separated
from handlers), and a transaction the test itself controls (hence stores that accept a `pgx.Tx`).
The three layers are what that requirement decomposes into.

`user-service` and `event-service` deliberately keep flat handlers. They own no multi-table
invariants, call no external providers, and have nothing to fake — layering them would add
indirection to buy nothing. Applying the structure only where it pays for itself is the point.

The cost, stated plainly: for simple reads like `GET /tickets/available`, the service layer is a
pass-through that adds no value. It is kept anyway, because two competing patterns inside one package
is more expensive to read than one uniform pattern with a few thin methods.

### 6. Module boundaries and dependency direction

The repository is five Go modules: four services and a shared `pkg`. The rule governing `pkg` is
that **a module may only carry dependencies it uses at runtime**, so `pkg` is split by dependency
footprint rather than by convenience.

| Package | Dependencies | Consumers |
|---|---|---|
| `pkg/httpx` | stdlib only | all four services |
| `pkg/auth` | `golang-jwt/v5` | all four services |
| `pkg/database` | `pgx/v5` | user, event, ticket — **never the gateway** |

The original layout bundled `Logging` and `IsAdmin` together in `pkg/middleware`. Because `IsAdmin`
opens a database connection, every consumer of the logging middleware inherited `pgx` — including the
API gateway, which has no database. `IsAdmin` is a domain query, not a cross-cutting concern, so it
moves into ticket-service's store layer and the gateway sheds Postgres entirely.

The same rule keeps **Stripe and Kafka out of `pkg`**. Both are used only by ticket-service, so both
live there. Putting them in the shared module would grow a payment SDK and a Kafka client into the
gateway, the user service and the catalog service, none of which have any use for them.

**One JWT library, one verification path.** The codebase previously signed tokens with
`golang-jwt/jwt` v3 in user-service, parsed them with v3 in `pkg/middleware`, and verified them with
v5 at the gateway — three code paths across two incompatible majors, one of them unmaintained. Worse,
only the gateway's path pinned the signing algorithm, so a request reaching a service directly could
present a token signed with `none`. Everything is unified on v5 behind `pkg/auth`, which pins the
algorithm in a single place.

**Only the gateway is published.** Compose previously exposed ports 8081-8083, which made the
"gateway is the sole entry point" claim false and allowed the auth bypass above. The base compose file
now publishes only the gateway and Postgres; the services remain reachable on the internal compose
network but not from the host. `make up-debug` layers `docker-compose.debug.yml` on top to republish
them when debugging one directly. That file is deliberately *not* named `docker-compose.override.yml`,
which Compose loads automatically -- that would re-expose the services by default and undo the
boundary.

Services still verify the token on every request rather than trusting their network position. Closing
the ports is one layer; authenticating regardless is the other.

A committed `go.work` ties the five modules together, so one `go build` or `go test` invocation can
span all five with unified dependency resolution, and gopls resolves symbols across module boundaries
in the editor. Note the workspace root is not itself a module, so patterns must name the modules
explicitly (`go build ./pkg/... ./api-gateway/...`) rather than a bare `./...`; the `make build-go`
and `make test` targets wrap this. Docker builds are unaffected: each image copies only `pkg` and its
own service, resolving through the `replace` directive rather than the workspace.

### 7. Topics

| Topic | Key | Produced when | Consumed by |
|---|---|---|---|
| `ticket.purchased` | `ticket_id` | Purchase transaction commits | Notification worker (confirmation + QR), future analytics |
| `ticket.cancelled` | `ticket_id` | Cancellation transaction commits | Notification worker (refund confirmation) |

Keying by `ticket_id` guarantees all events for one ticket land on the same partition and are
therefore consumed in order — a cancellation can never be processed before its purchase.

---

## Data model

Existing tables — `users`, `events`, `tickets`, `ticket_cancellation_logs` — plus three added by
Phases 2 and 3.

### `tickets` (existing)

| Column | Type | Notes |
|---|---|---|
| `id` | UUID PK | `gen_random_uuid()` |
| `event_id` | UUID FK → events | |
| `user_id` | UUID FK → users, NULL | NULL while in the available pool |
| `qr_code` | TEXT UNIQUE NULL | base64 PNG, populated on purchase |
| `status` | TEXT | Active values: `available`, `purchased` |
| `price` | NUMERIC(10,2) | |
| `purchased_at` | TIMESTAMP NULL | |

**Status semantics.** Only `available` and `purchased` are live states. Cancellation returns the
ticket to `available` and records history in `ticket_cancellation_logs` — the ticket row itself is
never `cancelled`, because the physical seat genuinely is back in the pool. A composite index on
`(event_id, status)` backs the availability query.

### `payments` (Phase 2)

| Column | Type | Notes |
|---|---|---|
| `id` | UUID PK | |
| `ticket_id` | UUID FK → tickets | |
| `user_id` | UUID FK → users | |
| `stripe_payment_intent_id` | TEXT UNIQUE | |
| `amount_cents` | BIGINT | Integer cents — never floats for money |
| `status` | TEXT | `pending`, `succeeded`, `failed`, `refunded` |
| `stripe_refund_id` | TEXT UNIQUE NULL | Uniqueness enforces at-most-one refund |

### `idempotency_keys` (Phase 2)

| Column | Type | Notes |
|---|---|---|
| `key` | TEXT PK | Client-supplied |
| `user_id` | UUID FK → users | Scopes keys per caller |
| `endpoint` | TEXT | |
| `request_hash` | TEXT | SHA-256 of the body; detects key reuse |
| `response_status` | INT NULL | NULL while in flight |
| `response_body` | JSONB NULL | Replayed verbatim on retry |
| `created_at` | TIMESTAMP | Reaped after 24h |

### `outbox` (Phase 3)

| Column | Type | Notes |
|---|---|---|
| `id` | BIGSERIAL PK | Monotonic; relay polls in order |
| `aggregate_id` | UUID | The `ticket_id`, used as the Kafka message key |
| `topic` | TEXT | |
| `payload` | JSONB | |
| `created_at` | TIMESTAMP | |
| `published_at` | TIMESTAMP NULL | NULL = pending. Partial index on `WHERE published_at IS NULL` |

---

## API

All routes are called through the gateway at `http://localhost:8000`.

### Public

| Method | Path | Body | Returns |
|---|---|---|---|
| `POST` | `/users/register` | `{email, password, full_name}` | `{message}` |
| `POST` | `/users/login` | `{email, password}` | `{token}` |
| `GET` | `/events` | — | `[Event]` — gains `?limit=&cursor=` in Phase 4 |

### Authenticated (`Authorization: Bearer <jwt>`)

| Method | Path | Body | Returns |
|---|---|---|---|
| `POST` | `/events/create` | `{name, description, location, start_time, end_time}` | `{message}` |
| `POST` | `/tickets/create` | `{event_id, price, quantity}` | `{message, quantity}` |
| `GET` | `/tickets/available?event_id=` | — | `[{id, price, created_at}]` — becomes `{available_count, tickets[]}` in Phase 4 |
| `POST` | `/tickets/purchase` | `{event_id, payment_method_id}` | `{ticket_id, qr_code, payment_intent_id}` |
| `POST` | `/tickets/cancel` | `{ticket_id, reason?}` | `{message, refund_id}` |
| `GET` | `/tickets/mine` | — | `[PurchasedTicket]` |
| `GET` | `/tickets/receipt?ticket_id=` | — | `{ticket, qr_code, payment}` |

`/tickets/purchase` and `/tickets/cancel` accept an **`Idempotency-Key`** header. It is optional but
strongly recommended; without it, retries may double-charge.

### Status codes

Meaningful codes matter for the load test — a sold-out race must be distinguishable from a genuine
server fault.

| Code | Meaning |
|---|---|
| `200` / `201` | Success |
| `401` | Missing/invalid JWT |
| `403` | Authenticated but not the organizer or an admin |
| `404` | Ticket not found, or not owned by caller |
| `409` | Sold out, or an idempotent request is still in flight |
| `422` | Idempotency key reused with a different payload |
| `502` | Stripe unreachable or returned an error |

---

## Delivery phases

### Phase 0 — Reproducible local stack ✅

The repository must clone and run. It could not. No service image could build: each build context was
the service's own directory, while every `go.mod` carries a `replace ... => ../pkg` directive pointing
outside it, so `go mod download` could not resolve the shared module. Had they built, every container
would still have exited on boot — each `main.go` called `godotenv.Load()` followed by `log.Fatal`, and
no Dockerfile copied a `.env`.

- [x] Move all four build contexts to `./services` so the shared `pkg` module resolves
- [x] Treat a missing `.env` as normal and read configuration from the environment
- [x] Build user-service from `cmd/main.go` rather than a stray root `main.go` stub that printed one
      line and exited
- [x] Fix compose service names and env var names; remove the `alpine` placeholder shadowing the
      event-service build
- [x] Gate startup on a Postgres healthcheck and a completed migration run
- [x] Run migrations automatically on `make up`
- [x] Port `ticket_cancellation_logs` into `db/migrations` — it existed only in an orphaned directory,
      so cancellation failed at runtime on any fresh database
- [x] Collapse duplicated gateway JWT enforcement to a single point, reorder CORS outside auth, and
      pin the JWT signing algorithm
- [x] Add `make seed` creating an admin, an upcoming event, and 50 tickets
- [x] Untrack the stray compiled binaries (`services/*/main`)

**Done when:** `make clean && make up && make seed && make smoke` passes from scratch.

**Verified:** all 17 smoke checks pass — the public/protected route split, register, login, inventory
listing, purchase with QR, receipt, and cancel returning the ticket to the pool.

### Phase 1 — Foundation and concurrency correctness ✅

Makes the locking claim genuinely true. Highest interview leverage: *"how did you prevent
overselling"* is the question this project most invites.

Split into two passes. The first settles module boundaries and dependencies so that later phases add
Stripe and Kafka to a structure that can hold them; the second fixes the concurrency defects.

**Pass A — module boundaries and dependencies**

- [x] Split `pkg` into `httpx` (stdlib only), `auth` (`golang-jwt/v5`) and `database` (`pgx/v5`), so
      the gateway stops inheriting Postgres
- [x] Move `IsAdmin` out of `pkg/middleware` into ticket-service's store — it is a domain query, not
      a cross-cutting concern
- [x] Unify on `golang-jwt/v5` and pin the signing algorithm in `pkg/auth`. Tokens are currently
      signed with v3, parsed with v3 in shared middleware, and verified with v5 at the gateway; only
      the gateway path checks the algorithm
- [x] Publish only the gateway and Postgres from the base compose file; move the service ports into
      `docker-compose.debug.yml`, layered on by `make up-debug`
- [x] Upgrade and pin: `pgx` v5.10.0, `x/crypto` v0.55.0, `golang-jwt` v5.3.1
- [x] Add a committed `go.work`; drop it from `.gitignore`
- [x] `make tidy` across all modules and a check that shared dependency versions agree

**Pass B — concurrency correctness**

- [x] Replace per-request `pgxpool.New` with a process-level pool (`sync.Once`), sized via
      `DB_MAX_CONNS`. Every handler currently builds and tears down an entire connection pool per
      request, which alone will fail a 100-VU test.
- [x] Fix `CancelTicket`: its `SELECT` and `UPDATE` run on `db` rather than `tx`, so the ticket is
      released outside the transaction and a failed audit-log insert cannot roll it back
- [x] Add `FOR UPDATE SKIP LOCKED` to the purchase claim
- [x] Move cancellation to `SERIALIZABLE` with a `40001`/`40P01` retry helper
- [x] Return `409` for sold-out instead of `500`
- [x] Replace `sql.ErrNoRows` comparisons with `pgx.ErrNoRows` (the current check never fires)
- [x] Hash passwords with bcrypt; migrate existing rows
- [x] Composite index on `tickets (event_id, status)`
- [x] k6 scenario: 100 VUs against an event with exactly 50 tickets

**Done when:** k6 reports exactly 50 × `200` and 50 × `409`, zero `5xx`, and a post-run SQL check
confirms no ticket has two owners and `count(purchased) = 50`. The smoke test still passes, and
`go mod tidy` leaves no `pgx` entry in `api-gateway/go.mod`.

**Result** (`make load`, 100 VUs against 50 tickets):

```
tickets_purchased..: 50      ✓ count==50
sold_out_409.......: 50      ✓ count==50
server_errors_5xx..: 0       ✓ count==0
checks_succeeded...: 100.00% 400 out of 400
http_req_duration..: avg=56.01ms med=59.4ms p(95)=67.15ms
```

Database state after the run: 50 purchased, 0 available, every purchased ticket has an owner, no QR
code issued twice.

### Phase 2 — Stripe ✅

- [x] `payments.Provider` interface with `stripe-go` v86 and fake implementations
- [x] `payments` and `idempotency_keys` migrations
- [x] Idempotency middleware: capture, replay, in-flight conflict, hash-mismatch detection
- [x] Charge on purchase, recorded as pending inside the claim transaction before the processor is
      called, then settled afterwards
- [x] Compensating release when the charge fails, since work done on another system has no rollback
- [x] Refund on cancel, guarded by the unique `stripe_refund_id`
- [x] The same key forwarded to Stripe, so a retry that reaches the processor returns the original
      PaymentIntent rather than creating a second one
- [x] Tests against the fake, plus store integration tests covering the double-refund rejection
- [x] k6 idempotency scenario and a capacity ramp

**Ordering.** The charge cannot sit inside the claim transaction: it is a side effect on another
system, holding a transaction open across a network call pins a connection for the length of
someone else's latency, and a rollback cannot un-charge a card. So:

```
claim ticket + insert payment 'pending'   -- one transaction
charge the processor                      -- outside it, keyed
mark payment 'succeeded'                  -- afterwards
```

A crash between the second and third steps leaves a payment row holding the intent id with status
`pending`, which reconciliation can match against Stripe. A charge with no local row could not be
matched to anything.

**Verified** against live Stripe test mode: a purchase creates a PaymentIntent and a cancellation
refunds it. Thirty concurrent requests sharing one idempotency key consume exactly one ticket --
one `200` and twenty-nine `409`, no `5xx`.

**Two bugs this shook out**, both the same mistake: deriving an idempotency identifier from the
ticket. A failed charge releases the seat, so the same ticket can be bought again, and both the
provisional intent id and the Stripe key then collided with their earlier use -- Stripe rejecting
the second with *"keys can only be used with the same parameters they were first used with"*. Both
now key off the payment row, which is one charge attempt.

#### Capacity

`make load-capacity` ramps arrival rate rather than virtual users, so a saturated system shows up as
growing latency and dropped iterations instead of a queue that quietly self-throttles.

| Target rate | Throughput | p95 | Dropped |
|---|---|---|---|
| 400/s | 400/s | 4.3 ms | 0 |
| 5000/s, `DB_MAX_CONNS=25` | 396/s | 792 ms | 25,467 |
| 5000/s, `DB_MAX_CONNS=100` | 713/s | 3.8 ms | 135 |

The ceiling is the connection pool, not Go and not the gateway: the gateway's own `/health`, which
touches no database, sustains 2000/s at 0.43 ms. Raising `DB_MAX_CONNS` from 25 to 100 nearly
doubled throughput and cut p95 by two orders of magnitude.

Throughput is not the interesting result, though -- these numbers come from a laptop running the
whole stack in Docker, so they describe relative behaviour, not production capacity. What matters is
the failure mode: the system degrades by getting slow, and returns zero `5xx` the entire way up.

### Phase 3 — Kafka ✅

- [x] Kafka in KRaft mode in compose, topics created explicitly at startup
- [x] `segmentio/kafka-go` in ticket-service only, never in `pkg`
- [x] `outbox` migration with a partial index on unpublished rows
- [x] Outbox writes inside the purchase and cancellation transactions
- [x] Relay goroutine: claim, produce, mark published, with a bounded write timeout
- [x] Notification worker as a separate binary joining its own consumer group
- [x] `notifications` table as the visible proof the async path ran
- [x] Verified: purchase produces an event and a notification row, and a broker outage loses nothing

**Why an outbox at all.** Postgres and Kafka share no transaction. Committing the ticket change and
then producing loses the event if the process dies in between; producing first announces a purchase
that may roll back. Neither is acceptable for money-adjacent events. The event is written to a table
inside the same transaction as the state change, and a relay publishes it afterwards, which makes the
state change and the intent-to-publish atomic.

The cost is **at-least-once** delivery: the relay can produce and then die before marking the row
sent. Consumers are therefore idempotent. Exactly-once across a database and a broker is not
achievable without distributed transactions, and pretending otherwise is worse than admitting it.

**Verified durability.** With the broker stopped, purchases still return `200` and events accumulate
in the outbox — the payment path does not depend on Kafka being up. Restarting the broker drains the
backlog and the notifications appear.

#### Two bugs worth recording

**The relay held a transaction across the network call.** The first version wrapped claim, produce
and mark in one transaction so they would be atomic. Under a broker outage `WriteMessages` blocked
inside kafka-go's internal retries while holding `FOR UPDATE` locks on the claimed rows — and the
failure handler, running on the pool, then blocked forever on locks the same relay held. A
self-deadlock, visible as a connection `idle in transaction` for ten minutes.

It is the same mistake the payment path documents against: never hold a database transaction open
across a call to another system. The relay now claims, commits, produces with a bounded timeout, then
marks in a second transaction. Two relays could publish the same batch twice, which at-least-once
already permits.

**Deduplicating on the wrong identity.** The consumer keyed on `(ticket_id, event_type)`, which
conflates a message delivered twice with a ticket genuinely bought twice. Cancelling returns a seat
to the pool, so the same ticket really can be purchased again — and that buyer silently received no
confirmation. Each event now carries a message id generated at enqueue time, and the consumer
deduplicates on that. Deduplication belongs on message identity, not on the business entity the
message is about.

#### Topics

| Topic | Key | Partitions | Consumed by |
|---|---|---|---|
| `ticket.purchased` | `ticket_id` | 3 | notification worker |
| `ticket.cancelled` | `ticket_id` | 3 | notification worker |

Keying by ticket puts every event for one ticket on the same partition, so a cancellation can never
be consumed before the purchase it refers to.

Topics are created explicitly at startup rather than relying on the broker's auto-creation, which
only fires on the first produce. A consumer group that joins before that is assigned no partitions
and does not recover on its own — which is exactly how the worker first came up idle.

### Phase 4 — Frontend + observability ⬜

- [ ] Event detail page with Stripe Elements checkout
- [ ] "My tickets" page with QR receipts and cancel flow
- [ ] `GET /events/{id}` — event detail, required by the detail page
- [ ] `GET /events/user` — events the caller holds tickets for
- [ ] Cursor pagination on `GET /events`, `GET /events/user` and `GET /tickets/mine`
      (`?limit=&cursor=`, opaque cursor, `next_cursor` in the response). Offset pagination is
      rejected: it drifts when rows are inserted between pages, and `OFFSET n` makes Postgres walk
      and discard n rows, so deep pages get progressively slower
- [ ] `GET /tickets/available` returns a **count plus a page**, not every row. The browse UI needs
      "42 left", not 42 ticket objects, and the current unbounded response grows with inventory
- [ ] `X-Request-Id` generated at the gateway, propagated downstream, logged everywhere
- [ ] Structured JSON logging
- [ ] Architecture diagram and k6 results in this README

---

## Running it

### Full stack

```bash
make up         # start everything (Postgres, migrations, all four services)
make up-debug   # same, but republish service ports 8081-8083 on the host
make go-build   # compile all five Go modules
make go-test    # test all five Go modules
make tidy       # go mod tidy in every module
make deps-check # fail if shared dependency versions diverge
make seed       # admin user + sample event + inventory
make load-idem  # concurrent purchases sharing one idempotency key
make load-capacity # ramp arrival rate to find where latency degrades
make smoke      # end-to-end check through the gateway
make logs       # tail
make down
```

### Services individually

Each service is its own Go module; run from its directory. Postgres must be up first.

```bash
cd services/api-gateway   && go run cmd/main.go   # :8000
cd services/user-service  && go run cmd/main.go   # :8081
cd services/event-service && go run cmd/main.go   # :8082
cd services/ticket-service && go run cmd/main.go  # :8083
```

### Frontend

```bash
cd frontend/event-ticketing-frontend
npm run dev     # :3000
```

### Migrations

```bash
migrate -path db/migrations \
  -database "postgres://admin:password@localhost:5433/event_ticketing?sslmode=disable" up
```

### Load tests

```bash
k6 run tests/load/purchase_contention.js
k6 run tests/load/idempotency.js
```

### Environment

Each service loads `.env` via `godotenv`. Dev defaults:

```bash
DATABASE_URL=postgres://admin:password@localhost:5433/event_ticketing?sslmode=disable
JWT_SECRET=my_secret_key_123
KAFKA_BROKERS=localhost:9092
STRIPE_SECRET_KEY=sk_test_...        # test mode only
```

The gateway additionally needs `USER_SERVICE_URL`, `EVENT_SERVICE_URL`, `TICKET_SERVICE_URL`.

> `.env` files are gitignored and never committed. Copy `.env.example` in each service directory and
> fill in local values; under Docker Compose the environment is supplied by the compose file instead.

---

## Known simplifications

Deliberate scope cuts, listed so they can be discussed rather than discovered:

- **One shared database.** Real microservices own their data. Here all four share a Postgres
  instance, and `ticket-service` reads `events` and `users` directly. The migration path is
  per-service schemas, then per-service databases, with cross-service reads replaced by API calls or
  by consuming the Kafka stream.
- **JWT in `localStorage`.** Vulnerable to XSS; httpOnly cookies with CSRF protection would be the
  production choice.
- **No seat selection.** Tickets are a fungible pool. Reserved seating changes the claim from "any
  available row" to "this specific row," which removes `SKIP LOCKED` as an option.
- **No pagination until Phase 4.** Every list endpoint returns its full result set. Fine at seed
  scale, wrong at any real size.
- **No hold/reservation window.** Real ticketing holds inventory for a few minutes during checkout.
  This claims at purchase time, so a Stripe failure rolls back rather than releasing a timed hold.
- **Single-region, no replicas.** Read replicas would require handling replication lag on
  read-after-write.
