package api

import (
	"net/http"

	"github.com/go-chi/chi/v5"
)

// NewRouter creates a new HTTP router
func NewRouter(h *Handler, auth *AuthManager, staticFS http.FileSystem) *chi.Mux {
	r := chi.NewRouter()

	// Global middleware
	r.Use(RecoveryMiddleware)
	r.Use(LoggingMiddleware)

	// Static files (no auth required)
	r.Handle("/static/*", http.StripPrefix("/static/", http.FileServer(staticFS)))

	// Health check (no auth required)
	r.Get("/health", h.Health)

	// Auth routes (no auth required)
	r.Get("/login", h.LoginPage)
	r.Post("/login", h.Login)
	r.Post("/logout", h.Logout)

	// Protected routes
	r.Group(func(r chi.Router) {
		r.Use(AuthMiddleware(auth))
		r.Use(NoCacheMiddleware)

		// Dashboard
		r.Get("/", h.Dashboard)

		// Project routes
		r.Get("/projects/new", h.NewProjectForm)
		r.Post("/projects", h.CreateProject)
		r.Get("/projects/{id}", h.ProjectDetail)
		r.Get("/projects/{id}/edit", h.EditProjectForm)
		r.Put("/projects/{id}", h.UpdateProject)
		r.Post("/projects/{id}", h.UpdateProject) // For HTML form support
		r.Delete("/projects/{id}", h.DeleteProject)

		// Project actions
		r.Post("/projects/{id}/deploy", h.Deploy)
		r.Post("/projects/{id}/stop", h.Stop)
		r.Post("/projects/{id}/restart", h.Restart)
		r.Get("/projects/{id}/logs", h.Logs)
		r.Get("/projects/{id}/status", h.ProjectStatus)
	})

	return r
}
