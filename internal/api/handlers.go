package api

import (
	"bufio"
	"context"
	"fmt"
	"io"
	"log"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"
	"github.com/mhenrichsen/slimdeploy/internal/db"
	"github.com/mhenrichsen/slimdeploy/internal/docker"
	gitpkg "github.com/mhenrichsen/slimdeploy/internal/git"
	"github.com/mhenrichsen/slimdeploy/internal/models"
)

// TemplateExecutor is an interface for executing templates
type TemplateExecutor interface {
	ExecuteTemplate(w io.Writer, name string, data interface{}) error
}

// Handler handles HTTP requests
type Handler struct {
	templates      TemplateExecutor
	projectRepo    *db.ProjectRepository
	dockerClient   *docker.Client
	composeManager *docker.ComposeManager
	gitManager     *gitpkg.Manager
	auth           *AuthManager
	baseDomain     string
}

// NewHandler creates a new handler
func NewHandler(
	templates TemplateExecutor,
	projectRepo *db.ProjectRepository,
	dockerClient *docker.Client,
	composeManager *docker.ComposeManager,
	gitManager *gitpkg.Manager,
	auth *AuthManager,
	baseDomain string,
) *Handler {
	return &Handler{
		templates:      templates,
		projectRepo:    projectRepo,
		dockerClient:   dockerClient,
		composeManager: composeManager,
		gitManager:     gitManager,
		auth:           auth,
		baseDomain:     baseDomain,
	}
}

// TemplateData is the base data for templates
type TemplateData struct {
	Title      string
	Error      string
	Success    string
	BaseDomain string
}

// ProjectCardData wraps a project with additional template data
type ProjectCardData struct {
	*models.Project
	BaseDomain string
}

// DashboardData is the data for the dashboard template
type DashboardData struct {
	TemplateData
	Projects    []*models.Project
	ProjectCards []ProjectCardData
}

// ProjectData is the data for the project template
type ProjectData struct {
	TemplateData
	Project *models.Project
	IsNew   bool
}

// render renders a template
func (h *Handler) render(w http.ResponseWriter, name string, data interface{}) {
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	if err := h.templates.ExecuteTemplate(w, name, data); err != nil {
		log.Printf("Template error: %v", err)
		http.Error(w, "Internal Server Error", http.StatusInternalServerError)
	}
}

// renderPartial renders a partial template for HTMX
func (h *Handler) renderPartial(w http.ResponseWriter, name string, data interface{}) {
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	if err := h.templates.ExecuteTemplate(w, name, data); err != nil {
		log.Printf("Template error: %v", err)
		http.Error(w, "Internal Server Error", http.StatusInternalServerError)
	}
}

// LoginPage shows the login page
func (h *Handler) LoginPage(w http.ResponseWriter, r *http.Request) {
	// If already authenticated, redirect to dashboard
	if h.auth.IsAuthenticated(r) {
		http.Redirect(w, r, "/", http.StatusSeeOther)
		return
	}
	h.render(w, "login.html", TemplateData{Title: "Login"})
}

// Login handles login form submission
func (h *Handler) Login(w http.ResponseWriter, r *http.Request) {
	password := r.FormValue("password")

	if !h.auth.ValidatePassword(password) {
		h.render(w, "login.html", TemplateData{
			Title: "Login",
			Error: "Invalid password",
		})
		return
	}

	// Create session
	token, err := h.auth.CreateSession()
	if err != nil {
		log.Printf("Failed to create session: %v", err)
		http.Error(w, "Internal Server Error", http.StatusInternalServerError)
		return
	}

	// Set cookie
	h.auth.SetSessionCookie(w, r, token)

	// Redirect to dashboard
	http.Redirect(w, r, "/", http.StatusSeeOther)
}

// Logout handles logout
func (h *Handler) Logout(w http.ResponseWriter, r *http.Request) {
	token := h.auth.GetSessionFromRequest(r)
	if token != "" {
		h.auth.DeleteSession(token)
	}
	h.auth.ClearSessionCookie(w, r)
	http.Redirect(w, r, "/login", http.StatusSeeOther)
}

