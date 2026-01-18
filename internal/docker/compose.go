package docker

import (
	"bytes"
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"github.com/mhenrichsen/slimdeploy/internal/models"
	"gopkg.in/yaml.v3"
)

// ComposeManager handles Docker Compose operations
type ComposeManager struct {
	baseDomain     string
	deploymentsDir string
}

// NewComposeManager creates a new ComposeManager
func NewComposeManager(baseDomain, deploymentsDir string) *ComposeManager {
	return &ComposeManager{
		baseDomain:     baseDomain,
		deploymentsDir: deploymentsDir,
	}
}

// ComposeFile represents a docker-compose.yml structure
type ComposeFile struct {
	Version  string                    `yaml:"version,omitempty"`
	Services map[string]ComposeService `yaml:"services"`
	Networks map[string]interface{}    `yaml:"networks,omitempty"`
	Volumes  map[string]interface{}    `yaml:"volumes,omitempty"`
}

// ComposeService represents a service in docker-compose.yml
type ComposeService struct {
	Image         string      `yaml:"image,omitempty"`
	Build         interface{} `yaml:"build,omitempty"`
	Ports         interface{} `yaml:"ports,omitempty"`
	Environment   interface{} `yaml:"environment,omitempty"`
	Volumes       interface{} `yaml:"volumes,omitempty"`
	Networks      interface{} `yaml:"networks,omitempty"`
	Labels        interface{} `yaml:"labels,omitempty"`
	DependsOn     interface{} `yaml:"depends_on,omitempty"`
	Restart       string      `yaml:"restart,omitempty"`
	Command       interface{} `yaml:"command,omitempty"`
	Entrypoint    interface{} `yaml:"entrypoint,omitempty"`
	WorkingDir    string      `yaml:"working_dir,omitempty"`
	User          string      `yaml:"user,omitempty"`
	ExtraHosts    interface{} `yaml:"extra_hosts,omitempty"`
	ContainerName string      `yaml:"container_name,omitempty"`
	Hostname      string      `yaml:"hostname,omitempty"`
	Expose        interface{} `yaml:"expose,omitempty"`
	HealthCheck   interface{} `yaml:"healthcheck,omitempty"`
	Logging       interface{} `yaml:"logging,omitempty"`
	Secrets       interface{} `yaml:"secrets,omitempty"`
	Configs       interface{} `yaml:"configs,omitempty"`
	Deploy        interface{} `yaml:"deploy,omitempty"`
	// Catch-all for any other fields
	Extra map[string]interface{} `yaml:",inline"`
}

// GetProjectDir returns the directory for a project's deployment files
func (cm *ComposeManager) GetProjectDir(projectName string) string {
	return filepath.Join(cm.deploymentsDir, projectName)
}

// FindComposeFile finds the docker-compose file in a project directory
func (cm *ComposeManager) FindComposeFile(projectDir string) (string, error) {
	candidates := []string{
		"docker-compose.yml",
		"docker-compose.yaml",
		"compose.yml",
		"compose.yaml",
	}

	for _, name := range candidates {
		path := filepath.Join(projectDir, name)
		if _, err := os.Stat(path); err == nil {
			return path, nil
		}
	}

	return "", fmt.Errorf("no docker-compose file found in %s", projectDir)
}

// ParseComposeFile parses a docker-compose.yml file
func (cm *ComposeManager) ParseComposeFile(path string) (*ComposeFile, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("failed to read compose file: %w", err)
	}

	var compose ComposeFile
	if err := yaml.Unmarshal(data, &compose); err != nil {
		return nil, fmt.Errorf("failed to parse compose file: %w", err)
	}

	return &compose, nil
}

