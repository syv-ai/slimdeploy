package models

import (
	"encoding/json"
	"time"
)

// DeployType represents the type of deployment
type DeployType string

const (
	DeployTypeImage   DeployType = "image"
	DeployTypeCompose DeployType = "compose"
)

// ProjectStatus represents the current status of a project
type ProjectStatus string

const (
	StatusRunning   ProjectStatus = "running"
	StatusStopped   ProjectStatus = "stopped"
	StatusError     ProjectStatus = "error"
	StatusDeploying ProjectStatus = "deploying"
	StatusPending   ProjectStatus = "pending"
)

// Project represents a deployment project
type Project struct {
	ID           string            `json:"id"`
	Name         string            `json:"name"`
	GitURL       string            `json:"git_url"`
	Branch       string            `json:"branch"`
	DeployType   DeployType        `json:"deploy_type"`
	Image        string            `json:"image"`
	Domain       string            `json:"domain"`
	UseSubdomain bool              `json:"use_subdomain"`
	Port         int               `json:"port"`
	EnvVars      map[string]string `json:"env_vars"`
	AutoDeploy   bool              `json:"auto_deploy"`
	LastCommit   string            `json:"last_commit"`
	Status       ProjectStatus     `json:"status"`
	StatusMsg    string            `json:"status_msg"`
	ContainerIDs []string          `json:"container_ids"`
	CreatedAt    time.Time         `json:"created_at"`
	UpdatedAt    time.Time         `json:"updated_at"`
}

// EnvVarsJSON returns the env vars as JSON string for database storage
func (p *Project) EnvVarsJSON() string {
	if p.EnvVars == nil {
		return "{}"
	}
	data, err := json.Marshal(p.EnvVars)
	if err != nil {
		return "{}"
	}
	return string(data)
}

// ContainerIDsJSON returns the container IDs as JSON string for database storage
func (p *Project) ContainerIDsJSON() string {
	if p.ContainerIDs == nil {
		return "[]"
	}
	data, err := json.Marshal(p.ContainerIDs)
	if err != nil {
		return "[]"
	}
	return string(data)
}

// ParseEnvVars parses a JSON string into the EnvVars map
func (p *Project) ParseEnvVars(data string) error {
	if data == "" {
		p.EnvVars = make(map[string]string)
		return nil
	}
	return json.Unmarshal([]byte(data), &p.EnvVars)
}

// ParseContainerIDs parses a JSON string into the ContainerIDs slice
func (p *Project) ParseContainerIDs(data string) error {
	if data == "" {
		p.ContainerIDs = []string{}
		return nil
	}
	return json.Unmarshal([]byte(data), &p.ContainerIDs)
}

// GetEffectiveDomain returns the domain to use for this project
func (p *Project) GetEffectiveDomain(baseDomain string) string {
	if p.Domain != "" {
		return p.Domain
	}
	if p.UseSubdomain && baseDomain != "" {
		return p.Name + "." + baseDomain
	}
	return ""
}