// Dashboard shows the project list
func (h *Handler) Dashboard(w http.ResponseWriter, r *http.Request) {
	projects, err := h.projectRepo.List()
	if err != nil {
		log.Printf("Failed to list projects: %v", err)
		http.Error(w, "Internal Server Error", http.StatusInternalServerError)
		return
	}

	// Create project cards with BaseDomain included
	projectCards := make([]ProjectCardData, len(projects))
	for i, p := range projects {
		projectCards[i] = ProjectCardData{
			Project:    p,
			BaseDomain: h.baseDomain,
		}
	}

	h.render(w, "dashboard.html", DashboardData{
		TemplateData: TemplateData{
			Title:      "Dashboard",
			BaseDomain: h.baseDomain,
		},
		Projects:     projects,
		ProjectCards: projectCards,
	})
}

// NewProjectForm shows the new project form
func (h *Handler) NewProjectForm(w http.ResponseWriter, r *http.Request) {
	h.render(w, "project.html", ProjectData{
		TemplateData: TemplateData{
			Title:      "New Project",
			BaseDomain: h.baseDomain,
		},
		Project: &models.Project{
			Branch:       "main",
			Port:         80,
			DeployType:   models.DeployTypeImage,
			UseSubdomain: true,
		},
		IsNew: true,
	})
}

// CreateProject creates a new project
func (h *Handler) CreateProject(w http.ResponseWriter, r *http.Request) {
	project := &models.Project{
		ID:           uuid.New().String(),
		Name:         strings.TrimSpace(r.FormValue("name")),
		GitURL:       strings.TrimSpace(r.FormValue("git_url")),
		Branch:       strings.TrimSpace(r.FormValue("branch")),
		Image:        strings.TrimSpace(r.FormValue("image")),
		Domain:       strings.TrimSpace(r.FormValue("domain")),
		UseSubdomain: r.FormValue("use_subdomain") == "on",
		AutoDeploy:   r.FormValue("auto_deploy") == "on",
		Status:       models.StatusPending,
	}

	// Parse deploy type
	deployType := r.FormValue("deploy_type")
	if deployType == "compose" {
		project.DeployType = models.DeployTypeCompose
	} else {
		project.DeployType = models.DeployTypeImage
	}

	// Parse port
	if port, err := strconv.Atoi(r.FormValue("port")); err == nil && port > 0 {
		project.Port = port
	} else {
		project.Port = 80
	}

	// Auto-detect default branch if not specified and git URL is provided
	if project.Branch == "" && project.GitURL != "" {
		detectedBranch, err := h.gitManager.GetDefaultBranch(project.GitURL)
		if err != nil {
			log.Printf("Failed to detect default branch for %s: %v, using 'main'", project.GitURL, err)
			project.Branch = "main"
		} else {
			log.Printf("Detected default branch for %s: %s", project.GitURL, detectedBranch)
			project.Branch = detectedBranch
		}
	} else if project.Branch == "" {
		project.Branch = "main"
	}

	// Parse environment variables
	project.EnvVars = parseEnvVars(r.FormValue("env_vars"))

	// Validate
	if project.Name == "" {
		h.render(w, "project.html", ProjectData{
			TemplateData: TemplateData{
				Title:      "New Project",
				Error:      "Project name is required",
				BaseDomain: h.baseDomain,
			},
			Project: project,
			IsNew:   true,
		})
		return
	}

	// Check for duplicate name
	existing, err := h.projectRepo.GetByName(project.Name)
	if err != nil {
		log.Printf("Failed to check for duplicate: %v", err)
		http.Error(w, "Internal Server Error", http.StatusInternalServerError)
		return
	}
	if existing != nil {
		h.render(w, "project.html", ProjectData{
			TemplateData: TemplateData{
				Title:      "New Project",
				Error:      "A project with this name already exists",
				BaseDomain: h.baseDomain,
			},
			Project: project,
			IsNew:   true,
		})
		return
	}

	// Save project
	if err := h.projectRepo.Create(project); err != nil {
		log.Printf("Failed to create project: %v", err)
		http.Error(w, "Internal Server Error", http.StatusInternalServerError)
		return
	}

	// Redirect to project page
	http.Redirect(w, r, fmt.Sprintf("/projects/%s", project.ID), http.StatusSeeOther)
}

