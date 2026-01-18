package watcher

import (
	"context"
	"log"
	"sync"
	"time"

	"github.com/mhenrichsen/slimdeploy/internal/db"
	gitpkg "github.com/mhenrichsen/slimdeploy/internal/git"
	"github.com/mhenrichsen/slimdeploy/internal/models"
)

// DeployFunc is a function that deploys a project
type DeployFunc func(ctx context.Context, project *models.Project) error

// Watcher watches git repositories for changes
type Watcher struct {
	projectRepo *db.ProjectRepository
	gitManager  *gitpkg.Manager
	deployFunc  DeployFunc
	interval    time.Duration
	stopCh      chan struct{}
	wg          sync.WaitGroup
	running     bool
	mu          sync.Mutex
}

// New creates a new Watcher
func New(projectRepo *db.ProjectRepository, gitManager *gitpkg.Manager, deployFunc DeployFunc, interval time.Duration) *Watcher {
	return &Watcher{
		projectRepo: projectRepo,
		gitManager:  gitManager,
		deployFunc:  deployFunc,
		interval:    interval,
		stopCh:      make(chan struct{}),
	}
}

// Start starts the watcher
func (w *Watcher) Start() {
	w.mu.Lock()
	if w.running {
		w.mu.Unlock()
		return
	}
	w.running = true
	w.stopCh = make(chan struct{})
	w.mu.Unlock()

	w.wg.Add(1)
	go w.run()

	log.Printf("Watcher started with interval %v", w.interval)
}

// Stop stops the watcher
func (w *Watcher) Stop() {
	w.mu.Lock()
	if !w.running {
		w.mu.Unlock()
		return
	}
	w.running = false
	close(w.stopCh)
	w.mu.Unlock()

	w.wg.Wait()
	log.Println("Watcher stopped")
}

// run is the main watcher loop
func (w *Watcher) run() {
	defer w.wg.Done()

	ticker := time.NewTicker(w.interval)
	defer ticker.Stop()

	// Run immediately on start
	w.checkAll()

	for {
		select {
		case <-ticker.C:
			w.checkAll()
		case <-w.stopCh:
			return
		}
	}
}

// checkAll checks all projects with auto-deploy enabled
func (w *Watcher) checkAll() {
	projects, err := w.projectRepo.ListAutoDeployEnabled()
	if err != nil {
		log.Printf("Watcher: failed to list projects: %v", err)
		return
	}

	for _, project := range projects {
		w.checkProject(project)
	}
}

// checkProject checks a single project for updates
func (w *Watcher) checkProject(project *models.Project) {
	// Skip if no git URL configured
	if project.GitURL == "" {
		return
	}

	// Skip if project is currently deploying
	if project.Status == models.StatusDeploying {
		return
	}

	// Check if repo exists locally
	if !w.gitManager.Exists(project.Name) {
		log.Printf("Watcher: repository not found for %s, skipping", project.Name)
		return
	}

	// Check for updates
	hasUpdates, newCommit, err := w.gitManager.CheckForUpdates(project.GitURL, project.Branch, project.Name)
	if err != nil {
		log.Printf("Watcher: failed to check for updates on %s: %v", project.Name, err)
		return
	}

	if !hasUpdates {
		return
	}

	log.Printf("Watcher: detected new commit on %s: %s", project.Name, newCommit[:8])

	// Pull the updates
	if err := w.gitManager.Pull(project.GitURL, project.Branch, project.Name); err != nil {
		log.Printf("Watcher: failed to pull updates for %s: %v", project.Name, err)
		return
	}

	// Update the last commit in database
	if err := w.projectRepo.UpdateLastCommit(project.ID, newCommit); err != nil {
		log.Printf("Watcher: failed to update last commit for %s: %v", project.Name, err)
	}

	// Trigger deployment
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Minute)
	defer cancel()

	if err := w.deployFunc(ctx, project); err != nil {
		log.Printf("Watcher: failed to deploy %s: %v", project.Name, err)
		w.projectRepo.UpdateStatus(project.ID, models.StatusError, err.Error())
		return
	}

	log.Printf("Watcher: successfully deployed %s", project.Name)
}

// CheckProject manually triggers a check for a specific project
func (w *Watcher) CheckProject(projectID string) error {
	project, err := w.projectRepo.GetByID(projectID)
	if err != nil {
		return err
	}
	if project == nil {
		return nil
	}

	w.checkProject(project)
	return nil
}

// IsRunning returns whether the watcher is running
func (w *Watcher) IsRunning() bool {
	w.mu.Lock()
	defer w.mu.Unlock()
	return w.running
}
