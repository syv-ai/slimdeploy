package main

import (
	"context"
	"fmt"
	"html/template"
	"io"
	"io/fs"
	"log"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/mhenrichsen/slimdeploy/internal/api"
	"github.com/mhenrichsen/slimdeploy/internal/db"
	"github.com/mhenrichsen/slimdeploy/internal/docker"
	gitpkg "github.com/mhenrichsen/slimdeploy/internal/git"
	"github.com/mhenrichsen/slimdeploy/internal/watcher"
	"github.com/mhenrichsen/slimdeploy/web"
)

func main() {
	log.SetFlags(log.LstdFlags | log.Lshortfile)
	log.Println("Starting SlimDeploy...")

	// Load configuration from environment
	config := loadConfig()

	// Initialize database
	database, err := db.New(config.DataDir)
	if err != nil {
		log.Fatalf("Failed to initialize database: %v", err)
	}
	defer database.Close()

	// Initialize repositories
	projectRepo := db.NewProjectRepository(database)

	// Initialize Docker client
	dockerClient, err := docker.NewClient(config.BaseDomain)
	if err != nil {
		log.Fatalf("Failed to initialize Docker client: %v", err)
	}
	defer dockerClient.Close()

	// Ensure Docker network exists
	ctx := context.Background()
	if err := dockerClient.EnsureNetwork(ctx); err != nil {
		log.Printf("Warning: Failed to ensure Docker network: %v", err)
	}

	// Initialize Compose manager
	composeManager := docker.NewComposeManager(config.BaseDomain, config.DeploymentsDir)

	// Initialize Git manager
	gitManager := gitpkg.NewManager(config.DeploymentsDir, config.SSHKeyPath)

	// Initialize auth manager
	authManager := api.NewAuthManager(database.DB, config.Password)

	// Clean up expired sessions periodically
	go func() {
		ticker := time.NewTicker(1 * time.Hour)
		defer ticker.Stop()
		for range ticker.C {
			if err := authManager.CleanupExpiredSessions(); err != nil {
				log.Printf("Failed to cleanup sessions: %v", err)
			}
		}
	}()

	// Parse templates
	templates, err := parseTemplates()
	if err != nil {
		log.Fatalf("Failed to parse templates: %v", err)
	}

	// Create handler
	handler := api.NewHandler(
		templates,
		projectRepo,
		dockerClient,
		composeManager,
		gitManager,
		authManager,
		config.BaseDomain,
	)

	// Initialize watcher
	watcherService := watcher.New(
		projectRepo,
		gitManager,
		handler.DeployProject,
		config.WatchInterval,
	)
	watcherService.Start()
	defer watcherService.Stop()

	// Create static file server
	staticSubFS, err := fs.Sub(web.StaticFS, "static")
	if err != nil {
		log.Fatalf("Failed to create static FS: %v", err)
	}

	// Create router
	router := api.NewRouter(handler, authManager, http.FS(staticSubFS))

	// Create server
	server := &http.Server{
		Addr:         config.ListenAddr,
		Handler:      router,
		ReadTimeout:  30 * time.Second,
		WriteTimeout: 30 * time.Second,
		IdleTimeout:  60 * time.Second,
	}

	// Start server in goroutine
	go func() {
		log.Printf("Server listening on %s", config.ListenAddr)
		if err := server.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			log.Fatalf("Failed to start server: %v", err)
		}
	}()

	// Wait for shutdown signal
	quit := make(chan os.Signal, 1)
	signal.Notify(quit, syscall.SIGINT, syscall.SIGTERM)
	<-quit

	log.Println("Shutting down server...")

	// Graceful shutdown
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	if err := server.Shutdown(ctx); err != nil {
		log.Printf("Server forced to shutdown: %v", err)
	}

	log.Println("Server stopped")
}

// Config holds application configuration
type Config struct {
	ListenAddr     string
	DataDir        string
	DeploymentsDir string
	Password       string
	Domain         string
	BaseDomain     string
	SSHKeyPath     string
	WatchInterval  time.Duration
}