// ProjectDetail shows project details
func (h *Handler) ProjectDetail(w http.ResponseWriter, r *http.Request) {
	projectID := chi.URLParam(r, "id")

	project, err := h.projectRepo.GetByID(projectID)
	if err != nil {
		log.Printf("Failed to get project: %v", err)
		http.Error(w, "Internal Server Error", http.StatusInternalServerError)
		return
	}
	if project == nil {
		http.NotFound(w, r)
		return
	}

	h.render(w, "project_detail.html", ProjectData{
		TemplateData: TemplateData{
			Title:      project.Name,
			BaseDomain: h.baseDomain,
		},
		Project: project,
	})
}

// EditProjectForm shows the edit project form
func (h *Handler) EditProjectForm(w http.ResponseWriter, r *http.Request) {
	projectID := chi.URLParam(r, "id")

	project, err := h.projectRepo.GetByID(projectID)
	if err != nil {
		log.Printf("Failed to get project: %v", err)
		http.Error(w, "Internal Server Error", http.StatusInternalServerError)
		return
	}
	if project == nil {
		http.NotFound(w, r)
		return
	}

	h.render(w, "project.html", ProjectData{
		TemplateData: TemplateData{
			Title:      "Edit " + project.Name,
			BaseDomain: h.baseDomain,
		},
		Project: project,
		IsNew:   false,
	})
}

// UpdateProject updates a project
func (h *Handler) UpdateProject(w http.ResponseWriter, r *http.Request) {
	projectID := chi.URLParam(r, "id")

	project, err := h.projectRepo.GetByID(projectID)
	if err != nil {
		log.Printf("Failed to get project: %v", err)
		http.Error(w, "Internal Server Error", http.StatusInternalServerError)
		return
	}
	if project == nil {
		http.NotFound(w, r)
		return
	}

	// Update fields
	project.Name = strings.TrimSpace(r.FormValue("name"))
	project.GitURL = strings.TrimSpace(r.FormValue("git_url"))
	project.Branch = strings.TrimSpace(r.FormValue("branch"))
	project.Image = strings.TrimSpace(r.FormValue("image"))
	project.Domain = strings.TrimSpace(r.FormValue("domain"))
	project.UseSubdomain = r.FormValue("use_subdomain") == "on"
	project.AutoDeploy = r.FormValue("auto_deploy") == "on"

	// Parse deploy type
	deployType := r.FormValue("deploy_type")
	if deployType == "compose" {
		project.DeployType = models.DeployTypeCompose
	} else {
		project.DeployType = models.DeployTypeImage
	}

	// Parse port
	if port, err := strconv.Atoi(r.FormValue("port")); err == nil && port > 0 {
		project.Port = port
	}

	// Auto-detect default branch if not specified and git URL is provided
	if project.Branch == "" && project.GitURL != "" {
		detectedBranch, err := h.gitManager.GetDefaultBranch(project.GitURL)
		if err != nil {
			log.Printf("Failed to detect default branch for %s: %v, using 'main'", project.GitURL, err)
			project.Branch = "main"
		} else {
			log.Printf("Detected default branch for %s: %s", project.GitURL, detectedBranch)
			project.Branch = detectedBranch
		}
	} else if project.Branch == "" {
		project.Branch = "main"
	}

	// Parse environment variables
	project.EnvVars = parseEnvVars(r.FormValue("env_vars"))

	// Validate
	if project.Name == "" {
		h.render(w, "project.html", ProjectData{
			TemplateData: TemplateData{
				Title:      "Edit Project",
				Error:      "Project name is required",
				BaseDomain: h.baseDomain,
			},
			Project: project,
			IsNew:   false,
		})
		return
	}

	// Save project
	if err := h.projectRepo.Update(project); err != nil {
		log.Printf("Failed to update project: %v", err)
		http.Error(w, "Internal Server Error", http.StatusInternalServerError)
		return
	}

	http.Redirect(w, r, fmt.Sprintf("/projects/%s", project.ID), http.StatusSeeOther)
}

