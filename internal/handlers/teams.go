package handlers

import (
	"crypto/rand"
	"database/sql"
	"encoding/hex"
	"encoding/json"
	"net/http"
	"time"

	"teamkudos/internal/middleware"
)

type Team struct {
	ID         int       `json:"id"`
	Name       string    `json:"name"`
	InviteCode string    `json:"invite_code"`
	CreatedAt  time.Time `json:"created_at"`
}

type TeamMember struct {
	ID       int       `json:"id"`
	UserID   int       `json:"user_id"`
	Name     string    `json:"name"`
	Email    string    `json:"email"`
	Avatar   string    `json:"avatar"`
	Role     string    `json:"role"`
	JoinedAt time.Time `json:"joined_at"`
}

func generateInviteCode() string {
	b := make([]byte, 4)
	rand.Read(b)
	return hex.EncodeToString(b)
}

// requireUserID looks up the internal user ID from the auth context.
// Returns 0 and sends an error response if the user hasn't created a profile.
func (h *Handler) requireUserID(w http.ResponseWriter, r *http.Request) int {
	authUser, _ := middleware.GetUser(r)
	var userID int
	err := h.db.QueryRow(`SELECT id FROM users WHERE auth_id = $1`, authUser.ID).Scan(&userID)
	if err != nil {
		h.Error(w, http.StatusBadRequest, "please create a profile first")
		return 0
	}
	return userID
}

// CreateTeam creates a new team and adds the creator as admin
func (h *Handler) CreateTeam(w http.ResponseWriter, r *http.Request) {
	userID := h.requireUserID(w, r)
	if userID == 0 {
		return
	}

	var req struct {
		Name string `json:"name"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		h.Error(w, http.StatusBadRequest, "invalid request body")
		return
	}
	if req.Name == "" {
		h.Error(w, http.StatusBadRequest, "team name is required")
		return
	}

	tx, err := h.db.Begin()
	if err != nil {
		h.Error(w, http.StatusInternalServerError, "failed to create team")
		return
	}
	defer tx.Rollback()

	var team Team
	err = tx.QueryRow(
		`INSERT INTO teams (name, invite_code) VALUES ($1, $2)
		 RETURNING id, name, invite_code, created_at`,
		req.Name, generateInviteCode(),
	).Scan(&team.ID, &team.Name, &team.InviteCode, &team.CreatedAt)
	if err != nil {
		h.Error(w, http.StatusInternalServerError, "failed to create team")
		return
	}

	_, err = tx.Exec(
		`INSERT INTO team_members (team_id, user_id, role) VALUES ($1, $2, 'admin')`,
		team.ID, userID,
	)
	if err != nil {
		h.Error(w, http.StatusInternalServerError, "failed to add team member")
		return
	}

	if err := tx.Commit(); err != nil {
		h.Error(w, http.StatusInternalServerError, "failed to create team")
		return
	}

	h.JSON(w, http.StatusCreated, team)
}

// JoinTeam joins a team using an invite code
func (h *Handler) JoinTeam(w http.ResponseWriter, r *http.Request) {
	userID := h.requireUserID(w, r)
	if userID == 0 {
		return
	}

	var req struct {
		InviteCode string `json:"invite_code"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		h.Error(w, http.StatusBadRequest, "invalid request body")
		return
	}
	if req.InviteCode == "" {
		h.Error(w, http.StatusBadRequest, "invite code is required")
		return
	}

	var teamID int
	err := h.db.QueryRow(`SELECT id FROM teams WHERE invite_code = $1`, req.InviteCode).Scan(&teamID)
	if err == sql.ErrNoRows {
		h.Error(w, http.StatusNotFound, "invalid invite code")
		return
	}
	if err != nil {
		h.Error(w, http.StatusInternalServerError, "failed to look up team")
		return
	}

	var exists bool
	h.db.QueryRow(
		`SELECT EXISTS(SELECT 1 FROM team_members WHERE team_id = $1 AND user_id = $2)`,
		teamID, userID,
	).Scan(&exists)
	if exists {
		h.Error(w, http.StatusConflict, "you are already a member of this team")
		return
	}

	_, err = h.db.Exec(
		`INSERT INTO team_members (team_id, user_id, role) VALUES ($1, $2, 'member')`,
		teamID, userID,
	)
	if err != nil {
		h.Error(w, http.StatusInternalServerError, "failed to join team")
		return
	}

	h.JSON(w, http.StatusOK, map[string]interface{}{
		"message": "joined team successfully",
		"team_id": teamID,
	})
}

// GetTeam returns team details
// Route: GET /api/teams/:teamId → segment index 2
func (h *Handler) GetTeam(w http.ResponseWriter, r *http.Request) {
	userID := h.requireUserID(w, r)
	if userID == 0 {
		return
	}

	teamID := pathSegment(r, 2)

	var isMember bool
	h.db.QueryRow(
		`SELECT EXISTS(SELECT 1 FROM team_members WHERE team_id = $1 AND user_id = $2)`,
		teamID, userID,
	).Scan(&isMember)
	if !isMember {
		h.Error(w, http.StatusForbidden, "you are not a member of this team")
		return
	}

	var team Team
	err := h.db.QueryRow(
		`SELECT id, name, invite_code, created_at FROM teams WHERE id = $1`, teamID,
	).Scan(&team.ID, &team.Name, &team.InviteCode, &team.CreatedAt)
	if err != nil {
		h.Error(w, http.StatusNotFound, "team not found")
		return
	}

	h.JSON(w, http.StatusOK, team)
}