func loadConfig() *Config {
	config := &Config{
		ListenAddr:     getEnv("LISTEN_ADDR", ":8080"),
		DataDir:        getEnv("DATA_DIR", "./data"),
		DeploymentsDir: getEnv("DEPLOYMENTS_DIR", "./deployments"),
		Password:       getEnv("SLIMDEPLOY_PASSWORD", "admin"),
		Domain:         getEnv("DOMAIN", "localhost"),
		BaseDomain:     getEnv("BASE_DOMAIN", "localhost"),
		SSHKeyPath:     getEnv("SSH_KEY_PATH", ""),
	}

	// Parse watch interval
	intervalStr := getEnv("WATCH_INTERVAL", "60s")
	interval, err := time.ParseDuration(intervalStr)
	if err != nil {
		log.Printf("Invalid WATCH_INTERVAL, using default 60s")
		interval = 60 * time.Second
	}
	config.WatchInterval = interval

	// Log configuration (without password)
	log.Printf("Configuration:")
	log.Printf("  Listen Address: %s", config.ListenAddr)
	log.Printf("  Data Directory: %s", config.DataDir)
	log.Printf("  Deployments Directory: %s", config.DeploymentsDir)
	log.Printf("  Domain: %s", config.Domain)
	log.Printf("  Base Domain: %s", config.BaseDomain)
	log.Printf("  Watch Interval: %s", config.WatchInterval)

	return config
}

func getEnv(key, defaultValue string) string {
	if value := os.Getenv(key); value != "" {
		return value
	}
	return defaultValue
}

// Templates holds parsed templates for each page
type Templates struct {
	templates map[string]*template.Template
	funcMap   template.FuncMap
}

// ExecuteTemplate executes a named template
func (t *Templates) ExecuteTemplate(w io.Writer, name string, data interface{}) error {
	tmpl, ok := t.templates[name]
	if !ok {
		return fmt.Errorf("template %s not found", name)
	}
	return tmpl.ExecuteTemplate(w, name, data)
}

func parseTemplates() (*Templates, error) {
	// Custom template functions
	funcMap := template.FuncMap{
		"formatTime": func(t time.Time) string {
			return t.Format("Jan 02, 2006 15:04")
		},
	}

	templatesSubFS, err := fs.Sub(web.TemplatesFS, "templates")
	if err != nil {
		return nil, err
	}

	// Read layout template
	layoutContent, err := fs.ReadFile(templatesSubFS, "layout.html")
	if err != nil {
		return nil, fmt.Errorf("failed to read layout.html: %w", err)
	}

	templates := &Templates{
		templates: make(map[string]*template.Template),
		funcMap:   funcMap,
	}

	// Parse each page template with its own isolated template set
	pageTemplates := []string{"login.html", "dashboard.html", "project.html", "project_detail.html"}

	for _, pageName := range pageTemplates {
		content, err := fs.ReadFile(templatesSubFS, pageName)
		if err != nil {
			return nil, fmt.Errorf("failed to read %s: %w", pageName, err)
		}

		// Create a new template set for this page
		tmpl := template.New(pageName).Funcs(funcMap)

		// Parse the layout
		if _, err := tmpl.New("layout.html").Parse(string(layoutContent)); err != nil {
			return nil, fmt.Errorf("failed to parse layout for %s: %w", pageName, err)
		}

		// Parse the page content (this includes the page's "content" block)
		if _, err := tmpl.Parse(string(content)); err != nil {
			return nil, fmt.Errorf("failed to parse %s: %w", pageName, err)
		}

		templates.templates[pageName] = tmpl
	}

	// Also create a special template for project_card partial (used by HTMX)
	// Reuse the dashboard template which already has the project_card define
	templates.templates["project_card"] = templates.templates["dashboard.html"]

	return templates, nil
}
