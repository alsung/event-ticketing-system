// Package idempotency deduplicates repeated write requests.
//
// This is layer one of two. It catches the common case -- a double-clicked
// button, a mobile client retrying on a flaky connection, a load balancer
// replaying a request -- by remembering the response to a key and replaying it.
//
// It cannot cover the window between calling the payment processor and
// committing locally: if the process dies there, a charge exists with no local
// record and no stored response. That gap is why the same key is also forwarded
// to Stripe, whose own idempotency guarantees the retry returns the original
// PaymentIntent rather than creating a second one.
package idempotency

import (
	"bytes"
	"encoding/json"
	"io"
	"log"
	"net/http"

	"github.com/alsung/event-ticketing-system/services/pkg/auth"
	"github.com/alsung/event-ticketing-system/services/pkg/database"
	"github.com/alsung/event-ticketing-system/services/ticket-service/internal/store"
	"github.com/jackc/pgx/v5/pgxpool"
)

// HeaderName is the header clients send to make a request idempotent.
const HeaderName = "Idempotency-Key"

// maxBodyBytes caps how much of a request body will be buffered for hashing.
const maxBodyBytes = 1 << 20 // 1 MiB

// capture buffers a handler's response so it can be stored and replayed.
// http.ResponseWriter is write-only, so wrapping is the only way to observe
// what a handler produced.
type capture struct {
	http.ResponseWriter
	status int
	body   bytes.Buffer
}

func (c *capture) WriteHeader(code int) {
	c.status = code
	c.ResponseWriter.WriteHeader(code)
}

func (c *capture) Write(b []byte) (int, error) {
	c.body.Write(b)
	return c.ResponseWriter.Write(b)
}

// Middleware deduplicates requests carrying an Idempotency-Key header.
//
// Requests without the header pass straight through. The header is optional
// rather than required so existing clients keep working, but without it a retry
// may charge twice.
func Middleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		key := r.Header.Get(HeaderName)
		if key == "" {
			next.ServeHTTP(w, r)
			return
		}

		claims, err := auth.DefaultVerifier().FromRequest(r)
		if err != nil {
			http.Error(w, "Unauthorized", http.StatusUnauthorized)
			return
		}

		ctx := r.Context()
		db, err := database.Pool(ctx)
		if err != nil {
			http.Error(w, "Database unavailable", http.StatusInternalServerError)
			return
		}

		body, err := io.ReadAll(io.LimitReader(r.Body, maxBodyBytes))
		if err != nil {
			http.Error(w, "Could not read request body", http.StatusBadRequest)
			return
		}
		// The handler still needs to read the body we just consumed.
		r.Body = io.NopCloser(bytes.NewReader(body))

		hash := store.HashRequest(body)
		existing, err := store.ReserveKey(ctx, db, key, claims.UserID, r.URL.Path, hash)

		switch {
		case err == nil:
			// Key is ours. Do the work, then record what happened.
			run(w, r, next, db, key)
			return

		case existing == nil:
			log.Printf("idempotency: reserve %s: %v", key, err)
			http.Error(w, "Could not process request", http.StatusInternalServerError)
			return

		case existing.RequestHash != hash:
			// Same key, different body. Guessing which request was meant would
			// be worse than refusing: this is a client bug worth surfacing.
			http.Error(w,
				"Idempotency-Key was already used with a different request body",
				http.StatusUnprocessableEntity)
			return

		case !existing.Completed():
			// The original attempt is still running. Replaying nothing and
			// starting a second attempt would defeat the point.
			http.Error(w,
				"A request with this Idempotency-Key is still in progress, retry shortly",
				http.StatusConflict)
			return

		default:
			replay(w, existing)
			return
		}
	})
}

// run executes the handler and stores its response against the key.
func run(w http.ResponseWriter, r *http.Request, next http.Handler, db *pgxpool.Pool, key string) {
	rec := &capture{ResponseWriter: w, status: http.StatusOK}
	next.ServeHTTP(rec, r)

	ctx := r.Context()

	// A 5xx means the work failed in a way that may not have changed anything,
	// so release the key and let the caller retry. Client errors keep their key:
	// replaying the same rejection is the correct answer to a repeated bad
	// request, and a failure that may have charged a card must never look fresh.
	if rec.status >= 500 {
		if err := store.ReleaseKey(ctx, db, key); err != nil {
			log.Printf("idempotency: release %s: %v", key, err)
		}
		return
	}

	if err := store.CompleteKey(ctx, db, key, rec.status, rec.body.Bytes()); err != nil {
		// The work succeeded even though recording it did not. Log loudly: a
		// retry will now re-run rather than replay.
		log.Printf("idempotency: complete %s: %v", key, err)
	}
}

// replay returns the stored response verbatim.
func replay(w http.ResponseWriter, rec *store.IdempotencyRecord) {
	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("Idempotency-Replayed", "true")
	w.WriteHeader(*rec.ResponseStatus)
	if len(rec.ResponseBody) > 0 {
		_, _ = w.Write(rec.ResponseBody)
		return
	}
	_ = json.NewEncoder(w).Encode(map[string]string{"message": "replayed"})
}
