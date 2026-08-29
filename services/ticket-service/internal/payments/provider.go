// Package payments wraps the payment processor behind an interface.
//
// The interface exists so the purchase path can be tested without a network or
// an account: FakeProvider can be told to fail on demand, which is the only way
// to exercise the failure this system most needs to get right -- the charge
// succeeds, then the local commit does not.
package payments

import (
	"context"
	"errors"
)

var (
	// ErrCardDeclined is a customer-facing failure: the charge was refused. It
	// maps to a 402, not a 500 -- nothing is broken on our side.
	ErrCardDeclined = errors.New("payments: card declined")

	// ErrProviderUnavailable means the processor could not be reached or
	// errored. It maps to a 502.
	ErrProviderUnavailable = errors.New("payments: provider unavailable")
)

// Charge is the result of a successful authorisation and capture.
type Charge struct {
	// ProviderID is Stripe's PaymentIntent id. Stored so a refund can reference
	// it and so a replayed request can be reconciled against what Stripe
	// already recorded.
	ProviderID  string
	AmountCents int64
	Currency    string
	Status      string
}

// Refund is the result of a successful refund.
type Refund struct {
	ProviderID  string
	ChargeID    string
	AmountCents int64
	Status      string
}

// ChargeRequest describes money to take.
//
// Amounts are integer cents. Money never touches a float: 0.1 + 0.2 is not 0.3
// in binary floating point, and a rounding error in a payment total is the kind
// of bug that shows up in a reconciliation report weeks later.
type ChargeRequest struct {
	AmountCents     int64
	Currency        string
	PaymentMethodID string

	// IdempotencyKey is forwarded to the processor. Our own idempotency table
	// cannot protect the window between "we called Stripe" and "we committed":
	// if the process dies there, a charge exists with no local record. Stripe's
	// key means the retry returns the original PaymentIntent instead of
	// creating a second one, so the two sides can be reconciled.
	IdempotencyKey string

	// Metadata is echoed back on the Stripe object, which makes a charge
	// traceable to a ticket from the dashboard during an incident.
	Metadata map[string]string
}

// RefundRequest describes money to give back.
type RefundRequest struct {
	ChargeID       string
	AmountCents    int64
	IdempotencyKey string
	Metadata       map[string]string
}

// Provider is the payment processor seam. Only ticket-service depends on it;
// putting this in the shared pkg module would compile a payment SDK into the
// gateway, the user service and the catalog service.
type Provider interface {
	Charge(ctx context.Context, req ChargeRequest) (*Charge, error)
	Refund(ctx context.Context, req RefundRequest) (*Refund, error)
}
