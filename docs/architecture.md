# Architecture

## Request flows

```text
Preview: URL + branch -> GitHub commit/tree + go.mod/package.json -> safe signals
        -> optional model recommendations -> Go-owned YAML -> graph + checklist

Connected: email/password -> HttpOnly session -> save encrypted Harness token
           -> discover org/project/connectors -> generate -> create in Harness
           -> explicit trigger -> fetch owned run status

Demo: authenticated user -> backend fixed sandbox pipeline -> Harness execution
      -> owned run status (never accepts repository or YAML from the caller)
```

The analyzer never clones or executes code. It downloads a public archive from the fixed `codeload.github.com` host with a 12-second timeout, no redirects, and compressed/uncompressed size and file-count bounds. File paths that become shell command arguments are constrained before YAML generation. This version supports a Go module or Node package; on a monorepo it picks a detected work directory. Dependency names and feature flags may go to the optional model. The model cannot choose executable commands or write Harness YAML.

The Go generator composes a Harness CI stage with clone, conditional test, build, and optional Docker publish steps. A Kubernetes CI infrastructure connector is required for a real run. CD deployment requires separate Harness service/environment definitions and is intentionally not represented as a fake deploy step.

## Session and ownership

The backend stores bcrypt password hashes, SHA-256 hashes of random session/remember values, encrypted Harness API tokens, and run records in one local JSON file. The encryption key is supplied through an environment variable; it is never stored in the data file. Auth cookies are HttpOnly and SameSite Strict. Browser writes need the exact configured Origin and a CSRF token.

- Idle timeout: 30 minutes without requests.
- Absolute timeout: one session ends after 8 hours even with activity.
- Remember: for 14 days, a rotating cookie can create a new low-assurance session. It does not renew password freshness.
- Fresh auth: save/remove Harness credentials and create/trigger executions require password verification in the last 5 minutes.

Every Connected credential and run is keyed to the signed-in user ID. Run detail lookup verifies ownership before contacting Harness. Demo uses platform credentials only for a fixed trusted pipeline; each user has a 3-run daily limit.

This is a single-process local store. A deployed multi-instance service should replace it with a transactional database, central session store, key management, audit logging, rate limiting, and registration verification.

## API

| Endpoint | Purpose |
| --- | --- |
| `POST /api/projects/analyze` | Public GitHub scan and optional model recommendations |
| `POST /api/pipelines/generate` | Deterministic Harness YAML draft |
| `POST /api/auth/register`, `/login`, `/reauth`, `/logout`; `GET /api/auth/me` | Login lifecycle |
| `GET/POST/DELETE /api/harness/connection` | User-owned encrypted connection |
| `GET /api/harness/resources` | Harness orgs, projects, connectors |
| `POST /api/pipelines/create` | Create a pipeline in the user's Harness project |
| `GET /api/runs`, `POST /api/runs/:id/trigger`, `GET /api/runs/:id` | Owned run lifecycle |
| `GET /api/demo/config`, `POST /api/demo/run` | Fixed sandbox |

The Harness paths, request bodies, `x-api-key`, and `Harness-Account` header follow the [Harness Pipeline API](https://apidocs.harness.io/pipelines), [Execution API](https://apidocs.harness.io/pipeline-execution), and [API authentication guide](https://developer.harness.io/docs/platform/automation/api/api-quickstart/). Real-account compatibility needs validation against the user's Harness region and connectors.
