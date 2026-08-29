package payments

import (
	"context"
	"errors"
	"fmt"

	stripe "github.com/stripe/stripe-go/v86"
)

// StripeProvider talks to Stripe. It is one of two Provider implementations;
// the other is FakeProvider, which is what the tests use.
type StripeProvider struct {
	client *stripe.Client
}

// NewStripeProvider builds a provider from a secret key.
//
// Refuses a live key outright. This project is a study build with no business
// entity behind it, and the cost of accidentally moving real money is far worse
// than the cost of an unnecessary guard.
func NewStripeProvider(secretKey string) (*StripeProvider, error) {
	if secretKey == "" {
		return nil, errors.New("payments: STRIPE_SECRET_KEY is empty")
	}
	if len(secretKey) > 8 && secretKey[:8] == "sk_live_" {
		return nil, errors.New("payments: refusing a live secret key; use a test key (sk_test_)")
	}
	return &StripeProvider{client: stripe.NewClient(secretKey)}, nil
}

// Charge creates and confirms a PaymentIntent.
//
// The idempotency key is forwarded to Stripe, which is the second of the two
// layers protecting against double-charging. Our own idempotency table cannot
// cover the window between this call returning and our transaction committing:
// if the process dies there, a charge exists that we have no record of. Stripe's
// key means the retry returns the original PaymentIntent instead of creating a
// second one, so the two sides can be reconciled.
func (s *StripeProvider) Charge(ctx context.Context, req ChargeRequest) (*Charge, error) {
	currency := req.Currency
	if currency == "" {
		currency = "usd"
	}

	params := &stripe.PaymentIntentCreateParams{
		Amount:   stripe.Int64(req.AmountCents),
		Currency: stripe.String(currency),
		// Create and confirm in one call: there is no separate authorise step
		// in this flow, and a second round trip is a second chance to fail
		// between the two.
		Confirm: stripe.Bool(true),
		// The payment method is attached server-side rather than confirmed from
		// the browser, so tell Stripe not to expect a redirect it cannot
		// perform from a backend call.
		AutomaticPaymentMethods: &stripe.PaymentIntentCreateAutomaticPaymentMethodsParams{
			Enabled:        stripe.Bool(true),
			AllowRedirects: stripe.String("never"),
		},
	}
	if req.PaymentMethodID != "" {
		params.PaymentMethod = stripe.String(req.PaymentMethodID)
	}
	if len(req.Metadata) > 0 {
		params.Metadata = req.Metadata
	}
	if req.IdempotencyKey != "" {
		params.SetIdempotencyKey(req.IdempotencyKey)
	}

	intent, err := s.client.V1PaymentIntents.Create(ctx, params)
	if err != nil {
		return nil, classify(err)
	}

	return &Charge{
		ProviderID:  intent.ID,
		AmountCents: intent.Amount,
		Currency:    string(intent.Currency),
		Status:      string(intent.Status),
	}, nil
}

// Refund returns money against a PaymentIntent.
func (s *StripeProvider) Refund(ctx context.Context, req RefundRequest) (*Refund, error) {
	params := &stripe.RefundCreateParams{
		PaymentIntent: stripe.String(req.ChargeID),
	}
	if req.AmountCents > 0 {
		params.Amount = stripe.Int64(req.AmountCents)
	}
	if len(req.Metadata) > 0 {
		params.Metadata = req.Metadata
	}
	if req.IdempotencyKey != "" {
		params.SetIdempotencyKey(req.IdempotencyKey)
	}

	refund, err := s.client.V1Refunds.Create(ctx, params)
	if err != nil {
		return nil, classify(err)
	}

	return &Refund{
		ProviderID:  refund.ID,
		ChargeID:    req.ChargeID,
		AmountCents: refund.Amount,
		Status:      string(refund.Status),
	}, nil
}

// classify separates a customer's problem from ours.
//
// A declined card is not a server fault: nothing is broken, the charge was
// refused, and the caller should be told so with a 402. Everything else --
// network failures, rate limits, malformed requests, Stripe outages -- is our
// problem and maps to a 502.
func classify(err error) error {
	var stripeErr *stripe.Error
	if !errors.As(err, &stripeErr) {
		return fmt.Errorf("%w: %v", ErrProviderUnavailable, err)
	}

	if stripeErr.Type == stripe.ErrorTypeCard {
		return fmt.Errorf("%w: %s", ErrCardDeclined, stripeErr.Msg)
	}
	return fmt.Errorf("%w: %s", ErrProviderUnavailable, stripeErr.Msg)
}

// compile-time assertion that the real provider satisfies the same interface as
// the fake, which is what lets tests substitute one for the other.
var _ Provider = (*StripeProvider)(nil)
