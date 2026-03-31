package main

import (
	"log"
	"net/http"

	"github.com/smarty/httprouter"

	"teamkudos/internal/config"
	"teamkudos/internal/database"
	"teamkudos/internal/handlers"
	"teamkudos/internal/middleware"
)

func main() {
	cfg, err := config.Load()
	if err != nil {
		log.Fatal("Failed to load config:", err)
	}

	db, err := database.Connect(cfg.DatabaseURL)
	if err != nil {
		log.Fatal("Failed to connect to database:", err)
	}
	defer db.Close()

	log.Println("✅ Connected to database!")

	h := handlers.New(db, cfg)

	// Build routes using smarty/httprouter
	var routes []httprouter.Route

	// Public routes
	routes = append(routes, httprouter.ParseRoute("GET", "/health", http.HandlerFunc(h.Health)))
	routes = append(routes, httprouter.ParseRoute("GET", "/api/badges", http.HandlerFunc(h.ListBadges)))
	routes = append(routes, httprouter.ParseRoute("GET", "/api/badges/:slug", http.HandlerFunc(h.GetBadge)))

	// Protected routes (require Supabase JWT)
	routes = append(routes, httprouter.ParseRoute("GET", "/api/users/me", http.HandlerFunc(h.WithAuth(h.GetCurrentUser))))
	routes = append(routes, httprouter.ParseRoute("POST", "/api/users/profile", http.HandlerFunc(h.WithAuth(h.CreateProfile))))
	routes = append(routes, httprouter.ParseRoute("GET", "/api/users/:userId/kudos", http.HandlerFunc(h.WithAuth(h.GetUserKudos))))

	routes = append(routes, httprouter.ParseRoute("POST", "/api/teams", http.HandlerFunc(h.WithAuth(h.CreateTeam))))
	routes = append(routes, httprouter.ParseRoute("POST", "/api/teams/join", http.HandlerFunc(h.WithAuth(h.JoinTeam))))
	routes = append(routes, httprouter.ParseRoute("GET", "/api/teams/:teamId", http.HandlerFunc(h.WithAuth(h.GetTeam))))
	routes = append(routes, httprouter.ParseRoute("GET", "/api/teams/:teamId/members", http.HandlerFunc(h.WithAuth(h.ListTeamMembers))))
	routes = append(routes, httprouter.ParseRoute("POST", "/api/teams/:teamId/members", http.HandlerFunc(h.WithAuth(h.InviteMember))))
	routes = append(routes, httprouter.ParseRoute("DELETE", "/api/teams/:teamId/members/:userId", http.HandlerFunc(h.WithAuth(h.RemoveMember))))

	routes = append(routes, httprouter.ParseRoute("POST", "/api/kudos", http.HandlerFunc(h.WithAuth(h.SendKudos))))
	routes = append(routes, httprouter.ParseRoute("GET", "/api/teams/:teamId/feed", http.HandlerFunc(h.WithAuth(h.GetTeamFeed))))

	router, err := httprouter.New(
		httprouter.Options.Routes(routes...),
	)
	if err != nil {
		log.Fatal("Failed to create router:", err)
	}

	// Wrap router with global middleware
	handler := middleware.Chain(
		router,
		middleware.Logger,
		middleware.Recoverer,
		middleware.CORS([]string{"http://localhost:3000", "http://localhost:5173"}),
	)

	log.Printf("🚀 Server starting on http://localhost:%s", cfg.Port)
	log.Fatal(http.ListenAndServe(":"+cfg.Port, handler))
}
