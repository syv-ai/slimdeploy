# SlimDeploy

A lightweight, self-hosted Docker deployment platform. Deploy containers from Docker images, Dockerfiles, or Docker Compose files with automatic Traefik reverse proxy integration.

![Go](https://img.shields.io/badge/Go-1.21+-00ADD8?style=flat&logo=go)
![License](https://img.shields.io/badge/License-MIT-green.svg)

## Features

- **Multiple Deploy Types**
  - Docker images from any registry
  - Dockerfiles from Git repositories
  - Docker Compose files from Git repositories

- **Automatic Networking**
  - Traefik reverse proxy integration
  - Auto-generated subdomains (`project.yourdomain.com`)
  - Custom domain support
  - Automatic HTTPS with Let's Encrypt

- **Git Integration**
  - Deploy from any Git repository (HTTPS or SSH)
  - Branch selection
  - Auto-deploy on push (webhook support)

- **Simple Management**
  - Clean web UI for project management
  - Environment variable configuration
  - Deploy logs and status monitoring
  - Start/stop/restart controls

## Quick Start

### Prerequisites

- Docker and Docker Compose
- A domain pointing to your server (for Traefik routing)

### 1. Clone and Configure

```bash
git clone https://github.com/syv-ai/slimdeploy.git
cd slimdeploy
cp .env.example .env
```

Edit `.env` with your settings:

```env
SLIMDEPLOY_DOMAIN=slimdeploy.yourdomain.com
SLIMDEPLOY_BASE_DOMAIN=yourdomain.com
SLIMDEPLOY_PASSWORD=your-secure-password
LETSENCRYPT_EMAIL=you@example.com
```

### 2. Start Traefik (if not already running)

```bash
cd traefik
docker compose up -d
cd ..
```

### 3. Start SlimDeploy

```bash
docker compose up -d
```

Visit `https://slimdeploy.yourdomain.com` and log in with your configured password.

## Development

### Local Development

```bash
# Install dependencies
make deps

# Run in development mode
make dev

# Or build and run
make build
make run
```

The app runs at `http://localhost:8080` by default.

### Project Structure

```
slimdeploy/
├── cmd/slimdeploy/     # Application entrypoint
├── internal/
│   ├── api/            # HTTP handlers and routing
│   ├── db/             # SQLite database layer
│   ├── docker/         # Docker and Traefik integration
│   ├── git/            # Git operations
│   ├── models/         # Data models
│   └── watcher/        # Auto-deploy watcher
├── web/
│   ├── templates/      # Go HTML templates
│   └── static/         # CSS and JavaScript
└── traefik/            # Traefik configuration
```

## Configuration

| Environment Variable | Description | Default |
|---------------------|-------------|---------|
| `SLIMDEPLOY_DOMAIN` | Domain for SlimDeploy UI | `slimdeploy.localhost` |
| `SLIMDEPLOY_BASE_DOMAIN` | Base domain for project subdomains | `localhost` |
| `SLIMDEPLOY_PASSWORD` | Login password | `admin` |
| `SLIMDEPLOY_PORT` | HTTP port | `8080` |
| `LETSENCRYPT_EMAIL` | Email for Let's Encrypt certs | - |

## How It Works

1. **Create a Project**: Use the wizard to configure your deployment
2. **SlimDeploy Clones/Pulls**: For Git-based deploys, code is cloned locally
3. **Docker Build/Run**: Containers are built and started
4. **Traefik Routes Traffic**: Automatic routing via labels

### Deploy Types

| Type | Source | Use Case |
|------|--------|----------|
| **Docker Image** | Registry (Docker Hub, GHCR, etc.) | Pre-built images |
| **Dockerfile** | Git repository | Build from source |
| **Docker Compose** | Git repository | Multi-container apps |

## License

MIT

## Contributing

Contributions welcome! Please open an issue or PR.