// ListTeamMembers returns all members of a team
// Route: GET /api/teams/:teamId/members → segment index 2
func (h *Handler) ListTeamMembers(w http.ResponseWriter, r *http.Request) {
	userID := h.requireUserID(w, r)
	if userID == 0 {
		return
	}

	teamID := pathSegment(r, 2)

	var isMember bool
	h.db.QueryRow(
		`SELECT EXISTS(SELECT 1 FROM team_members WHERE team_id = $1 AND user_id = $2)`,
		teamID, userID,
	).Scan(&isMember)
	if !isMember {
		h.Error(w, http.StatusForbidden, "you are not a member of this team")
		return
	}

	rows, err := h.db.Query(
		`SELECT tm.id, tm.user_id, u.name, u.email, u.avatar, tm.role, tm.joined_at
		 FROM team_members tm
		 JOIN users u ON u.id = tm.user_id
		 WHERE tm.team_id = $1
		 ORDER BY tm.joined_at`, teamID,
	)
	if err != nil {
		h.Error(w, http.StatusInternalServerError, "failed to fetch members")
		return
	}
	defer rows.Close()

	var members []TeamMember
	for rows.Next() {
		var m TeamMember
		if err := rows.Scan(&m.ID, &m.UserID, &m.Name, &m.Email, &m.Avatar, &m.Role, &m.JoinedAt); err != nil {
			h.Error(w, http.StatusInternalServerError, "failed to scan member")
			return
		}
		members = append(members, m)
	}

	if members == nil {
		members = []TeamMember{}
	}

	h.JSON(w, http.StatusOK, members)
}

// InviteMember adds a user to a team (admin only)
// Route: POST /api/teams/:teamId/members → segment index 2
func (h *Handler) InviteMember(w http.ResponseWriter, r *http.Request) {
	userID := h.requireUserID(w, r)
	if userID == 0 {
		return
	}

	teamID := pathSegment(r, 2)

	var role string
	err := h.db.QueryRow(
		`SELECT role FROM team_members WHERE team_id = $1 AND user_id = $2`,
		teamID, userID,
	).Scan(&role)
	if err != nil {
		h.Error(w, http.StatusForbidden, "you are not a member of this team")
		return
	}
	if role != "admin" {
		h.Error(w, http.StatusForbidden, "only admins can invite members")
		return
	}

	var req struct {
		Email string `json:"email"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		h.Error(w, http.StatusBadRequest, "invalid request body")
		return
	}
	if req.Email == "" {
		h.Error(w, http.StatusBadRequest, "email is required")
		return
	}

	var inviteeID int
	err = h.db.QueryRow(`SELECT id FROM users WHERE email = $1`, req.Email).Scan(&inviteeID)
	if err == sql.ErrNoRows {
		h.Error(w, http.StatusNotFound, "user not found — they need to create a profile first")
		return
	}
	if err != nil {
		h.Error(w, http.StatusInternalServerError, "failed to look up user")
		return
	}

	var exists bool
	h.db.QueryRow(
		`SELECT EXISTS(SELECT 1 FROM team_members WHERE team_id = $1 AND user_id = $2)`,
		teamID, inviteeID,
	).Scan(&exists)
	if exists {
		h.Error(w, http.StatusConflict, "user is already a member of this team")
		return
	}

	_, err = h.db.Exec(
		`INSERT INTO team_members (team_id, user_id, role) VALUES ($1, $2, 'member')`,
		teamID, inviteeID,
	)
	if err != nil {
		h.Error(w, http.StatusInternalServerError, "failed to add member")
		return
	}

	h.JSON(w, http.StatusOK, map[string]string{"message": "member added successfully"})
}

// RemoveMember removes a user from a team (admin only)
// Route: DELETE /api/teams/:teamId/members/:userId → segments 2 and 4
func (h *Handler) RemoveMember(w http.ResponseWriter, r *http.Request) {
	userID := h.requireUserID(w, r)
	if userID == 0 {
		return
	}

	teamID := pathSegment(r, 2)
	targetUserID := pathSegment(r, 4)

	var role string
	err := h.db.QueryRow(
		`SELECT role FROM team_members WHERE team_id = $1 AND user_id = $2`,
		teamID, userID,
	).Scan(&role)
	if err != nil {
		h.Error(w, http.StatusForbidden, "you are not a member of this team")
		return
	}
	if role != "admin" {
		h.Error(w, http.StatusForbidden, "only admins can remove members")
		return
	}

	result, err := h.db.Exec(
		`DELETE FROM team_members WHERE team_id = $1 AND user_id = $2`,
		teamID, targetUserID,
	)
	if err != nil {
		h.Error(w, http.StatusInternalServerError, "failed to remove member")
		return
	}

	rowsAffected, _ := result.RowsAffected()
	if rowsAffected == 0 {
		h.Error(w, http.StatusNotFound, "member not found in this team")
		return
	}

	h.JSON(w, http.StatusOK, map[string]string{"message": "member removed"})
}
