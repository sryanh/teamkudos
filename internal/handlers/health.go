package handlers

import (
	"encoding/json"
	"net/http"
	"strings"

	"teamkudos/internal/config"
	"teamkudos/internal/database"
	"teamkudos/internal/middleware"
)

type Handler struct {
	db  *database.DB
	cfg *config.Config
}

func New(db *database.DB, cfg *config.Config) *Handler {
	return &Handler{db: db, cfg: cfg}
}

// WithAuth wraps a handler with JWT authentication
func (h *Handler) WithAuth(next http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		token := middleware.ExtractBearerToken(r)
		if token == "" {
			h.Error(w, http.StatusUnauthorized, "missing authorization header")
			return
		}

		user, err := middleware.VerifyToken(token, h.cfg.SupabaseJWTSecret)
		if err != nil {
			h.Error(w, http.StatusUnauthorized, "invalid token")
			return
		}

		r = middleware.SetUser(r, user)
		next(w, r)
	}
}

// JSON writes a JSON response with the given status code
func (h *Handler) JSON(w http.ResponseWriter, status int, data interface{}) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	json.NewEncoder(w).Encode(data)
}

// Error writes a JSON error response
func (h *Handler) Error(w http.ResponseWriter, status int, message string) {
	h.JSON(w, status, map[string]string{"error": message})
}

// pathSegment returns the URL path segment at the given index (0-based).
// e.g. for "/api/teams/42/members", index 2 returns "42".
func pathSegment(r *http.Request, index int) string {
	// Trim leading slash then split
	parts := strings.Split(strings.TrimPrefix(r.URL.Path, "/"), "/")
	if index < len(parts) {
		return parts[index]
	}
	return ""
}

// Health check endpoint
func (h *Handler) Health(w http.ResponseWriter, r *http.Request) {
	err := h.db.Ping()

	status := "healthy"
	dbStatus := "connected"

	if err != nil {
		status = "unhealthy"
		dbStatus = "disconnected"
	}

	h.JSON(w, http.StatusOK, map[string]string{
		"status":   status,
		"database": dbStatus,
	})
}
