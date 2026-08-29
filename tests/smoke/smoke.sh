#!/usr/bin/env bash
#
# End-to-end smoke test against the API gateway. Exercises the full user
# journey -- register, login, browse, purchase, receipt, cancel -- plus the
# public/protected route split.
#
# This is the Phase 0 acceptance gate: a fresh clone running
#   make up && make seed && make smoke
# must pass. Requires curl and jq.

set -euo pipefail

GATEWAY="${GATEWAY:-http://localhost:8000}"
EMAIL="smoke-$(date +%s)-$$@example.com"
PASSWORD="smoke-password"

TMP="$(mktemp -d)"
trap 'rm -rf "$TMP"' EXIT

BODY="$TMP/body.json"

pass() { printf '  \033[32mok\033[0m   %s\n' "$1"; }
fail() {
    printf '  \033[31mFAIL\033[0m %s\n' "$1" >&2
    [ -s "$BODY" ] && printf '       response: %s\n' "$(head -c 400 "$BODY")" >&2
    exit 1
}

# req METHOD PATH [TOKEN] [JSON_BODY] -> echoes the HTTP status, body in $BODY
req() {
    local method="$1" path="$2" token="${3:-}" payload="${4:-}"
    local args=(-sS -o "$BODY" -w '%{http_code}' -X "$method" "$GATEWAY$path")
    if [ -n "$token" ]; then
        args+=(-H "Authorization: Bearer $token")
    fi
    if [ -n "$payload" ]; then
        args+=(-H 'Content-Type: application/json' -d "$payload")
    fi
    curl "${args[@]}"
}

expect() {
    local actual="$1" want="$2" label="$3"
    if [ "$actual" = "$want" ]; then
        pass "$label ($actual)"
    else
        fail "$label -- expected $want, got $actual"
    fi
}

echo "smoke test against $GATEWAY"

# --- readiness ------------------------------------------------------------
printf '  ..   waiting for gateway'
for i in $(seq 1 40); do
    if curl -sS -o /dev/null "$GATEWAY/events" 2>/dev/null; then
        printf '\r'
        pass "gateway reachable"
        break
    fi
    if [ "$i" = 40 ]; then
        printf '\n'
        fail "gateway not reachable after 40s -- is the stack up?"
    fi
    printf '.'
    sleep 1
done

# --- public routes --------------------------------------------------------
status="$(req GET /events)"
expect "$status" 200 "GET /events is public"

events_count="$(jq 'length' < "$BODY")"
[ "$events_count" -gt 0 ] || fail "GET /events returned no events -- run 'make seed' first"
EVENT_ID="$(jq -r '.[0].id' < "$BODY")"
pass "found seeded event $EVENT_ID"

# --- protected routes reject anonymous callers ----------------------------
status="$(req GET /tickets/mine)"
expect "$status" 401 "GET /tickets/mine rejects anonymous"

# --- auth -----------------------------------------------------------------
status="$(req POST /users/register "" "{\"email\":\"$EMAIL\",\"password\":\"$PASSWORD\",\"full_name\":\"Smoke Test\"}")"
expect "$status" 201 "POST /users/register"

status="$(req POST /users/login "" "{\"email\":\"$EMAIL\",\"password\":\"$PASSWORD\"}")"
expect "$status" 200 "POST /users/login"
TOKEN="$(jq -r '.token' < "$BODY")"
[ -n "$TOKEN" ] && [ "$TOKEN" != "null" ] || fail "login returned no token"
pass "received JWT"

# --- inventory ------------------------------------------------------------
status="$(req GET "/tickets/available?event_id=$EVENT_ID" "$TOKEN")"
expect "$status" 200 "GET /tickets/available"
available_before="$(jq 'length' < "$BODY")"
pass "$available_before tickets available"

# --- purchase -------------------------------------------------------------
# pm_card_visa is Stripe's always-succeeds test payment method. Supplying one is
# not optional against the real provider: Stripe refuses to confirm a
# PaymentIntent that has no payment method. The fake provider ignores it.
status="$(req POST /tickets/purchase "$TOKEN" "{\"event_id\":\"$EVENT_ID\",\"payment_method_id\":\"pm_card_visa\"}")"
expect "$status" 200 "POST /tickets/purchase"
TICKET_ID="$(jq -r '.ticket_id' < "$BODY")"
[ -n "$TICKET_ID" ] && [ "$TICKET_ID" != "null" ] || fail "purchase returned no ticket_id"
jq -e '.qr_code | length > 0' < "$BODY" >/dev/null || fail "purchase returned no QR code"
pass "purchased ticket $TICKET_ID with QR"

# --- ownership ------------------------------------------------------------
status="$(req GET /tickets/mine "$TOKEN")"
expect "$status" 200 "GET /tickets/mine"
jq -e --arg id "$TICKET_ID" 'any(.[]; .id == $id)' < "$BODY" >/dev/null \
    || fail "purchased ticket missing from /tickets/mine"
pass "ticket appears in /tickets/mine"

status="$(req GET "/tickets/receipt?ticket_id=$TICKET_ID" "$TOKEN")"
expect "$status" 200 "GET /tickets/receipt"

# --- cancel ---------------------------------------------------------------
status="$(req POST /tickets/cancel "$TOKEN" "{\"ticket_id\":\"$TICKET_ID\",\"reason\":\"smoke test\"}")"
expect "$status" 200 "POST /tickets/cancel"

status="$(req GET "/tickets/available?event_id=$EVENT_ID" "$TOKEN")"
expect "$status" 200 "GET /tickets/available after cancel"
available_after="$(jq 'length' < "$BODY")"
[ "$available_after" = "$available_before" ] \
    || fail "cancel did not return the ticket to the pool ($available_before -> $available_after)"
pass "ticket returned to pool ($available_after available)"

printf '\n\033[32mall smoke checks passed\033[0m\n'
