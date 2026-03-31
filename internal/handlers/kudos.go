package handlers

import (
	"encoding/json"
	"net/http"
	"time"

	"teamkudos/internal/middleware"
)

type sendKudosRequest struct {
	BadgeID     int    `json:"badge_id"`
	RecipientID int    `json:"recipient_id"`
	TeamID      int    `json:"team_id"`
	Message     string `json:"message"`
}

// SendKudos sends a kudos badge to a teammate
func (h *Handler) SendKudos(w http.ResponseWriter, r *http.Request) {
	userID := h.requireUserID(w, r)
	if userID == 0 {
		return
	}

	var req sendKudosRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		h.Error(w, http.StatusBadRequest, "invalid request body")
		return
	}

	if req.BadgeID == 0 || req.RecipientID == 0 || req.TeamID == 0 {
		h.Error(w, http.StatusBadRequest, "badge_id, recipient_id, and team_id are required")
		return
	}

	if req.RecipientID == userID {
		h.Error(w, http.StatusBadRequest, "you can't send kudos to yourself!")
		return
	}

	// Verify both sender and recipient are in the team
	var memberCount int
	err := h.db.QueryRow(
		`SELECT COUNT(*) FROM team_members
		 WHERE team_id = $1 AND user_id IN ($2, $3)`,
		req.TeamID, userID, req.RecipientID,
	).Scan(&memberCount)
	if err != nil || memberCount != 2 {
		h.Error(w, http.StatusForbidden, "both sender and recipient must be members of the team")
		return
	}

	// Verify badge exists
	var badgeExists bool
	h.db.QueryRow(`SELECT EXISTS(SELECT 1 FROM badges WHERE id = $1)`, req.BadgeID).Scan(&badgeExists)
	if !badgeExists {
		h.Error(w, http.StatusBadRequest, "badge not found")
		return
	}

	var kudosID int
	var createdAt time.Time
	err = h.db.QueryRow(
		`INSERT INTO kudos (badge_id, sender_id, recipient_id, team_id, message)
		 VALUES ($1, $2, $3, $4, $5)
		 RETURNING id, created_at`,
		req.BadgeID, userID, req.RecipientID, req.TeamID, req.Message,
	).Scan(&kudosID, &createdAt)
	if err != nil {
		h.Error(w, http.StatusInternalServerError, "failed to send kudos")
		return
	}

	h.JSON(w, http.StatusCreated, map[string]interface{}{
		"id":         kudosID,
		"message":    "kudos sent!",
		"created_at": createdAt,
	})
}

// GetTeamFeed returns the kudos feed for a team
// Route: GET /api/teams/:teamId/feed → segment index 2
func (h *Handler) GetTeamFeed(w http.ResponseWriter, r *http.Request) {
	authUser, _ := middleware.GetUser(r)
	teamID := pathSegment(r, 2)

	// Verify membership via auth_id
	var isMember bool
	h.db.QueryRow(
		`SELECT EXISTS(
			SELECT 1 FROM team_members tm
			JOIN users u ON u.id = tm.user_id
			WHERE tm.team_id = $1 AND u.auth_id = $2
		)`, teamID, authUser.ID,
	).Scan(&isMember)
	if !isMember {
		h.Error(w, http.StatusForbidden, "you are not a member of this team")
		return
	}

	rows, err := h.db.Query(
		`SELECT k.id, k.message, k.created_at,
		        b.slug, b.name, b.emoji,
		        s.id, s.name, s.avatar,
		        r.id, r.name, r.avatar
		 FROM kudos k
		 JOIN badges b ON b.id = k.badge_id
		 JOIN users s ON s.id = k.sender_id
		 JOIN users r ON r.id = k.recipient_id
		 WHERE k.team_id = $1
		 ORDER BY k.created_at DESC
		 LIMIT 50`, teamID,
	)
	if err != nil {
		h.Error(w, http.StatusInternalServerError, "failed to fetch feed")
		return
	}
	defer rows.Close()

	type feedItem struct {
		ID              int       `json:"id"`
		Message         string    `json:"message"`
		CreatedAt       time.Time `json:"created_at"`
		BadgeSlug       string    `json:"badge_slug"`
		BadgeName       string    `json:"badge_name"`
		BadgeEmoji      string    `json:"badge_emoji"`
		SenderID        int       `json:"sender_id"`
		SenderName      string    `json:"sender_name"`
		SenderAvatar    string    `json:"sender_avatar"`
		RecipientID     int       `json:"recipient_id"`
		RecipientName   string    `json:"recipient_name"`
		RecipientAvatar string    `json:"recipient_avatar"`
	}

	var feed []feedItem
	for rows.Next() {
		var f feedItem
		if err := rows.Scan(
			&f.ID, &f.Message, &f.CreatedAt,
			&f.BadgeSlug, &f.BadgeName, &f.BadgeEmoji,
			&f.SenderID, &f.SenderName, &f.SenderAvatar,
			&f.RecipientID, &f.RecipientName, &f.RecipientAvatar,
		); err != nil {
			h.Error(w, http.StatusInternalServerError, "failed to scan feed item")
			return
		}
		feed = append(feed, f)
	}

	if feed == nil {
		feed = []feedItem{}
	}

	h.JSON(w, http.StatusOK, feed)
}
