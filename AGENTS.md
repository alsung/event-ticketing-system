# AGENTS.md

This file provides guidance to coding agents — WARP (warp.dev), and Claude Code via the `CLAUDE.md`
symlink — when working on **EventMaster**, the event ticketing system in this repository.

## Commands

### Docker (full stack)
```bash
make build      # Build all Docker images
make up         # Start all services in detached mode (docker-compose up -d)
make down       # Stop all services
make logs       # Tail all service logs
```

### Running services locally (outside Docker)
Each service is an independent Go module. Run from the service directory:
```bash
cd services/api-gateway  && go run cmd/main.go   # :8000
cd services/user-service && go run cmd/main.go   # :8081
cd services/event-service && go run cmd/main.go  # :8082
cd services/ticket-service && go run cmd/main.go # :8083
```
PostgreSQL must be running first (`make up` starts it via Docker on port 5433).

### Frontend
```bash
cd frontend/event-ticketing-frontend
npm run dev    # dev server on :3000
npm run build  # production build
npm run lint   # ESLint
```

### Database migrations
Migrations live in `db/migrations/` and use the `migrate` CLI tool with sequential integer versioning. Apply with:
```bash
migrate -path db/migrations -database "postgres://admin:password@localhost:5433/event_ticketing?sslmode=disable" up
```

### Go module management
Each service (`api-gateway`, `user-service`, `event-service`, `ticket-service`) and `services/pkg` is its own Go module. When adding dependencies or running `go mod tidy`, do so from the specific service directory. Shared code is referenced via a `replace` directive pointing to `../pkg`.

### Testing
Standard `go test`, run across all modules via the workspace:

```bash
make go-test      # unit tests only; the store integration tests skip
make go-test-db   # also runs them, against the compose Postgres on :5433
```

Coverage is `ticket-service/internal/payments` (unit) and `ticket-service/internal/store`
(integration — these skip unless `TEST_DATABASE_URL` is set, and **CI does not set it**, so a green
CI run has not exercised them). `tests/smoke/smoke.sh` (via `make smoke`) covers the routes end to end against a
running stack. See "Not built, and why" in the README before assuming a behaviour is under test.

---

## Architecture

### Request flow
```
Next.js Frontend (:3000) → API Gateway (:8000) → User Service (:8081)
                                                 → Event Service (:8082)
                                                 → Ticket Service (:8083)
                                                         ↓
                                               PostgreSQL (:5433 → container :5432)
                                               db: event_ticketing
```

### API Gateway (`services/api-gateway`)
The gateway is the sole entry point for the frontend. It:
- Applies middleware in order: CORS → JWT auth → request logging
- Routes requests by path prefix (`/users` → user-service, `/events` → event-service, `/tickets` → ticket-service) using a reverse proxy that forwards the full path unchanged to the downstream service
- Enforces JWT on protected routes (`/events/create`, `PUT /events/{id}`, `/organizer/events`, `/tickets/create`, `/tickets/purchase`, `/tickets/cancel`, `/tickets/mine`, `/tickets/receipt`) — public routes (`/users/register`, `/users/login`, `GET /events`, `GET /events/{id}`, health) pass through without auth
- Allows `Idempotency-Key` in CORS, alongside `Content-Type` and `Authorization`. Omitting it silently breaks every browser purchase at preflight while curl keeps working
- Reads upstream URLs from env: `USER_SERVICE_URL`, `EVENT_SERVICE_URL`, `TICKET_SERVICE_URL`

Internal structure: `gateway/exported/` contains the public API (`GatewayHandler`, middleware); `gateway/internal/` contains the router and proxy implementation.

### Shared package (`services/pkg`)
Split by dependency footprint, so no module carries a dependency it does not use at runtime:
- `pkg/httpx` — stdlib only. Logging middleware, JSON helpers. Used by all four services.
- `pkg/auth` — `golang-jwt/v5` only. Signing and verification behind one verifier that pins the algorithm in a single place. Used by all four services.
- `pkg/database` — `pgx/v5`. Process-wide `Pool(ctx)`, plus `InTx` and `InSerializableTx` (the latter retries SQLSTATE `40001`/`40P01` with jittered backoff). Used by user, event and ticket services — **never the gateway**, which has no database.

There is no `pkg/middleware` and no `NewDatabaseConnection`; both were removed. Stripe and Kafka
deliberately stay out of `pkg` because only ticket-service uses them.

