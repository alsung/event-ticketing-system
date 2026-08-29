// Idempotency under concurrency: many simultaneous requests, one shared key.
//
// The assertion that matters is not the status distribution -- it is that the
// inventory moved by exactly one ticket. A double-charge would show up as two
// seats consumed for one intended purchase.
//
//   make load-idem

import http from "k6/http";
import { check } from "k6";
import { Counter } from "k6/metrics";

const GATEWAY = __ENV.GATEWAY || "http://localhost:8000";
const VUS = Number(__ENV.VUS || 30);

const replayed = new Counter("replayed_200");
const inFlight = new Counter("in_flight_409");
const serverErrors = new Counter("server_errors_5xx");

export const options = {
  scenarios: {
    same_key: {
      executor: "per-vu-iterations",
      vus: VUS,
      iterations: 1,
      maxDuration: "60s",
    },
  },
  thresholds: {
    server_errors_5xx: ["count==0"],
    // Every response must be either the replayed original or an honest
    // "still in flight, retry" -- never a second purchase.
    checks: ["rate==1.0"],
  },
};

export function setup() {
  const email = `k6-idem-${Date.now()}@example.com`;
  const password = "k6-password";

  http.post(`${GATEWAY}/users/register`,
    JSON.stringify({ email, password, full_name: "k6 idempotency" }),
    { headers: { "Content-Type": "application/json" } });

  const login = http.post(`${GATEWAY}/users/login`,
    JSON.stringify({ email, password }),
    { headers: { "Content-Type": "application/json" } });
  const token = login.json("token");

  const events = http.get(`${GATEWAY}/events`);
  const eventId = events.json()[0].id;

  const before = http.get(`${GATEWAY}/tickets/available?event_id=${eventId}`,
    { headers: { Authorization: `Bearer ${token}` } }).json().length;

  return { token, eventId, before, key: `k6-shared-${Date.now()}` };
}

export default function (data) {
  const res = http.post(`${GATEWAY}/tickets/purchase`,
    JSON.stringify({ event_id: data.eventId, payment_method_id: "pm_card_visa" }),
    {
      headers: {
        "Content-Type": "application/json",
        Authorization: `Bearer ${data.token}`,
        "Idempotency-Key": data.key,
      },
      tags: { name: "POST /tickets/purchase (shared key)" },
    });

  if (res.status === 200) replayed.add(1);
  else if (res.status === 409) inFlight.add(1);
  else if (res.status >= 500) serverErrors.add(1);

  check(res, {
    "200 replay or 409 in-flight": (r) => r.status === 200 || r.status === 409,
  });
}

export function teardown(data) {
  const after = http.get(`${GATEWAY}/tickets/available?event_id=${data.eventId}`,
    { headers: { Authorization: `Bearer ${data.token}` } }).json().length;

  const consumed = data.before - after;
  console.log(`inventory consumed: ${consumed} (expected exactly 1)`);
  if (consumed !== 1) {
    throw new Error(`IDEMPOTENCY VIOLATED: ${VUS} requests with one key consumed ${consumed} tickets`);
  }
}
