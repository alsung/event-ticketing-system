// Purchase contention: 100 virtual users race for an event with exactly 50
// tickets.
//
// This is the test the README's concurrency claim rests on. It asserts three
// things:
//
//   1. Exactly 50 purchases succeed -- no oversell.
//   2. The other 50 get 409 Conflict, not 500. Contention is an expected
//      outcome, not a server fault, and a run full of 500s would mean the
//      SKIP LOCKED claim is returning spurious sold-out errors.
//   3. No 5xx at all.
//
// A post-run SQL check in tests/load/verify.sh confirms no ticket ended up with
// two owners.
//
//   k6 run tests/load/purchase_contention.js

import http from "k6/http";
import { check } from "k6";
import { Counter } from "k6/metrics";

const GATEWAY = __ENV.GATEWAY || "http://localhost:8000";
const INVENTORY = Number(__ENV.INVENTORY || 50);
const VUS = Number(__ENV.VUS || 100);

const purchased = new Counter("tickets_purchased");
const soldOut = new Counter("sold_out_409");
const serverErrors = new Counter("server_errors_5xx");

export const options = {
  scenarios: {
    // Every VU fires a single purchase at once. Not a ramp: the point is
    // simultaneous contention on the same inventory, not sustained throughput.
    stampede: {
      executor: "per-vu-iterations",
      vus: VUS,
      iterations: 1,
      maxDuration: "60s",
    },
  },
  thresholds: {
    // The run fails unless the arithmetic works out exactly.
    tickets_purchased: [`count==${INVENTORY}`],
    sold_out_409: [`count==${VUS - INVENTORY}`],
    server_errors_5xx: ["count==0"],
    http_req_failed: ["rate<0.51"], // 409s count as failures to k6; 5xx are asserted separately
  },
};

// setup runs once before the VUs start. It registers one user, logs in, and
// finds the seeded event, so the measured iteration is purchase only.
export function setup() {
  const email = `k6-${Date.now()}@example.com`;
  const password = "k6-password";

  const reg = http.post(
    `${GATEWAY}/users/register`,
    JSON.stringify({ email, password, full_name: "k6 load test" }),
    { headers: { "Content-Type": "application/json" } },
  );
  if (reg.status !== 201) {
    throw new Error(`register failed: ${reg.status} ${reg.body}`);
  }

  const login = http.post(
    `${GATEWAY}/users/login`,
    JSON.stringify({ email, password }),
    { headers: { "Content-Type": "application/json" } },
  );
  if (login.status !== 200) {
    throw new Error(`login failed: ${login.status} ${login.body}`);
  }
  const token = login.json("token");

  const events = http.get(`${GATEWAY}/events`);
  if (events.status !== 200 || events.json().length === 0) {
    throw new Error("no events found -- run 'make seed' first");
  }
  const eventId = events.json()[0].id;

  const available = http.get(
    `${GATEWAY}/tickets/available?event_id=${eventId}`,
    { headers: { Authorization: `Bearer ${token}` } },
  );
  const count = available.json().length;
  if (count !== INVENTORY) {
    throw new Error(
      `expected ${INVENTORY} available tickets, found ${count} -- run 'make seed'`,
    );
  }

  return { token, eventId };
}

export default function (data) {
  const res = http.post(
    `${GATEWAY}/tickets/purchase`,
    JSON.stringify({ event_id: data.eventId }),
    {
      headers: {
        "Content-Type": "application/json",
        Authorization: `Bearer ${data.token}`,
      },
      tags: { name: "POST /tickets/purchase" },
    },
  );

  if (res.status === 200) {
    purchased.add(1);
  } else if (res.status === 409) {
    soldOut.add(1);
  } else if (res.status >= 500) {
    serverErrors.add(1);
    console.error(`5xx: ${res.status} ${res.body}`);
  }

  check(res, {
    "status is 200 or 409": (r) => r.status === 200 || r.status === 409,
    "no server error": (r) => r.status < 500,
    "200 carries a ticket id": (r) => r.status !== 200 || !!r.json("ticket_id"),
    "200 carries a QR code": (r) => r.status !== 200 || !!r.json("qr_code"),
  });
}
