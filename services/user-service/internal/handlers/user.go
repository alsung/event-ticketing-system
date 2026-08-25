package handlers

import (
	"encoding/json"
	"errors"
	"log"
	"net/http"
	"strings"

	"github.com/alsung/event-ticketing-system/services/pkg/database"
	"github.com/alsung/event-ticketing-system/services/user-service/internal/models"
	"github.com/alsung/event-ticketing-system/services/user-service/internal/utils"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"golang.org/x/crypto/bcrypt"
)

// uniqueViolation is the SQLSTATE Postgres returns when a unique constraint is
// breached -- here, a duplicate email.
const uniqueViolation = "23505"

// RegisterUser handles user registration
func RegisterUser(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	var user models.User
	if err := json.NewDecoder(r.Body).Decode(&user); err != nil {
		http.Error(w, "Invalid request payload", http.StatusBadRequest)
		return
	}

	user.Email = strings.TrimSpace(strings.ToLower(user.Email))
	if user.Email == "" || user.Password == "" {
		http.Error(w, "Email and password are required", http.StatusBadRequest)
		return
	}

	ctx := r.Context()
	pool, err := database.Pool(ctx)
	if err != nil {
		http.Error(w, "Database unavailable", http.StatusInternalServerError)
		return
	}

	// bcrypt, not a bare SHA. It is deliberately slow and salts each digest
	// internally, so identical passwords produce different hashes and an offline
	// attacker cannot precompute a rainbow table. DefaultCost is the tuning knob:
	// raise it as hardware gets faster.
	hash, err := bcrypt.GenerateFromPassword([]byte(user.Password), bcrypt.DefaultCost)
	if err != nil {
		http.Error(w, "Failed to process password", http.StatusInternalServerError)
		return
	}

	_, err = pool.Exec(ctx,
		"INSERT INTO users (email, password_hash, full_name) VALUES ($1, $2, $3)",
		user.Email, string(hash), user.FullName,
	)
	if err != nil {
		var pgErr *pgconn.PgError
		if errors.As(err, &pgErr) && pgErr.Code == uniqueViolation {
			http.Error(w, "An account with that email already exists", http.StatusConflict)
			return
		}
		log.Printf("register failed: %v", err)
		http.Error(w, "Failed to register user", http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusCreated)
	_ = json.NewEncoder(w).Encode(map[string]string{
		"message": "User registered successfully",
	})
}

// LoginUser handles user login and JWT generation
func LoginUser(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	var req struct {
		Email    string `json:"email"`
		Password string `json:"password"`
	}

	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}

	ctx := r.Context()
	pool, err := database.Pool(ctx)
	if err != nil {
		http.Error(w, "Database unavailable", http.StatusInternalServerError)
		return
	}

	// The password can no longer be part of the WHERE clause: a bcrypt digest is
	// salted, so the stored value cannot be recomputed from the input. Look the
	// user up by email, then compare in constant time.
	var (
		user models.User
		hash string
	)
	err = pool.QueryRow(ctx,
		"SELECT id, email, password_hash FROM users WHERE email = $1",
		strings.TrimSpace(strings.ToLower(req.Email)),
	).Scan(&user.ID, &user.Email, &hash)

	switch {
	case errors.Is(err, pgx.ErrNoRows):
		// Run a comparison anyway before failing. Returning immediately would
		// make a missing account measurably faster than a wrong password, which
		// leaks which emails are registered.
		_ = bcrypt.CompareHashAndPassword([]byte("$2a$10$invalidinvalidinvalidinvalidinvalidinvalidinvalidinvalidinv"), []byte(req.Password))
		http.Error(w, "Invalid credentials", http.StatusUnauthorized)
		return
	case err != nil:
		log.Printf("login lookup failed: %v", err)
		http.Error(w, "Login failed", http.StatusInternalServerError)
		return
	}

	// CompareHashAndPassword is constant-time, so it does not leak how much of
	// the hash matched.
	if err := bcrypt.CompareHashAndPassword([]byte(hash), []byte(req.Password)); err != nil {
		http.Error(w, "Invalid credentials", http.StatusUnauthorized)
		return
	}

	token, err := utils.GenerateJWT(user.ID, user.Email)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(map[string]string{
		"token": token,
	})
}
