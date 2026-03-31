package handlers

import (
	"net/http"
)

type Badge struct {
	ID          int    `json:"id"`
	Slug        string `json:"slug"`
	Name        string `json:"name"`
	Description string `json:"description"`
	Emoji       string `json:"emoji"`
	Category    string `json:"category"`
}

// ListBadges returns all available badges
func (h *Handler) ListBadges(w http.ResponseWriter, r *http.Request) {
	rows, err := h.db.Query(`SELECT id, slug, name, description, emoji, category FROM badges ORDER BY category, name`)
	if err != nil {
		h.Error(w, http.StatusInternalServerError, "failed to fetch badges")
		return
	}
	defer rows.Close()

	var badges []Badge
	for rows.Next() {
		var b Badge
		if err := rows.Scan(&b.ID, &b.Slug, &b.Name, &b.Description, &b.Emoji, &b.Category); err != nil {
			h.Error(w, http.StatusInternalServerError, "failed to scan badge")
			return
		}
		badges = append(badges, b)
	}

	if badges == nil {
		badges = []Badge{}
	}

	h.JSON(w, http.StatusOK, badges)
}

// GetBadge returns a single badge by slug
// Route: GET /api/badges/:slug → segment index 2
func (h *Handler) GetBadge(w http.ResponseWriter, r *http.Request) {
	slug := pathSegment(r, 2)

	var b Badge
	err := h.db.QueryRow(
		`SELECT id, slug, name, description, emoji, category FROM badges WHERE slug = $1`, slug,
	).Scan(&b.ID, &b.Slug, &b.Name, &b.Description, &b.Emoji, &b.Category)

	if err != nil {
		h.Error(w, http.StatusNotFound, "badge not found")
		return
	}

	h.JSON(w, http.StatusOK, b)
}
