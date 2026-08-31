#!/usr/bin/env bash
#
# Post-run database assertions for the purchase contention scenario. k6 proves
# the API returned the right status codes; this proves the database ended up in
# the right state, which is the claim that actually matters.

set -euo pipefail

INVENTORY="${INVENTORY:-50}"
PSQL=(docker compose exec -T postgres psql -U admin -d event_ticketing -tAc)

# Every assertion is scoped to the event the scenario actually bought from --
# the earliest by start_time, which is the one GET /events returns first and the
# one the seed tops up to exactly INVENTORY tickets. The seed now creates twelve
# events, so a table-wide count would include inventory nobody touched.
EVENT_ID="$("${PSQL[@]}" "SELECT id FROM events ORDER BY start_time LIMIT 1;")"

fail() { printf '  \033[31mFAIL\033[0m %s\n' "$1" >&2; exit 1; }
pass() { printf '  \033[32mok\033[0m   %s\n' "$1"; }

purchased="$("${PSQL[@]}" "SELECT count(*) FROM tickets WHERE status='purchased' AND event_id='$EVENT_ID';")"
[ "$purchased" = "$INVENTORY" ] \
    || fail "expected $INVENTORY purchased tickets, found $purchased"
pass "exactly $purchased tickets purchased"

available="$("${PSQL[@]}" "SELECT count(*) FROM tickets WHERE status='available' AND event_id='$EVENT_ID';")"
[ "$available" = "0" ] || fail "expected 0 available, found $available"
pass "inventory fully drained"

# The real oversell check: a ticket must never be claimed twice. Each row has one
# user_id, so a double-sell shows up as a purchased ticket with no owner, or as
# more purchased rows than the inventory that existed.
orphaned="$("${PSQL[@]}" "SELECT count(*) FROM tickets WHERE status='purchased' AND user_id IS NULL AND event_id='$EVENT_ID';")"
[ "$orphaned" = "0" ] || fail "$orphaned purchased tickets have no owner"
pass "every purchased ticket has an owner"

dupes="$("${PSQL[@]}" "SELECT count(*) FROM (SELECT qr_code FROM tickets WHERE qr_code IS NOT NULL GROUP BY qr_code HAVING count(*) > 1) d;")"
[ "$dupes" = "0" ] || fail "$dupes QR codes issued more than once"
pass "no QR code issued twice"

printf '\n\033[32mdatabase state verified: no oversell\033[0m\n'
