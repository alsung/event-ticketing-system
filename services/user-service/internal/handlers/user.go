package handlers

import (
	"context"
	"encoding/json"
	"net/http"

	"github.com/alsung/event-ticketing-system/services/user-service/internal/database"
	"github.com/alsung/event-ticketing-system/services/user-service/internal/models"
	"github.com/alsung/event-ticketing-system/services/user-service/internal/utils"
)

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

	conn, err := database.NewDatabaseConnection(context.Background())
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	defer func() { _ = conn.Close(context.Background()) }()

	_, err = conn.Exec(context.Background(),
		"INSERT INTO users (email, password, full_name) VALUEs ($1, $2, $3)",
		user.Email, user.Password, user.FullName,
	)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
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

	conn, err := database.NewDatabaseConnection(context.Background())
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	defer func() { _ = conn.Close(context.Background()) }()

	var user models.User
	err = conn.QueryRow(context.Background(),
		"SELECT id, email FROM users WHERE email = $1 AND password = $2",
		req.Email, req.Password,
	).Scan(&user.ID, &user.Email)

	if err != nil {
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