// DeleteProject deletes a project
func (h *Handler) DeleteProject(w http.ResponseWriter, r *http.Request) {
	projectID := chi.URLParam(r, "id")

	project, err := h.projectRepo.GetByID(projectID)
	if err != nil {
		log.Printf("Failed to get project: %v", err)
		http.Error(w, "Internal Server Error", http.StatusInternalServerError)
		return
	}
	if project == nil {
		http.NotFound(w, r)
		return
	}

	ctx := r.Context()

	// Stop and remove containers
	if project.DeployType == models.DeployTypeCompose {
		h.composeManager.Down(ctx, project)
	} else {
		h.dockerClient.RemoveProjectContainers(ctx, project.ID)
	}

	// Remove git repository
	h.gitManager.Remove(project.Name)

	// Delete from database
	if err := h.projectRepo.Delete(projectID); err != nil {
		log.Printf("Failed to delete project: %v", err)
		http.Error(w, "Internal Server Error", http.StatusInternalServerError)
		return
	}

	// Check if HTMX request
	if r.Header.Get("HX-Request") == "true" {
		w.Header().Set("HX-Redirect", "/")
		w.WriteHeader(http.StatusOK)
		return
	}

	http.Redirect(w, r, "/", http.StatusSeeOther)
}

// Deploy triggers a deployment
func (h *Handler) Deploy(w http.ResponseWriter, r *http.Request) {
	projectID := chi.URLParam(r, "id")

	project, err := h.projectRepo.GetByID(projectID)
	if err != nil {
		log.Printf("Failed to get project: %v", err)
		http.Error(w, "Internal Server Error", http.StatusInternalServerError)
		return
	}
	if project == nil {
		http.NotFound(w, r)
		return
	}

	ctx := r.Context()

	// Update status to deploying
	h.projectRepo.UpdateStatus(project.ID, models.StatusDeploying, "Starting deployment...")

	// Deploy asynchronously
	go func() {
		if err := h.deployProject(context.Background(), project); err != nil {
			log.Printf("Deployment failed for %s: %v", project.Name, err)
			h.projectRepo.UpdateStatus(project.ID, models.StatusError, err.Error())
		}
	}()

	// Return updated project card for HTMX
	if r.Header.Get("HX-Request") == "true" {
		// Refresh project data
		project, _ = h.projectRepo.GetByID(projectID)
		h.renderPartial(w, "project_card", ProjectCardData{Project: project, BaseDomain: h.baseDomain})
		return
	}

	http.Redirect(w, r, fmt.Sprintf("/projects/%s", project.ID), http.StatusSeeOther)
	_ = ctx
}