### Services
Each service follows the same internal layout:
```
cmd/main.go            — registers routes on net/http ServeMux, starts server
internal/handlers/     — HTTP handler functions; these also own transactions
internal/models/       — struct definitions
internal/utils/        — service-specific utilities (QR code for ticket-service)
```

Every route pattern is method-explicit (`GET /events`, `PUT /events/{id}`). A bare `/events/create`
also matches `GET /events/create`, which overlaps `GET /events/{id}` without either being a strict
subset — Go's ServeMux panics at registration.

`ticket-service` additionally has `internal/store/` (SQL only, every write takes a `pgx.Tx` from its
caller), `internal/payments/` (a `Provider` interface with Stripe and fake implementations),
`internal/idempotency/`, and `internal/outbox/` (relay and consumer). There is **no `service`
package** — see "Not built, and why" in the README.

**User Service**: handles `/users/register` and `/users/login`. Issues JWTs with claims `user_id`, `email`, `exp`, signed with `pkg/auth`. Passwords are **bcrypt digests** at default cost.

**Event Service**: catalog and organiser management — `GET /events` (with `?q=` running a ranked Postgres full-text search over a generated `tsvector`), `GET /events/{id}`, `POST /events/create`, `PUT /events/{id}`, and `GET /organizer/events` (per-event sold/available/revenue aggregates). "Upcoming" events are those with `start_time >= now`. Ownership is enforced in the `WHERE` clause and a failed edit returns `404`, not `403`.

**Ticket Service**: the full ticket lifecycle, and the only service with real concurrency concerns.
- **Purchase** runs `SELECT ... FOR UPDATE SKIP LOCKED` under READ COMMITTED. `SKIP LOCKED` is load-bearing: without it concurrent buyers serialise behind one lock and a `LIMIT 1` waiter can return zero rows while stock remains.
- **Cancel** runs at SERIALIZABLE with retry, because "refunded at most once" is a read-then-write spanning three tables.
- The **Stripe charge happens outside the transaction** — a rollback cannot un-charge a card — with an explicit compensating transaction on failure.
- The purchase transaction also writes an **outbox row**; a relay goroutine publishes it to Kafka and the notification worker consumes it. Delivery is at-least-once, so consumers dedup on `message_id`.
- Caller identity comes from `pkg/auth` on every protected operation; roles are `attendee` / `organizer` / `admin` in `users.role`.

### Frontend (`frontend/event-ticketing-frontend`)
Next.js 16 App Router with TypeScript and Tailwind CSS 4. All API calls go to `http://localhost:8000` (the gateway). JWT is stored in `localStorage`. `context/UserContext.tsx` decodes the JWT and provides `user` and `signOut()` app-wide.

Pages: `/`, `login`, `register`, `events`, `events/[id]`, `events/[id]/checkout`, `tickets`, `organizer`, `organizer/new`, `organizer/[id]`. Pages that do not need the token are Server Components; the ones that do isolate it in a small client component. Note `eslint-plugin-react-hooks` v6 rules (`set-state-in-effect`, `purity`) are enforced in CI.

### Database
Single shared PostgreSQL instance (dev). Migrations are numbered sequentially in `db/migrations/` using `.up.sql` / `.down.sql` pairs — 16 at present.

Eight tables: `users`, `events`, `tickets`, `ticket_cancellation_logs`, `payments`, `idempotency_keys`, `outbox`, `notifications`. The `tickets.status` active values are `available` and `purchased`; cancellations return the ticket to `available` and write an audit row. `outbox` and `notifications` carry no foreign keys on purpose — an outbox row must outlive its subject, and notifications are written by a separate consumer.

### Environment variables
Each service loads a `.env` file at startup via `godotenv`. Dev defaults:
- `DATABASE_URL=postgres://admin:password@localhost:5433/event_ticketing?sslmode=disable`
- `JWT_SECRET=my_secret_key_123`
- Gateway additionally needs: `USER_SERVICE_URL`, `EVENT_SERVICE_URL`, `TICKET_SERVICE_URL`
- Ticket-service additionally reads: `STRIPE_SECRET_KEY` (absent → the fake provider runs, which keeps the stack usable without a Stripe account), `KAFKA_BROKERS`, `DB_MAX_CONNS`

A missing `.env` is treated as normal — configuration falls back to the environment, which is how
the containers are configured. Only the gateway (`:8000`), Postgres (`:5433`) and Kafka (`:9092`)
are published to the host; `make up-debug` layers a second compose file to republish the services.
