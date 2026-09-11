# Architecture

## Big Picture

Harness Onboarding Agent has three moving parts:

```text
Repository URL + branch + mode -> repository analysis -> generated defaults -> missing setup -> Go YAML generator -> React pipeline workbench
```

In plain language:

- Harness is the CI/CD engine.
- Pipeline YAML is the build-and-release instruction sheet.
- The analysis layer explains what the system detected and recommends.
- Missing setup explains why Preview Mode cannot run yet.
- Generated defaults reduce manual input for service name, image name, and detected context.
- The Go backend writes the final instruction sheet.
- The React frontend lets a developer inspect it before sending it to Harness.

## Modes

Preview Mode does not need credentials and never executes user code. It generates a Harness pipeline draft and lists the account, token, connector, and secret setup needed for a real run.

Demo Mode is reserved for a backend-owned Harness sandbox and trusted demo repositories. It should not run arbitrary user repositories with platform-owned credentials.

Connected Mode uses a user's Harness Account ID and API token. Tokens should stay on the backend and should not be sent to an LLM prompt.

## Current Milestone

The current implementation uses deterministic analysis rules rather than a live LLM call. This keeps the demo reliable and makes the architecture easy to explain in interviews.

Repository analysis is the bridge toward a real agent. Today it parses the URL and uses keyword rules over inferred context. Later, the backend can scan repository files and send structured context to an LLM provider for richer project understanding.

## Next Milestone

Add real Harness integration:

1. Store Harness account, org, project, and API token in environment variables.
2. Add repository file scanning for go.mod, Dockerfile, tests, and manifests.
3. Add an LLM provider interface for project analysis.
4. Validate LLM output before generating YAML.
5. Add an endpoint that creates or updates a pipeline through Harness APIs.
6. Add an endpoint that triggers a pipeline execution.
7. Stream pipeline execution status and logs into the frontend.
