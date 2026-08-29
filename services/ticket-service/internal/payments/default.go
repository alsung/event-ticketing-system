package payments

import (
	"log"
	"os"
	"sync"
)

var (
	defaultOnce     sync.Once
	defaultProvider Provider
)

// Default returns the process-wide provider, chosen from the environment.
//
// With STRIPE_SECRET_KEY set it talks to Stripe; without one it falls back to
// the fake so the stack still runs end to end for anyone who clones this
// without a Stripe account. The fallback is logged loudly, because a service
// quietly not charging anyone is a failure that should never be discovered from
// a revenue report.
//
// Handlers are package-level functions, so there is nowhere to inject a
// provider yet. Converting them to structs is the natural next refactor, at
// which point this can go away -- the same shape as auth.DefaultVerifier.
func Default() Provider {
	defaultOnce.Do(func() {
		key := os.Getenv("STRIPE_SECRET_KEY")
		if key == "" {
			log.Println("payments: STRIPE_SECRET_KEY not set, using the fake provider; no money will move")
			defaultProvider = NewFakeProvider()
			return
		}

		p, err := NewStripeProvider(key)
		if err != nil {
			log.Printf("payments: %v; using the fake provider", err)
			defaultProvider = NewFakeProvider()
			return
		}
		log.Println("payments: Stripe provider active (test mode)")
		defaultProvider = p
	})
	return defaultProvider
}
