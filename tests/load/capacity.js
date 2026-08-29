// Capacity probe: where does this system stop keeping up?
//
// Distinct from purchase_contention.js, which asks whether the system is
// *correct* under contention. This one asks how much load it absorbs before
// latency degrades, and what the limiting resource turns out to be.
//
// Arrival-rate executor, not fixed VUs: it holds a request rate regardless of
// how slow responses get, so a saturated system shows up as growing latency and
// dropped iterations rather than as a queue that quietly self-throttles.
//
// Expect the ceiling to be the database pool (DB_MAX_CONNS, default 25), not Go.
// The purchase path holds a connection for a locking transaction, so concurrent
// purchases in flight cannot exceed the pool.
//
//   make load-capacity
//
// Caveat: numbers from a laptop running the whole stack in Docker describe
// relative behaviour, not production throughput.

import http from "k6/http";
import { check } from "k6";
import { Counter, Trend } from "k6/metrics";

const GATEWAY = __ENV.GATEWAY || "http://localhost:8000";
const PEAK = Number(__ENV.PEAK || 400);

const soldOut = new Counter("sold_out_409");
const serverErrors = new Counter("server_errors_5xx");
const purchaseLatency = new Trend("purchase_latency", true);

export const options = {
  scenarios: {
    ramp: {
      executor: "ramping-arrival-rate",
      startRate: 25,
      timeUnit: "1s",
      preAllocatedVUs: 50,
      maxVUs: 600,
      stages: [
        { target: 50, duration: "20s" },
        { target: 100, duration: "20s" },
        { target: 200, duration: "20s" },
        { target: PEAK, duration: "20s" },
      ],
    },
  },
  thresholds: {
    // Deliberately loose. This run is a measurement, not a pass/fail gate --
    // the point is to find the knee, so a threshold that fails at the knee
    // would just hide it. Only a genuine server fault fails the run.
    server_errors_5xx: ["count==0"],
  },
};

export function setup() {
  const email = `k6-cap-${Date.now()}@example.com`;
  const password = "k6-password";
  http.post(`${GATEWAY}/users/register`,
    JSON.stringify({ email, password, full_name: "k6 capacity" }),
    { headers: { "Content-Type": "application/json" } });
  const login = http.post(`${GATEWAY}/users/login`,
    JSON.stringify({ email, password }),
    { headers: { "Content-Type": "application/json" } });
  const events = http.get(`${GATEWAY}/events`);
  return { token: login.json("token"), eventId: events.json()[0].id };
}

// Reads, not purchases. Inventory is finite, so a sustained purchase ramp would
// measure how fast the pool drains rather than how much load the system takes.
// The availability query shares the hot table, the index and the connection
// pool, which is where saturation shows up.
export default function (data) {
  const res = http.get(`${GATEWAY}/tickets/available?event_id=${data.eventId}`, {
    headers: { Authorization: `Bearer ${data.token}` },
    tags: { name: "GET /tickets/available" },
  });

  purchaseLatency.add(res.timings.duration);
  if (res.status === 409) soldOut.add(1);
  else if (res.status >= 500) serverErrors.add(1);

  check(res, { "not a server error": (r) => r.status < 500 });
}
