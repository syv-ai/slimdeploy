package docker

import (
	"fmt"
	"strings"

	"github.com/mhenrichsen/slimdeploy/internal/models"
)

// GenerateTraefikLabels generates Traefik labels for a project
func GenerateTraefikLabels(project *models.Project, baseDomain string) map[string]string {
	// Sanitize the project name for use as a router name
	routerName := sanitizeRouterName(project.Name)

	// Get the domain to use
	domain := project.GetEffectiveDomain(baseDomain)
	if domain == "" {
		// No domain configured, skip Traefik labels
		return map[string]string{}
	}

	// Get the port
	port := project.Port
	if port == 0 {
		port = 80
	}

	// Check if we're in local/dev mode (localhost domain means no SSL)
	isLocal := strings.HasSuffix(domain, ".localhost") || domain == "localhost"

	labels := map[string]string{
		"traefik.enable": "true",
		// Service
		fmt.Sprintf("traefik.http.services.%s.loadbalancer.server.port", routerName): fmt.Sprintf("%d", port),
	}

	// Always set the Docker network for Traefik to use
	labels["traefik.docker.network"] = "slimdeploy"

	if isLocal {
		// Simple HTTP-only routing for local development
		labels[fmt.Sprintf("traefik.http.routers.%s.rule", routerName)] = fmt.Sprintf("Host(`%s`)", domain)
		labels[fmt.Sprintf("traefik.http.routers.%s.entrypoints", routerName)] = "web"
	} else {
		// Production routing with HTTPS redirect
		// HTTP router (for redirect to HTTPS)
		labels[fmt.Sprintf("traefik.http.routers.%s-http.rule", routerName)] = fmt.Sprintf("Host(`%s`)", domain)
		labels[fmt.Sprintf("traefik.http.routers.%s-http.entrypoints", routerName)] = "web"
		labels[fmt.Sprintf("traefik.http.routers.%s-http.middlewares", routerName)] = "redirect-to-https@docker"

		// HTTPS router
		labels[fmt.Sprintf("traefik.http.routers.%s.rule", routerName)] = fmt.Sprintf("Host(`%s`)", domain)
		labels[fmt.Sprintf("traefik.http.routers.%s.entrypoints", routerName)] = "websecure"
		labels[fmt.Sprintf("traefik.http.routers.%s.tls.certresolver", routerName)] = "letsencrypt"
	}

	return labels
}

// GenerateTraefikLabelsForCompose generates Traefik labels for docker-compose services
// Returns a map of service name to labels
func GenerateTraefikLabelsForCompose(project *models.Project, baseDomain string, serviceName string) map[string]string {
	// For compose, we use project-service as the router name
	routerName := sanitizeRouterName(fmt.Sprintf("%s-%s", project.Name, serviceName))

	// Get the domain to use
	domain := project.GetEffectiveDomain(baseDomain)
	if domain == "" {
		return map[string]string{}
	}

	// Get the port
	port := project.Port
	if port == 0 {
		port = 80
	}

	// Check if we're in local/dev mode (localhost domain means no SSL)
	isLocal := strings.HasSuffix(domain, ".localhost") || domain == "localhost"

	labels := map[string]string{
		"traefik.enable": "true",
		// Service
		fmt.Sprintf("traefik.http.services.%s.loadbalancer.server.port", routerName): fmt.Sprintf("%d", port),
	}

	// Always set the Docker network for Traefik to use
	labels["traefik.docker.network"] = "slimdeploy"

	if isLocal {
		// Simple HTTP-only routing for local development
		labels[fmt.Sprintf("traefik.http.routers.%s.rule", routerName)] = fmt.Sprintf("Host(`%s`)", domain)
		labels[fmt.Sprintf("traefik.http.routers.%s.entrypoints", routerName)] = "web"
	} else {
		// Production routing with HTTPS redirect
		// HTTP router (for redirect to HTTPS)
		labels[fmt.Sprintf("traefik.http.routers.%s-http.rule", routerName)] = fmt.Sprintf("Host(`%s`)", domain)
		labels[fmt.Sprintf("traefik.http.routers.%s-http.entrypoints", routerName)] = "web"
		labels[fmt.Sprintf("traefik.http.routers.%s-http.middlewares", routerName)] = "redirect-to-https@docker"

		// HTTPS router
		labels[fmt.Sprintf("traefik.http.routers.%s.rule", routerName)] = fmt.Sprintf("Host(`%s`)", domain)
		labels[fmt.Sprintf("traefik.http.routers.%s.entrypoints", routerName)] = "websecure"
		labels[fmt.Sprintf("traefik.http.routers.%s.tls.certresolver", routerName)] = "letsencrypt"
	}

	return labels
}

// sanitizeRouterName creates a valid Traefik router name from a project name
func sanitizeRouterName(name string) string {
	// Replace any non-alphanumeric characters with hyphens
	var result strings.Builder
	for _, r := range strings.ToLower(name) {
		if (r >= 'a' && r <= 'z') || (r >= '0' && r <= '9') {
			result.WriteRune(r)
		} else {
			result.WriteRune('-')
		}
	}

	// Remove consecutive hyphens and trim
	cleaned := result.String()
	for strings.Contains(cleaned, "--") {
		cleaned = strings.ReplaceAll(cleaned, "--", "-")
	}
	cleaned = strings.Trim(cleaned, "-")

	return cleaned
}

// GenerateRedirectMiddleware generates the HTTPS redirect middleware labels
// This should be added to a single container (like Traefik itself or a dedicated middleware container)
func GenerateRedirectMiddleware() map[string]string {
	return map[string]string{
		"traefik.http.middlewares.redirect-to-https.redirectscheme.scheme":    "https",
		"traefik.http.middlewares.redirect-to-https.redirectscheme.permanent": "true",
	}
}
