# Agents Documentation

## Overview
This project utilizes multiple agents to manage different aspects of the system. Agents are defined in the `.agents` directory and configured in `.codex`.

## Agents List

### 1. API Agent
- **Role:** Manages the REST API endpoints described in `API.md`
- **Integration:** Works with Docker containers in `docker-compose.yml`
- **Tools:** Uses Go libraries from `internal/api`

### 2. Database Agent
- **Role:** Handles database migrations in `migrations/`
- **Integration:** Connects to PostgreSQL via Docker
- **Tools:** Uses Go's database/sql package

### 3. CI/CD Agent
- **Role:** Automates testing via GitHub Actions in `.github/`
- **Integration:** Monitors `go.mod` and `go.sum` changes
- **Tools:** Uses GitHub API and Go test framework

## Communication
Agents communicate through shared state in `internal/state.go` and use message queues from `internal/queue/`.

## Security
All agents enforce authentication via JWT tokens defined in `internal/auth/`.