# Interview Notes

## One-minute pitch

I built Harness Agent Studio to shorten the path from a GitHub repository to a reviewable Harness CI pipeline. The Go backend scans a public Go or Node project, extracts build signals, optionally asks a model for bounded recommendations, and generates YAML itself. The React frontend shows the project evidence, pipeline graph, configuration gaps, and run lifecycle. Users can preview without credentials, run a fixed sandbox demo, or connect their own Harness account.

## Explain the three modes

- Preview solves the "what pipeline and setup do I need?" problem. It never executes code or asks for a token.
- Demo proves the run experience using one trusted pipeline owned by the platform. The user cannot swap in an arbitrary repository.
- Connected uses the user's own Harness token to discover resources, create a pipeline, trigger it, and fetch execution status. A run belongs to one app user.

## Hard questions

**Why call it an agent?** It observes repository metadata, chooses applicable CI stages through deterministic rules, and can request additional recommendations from a model. The model cannot invent shell commands or directly call Harness. Calling it a fully autonomous agent would be inaccurate.

**Why use Harness instead of building a runner?** Harness performs execution, connector-based access, and pipeline orchestration. The application solves onboarding, review, and account-scoped control.

**What if users have different projects?** The backend scans each GitHub branch and selects Go or Node steps. The user picks Harness org/project/connectors from their own account. The app does not assume one deployment environment fits every repo.

**What if the token leaks through the browser?** The browser sends it once to the backend over HTTPS in a real deployment. The backend verifies and encrypts it; subsequent browser requests use an HttpOnly session cookie. It never appears in localStorage, YAML, model prompts, or API responses.

**Does an 8-hour absolute timeout force a login every 8 hours?** It ends that individual session. A valid remember cookie can create a new session for normal use. Sensitive operations still need a password verified in the last five minutes.

**How are users isolated?** All credentials and run records are keyed by the authenticated app user. The server checks ownership before reading run details or triggering work. Demo credentials are used only for a backend-fixed pipeline.

**Is this full CI/CD?** The implemented path is CI through test/build and optional image publishing. Deployment to a real environment needs a Harness CD service and environment, which are not created by this app. This boundary is explicit in the UI and API.

## Demo order

1. Enter a public GitHub Go or Node repository, analyze it, and point out concrete file evidence.
2. Generate YAML and explain how the graph corresponds to the Go-generated pipeline.
3. Show missing resources in Preview, then explain the Connected account scope.
4. If a Sandbox is configured, trigger the fixed Demo pipeline and refresh its status.
5. If a user Harness account is configured, choose resources, create, trigger, and inspect the owned run.
