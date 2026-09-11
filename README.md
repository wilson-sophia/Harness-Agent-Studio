# Harness Onboarding Agent

Harness Onboarding Agent is a weekend-sized AI DevOps workbench for helping Go services onboard to Harness CI/CD.

The first milestone focuses on a clear interview demo:

1. Enter a repository URL and branch.
2. Analyze the repository metadata and generate sensible defaults.
3. Review analysis and recommendations.
4. Generate a Go-oriented Harness pipeline YAML.
5. Visualize the generated CI flow with React Flow.
6. Show what is still missing before the pipeline can run in Harness.

## Product Modes

```text
Preview Mode
Analyze any public repository, generate Harness YAML, and show missing setup without credentials.

Demo Mode
Future sandbox path for trusted demo repositories only.

Connected Mode
Future path for using a user's Harness token to discover orgs, projects, connectors, create pipelines, and trigger runs.
```

## Project Layout

```text
frontend/   React + TypeScript + React Flow control plane
backend/    Go + Gin API that generates Harness pipeline YAML
docs/       Architecture notes and interview talking points
```

## Local Development

Start the backend:

```bash
cd backend
GOPATH=../../work/go GOCACHE=../../work/go-build go run .
```

Start the frontend:

```bash
cd frontend
npm_config_cache=../../work/npm-cache npm run dev
```

Then open the frontend URL printed by Vite.

## MVP Scope

This version is intentionally preview-first. It generates Harness YAML locally and makes missing Harness setup visible before any real API calls.

The "Agent Analysis" panel currently uses deterministic rules instead of a live LLM. The intended next step is to call an LLM provider for project analysis, validate its structured output in Go, and then generate stable Harness YAML from that validated structure.

For the current milestone, repository analysis is URL-based. The backend parses the provider, owner, and repository name, then derives service name, Docker image name, and detected context. Later, this can be replaced by repository file scanning plus an LLM provider.

## API Shape

```text
POST /api/projects/analyze
```

Parses repository metadata and returns defaults, analysis, recommendations, mode, and missing setup.

```text
POST /api/pipelines/generate
```

Uses the selected defaults and Harness metadata to generate pipeline nodes, YAML, mode, and missing setup.
