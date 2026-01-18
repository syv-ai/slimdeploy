package docker

import (
	"context"
	"fmt"
	"io"
	"strings"
	"time"

	"github.com/docker/docker/api/types"
	"github.com/docker/docker/api/types/container"
	"github.com/docker/docker/api/types/filters"
	"github.com/docker/docker/api/types/network"
	"github.com/docker/docker/client"
	"github.com/mhenrichsen/slimdeploy/internal/models"
)

const (
	// NetworkName is the shared Docker network for SlimDeploy
	NetworkName = "slimdeploy"
	// LabelPrefix is the prefix for SlimDeploy labels
	LabelPrefix = "slimdeploy"
)

// Client wraps the Docker client
type Client struct {
	cli        *client.Client
	baseDomain string
}

// NewClient creates a new Docker client
func NewClient(baseDomain string) (*Client, error) {
	cli, err := client.NewClientWithOpts(client.FromEnv, client.WithAPIVersionNegotiation())
	if err != nil {
		return nil, fmt.Errorf("failed to create Docker client: %w", err)
	}

	return &Client{
		cli:        cli,
		baseDomain: baseDomain,
	}, nil
}

// Close closes the Docker client
func (c *Client) Close() error {
	return c.cli.Close()
}

// Ping checks if Docker is available
func (c *Client) Ping(ctx context.Context) error {
	_, err := c.cli.Ping(ctx)
	return err
}

// EnsureNetwork ensures the slimdeploy network exists
func (c *Client) EnsureNetwork(ctx context.Context) error {
	networks, err := c.cli.NetworkList(ctx, types.NetworkListOptions{
		Filters: filters.NewArgs(filters.Arg("name", NetworkName)),
	})
	if err != nil {
		return fmt.Errorf("failed to list networks: %w", err)
	}

	for _, n := range networks {
		if n.Name == NetworkName {
			return nil // Network already exists
		}
	}

	// Create network
	_, err = c.cli.NetworkCreate(ctx, NetworkName, types.NetworkCreate{
		Driver: "bridge",
		Labels: map[string]string{
			LabelPrefix + ".managed": "true",
		},
	})
	if err != nil {
		return fmt.Errorf("failed to create network: %w", err)
	}

	return nil
}

// PullImage pulls a Docker image
func (c *Client) PullImage(ctx context.Context, imageName string) error {
	reader, err := c.cli.ImagePull(ctx, imageName, types.ImagePullOptions{})
	if err != nil {
		return fmt.Errorf("failed to pull image %s: %w", imageName, err)
	}
	defer reader.Close()

	// Consume the output (required to complete the pull)
	_, err = io.Copy(io.Discard, reader)
	if err != nil {
		return fmt.Errorf("failed to read pull output: %w", err)
	}

	return nil
}

// RunContainer runs a container for a project
func (c *Client) RunContainer(ctx context.Context, project *models.Project) (string, error) {
	// Generate container name
	containerName := fmt.Sprintf("slimdeploy-%s", project.Name)

	// Stop and remove existing container if any
	if err := c.RemoveContainer(ctx, containerName); err != nil {
		// Ignore errors, container might not exist
	}

	// Generate labels
	labels := GenerateTraefikLabels(project, c.baseDomain)
	labels[LabelPrefix+".managed"] = "true"
	labels[LabelPrefix+".project"] = project.ID

	// Build environment variables
	var env []string
	for k, v := range project.EnvVars {
		env = append(env, fmt.Sprintf("%s=%s", k, v))
	}

	// Create container
	resp, err := c.cli.ContainerCreate(ctx,
		&container.Config{
			Image:  project.Image,
			Env:    env,
			Labels: labels,
		},
		&container.HostConfig{
			RestartPolicy: container.RestartPolicy{
				Name: "unless-stopped",
			},
		},
		&network.NetworkingConfig{
			EndpointsConfig: map[string]*network.EndpointSettings{
				NetworkName: {},
			},
		},
		nil,
		containerName,
	)
	if err != nil {
		return "", fmt.Errorf("failed to create container: %w", err)
	}

	// Start container
	if err := c.cli.ContainerStart(ctx, resp.ID, types.ContainerStartOptions{}); err != nil {
		return "", fmt.Errorf("failed to start container: %w", err)
	}

	return resp.ID, nil
}

