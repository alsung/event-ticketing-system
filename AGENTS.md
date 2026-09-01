# AGENTS.md

This file provides guidance to WARP (warp.dev) when working with code in this repository.

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
CI run has not exercised them). `scripts/smoke.sh` covers the routes end to end against a running
stack. See "Not built, and why" in the README before assuming a behaviour is under test.

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
- Enforces JWT on protected routes (`/events/create`, `/tickets/create`, `/tickets/purchase`, `/tickets/mine`) — public routes (`/users/register`, `/users/login`) pass through without auth
- Reads upstream URLs from env: `USER_SERVICE_URL`, `EVENT_SERVICE_URL`, `TICKET_SERVICE_URL`

Internal structure: `gateway/exported/` contains the public API (`GatewayHandler`, middleware); `gateway/internal/` contains the router and proxy implementation.

### Shared package (`services/pkg`)
Two shared utilities consumed by all services via `replace` directives:
- `pkg/database`: `NewDatabaseConnection(ctx)` — reads `DATABASE_URL` env var, returns a `pgxpool.Pool` (note: individual service `internal/database/` packages open single `pgx.Conn` connections instead — the shared pool in `pkg` is used by ticket-service handlers and the `IsAdmin` helper)
- `pkg/middleware`: `Logging` (HTTP middleware) and `GetUserIDFromJWT` / `IsAdmin` auth helpers

### Services
Each service follows the same internal layout:
```
cmd/main.go            — registers routes on net/http ServeMux, starts server
internal/handlers/     — HTTP handler functions
internal/models/       — struct definitions
internal/database/     — local pgx.Conn helper (single connection per request)
internal/utils/        — service-specific utilities (JWT for user-service, QR code for ticket-service)
```

**User Service**: handles `/users/register` and `/users/login`. Issues JWTs with claims `user_id`, `email`, `exp`. Passwords are stored in plaintext (dev only).

**Event Service**: handles `GET /events` and `POST /events/create`. "Upcoming" events are those with `start_time >= now`.

**Ticket Service**: handles the full ticket lifecycle — create inventory, list available, purchase (atomic `SELECT ... FOR UPDATE` + `UPDATE` in a transaction to prevent overselling), cancel (return-to-pool + write to `ticket_cancellation_logs`), get user tickets, and get receipt with base64 QR code. Uses `pkg/middleware.GetUserIDFromJWT` to extract the caller's user ID from the JWT on every protected operation.

### Frontend (`frontend/event-ticketing-frontend`)
Next.js 15 App Router with TypeScript and Tailwind CSS 4. All API calls go to `http://localhost:8000` (the gateway). JWT is stored in `localStorage`. `context/UserContext.tsx` decodes the JWT and provides `user` and `signOut()` app-wide. Current pages: `login`, `register`, `events`.

### Database
Single shared PostgreSQL instance (dev). Migrations are numbered sequentially in `db/migrations/` using `.up.sql` / `.down.sql` pairs. Schema: `users`, `events`, `tickets`, `ticket_cancellation_logs`. The `tickets.status` active values are `available` and `purchased`; cancellations return the ticket to `available` and write an audit row to `ticket_cancellation_logs`.

### Environment variables
Each service loads a `.env` file at startup via `godotenv`. Dev defaults:
- `DATABASE_URL=postgres://admin:password@localhost:5433/event_ticketing?sslmode=disable`
- `JWT_SECRET=my_secret_key_123`
- Gateway additionally needs: `USER_SERVICE_URL`, `EVENT_SERVICE_URL`, `TICKET_SERVICE_URL`
