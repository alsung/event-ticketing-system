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
| **0** | Reproducible local stack (`docker compose up` works end to end) | 🔨 In progress |
| **1** | Concurrency correctness + k6 load validation | ⬜ Not started |
| **2** | Stripe charge/refund with idempotency keys | ⬜ Not started |
| **3** | Kafka async flows via transactional outbox | ⬜ Not started |
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

### 6. Topics

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
| `GET` | `/events` | — | `[Event]` |

### Authenticated (`Authorization: Bearer <jwt>`)

| Method | Path | Body | Returns |
|---|---|---|---|
| `POST` | `/events/create` | `{name, description, location, start_time, end_time}` | `{message}` |
| `POST` | `/tickets/create` | `{event_id, price, quantity}` | `{message, quantity}` |
| `GET` | `/tickets/available?event_id=` | — | `[{id, price, created_at}]` |
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

### Phase 0 — Reproducible local stack 🔨

The repository must clone and run. Currently `docker-compose.yml` cannot start: the event service
carries an `image: alpine` placeholder that overrides its build, services receive `DB_URL` while the
code reads `DATABASE_URL`, and the gateway points at `http://user-service:8081` while the compose
service is named `user_service`.

- [ ] Fix compose service names, env var names, remove the `alpine` placeholder
- [ ] Add a healthcheck-gated `depends_on` so services wait for Postgres
- [ ] Run migrations automatically on startup
- [ ] Add a `make seed` target creating an admin, an event, and inventory
- [ ] Commit the stray built binaries currently tracked (`services/*/main`) to `.gitignore`

**Done when:** `git clone && make up && make seed` yields a working system, verified by a smoke script.

### Phase 1 — Concurrency correctness ⬜

Makes the locking claim genuinely true. Highest interview leverage: *"how did you prevent
overselling"* is the question this project most invites.

- [ ] Replace per-request `pgxpool.New` with a process-level pool (`sync.Once`), sized via
      `DB_MAX_CONNS`. Every handler currently builds and tears down an entire connection pool per
      request, which alone will fail a 100-VU test.
- [ ] Fix `CancelTicket`: its `SELECT` and `UPDATE` run on `db` rather than `tx`, so the ticket is
      released outside the transaction and a failed audit-log insert cannot roll it back
- [ ] Add `FOR UPDATE SKIP LOCKED` to the purchase claim
- [ ] Move cancellation to `SERIALIZABLE` with a `40001`/`40P01` retry helper
- [ ] Return `409` for sold-out instead of `500`
- [ ] Replace `sql.ErrNoRows` comparisons with `pgx.ErrNoRows` (the current check never fires)
- [ ] Hash passwords with bcrypt; migrate existing rows
- [ ] Composite index on `tickets (event_id, status)`
- [ ] k6 scenario: 100 VUs against an event with exactly 50 tickets

**Done when:** k6 reports exactly 50 × `200` and 50 × `409`, zero `5xx`, and a post-run SQL check
confirms no ticket has two owners and `count(purchased) = 50`.

### Phase 2 — Stripe ⬜

- [ ] `PaymentProvider` interface with `stripe-go` and fake implementations
- [ ] `payments` + `idempotency_keys` migrations
- [ ] Idempotency middleware (capture, replay, hash-mismatch detection)
- [ ] Charge on purchase: PaymentIntent created before the ticket claim commits; failure rolls back
- [ ] Refund on cancel, guarded by the unique `stripe_refund_id`
- [ ] Forward the idempotency key to Stripe via `SetIdempotencyKey`
- [ ] Unit tests against the fake, covering the replay and hash-mismatch paths

**Done when:** k6 fires the same idempotency key 100× concurrently and the Stripe test dashboard
shows exactly one PaymentIntent.

### Phase 3 — Kafka ⬜

- [ ] Kafka in KRaft mode in compose; topics auto-created on boot
- [ ] `outbox` migration with the partial index
- [ ] Outbox writes inside the purchase and cancellation transactions
- [ ] Relay goroutine: poll → produce → mark published, with backoff
- [ ] Notification worker as a consumer group, idempotent on `(ticket_id, event_type)`
- [ ] `notifications` table as the visible proof the async path ran
- [ ] Integration test: purchase → assert the notification row appears

**Done when:** a purchase through the UI produces a `ticket.purchased` message and a notification row,
and killing the relay mid-run loses no events on restart.

### Phase 4 — Frontend + observability ⬜

- [ ] Event detail page with Stripe Elements checkout
- [ ] "My tickets" page with QR receipts and cancel flow
- [ ] `GET /events/user` — events the caller holds tickets for
- [ ] `X-Request-Id` generated at the gateway, propagated downstream, logged everywhere
- [ ] Structured JSON logging
- [ ] Architecture diagram and k6 results in this README

---

## Running it

### Full stack

```bash
make up      # start everything (Postgres, Kafka, all four services)
make seed    # admin user + sample event + inventory
make logs    # tail
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
- **No hold/reservation window.** Real ticketing holds inventory for a few minutes during checkout.
  This claims at purchase time, so a Stripe failure rolls back rather than releasing a timed hold.
- **Single-region, no replicas.** Read replicas would require handling replication lag on
  read-after-write.
