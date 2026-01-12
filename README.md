# Event Ticketing System - DESIGN.md

## 1) Purpose

Build a full-stack event ticketing platform where: 

- **Users** can register/login, browse events, purchase and cancel tickets, and view receipts containing **QR codes** for admission. 
- **Admins/Organizers** can create events and create ticket inventory.
- The backend uses a **microservice-style architecture** (Go services) behind a **Go API Gateway**, and a **Next.js frontend** consumes only the gateway.

This repo is also meant to be a **system design interview artifact**:
- clear components + boundaries
- precise APIs + data model
- reliable flows (purchase/cancel)
- scaling + hardening roadmap

Non-goals (for now):
- payments integration
- seat maps / reserved seating
- complex promotion/discount engine
- event discovery ranking/search relevance

---

## 2) Architecture Overview

### 2.1 High-level request flow

**Next.js Frontend (localhost:3000)**
--> **API Gateway (localhost:8000)**
--> **User Service (8081)** / **Event Service (8082)** / **Ticket Service (8083)**
--> **PostgreSQL (localhost:5433 --> container 5432)**

### 2.2 Logical diagram

```text
+-------------------------+        +--------------------------+        +------------------------+
|  Next.js Frontend       |  HTTP  |        API Gateway       |  HTTP  |      User Service      |
|  http://localhost:3000  | -----> |   http://localhost:8000  | -----> |   http://localhost:8081|
|                         |        | - routing (reverse proxy)|        +------------------------+
|                         |        | - CORS + OPTIONS         |
|                         |        | - JWT auth (protected)   |        +------------------------+
|                         |        | - request logging        |  HTTP  |     Event Service      |
+-------------------------+        +--------------------------+ -----> |   http://localhost:8082|
                                                          |            +------------------------+
                                                          |
                                                          |            +------------------------+
                                                          |     HTTP   |     Ticket Service     |
                                                          +----------> |   http://localhost:8083|
                                                                       +------------------------+
                                                                                  |
                                                                                  | SQL
                                                                                  v
                                                                       +------------------------+
                                                                       |   PostgreSQL Database  |
                                                                       |   localhost:5433       |
                                                                       |   (container:5432)     |
                                                                       |   db: event_ticketing  |
                                                                       +------------------------+
```

### 2.3 Component responsibilities (detailed)

#### Frontend (Next.js + Tailwind + TypeScript)
- UI flows:
    - login/register
    - "My events" (events user has tickets for)
    - browse all upcoming events
    - view owned tickets + cancel
- State:
    - stores JWT in localStorage (dev)
    - uses a `UserContext` to decode claims and expose `user`, `signOut()`
- Networking:
    - all calls go to `http://localhost:8000/...` (gateway only)

#### API Gateway (Go, `net/http`, reverse proxy)
- Central entrypoint and "edge" layer:
    - CORS headers + preflight response for `OPTIONS`
    - JWT verification for protected routes
    - request logging (shared middleware)
    - service routing (path prefix -> upstream)
- Important property:
    - gateway should forward request method/body/query/headers to downstream.
    - for protected routes, gateway checks JWT and forwards request as-is.

#### User Service
- Responsibilities:
    - user creation
    - login and JWT issuance
    - admin flag stored on user (`is_admin`)
- Current simplifications:
    - passwords are plaintext in DB (should be hashed later)
    - no email verification / password reset yet

#### Event Service
- Responsibilities:
    - create events
    - list events (all upcoming)
    - list events a user has tickets for (planned)
- Owns event semantics:
    - what is "upcoming" (start_time >= now)
    - organizer_id relationship

#### Ticket Service
- Responsibilities: 
    - inventory creation: "mint" tickets for event
    - list available inventory by event
    - purchase flow: claim a ticket atomically
    - cancellation: return-to-pool + log
    - receipts: view ticket details + QR
- Guarantees:
    - prevent oversell by locking inventory rows during purchase (row-level locking)
    - only ticket owner can cancel
    - cancellation writes an audit record

#### PostgreSQL
- Source of truth for: 
    - users
    - events
    - ticket inventory + ownership + status
    - cancellation logs (audit trail)
- Migration management:
    - `schema_migrations` table used by `migrate` tool
    - migrations tracked by version integer

> NOTE: The project currently uses one shared DB (dev). In a production microservices narrative, this can evolve to per-service DB ownership.

---

## 3) Key Workflows (Sequence Diagrams)

### 3.1 Register + Login

1. User registers:
    - frontend `POST /users/register`
    - gateway forwards to user service
    - user service inserts user row
2. User logs in:
    - frontend `POST /users/login`
    - user service validates credentials, return JWT with `user_id`

**JWT claims:**
- `email`
- `user_id`
- `exp`

### 3.2 Create event (admin/organizer)

- frontend calls `POST /events/create` with JWT
- gateway verifies JWT and forwards
- event service inserts into `events` with `organizer_id`

### 3.3 Create ticket inventory (admin/organizer)

