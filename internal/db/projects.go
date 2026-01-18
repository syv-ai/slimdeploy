package db

import (
	"database/sql"
	"fmt"
	"time"

	"github.com/mhenrichsen/slimdeploy/internal/models"
)

// ProjectRepository handles project database operations
type ProjectRepository struct {
	db *DB
}

// NewProjectRepository creates a new project repository
func NewProjectRepository(db *DB) *ProjectRepository {
	return &ProjectRepository{db: db}
}

// Create creates a new project
func (r *ProjectRepository) Create(p *models.Project) error {
	now := time.Now()
	p.CreatedAt = now
	p.UpdatedAt = now

	_, err := r.db.Exec(`
		INSERT INTO projects (
			id, name, git_url, branch, deploy_type, image, domain, use_subdomain,
			port, env_vars, auto_deploy, last_commit, status, status_msg, container_ids,
			created_at, updated_at
		) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
	`,
		p.ID, p.Name, p.GitURL, p.Branch, p.DeployType, p.Image, p.Domain,
		p.UseSubdomain, p.Port, p.EnvVarsJSON(), p.AutoDeploy, p.LastCommit,
		p.Status, p.StatusMsg, p.ContainerIDsJSON(), p.CreatedAt, p.UpdatedAt,
	)
	if err != nil {
		return fmt.Errorf("failed to create project: %w", err)
	}
	return nil
}

// GetByID retrieves a project by ID
func (r *ProjectRepository) GetByID(id string) (*models.Project, error) {
	p := &models.Project{}
	var envVars, containerIDs string
	var useSubdomain, autoDeploy int

	err := r.db.QueryRow(`
		SELECT id, name, git_url, branch, deploy_type, image, domain, use_subdomain,
			port, env_vars, auto_deploy, last_commit, status, status_msg, container_ids,
			created_at, updated_at
		FROM projects WHERE id = ?
	`, id).Scan(
		&p.ID, &p.Name, &p.GitURL, &p.Branch, &p.DeployType, &p.Image, &p.Domain,
		&useSubdomain, &p.Port, &envVars, &autoDeploy, &p.LastCommit,
		&p.Status, &p.StatusMsg, &containerIDs, &p.CreatedAt, &p.UpdatedAt,
	)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("failed to get project: %w", err)
	}

	p.UseSubdomain = useSubdomain == 1
	p.AutoDeploy = autoDeploy == 1
	if err := p.ParseEnvVars(envVars); err != nil {
		return nil, fmt.Errorf("failed to parse env vars: %w", err)
	}
	if err := p.ParseContainerIDs(containerIDs); err != nil {
		return nil, fmt.Errorf("failed to parse container IDs: %w", err)
	}

	return p, nil
}

// GetByName retrieves a project by name
func (r *ProjectRepository) GetByName(name string) (*models.Project, error) {
	p := &models.Project{}
	var envVars, containerIDs string
	var useSubdomain, autoDeploy int

	err := r.db.QueryRow(`
		SELECT id, name, git_url, branch, deploy_type, image, domain, use_subdomain,
			port, env_vars, auto_deploy, last_commit, status, status_msg, container_ids,
			created_at, updated_at
		FROM projects WHERE name = ?
	`, name).Scan(
		&p.ID, &p.Name, &p.GitURL, &p.Branch, &p.DeployType, &p.Image, &p.Domain,
		&useSubdomain, &p.Port, &envVars, &autoDeploy, &p.LastCommit,
		&p.Status, &p.StatusMsg, &containerIDs, &p.CreatedAt, &p.UpdatedAt,
	)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("failed to get project by name: %w", err)
	}

	p.UseSubdomain = useSubdomain == 1
	p.AutoDeploy = autoDeploy == 1
	if err := p.ParseEnvVars(envVars); err != nil {
		return nil, fmt.Errorf("failed to parse env vars: %w", err)
	}
	if err := p.ParseContainerIDs(containerIDs); err != nil {
		return nil, fmt.Errorf("failed to parse container IDs: %w", err)
	}

	return p, nil
}

// List retrieves all projects
func (r *ProjectRepository) List() ([]*models.Project, error) {
	rows, err := r.db.Query(`
		SELECT id, name, git_url, branch, deploy_type, image, domain, use_subdomain,
			port, env_vars, auto_deploy, last_commit, status, status_msg, container_ids,
			created_at, updated_at
		FROM projects ORDER BY created_at DESC
	`)
	if err != nil {
		return nil, fmt.Errorf("failed to list projects: %w", err)
	}
	defer rows.Close()

	var projects []*models.Project
	for rows.Next() {
		p := &models.Project{}
		var envVars, containerIDs string
		var useSubdomain, autoDeploy int

		err := rows.Scan(
			&p.ID, &p.Name, &p.GitURL, &p.Branch, &p.DeployType, &p.Image, &p.Domain,
			&useSubdomain, &p.Port, &envVars, &autoDeploy, &p.LastCommit,
			&p.Status, &p.StatusMsg, &containerIDs, &p.CreatedAt, &p.UpdatedAt,
		)
		if err != nil {
			return nil, fmt.Errorf("failed to scan project: %w", err)
		}

		p.UseSubdomain = useSubdomain == 1
		p.AutoDeploy = autoDeploy == 1
		if err := p.ParseEnvVars(envVars); err != nil {
			return nil, fmt.Errorf("failed to parse env vars: %w", err)
		}
		if err := p.ParseContainerIDs(containerIDs); err != nil {
			return nil, fmt.Errorf("failed to parse container IDs: %w", err)
		}

		projects = append(projects, p)
	}

	return projects, nil
}