// deployProject performs the actual deployment
func (h *Handler) deployProject(ctx context.Context, project *models.Project) error {
	// Clone or pull git repo if configured
	if project.GitURL != "" {
		if h.gitManager.Exists(project.Name) {
			if err := h.gitManager.Pull(project.GitURL, project.Branch, project.Name); err != nil {
				return fmt.Errorf("failed to pull repository: %w", err)
			}
		} else {
			if err := h.gitManager.Clone(project.GitURL, project.Branch, project.Name); err != nil {
				return fmt.Errorf("failed to clone repository: %w", err)
			}
		}

		// Update last commit
		commit, err := h.gitManager.GetLatestCommit(project.Name)
		if err == nil {
			h.projectRepo.UpdateLastCommit(project.ID, commit)
		}
	}

	var containerIDs []string

	if project.DeployType == models.DeployTypeCompose {
		// Docker Compose deployment
		if err := h.composeManager.Up(ctx, project); err != nil {
			return fmt.Errorf("docker compose up failed: %w", err)
		}
	} else {
		// Docker image deployment
		// Pull image if specified
		if project.Image != "" {
			if err := h.dockerClient.PullImage(ctx, project.Image); err != nil {
				return fmt.Errorf("failed to pull image: %w", err)
			}
		}

		// Run container
		containerID, err := h.dockerClient.RunContainer(ctx, project)
		if err != nil {
			return fmt.Errorf("failed to run container: %w", err)
		}
		containerIDs = append(containerIDs, containerID)

		// Wait for container to be healthy
		if err := h.dockerClient.WaitForHealthy(ctx, containerID, 60*time.Second); err != nil {
			return fmt.Errorf("container health check failed: %w", err)
		}
	}

	// Update project status
	h.projectRepo.UpdateContainerIDs(project.ID, containerIDs)
	h.projectRepo.UpdateStatus(project.ID, models.StatusRunning, "")

	return nil
}

// DeployProject is a public wrapper for deployProject (for watcher)
func (h *Handler) DeployProject(ctx context.Context, project *models.Project) error {
	return h.deployProject(ctx, project)
}

// Stop stops a project
func (h *Handler) Stop(w http.ResponseWriter, r *http.Request) {
	projectID := chi.URLParam(r, "id")

	project, err := h.projectRepo.GetByID(projectID)
	if err != nil {
		log.Printf("Failed to get project: %v", err)
		http.Error(w, "Internal Server Error", http.StatusInternalServerError)
		return
	}
	if project == nil {
		http.NotFound(w, r)
		return
	}

	ctx := r.Context()

	if project.DeployType == models.DeployTypeCompose {
		if err := h.composeManager.Down(ctx, project); err != nil {
			log.Printf("Failed to stop compose project: %v", err)
		}
	} else {
		if err := h.dockerClient.StopProjectContainers(ctx, project.ID); err != nil {
			log.Printf("Failed to stop containers: %v", err)
		}
	}

	h.projectRepo.UpdateStatus(project.ID, models.StatusStopped, "")

	// Return updated project card for HTMX
	if r.Header.Get("HX-Request") == "true" {
		project, _ = h.projectRepo.GetByID(projectID)
		h.renderPartial(w, "project_card", ProjectCardData{Project: project, BaseDomain: h.baseDomain})
		return
	}

	http.Redirect(w, r, fmt.Sprintf("/projects/%s", project.ID), http.StatusSeeOther)
}

// Restart restarts a project
func (h *Handler) Restart(w http.ResponseWriter, r *http.Request) {
	projectID := chi.URLParam(r, "id")

	project, err := h.projectRepo.GetByID(projectID)
	if err != nil {
		log.Printf("Failed to get project: %v", err)
		http.Error(w, "Internal Server Error", http.StatusInternalServerError)
		return
	}
	if project == nil {
		http.NotFound(w, r)
		return
	}

	ctx := r.Context()

	if project.DeployType == models.DeployTypeCompose {
		if err := h.composeManager.Restart(ctx, project); err != nil {
			log.Printf("Failed to restart compose project: %v", err)
			h.projectRepo.UpdateStatus(project.ID, models.StatusError, err.Error())
		} else {
			h.projectRepo.UpdateStatus(project.ID, models.StatusRunning, "")
		}
	} else {
		for _, containerID := range project.ContainerIDs {
			if err := h.dockerClient.RestartContainer(ctx, containerID); err != nil {
				log.Printf("Failed to restart container %s: %v", containerID, err)
			}
		}
		h.projectRepo.UpdateStatus(project.ID, models.StatusRunning, "")
	}

	// Return updated project card for HTMX
	if r.Header.Get("HX-Request") == "true" {
		project, _ = h.projectRepo.GetByID(projectID)
		h.renderPartial(w, "project_card", ProjectCardData{Project: project, BaseDomain: h.baseDomain})
		return
	}

	http.Redirect(w, r, fmt.Sprintf("/projects/%s", project.ID), http.StatusSeeOther)
}

