package payments

import (
	"context"
	"fmt"
	"sync"
)

// FakeProvider is an in-memory Provider for tests.
//
// It records what it was asked to do and can be told to fail, which is what
// makes the interesting tests possible: a charge that succeeds followed by a
// commit that does not, a decline, a processor outage, and a retry that must
// not double-charge.
type FakeProvider struct {
	mu sync.Mutex

	// ChargeErr, when set, is returned by the next Charge call.
	ChargeErr error
	// RefundErr, when set, is returned by the next Refund call.
	RefundErr error

	charges []ChargeRequest
	refunds []RefundRequest

	// byKey records the charge returned for an idempotency key, mirroring how
	// Stripe replays the original object rather than creating a second one.
	byKey map[string]*Charge

	seq int
}

func NewFakeProvider() *FakeProvider {
	return &FakeProvider{byKey: make(map[string]*Charge)}
}

func (f *FakeProvider) Charge(ctx context.Context, req ChargeRequest) (*Charge, error) {
	f.mu.Lock()
	defer f.mu.Unlock()

	if f.ChargeErr != nil {
		return nil, f.ChargeErr
	}

	// Replay rather than re-charge, the behaviour the real processor guarantees
	// for a repeated idempotency key.
	if req.IdempotencyKey != "" {
		if existing, ok := f.byKey[req.IdempotencyKey]; ok {
			f.charges = append(f.charges, req)
			return existing, nil
		}
	}

	f.seq++
	charge := &Charge{
		ProviderID:  fmt.Sprintf("pi_fake_%d", f.seq),
		AmountCents: req.AmountCents,
		Currency:    req.Currency,
		Status:      "succeeded",
	}
	if req.IdempotencyKey != "" {
		f.byKey[req.IdempotencyKey] = charge
	}
	f.charges = append(f.charges, req)
	return charge, nil
}

func (f *FakeProvider) Refund(ctx context.Context, req RefundRequest) (*Refund, error) {
	f.mu.Lock()
	defer f.mu.Unlock()

	if f.RefundErr != nil {
		return nil, f.RefundErr
	}

	f.seq++
	f.refunds = append(f.refunds, req)
	return &Refund{
		ProviderID:  fmt.Sprintf("re_fake_%d", f.seq),
		ChargeID:    req.ChargeID,
		AmountCents: req.AmountCents,
		Status:      "succeeded",
	}, nil
}

// Charges returns every charge attempt, including replays. A test asserting
// "exactly one charge" should check DistinctCharges instead.
func (f *FakeProvider) Charges() []ChargeRequest {
	f.mu.Lock()
	defer f.mu.Unlock()
	out := make([]ChargeRequest, len(f.charges))
	copy(out, f.charges)
	return out
}

// DistinctCharges counts charges that actually created a new PaymentIntent,
// which is the number a customer would see on their statement.
func (f *FakeProvider) DistinctCharges() int {
	f.mu.Lock()
	defer f.mu.Unlock()
	return len(f.byKey)
}

// Refunds returns every refund attempt.
func (f *FakeProvider) Refunds() []RefundRequest {
	f.mu.Lock()
	defer f.mu.Unlock()
	out := make([]RefundRequest, len(f.refunds))
	copy(out, f.refunds)
	return out
}

// compile-time assertion that the fake satisfies the interface.
var _ Provider = (*FakeProvider)(nil)