- frontend calls `POST /tickets/create` with JWT
- ticket service:
    - verifies JWT -> extracts `user_id`
    - checks user admin flag (and optionally organizer match for event)
    - inserts N tickets for event with price, initial status

### 3.4 Purchase ticket (critical path)

Goal: purchase exactly one available ticket, never oversell.

Recommended DB pattern:
- begin transaction
- `SELECT id FROM tickets WHERE event_id=$1 AND status='available' LIMIT 1 FOR UPDATE`
- `UPDATE tickets SET status='purchased', user_id=$2, purchased_at=NOW(), qr_code=$3 WHERE id=$ticketID`
- commit
- return `{ticket_id, qr_code_base64}`

QR:
- generate data payload (ticket_id + event_id + user_id)
- encode as QR image
- store base64 (or store raw data + generate later)
- return base64 to client in purchase response

### 3.5 Cancel ticket (return to pool + audit)

- user calls `POST /tickets/cancel` with `ticket_id` (+ optional reason)
- ticket service:
    - verifies JWT (user_id)
    - verifies ticket belongs to user and status = purchased
    - update ticket to available:
        - `status='available'`
        - `user_id=NULL`
        - `purchased_at=NULL`
        - `qr_code=NULL` (or keep but invalidate)
    - insert audit log row into `ticket_cancellation_logs`
    - return success

**Return-to-pool policy:** immediate. (Later could introduce "cooldown window")

### 3.6 View receipt (QR later)

- client calls `GET /tickets/receipt?...`
- ticket service returns ticket details + stored base64 QR

---

## 4) Authentication & Authorization

### 4.1 JWT Authentication
- Gateway enforces auth for protected routes.
- Open routes:
    - `/users/register`
    - `/users/login`

Token verification:
- HMAC secret `JWT_SECRET` (dev)
- verify signature, expiration, and algorithm

### 4.2 Authorization Rules (current / intended)

**Admin**
- `users.is_admin = true`
- can create inventory (`/tickets/create`)
- may create events (`/events/create`) depending on rule

**Organizer**
- events have `organizer_id`
- organizer can mint tickets for their events
- rule could be:
    - allow if `is_admin == true OR organizer_id == user_id`

Future extension:
- roles table (admin, organizer, staff)
- per-event permissions

---

## 5) Database Connections & Env

### 5.1 Common env vars
- `DB_URL=postgres://admin:password@localhost:5433/event_ticketing?sslmode=disable`
- `JWT_SECRET=my_dev_secret_key_123`

### 5.2 Service DB usage
- Services use shared db connector:
    - `services/pkg/database.NewDatabaseConnection(ctx)`

---

## 6) Data Model (Tables + Constraints)

### 6.1 `users`

Fields: 
- `id` UUID PK
- `email` TEXT UNIQUE NOT NULL
- `password` TEXT NOT NULL *(dev plaintext; must hash later)*
- `full_name` TEXT NOT NULL
- `is_admin` BOOLEAN NOT NULL DEFAULT false
- `created_at` TIMESTAMP DEFAULT now()

Indexes:
- unique on email

### 6.2 `events`

Fields:
- `id` UUID PK
- `name` TEXT NOT NULL
- `description` TEXT
- `location` TEXT
- `start_time` TIMESTAMP NOT NULL
- `end_time` TIMESTAMP NOT NULL
- `organizer_id` UUID REFERENCES users(id)
- `created_at` TIMESTAMP DEFAULT now()

Indexes (recommended):
- index on `start_time`
- index on `organizer_id`

### 6.3 `tickets`

Confirmed:
- `id` UUID PK DEFAULT gen_random_uuid()
- `event_id` UUID REFERENCES events(id)
- `user_id` UUID REFERENCES users(id) NULL
- `qr_code` TEXT UNIQUE NULL
- `status` TEXT NOT NULL CHECK (status IN ('reserved', 'purchased', 'cancelled')) DEFAULT 'reserved'
- `purchased_at` TIMESTAMP NULL
- `created_at` TIMESTAMP DEFAULT now()
- `price` NUMERIC(10,2) NOT NULL DEFAULT 0.00

Indexes:
- `idx_event_id(event_id)`
- recommended composite index `(event_id, status)` for availability queries

**Status semantics (recommended standardization)**
- `available`: in pool, unowned
- `purchased`: owned by user
- `cancelled`: logically cancelled and removed from pool (if not returning to pool)

Given return-to-pool policy, `cancelled` can mean "cancel action occurred" but ticket returns to `available`.
- either keep `cancelled` but create a new ticket row when returning to pool (more complex)
- OR treat cancellation as returning to `available` (simpler; audit logs capture the cancellation)

**Recommendation:** use `available` and `purchased` as the active states; keep cancellation history in `ticket_cancellation_logs`.

### 6.4 `ticket_cancellation_logs`