// StopContainer stops a container
func (c *Client) StopContainer(ctx context.Context, containerID string) error {
	timeout := 30
	stopOptions := container.StopOptions{Timeout: &timeout}
	if err := c.cli.ContainerStop(ctx, containerID, stopOptions); err != nil {
		return fmt.Errorf("failed to stop container %s: %w", containerID, err)
	}
	return nil
}

// RemoveContainer stops and removes a container
func (c *Client) RemoveContainer(ctx context.Context, containerIDOrName string) error {
	// Try to stop first (ignore errors)
	timeout := 10
	stopOptions := container.StopOptions{Timeout: &timeout}
	_ = c.cli.ContainerStop(ctx, containerIDOrName, stopOptions)

	// Remove container
	if err := c.cli.ContainerRemove(ctx, containerIDOrName, types.ContainerRemoveOptions{Force: true}); err != nil {
		if strings.Contains(err.Error(), "No such container") {
			return nil
		}
		return fmt.Errorf("failed to remove container %s: %w", containerIDOrName, err)
	}
	return nil
}

// GetContainerStatus gets the status of a container
func (c *Client) GetContainerStatus(ctx context.Context, containerID string) (string, error) {
	info, err := c.cli.ContainerInspect(ctx, containerID)
	if err != nil {
		if strings.Contains(err.Error(), "No such container") {
			return "not_found", nil
		}
		return "", fmt.Errorf("failed to inspect container: %w", err)
	}
	return info.State.Status, nil
}

// GetContainerLogs gets the logs of a container
func (c *Client) GetContainerLogs(ctx context.Context, containerID string, tail int, follow bool) (io.ReadCloser, error) {
	options := types.ContainerLogsOptions{
		ShowStdout: true,
		ShowStderr: true,
		Follow:     follow,
		Timestamps: true,
	}
	if tail > 0 {
		options.Tail = fmt.Sprintf("%d", tail)
	}

	return c.cli.ContainerLogs(ctx, containerID, options)
}

// ListProjectContainers lists all containers for a project
func (c *Client) ListProjectContainers(ctx context.Context, projectID string) ([]types.Container, error) {
	containers, err := c.cli.ContainerList(ctx, types.ContainerListOptions{
		All: true,
		Filters: filters.NewArgs(
			filters.Arg("label", fmt.Sprintf("%s.project=%s", LabelPrefix, projectID)),
		),
	})
	if err != nil {
		return nil, fmt.Errorf("failed to list containers: %w", err)
	}
	return containers, nil
}

// ListAllManagedContainers lists all SlimDeploy-managed containers
func (c *Client) ListAllManagedContainers(ctx context.Context) ([]types.Container, error) {
	containers, err := c.cli.ContainerList(ctx, types.ContainerListOptions{
		All: true,
		Filters: filters.NewArgs(
			filters.Arg("label", LabelPrefix+".managed=true"),
		),
	})
	if err != nil {
		return nil, fmt.Errorf("failed to list containers: %w", err)
	}
	return containers, nil
}

// RestartContainer restarts a container
func (c *Client) RestartContainer(ctx context.Context, containerID string) error {
	timeout := 30
	if err := c.cli.ContainerRestart(ctx, containerID, container.StopOptions{Timeout: &timeout}); err != nil {
		return fmt.Errorf("failed to restart container %s: %w", containerID, err)
	}
	return nil
}

// WaitForHealthy waits for a container to be running
func (c *Client) WaitForHealthy(ctx context.Context, containerID string, timeout time.Duration) error {
	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		status, err := c.GetContainerStatus(ctx, containerID)
		if err != nil {
			return err
		}
		if status == "running" {
			return nil
		}
		if status == "exited" || status == "dead" {
			return fmt.Errorf("container exited unexpectedly")
		}
		time.Sleep(500 * time.Millisecond)
	}
	return fmt.Errorf("timeout waiting for container to be healthy")
}

// StopProjectContainers stops all containers for a project
func (c *Client) StopProjectContainers(ctx context.Context, projectID string) error {
	containers, err := c.ListProjectContainers(ctx, projectID)
	if err != nil {
		return err
	}

	for _, cont := range containers {
		if err := c.StopContainer(ctx, cont.ID); err != nil {
			return err
		}
	}
	return nil
}

// RemoveProjectContainers removes all containers for a project
func (c *Client) RemoveProjectContainers(ctx context.Context, projectID string) error {
	containers, err := c.ListProjectContainers(ctx, projectID)
	if err != nil {
		return err
	}

	for _, cont := range containers {
		if err := c.RemoveContainer(ctx, cont.ID); err != nil {
			return err
		}
	}
	return nil
}
