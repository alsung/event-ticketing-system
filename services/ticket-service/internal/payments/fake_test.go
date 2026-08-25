package payments

import (
	"context"
	"errors"
	"testing"
)

func TestFakeChargeReplaysOnRepeatedKey(t *testing.T) {
	p := NewFakeProvider()
	ctx := context.Background()
	req := ChargeRequest{AmountCents: 4999, Currency: "usd", IdempotencyKey: "key-1"}

	first, err := p.Charge(ctx, req)
	if err != nil {
		t.Fatalf("first charge: %v", err)
	}
	second, err := p.Charge(ctx, req)
	if err != nil {
		t.Fatalf("second charge: %v", err)
	}

	if first.ProviderID != second.ProviderID {
		t.Errorf("repeated key created a second intent: %s then %s",
			first.ProviderID, second.ProviderID)
	}
	if got := p.DistinctCharges(); got != 1 {
		t.Errorf("expected 1 distinct charge, got %d", got)
	}
	if got := len(p.Charges()); got != 2 {
		t.Errorf("expected 2 recorded attempts, got %d", got)
	}
}

func TestFakeChargeDistinctKeysCreateDistinctIntents(t *testing.T) {
	p := NewFakeProvider()
	ctx := context.Background()

	a, _ := p.Charge(ctx, ChargeRequest{AmountCents: 100, IdempotencyKey: "a"})
	b, _ := p.Charge(ctx, ChargeRequest{AmountCents: 100, IdempotencyKey: "b"})

	if a.ProviderID == b.ProviderID {
		t.Errorf("distinct keys reused an intent: %s", a.ProviderID)
	}
	if got := p.DistinctCharges(); got != 2 {
		t.Errorf("expected 2 distinct charges, got %d", got)
	}
}

func TestFakeChargePropagatesFailure(t *testing.T) {
	p := NewFakeProvider()
	p.ChargeErr = ErrCardDeclined

	_, err := p.Charge(context.Background(), ChargeRequest{AmountCents: 100})
	if !errors.Is(err, ErrCardDeclined) {
		t.Fatalf("expected ErrCardDeclined, got %v", err)
	}
	if got := p.DistinctCharges(); got != 0 {
		t.Errorf("a declined charge should record nothing, got %d", got)
	}
}

func TestFakeRefundRecordsCharge(t *testing.T) {
	p := NewFakeProvider()
	ctx := context.Background()

	charge, _ := p.Charge(ctx, ChargeRequest{AmountCents: 4999, IdempotencyKey: "k"})
	refund, err := p.Refund(ctx, RefundRequest{
		ChargeID:       charge.ProviderID,
		AmountCents:    charge.AmountCents,
		IdempotencyKey: "k-refund",
	})
	if err != nil {
		t.Fatalf("refund: %v", err)
	}
	if refund.ChargeID != charge.ProviderID {
		t.Errorf("refund references %s, expected %s", refund.ChargeID, charge.ProviderID)
	}
	if got := len(p.Refunds()); got != 1 {
		t.Errorf("expected 1 refund, got %d", got)
	}
}
