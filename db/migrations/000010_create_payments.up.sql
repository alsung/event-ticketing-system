-- Money taken and given back, recorded locally so the system can answer "what
-- did we charge for this ticket" without calling Stripe.

CREATE TABLE IF NOT EXISTS payments (
    id                       UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    ticket_id                UUID NOT NULL REFERENCES tickets(id),
    user_id                  UUID NOT NULL REFERENCES users(id),

    -- Stripe's PaymentIntent id. UNIQUE so a retry that reaches Stripe twice
    -- cannot produce two local rows for one intent.
    stripe_payment_intent_id TEXT UNIQUE NOT NULL,

    -- Integer cents, never a float. 0.1 + 0.2 is not 0.3 in binary floating
    -- point, and a rounding error in a payment total surfaces weeks later in a
    -- reconciliation report.
    amount_cents             BIGINT NOT NULL CHECK (amount_cents >= 0),
    currency                 TEXT NOT NULL DEFAULT 'usd',

    status                   TEXT NOT NULL
                                 CHECK (status IN ('pending', 'succeeded', 'failed', 'refunded')),

    -- UNIQUE is the refunded-at-most-once guard. Even if the application logic
    -- were wrong, the database refuses a second refund against one payment.
    stripe_refund_id         TEXT UNIQUE,
    refunded_at              TIMESTAMP,

    created_at               TIMESTAMP NOT NULL DEFAULT NOW(),
    updated_at               TIMESTAMP NOT NULL DEFAULT NOW()
);

-- The cancel path looks a payment up by ticket to refund it.
CREATE INDEX IF NOT EXISTS idx_payments_ticket_id ON payments (ticket_id);
CREATE INDEX IF NOT EXISTS idx_payments_user_id   ON payments (user_id);
