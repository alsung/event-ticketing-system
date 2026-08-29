package payments

import "testing"

func TestNewStripeProviderRejectsEmptyKey(t *testing.T) {
	if _, err := NewStripeProvider(""); err == nil {
		t.Fatal("expected an error for an empty key")
	}
}

func TestNewStripeProviderRefusesLiveKey(t *testing.T) {
	// This project has no business entity behind it. A live key here would move
	// real money, so the guard matters more than the inconvenience.
	if _, err := NewStripeProvider("sk_live_abcdef0123456789"); err == nil {
		t.Fatal("expected a live key to be refused")
	}
}

func TestNewStripeProviderAcceptsTestKey(t *testing.T) {
	p, err := NewStripeProvider("sk_test_abcdef0123456789")
	if err != nil {
		t.Fatalf("test key rejected: %v", err)
	}
	if p == nil {
		t.Fatal("expected a provider")
	}
}