// Logs streams container logs
func (h *Handler) Logs(w http.ResponseWriter, r *http.Request) {
	projectID := chi.URLParam(r, "id")

	project, err := h.projectRepo.GetByID(projectID)
	if err != nil {
		log.Printf("Failed to get project: %v", err)
		http.Error(w, "Internal Server Error", http.StatusInternalServerError)
		return
	}
	if project == nil {
		http.NotFound(w, r)
		return
	}

	ctx := r.Context()
	follow := r.URL.Query().Get("follow") == "true"
	tail := 100

	if project.DeployType == models.DeployTypeCompose {
		logs, err := h.composeManager.Logs(ctx, project, follow, tail)
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		w.Header().Set("Content-Type", "text/plain; charset=utf-8")
		w.Write([]byte(logs))
		return
	}

	// For single container deployments
	if len(project.ContainerIDs) == 0 {
		w.Header().Set("Content-Type", "text/plain; charset=utf-8")
		w.Write([]byte("No containers running"))
		return
	}

	// Get logs from first container
	reader, err := h.dockerClient.GetContainerLogs(ctx, project.ContainerIDs[0], tail, follow)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	defer reader.Close()

	w.Header().Set("Content-Type", "text/plain; charset=utf-8")

	if follow {
		// Stream logs with SSE
		w.Header().Set("Content-Type", "text/event-stream")
		w.Header().Set("Cache-Control", "no-cache")
		w.Header().Set("Connection", "keep-alive")

		flusher, ok := w.(http.Flusher)
		if !ok {
			http.Error(w, "Streaming not supported", http.StatusInternalServerError)
			return
		}

		scanner := bufio.NewScanner(reader)
		for scanner.Scan() {
			line := scanner.Text()
			// Skip the Docker log header (first 8 bytes)
			if len(line) > 8 {
				line = line[8:]
			}
			fmt.Fprintf(w, "data: %s\n\n", line)
			flusher.Flush()
		}
	} else {
		// Return all logs at once
		data, err := io.ReadAll(reader)
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		// Process logs to remove Docker headers
		lines := strings.Split(string(data), "\n")
		var cleanLines []string
		for _, line := range lines {
			if len(line) > 8 {
				cleanLines = append(cleanLines, line[8:])
			}
		}
		w.Write([]byte(strings.Join(cleanLines, "\n")))
	}
}

// Health returns health status
func (h *Handler) Health(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()

	// Check Docker connection
	if err := h.dockerClient.Ping(ctx); err != nil {
		w.WriteHeader(http.StatusServiceUnavailable)
		w.Write([]byte("Docker unavailable"))
		return
	}

	w.WriteHeader(http.StatusOK)
	w.Write([]byte("OK"))
}

// ProjectStatus returns the current status of a project (for polling)
func (h *Handler) ProjectStatus(w http.ResponseWriter, r *http.Request) {
	projectID := chi.URLParam(r, "id")

	project, err := h.projectRepo.GetByID(projectID)
	if err != nil {
		log.Printf("Failed to get project: %v", err)
		http.Error(w, "Internal Server Error", http.StatusInternalServerError)
		return
	}
	if project == nil {
		http.NotFound(w, r)
		return
	}

	h.renderPartial(w, "project_card", ProjectCardData{Project: project, BaseDomain: h.baseDomain})
}

// parseEnvVars parses environment variables from text format (KEY=VALUE per line)
func parseEnvVars(text string) map[string]string {
	envVars := make(map[string]string)
	lines := strings.Split(text, "\n")
	for _, line := range lines {
		line = strings.TrimSpace(line)
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		parts := strings.SplitN(line, "=", 2)
		if len(parts) == 2 {
			key := strings.TrimSpace(parts[0])
			value := strings.TrimSpace(parts[1])
			if key != "" {
				envVars[key] = value
			}
		}
	}
	return envVars
}
