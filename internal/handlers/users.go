package handlers

import (
	"database/sql"
	"encoding/json"
	"net/http"
	"time"

	"teamkudos/internal/middleware"
)

type User struct {
	ID                 int       `json:"id"`
	AuthID             string    `json:"auth_id"`
	Email              string    `json:"email"`
	Name               string    `json:"name"`
	Avatar             string    `json:"avatar"`
	EmailNotifications bool      `json:"email_notifications"`
	CreatedAt          time.Time `json:"created_at"`
}

type profileRequest struct {
	Name   string `json:"name"`
	Avatar string `json:"avatar"`
}

// GetCurrentUser returns the authenticated user's profile
func (h *Handler) GetCurrentUser(w http.ResponseWriter, r *http.Request) {
	authUser, _ := middleware.GetUser(r)

	var u User
	err := h.db.QueryRow(
		`SELECT id, auth_id, email, name, avatar, email_notifications, created_at
		 FROM users WHERE auth_id = $1`, authUser.ID,
	).Scan(&u.ID, &u.AuthID, &u.Email, &u.Name, &u.Avatar, &u.EmailNotifications, &u.CreatedAt)

	if err == sql.ErrNoRows {
		h.JSON(w, http.StatusOK, map[string]interface{}{
			"auth_id":        authUser.ID,
			"email":          authUser.Email,
			"profile_exists": false,
		})
		return
	}
	if err != nil {
		h.Error(w, http.StatusInternalServerError, "failed to fetch user")
		return
	}

	h.JSON(w, http.StatusOK, u)
}

// CreateProfile creates or updates the authenticated user's profile
func (h *Handler) CreateProfile(w http.ResponseWriter, r *http.Request) {
	authUser, _ := middleware.GetUser(r)

	var req profileRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		h.Error(w, http.StatusBadRequest, "invalid request body")
		return
	}

	if req.Name == "" {
		h.Error(w, http.StatusBadRequest, "name is required")
		return
	}

	var u User
	err := h.db.QueryRow(
		`INSERT INTO users (auth_id, email, name, avatar)
		 VALUES ($1, $2, $3, $4)
		 ON CONFLICT (auth_id) DO UPDATE SET name = $3, avatar = $4
		 RETURNING id, auth_id, email, name, avatar, email_notifications, created_at`,
		authUser.ID, authUser.Email, req.Name, req.Avatar,
	).Scan(&u.ID, &u.AuthID, &u.Email, &u.Name, &u.Avatar, &u.EmailNotifications, &u.CreatedAt)

	if err != nil {
		h.Error(w, http.StatusInternalServerError, "failed to save profile")
		return
	}

	h.JSON(w, http.StatusOK, u)
}

// GetUserKudos returns kudos received by a specific user
// Route: GET /api/users/:userId/kudos → segment index 2
func (h *Handler) GetUserKudos(w http.ResponseWriter, r *http.Request) {
	userID := pathSegment(r, 2)

	rows, err := h.db.Query(
		`SELECT k.id, k.message, k.created_at,
		        b.slug, b.name, b.emoji,
		        s.name, s.avatar
		 FROM kudos k
		 JOIN badges b ON b.id = k.badge_id
		 JOIN users s ON s.id = k.sender_id
		 WHERE k.recipient_id = $1
		 ORDER BY k.created_at DESC
		 LIMIT 50`, userID,
	)
	if err != nil {
		h.Error(w, http.StatusInternalServerError, "failed to fetch kudos")
		return
	}
	defer rows.Close()

	type kudosItem struct {
		ID           int       `json:"id"`
		Message      string    `json:"message"`
		CreatedAt    time.Time `json:"created_at"`
		BadgeSlug    string    `json:"badge_slug"`
		BadgeName    string    `json:"badge_name"`
		BadgeEmoji   string    `json:"badge_emoji"`
		SenderName   string    `json:"sender_name"`
		SenderAvatar string    `json:"sender_avatar"`
	}

	var kudos []kudosItem
	for rows.Next() {
		var k kudosItem
		if err := rows.Scan(&k.ID, &k.Message, &k.CreatedAt, &k.BadgeSlug, &k.BadgeName, &k.BadgeEmoji, &k.SenderName, &k.SenderAvatar); err != nil {
			h.Error(w, http.StatusInternalServerError, "failed to scan kudos")
			return
		}
		kudos = append(kudos, k)
	}

	if kudos == nil {
		kudos = []kudosItem{}
	}

	h.JSON(w, http.StatusOK, kudos)
}