// InjectLabels injects Traefik and SlimDeploy labels into a compose file
func (cm *ComposeManager) InjectLabels(project *models.Project, compose *ComposeFile, mainService string) *ComposeFile {
	// Make a copy to avoid modifying the original
	modified := *compose
	modified.Services = make(map[string]ComposeService)
	for k, v := range compose.Services {
		modified.Services[k] = v
	}

	// Ensure networks include slimdeploy
	if modified.Networks == nil {
		modified.Networks = make(map[string]interface{})
	}
	modified.Networks[NetworkName] = map[string]interface{}{
		"external": true,
	}

	// Modify services
	for name, service := range modified.Services {
		// Add slimdeploy network to all services
		networks := cm.getNetworksAsList(service.Networks)
		hasNetwork := false
		for _, n := range networks {
			if n == NetworkName {
				hasNetwork = true
				break
			}
		}
		if !hasNetwork {
			networks = append(networks, NetworkName)
		}
		service.Networks = networks

		// Convert labels to map format for easier manipulation
		labels := cm.getLabelsAsMap(service.Labels)

		// Remove conflicting Traefik labels (keep only traefik.enable)
		labelsToRemove := []string{}
		for k := range labels {
			if strings.HasPrefix(k, "traefik.") && k != "traefik.enable" {
				labelsToRemove = append(labelsToRemove, k)
			}
		}
		for _, k := range labelsToRemove {
			delete(labels, k)
		}

		// Add SlimDeploy management labels
		labels[LabelPrefix+".managed"] = "true"
		labels[LabelPrefix+".project"] = project.ID

		// Add Traefik labels only to the main service
		if name == mainService || (mainService == "" && name == cm.findMainService(compose)) {
			traefikLabels := GenerateTraefikLabelsForCompose(project, cm.baseDomain, name)
			for k, v := range traefikLabels {
				labels[k] = v
			}
		}

		service.Labels = labels
		modified.Services[name] = service
	}

	return &modified
}

// getLabelsAsMap converts labels (array or map) to map format
func (cm *ComposeManager) getLabelsAsMap(labels interface{}) map[string]string {
	result := make(map[string]string)
	if labels == nil {
		return result
	}

	switch l := labels.(type) {
	case map[string]string:
		return l
	case map[string]interface{}:
		for k, v := range l {
			if s, ok := v.(string); ok {
				result[k] = s
			}
		}
	case []interface{}:
		for _, item := range l {
			if s, ok := item.(string); ok {
				parts := strings.SplitN(s, "=", 2)
				if len(parts) == 2 {
					result[parts[0]] = parts[1]
				} else if len(parts) == 1 {
					result[parts[0]] = ""
				}
			}
		}
	}
	return result
}

// getNetworksAsList converts networks (array or map) to array format
func (cm *ComposeManager) getNetworksAsList(networks interface{}) []string {
	var result []string
	if networks == nil {
		return result
	}

	switch n := networks.(type) {
	case []string:
		return n
	case []interface{}:
		for _, item := range n {
			if s, ok := item.(string); ok {
				result = append(result, s)
			}
		}
	case map[string]interface{}:
		for name := range n {
			result = append(result, name)
		}
	}
	return result
}

// findMainService tries to identify the main service in a compose file
func (cm *ComposeManager) findMainService(compose *ComposeFile) string {
	// Look for common patterns
	mainCandidates := []string{"app", "web", "api", "server", "frontend", "backend", "nginx"}

	for _, candidate := range mainCandidates {
		if _, ok := compose.Services[candidate]; ok {
			return candidate
		}
	}

	// Return the first service
	for name := range compose.Services {
		return name
	}

	return ""
}

// WriteComposeFile writes a compose file to disk
func (cm *ComposeManager) WriteComposeFile(path string, compose *ComposeFile) error {
	data, err := yaml.Marshal(compose)
	if err != nil {
		return fmt.Errorf("failed to marshal compose file: %w", err)
	}

	if err := os.WriteFile(path, data, 0644); err != nil {
		return fmt.Errorf("failed to write compose file: %w", err)
	}

	return nil
}

// Up runs docker compose up for a project
func (cm *ComposeManager) Up(ctx context.Context, project *models.Project) error {
	projectDir := cm.GetProjectDir(project.Name)

	// Find compose file
	composePath, err := cm.FindComposeFile(projectDir)
	if err != nil {
		return err
	}

	// Parse compose file
	compose, err := cm.ParseComposeFile(composePath)
	if err != nil {
		return err
	}

	// Inject labels
	modified := cm.InjectLabels(project, compose, "")

	// Write modified compose file
	modifiedPath := filepath.Join(projectDir, ".slimdeploy-compose.yml")
	if err := cm.WriteComposeFile(modifiedPath, modified); err != nil {
		return err
	}

	// Build environment variables
	var envList []string
	for k, v := range project.EnvVars {
		envList = append(envList, fmt.Sprintf("%s=%s", k, v))
	}

	// Run docker compose up
	cmd := exec.CommandContext(ctx, "docker", "compose", "-f", modifiedPath, "-p", fmt.Sprintf("slimdeploy-%s", project.Name), "up", "-d", "--build", "--remove-orphans")
	cmd.Dir = projectDir
	cmd.Env = append(os.Environ(), envList...)

	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr

	if err := cmd.Run(); err != nil {
		return fmt.Errorf("docker compose up failed: %w\nstdout: %s\nstderr: %s", err, stdout.String(), stderr.String())
	}

	return nil
}