// ListAutoDeployEnabled retrieves all projects with auto-deploy enabled
func (r *ProjectRepository) ListAutoDeployEnabled() ([]*models.Project, error) {
	rows, err := r.db.Query(`
		SELECT id, name, git_url, branch, deploy_type, image, domain, use_subdomain,
			port, env_vars, auto_deploy, last_commit, status, status_msg, container_ids,
			created_at, updated_at
		FROM projects WHERE auto_deploy = 1 ORDER BY created_at DESC
	`)
	if err != nil {
		return nil, fmt.Errorf("failed to list auto-deploy projects: %w", err)
	}
	defer rows.Close()

	var projects []*models.Project
	for rows.Next() {
		p := &models.Project{}
		var envVars, containerIDs string
		var useSubdomain, autoDeploy int

		err := rows.Scan(
			&p.ID, &p.Name, &p.GitURL, &p.Branch, &p.DeployType, &p.Image, &p.Domain,
			&useSubdomain, &p.Port, &envVars, &autoDeploy, &p.LastCommit,
			&p.Status, &p.StatusMsg, &containerIDs, &p.CreatedAt, &p.UpdatedAt,
		)
		if err != nil {
			return nil, fmt.Errorf("failed to scan project: %w", err)
		}

		p.UseSubdomain = useSubdomain == 1
		p.AutoDeploy = autoDeploy == 1
		if err := p.ParseEnvVars(envVars); err != nil {
			return nil, fmt.Errorf("failed to parse env vars: %w", err)
		}
		if err := p.ParseContainerIDs(containerIDs); err != nil {
			return nil, fmt.Errorf("failed to parse container IDs: %w", err)
		}

		projects = append(projects, p)
	}

	return projects, nil
}

// Update updates an existing project
func (r *ProjectRepository) Update(p *models.Project) error {
	p.UpdatedAt = time.Now()

	result, err := r.db.Exec(`
		UPDATE projects SET
			name = ?, git_url = ?, branch = ?, deploy_type = ?, image = ?,
			domain = ?, use_subdomain = ?, port = ?, env_vars = ?, auto_deploy = ?,
			last_commit = ?, status = ?, status_msg = ?, container_ids = ?, updated_at = ?
		WHERE id = ?
	`,
		p.Name, p.GitURL, p.Branch, p.DeployType, p.Image, p.Domain,
		p.UseSubdomain, p.Port, p.EnvVarsJSON(), p.AutoDeploy,
		p.LastCommit, p.Status, p.StatusMsg, p.ContainerIDsJSON(), p.UpdatedAt, p.ID,
	)
	if err != nil {
		return fmt.Errorf("failed to update project: %w", err)
	}

	rowsAffected, err := result.RowsAffected()
	if err != nil {
		return fmt.Errorf("failed to get rows affected: %w", err)
	}
	if rowsAffected == 0 {
		return fmt.Errorf("project not found")
	}

	return nil
}

// UpdateStatus updates the status of a project
func (r *ProjectRepository) UpdateStatus(id string, status models.ProjectStatus, statusMsg string) error {
	_, err := r.db.Exec(`
		UPDATE projects SET status = ?, status_msg = ?, updated_at = ? WHERE id = ?
	`, status, statusMsg, time.Now(), id)
	if err != nil {
		return fmt.Errorf("failed to update project status: %w", err)
	}
	return nil
}

// UpdateContainerIDs updates the container IDs of a project
func (r *ProjectRepository) UpdateContainerIDs(id string, containerIDs []string) error {
	p := &models.Project{ContainerIDs: containerIDs}
	_, err := r.db.Exec(`
		UPDATE projects SET container_ids = ?, updated_at = ? WHERE id = ?
	`, p.ContainerIDsJSON(), time.Now(), id)
	if err != nil {
		return fmt.Errorf("failed to update container IDs: %w", err)
	}
	return nil
}

// UpdateLastCommit updates the last commit of a project
func (r *ProjectRepository) UpdateLastCommit(id string, commit string) error {
	_, err := r.db.Exec(`
		UPDATE projects SET last_commit = ?, updated_at = ? WHERE id = ?
	`, commit, time.Now(), id)
	if err != nil {
		return fmt.Errorf("failed to update last commit: %w", err)
	}
	return nil
}

// Delete deletes a project
func (r *ProjectRepository) Delete(id string) error {
	result, err := r.db.Exec("DELETE FROM projects WHERE id = ?", id)
	if err != nil {
		return fmt.Errorf("failed to delete project: %w", err)
	}

	rowsAffected, err := result.RowsAffected()
	if err != nil {
		return fmt.Errorf("failed to get rows affected: %w", err)
	}
	if rowsAffected == 0 {
		return fmt.Errorf("project not found")
	}

	return nil
}