Fields:
- `id` UUID PK
- `ticket_id` UUID NOT NULL REFERENCES tickets(id)
- `user_id` UUID NOT NULL REFERENCES users(id)
- `event_id` UUID NOT NULL REFERENCES events(id)
- `cancelled_at` TIMESTAMP DEFAULT now()
- `reason` TEXT NULL

Indexes (recommended):
- `(user_id, cancelled_at)`
- `(ticket_id)`

---

## 7) API Endpoints (External Contract via Gateway)

All endpoints below are called via `http://localhost:8000`

### 7.1 User APIs
- `POST /users/register`
    - body: `{ email, password, full_name }`
    - returns: `{ message }`

- `POST /users/login`
    - body: `{ email, password }`
    - returns: `{ token }`

### 7.2 Event APIs
- `GET /events/`
    returns: `[Event]`

- `POST /events/create`
    - body: `{ name, description, location, start_time, end_time, organizer_id }`
    - returns: `{ message }`

Planned: 
- `GET /events/user`
    - returns: only events the authenticated user has tickets for

### 7.3 Ticket APIs
- `POST /tickets/create` (admin/organizer)
    - body: `{ event_id, price, quantity }`
    - returns `{ message, quantity }`

- `GET /tickets/available?event_id=<uuid>`
    - returns `[ { id, price, created_at } ]`

- `POST /tickets/purchase`
    - body: `{ event_id, user_id }` (future: infer user_id from JWT)
    - returns `{ ticket_id, message, qr_code_base64 }`

- `GET /tickets/mine`
    - returns: purchased tickets for authenticated user

- `POST /tickets/cancel`
    - body: `{ ticket_id, reason }`
    - returns: `{ message }`

- `GET /tickets/receipt?...`
    - returns: ticket + QR

---

## 8) Gateway Routing & Best Practices

### 8.1 Forward-full-path convention (recommended here)

Gateway:
- routes by prefix (`/users`, `/events`, `/tickets`)
- forwards the full path downstream unchanged

Service routes:
- user-service defines `/users/login`, `/users/register`
- event-service defines `/events/`, `/events/create`
- ticket-service defines `/tickets/available`, etc.

Why:
- fewer mismatches
- less mental overhead during debugging
- consistent curl calls (always via gateway)

### 8.2 Cross-cutting middleware at gateway
- CORS (always set headers)
- OPTIONS preflight
- JWT validation
- request logging (request id is a future enhancement)

---

## 9) Observability & Logging

Current:
- request loggin middleware prints:
    - request method + path
    - duration
    - remote addr

Recommended upgrades:
- add request ID header:
    - gateway generates `X-Request-Id` if missing
    - forwards downstream
    - log includes request id everywhere
- structured logs (JSON) for easier parsing
- metrics (Prometheus) for:
    - request count / latency per route
    - purchase failures, cancellations, available inventory

---

## 10) Current Status (Built vs Remaining)

### Completed
Backend:
- user-service: register/login + JWT contains user_id
- api-gateway: routing + CORS + JWT middleware + logging
- event-service: list + create
- ticket-service:
    - create tickets (admin/organizer)
    - list available tickets
    - purchase ticket (atomic row lock) + QR base64 return
    - cancel ticket (return-to-pool) + cancellation logs
    - get user tickets (`/tickets/mine`) fixed for nullable QR
    - receipt endpoint works

Frontend:
- Next.js scaffold
- login + register
- UserContext + signOut
- Navbar
- event page fetch works

### Remaining (feature roadmap)
1. **Backend:** `GET /events/user` (events the user has tickets for)
2. **Frontend:** split UI into:
    - `/events` = "My upcoming events"
    - `/browse-events` = all upcoming events grouped by date
3. **Frontend:** event detail page + purchase flow + receipt view
4. **Frontend:** "My tickets" page + cancel flow
5. Hardening:
    - password hashing
    - pagination
    - better error models
    - request id tracing
    - caching

---

## 11) Future Scaling Path (Interview-ready)

### Data & load scaling
- Hot reads:
    - events list
    - ticket availability counts
    - receipts
- Add Redis caching at gateway or per service:
    - cache event list, invalidate on create
    - cache available ticket counts, invalidate on purchase/cancel

### Prevent abuse
- rate-limit login and purchase endpoints
- idempotency keys on purchase to avoid double-charge patterns (even before payments)

### Data integrity
- enforce ticket "available" state properly (standardize status)
- add a uniqueness strategy for purchase operations:
    - lock row (`FOR UPDATE`)
    - update guarded by status
    - ensure exactly one row updated

### Service ownership (advanced)
- split DB by service
- event-service owns events DB
- ticket-service owns tickets DB
- user-service owns users DB
- use events via IDs and APIs rather than cross-table joins

---

## 12) Glossary
- **JWT**: signed auth token containing claims (`user_id`, `email`, `exp`)
- **CORS**: browser policy requiring cross-origin headers
- **Return-to-pool**: cancelled ticket becomes available again
- **Receipt**: endpoint returning ticket info + QR code in base64
- **Inventory**: pool of unowned tickets for an event