// Down runs docker compose down for a project
func (cm *ComposeManager) Down(ctx context.Context, project *models.Project) error {
	projectDir := cm.GetProjectDir(project.Name)

	modifiedPath := filepath.Join(projectDir, ".slimdeploy-compose.yml")

	// Check if modified compose file exists
	if _, err := os.Stat(modifiedPath); os.IsNotExist(err) {
		// Try to find original compose file
		var composeErr error
		modifiedPath, composeErr = cm.FindComposeFile(projectDir)
		if composeErr != nil {
			return composeErr
		}
	}

	cmd := exec.CommandContext(ctx, "docker", "compose", "-f", modifiedPath, "-p", fmt.Sprintf("slimdeploy-%s", project.Name), "down", "--remove-orphans")
	cmd.Dir = projectDir

	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr

	if err := cmd.Run(); err != nil {
		return fmt.Errorf("docker compose down failed: %w\nstdout: %s\nstderr: %s", err, stdout.String(), stderr.String())
	}

	return nil
}

// Restart runs docker compose restart for a project
func (cm *ComposeManager) Restart(ctx context.Context, project *models.Project) error {
	projectDir := cm.GetProjectDir(project.Name)

	modifiedPath := filepath.Join(projectDir, ".slimdeploy-compose.yml")

	// Check if modified compose file exists
	if _, err := os.Stat(modifiedPath); os.IsNotExist(err) {
		// Need to run Up instead
		return cm.Up(ctx, project)
	}

	cmd := exec.CommandContext(ctx, "docker", "compose", "-f", modifiedPath, "-p", fmt.Sprintf("slimdeploy-%s", project.Name), "restart")
	cmd.Dir = projectDir

	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr

	if err := cmd.Run(); err != nil {
		return fmt.Errorf("docker compose restart failed: %w\nstdout: %s\nstderr: %s", err, stdout.String(), stderr.String())
	}

	return nil
}

// Logs gets logs from docker compose services
func (cm *ComposeManager) Logs(ctx context.Context, project *models.Project, follow bool, tail int) (string, error) {
	projectDir := cm.GetProjectDir(project.Name)

	modifiedPath := filepath.Join(projectDir, ".slimdeploy-compose.yml")

	// Check if modified compose file exists
	if _, err := os.Stat(modifiedPath); os.IsNotExist(err) {
		var composeErr error
		modifiedPath, composeErr = cm.FindComposeFile(projectDir)
		if composeErr != nil {
			return "", composeErr
		}
	}

	args := []string{"compose", "-f", modifiedPath, "-p", fmt.Sprintf("slimdeploy-%s", project.Name), "logs", "--timestamps"}
	if tail > 0 {
		args = append(args, "--tail", fmt.Sprintf("%d", tail))
	}

	cmd := exec.CommandContext(ctx, "docker", args...)
	cmd.Dir = projectDir

	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr

	if err := cmd.Run(); err != nil {
		return "", fmt.Errorf("docker compose logs failed: %w\nstderr: %s", err, stderr.String())
	}

	return stdout.String(), nil
}

// PS gets the status of docker compose services
func (cm *ComposeManager) PS(ctx context.Context, project *models.Project) (string, error) {
	projectDir := cm.GetProjectDir(project.Name)

	modifiedPath := filepath.Join(projectDir, ".slimdeploy-compose.yml")

	// Check if modified compose file exists
	if _, err := os.Stat(modifiedPath); os.IsNotExist(err) {
		var composeErr error
		modifiedPath, composeErr = cm.FindComposeFile(projectDir)
		if composeErr != nil {
			return "", composeErr
		}
	}

	cmd := exec.CommandContext(ctx, "docker", "compose", "-f", modifiedPath, "-p", fmt.Sprintf("slimdeploy-%s", project.Name), "ps", "--format", "table")
	cmd.Dir = projectDir

	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr

	if err := cmd.Run(); err != nil {
		// If project doesn't exist, return empty
		if strings.Contains(stderr.String(), "no configuration file") || strings.Contains(stderr.String(), "no such file") {
			return "", nil
		}
		return "", fmt.Errorf("docker compose ps failed: %w\nstderr: %s", err, stderr.String())
	}

	return stdout.String(), nil
}
