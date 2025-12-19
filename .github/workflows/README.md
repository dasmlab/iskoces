# Iskoces CI/CD Pipeline Documentation

## Overview

Iskoces uses GitHub Actions for continuous integration. The pipeline:

1. **Builds** the Iskoces server container
2. **Scans** for vulnerabilities (permissive mode)
3. **Tests** code coverage (never fails on syntax)
4. **Publishes** manually or on main branch

## Pipeline Flow

### 1. Build
- Builds Iskoces server container
- Tags: `branch-name`, `dev-<commit-sha>`, `pr-<number>`
- Pushes to GHCR (except PRs)

### 2. Scan
- Trivy vulnerability scanning
- Permissive mode (logs but doesn't block)
- Uploads to GitHub Security

### 3. Coverage
- Runs Go tests
- Generates coverage report
- Never fails on syntax errors
- Uploads to Codecov

### 4. Publish (Manual)
- Trigger via `workflow_dispatch` with `publish: true`
- Tags as `dev-<commit-sha>`
- Auto-cleans old dev packages (keeps 3)

### 5. Main Branch
- Publishes as `latest` tag automatically

## Usage

See Glooscap CI/CD documentation for detailed usage instructions.

## Image Tags

- `dev-<commit-sha>`: Development builds
- `<branch-name>`: Branch builds
- `latest`: Main branch builds

