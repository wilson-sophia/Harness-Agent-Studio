# Harness Agent Studio

A Go + React workbench that turns a public GitHub repository into a reviewable Harness CI pipeline. It has three separate paths:

| Mode | What it does | Credentials |
| --- | --- | --- |
| Preview | Scans a public GitHub branch, infers Go or Node, recommends CI steps, creates a YAML draft and setup checklist | None |
| Demo | Runs one backend-configured, trusted pipeline in a Harness Sandbox and shows its status | Platform-owned, server-side |
| Connected | Saves a user's own Harness connection, lists their resources, creates a pipeline, triggers it and shows execution status | User-owned, encrypted server-side |

Harness executes the pipeline. This application does not run repository code on the Go server. The model is optional: when configured, it writes up to three recommendations from sanitized repository signals. Deterministic Go code creates the YAML.

## Run locally

Requires Go 1.27+ and Node.js compatible with Vite 8. From the repository root:

```bash
cp backend/.env.example backend/.env
openssl rand -base64 32
```

Paste the generated base64 string into `APP_ENCRYPTION_KEY` in `backend/.env`. Then start the backend in terminal one:

```bash
cd backend
set -a
source .env
set +a
go run .
```

Start the frontend in terminal two:

```bash
cd frontend
npm install
npm run dev -- --host 127.0.0.1 --port 5173
```

Open [http://127.0.0.1:5173/](http://127.0.0.1:5173/). If 5173 is in use, choose another Vite port and set `APP_ORIGIN` to the exact frontend URL before restarting Go. `VITE_API_BASE_URL` defaults to `http://127.0.0.1:8080`.

Preview requires outbound access to `codeload.github.com` and a public repository. It scans a bounded archive for file names, `go.mod` or `package.json`, and derived build signals. No repository code executes during analysis.

## Configure real Harness paths

For Connected Mode, create an account in the app and enter your Harness Account ID and API Token there. The Go backend verifies the token with Harness and encrypts it at rest using `APP_ENCRYPTION_KEY`. The token is never put in localStorage, a model prompt, a response, or generated YAML. Choose the real org, project, Git connector, and CI Kubernetes infrastructure connector. A Docker registry connector is optional when a Dockerfile is present. Create a pipeline, then trigger it explicitly. The app only reads runs owned by the signed-in user.

For Demo Mode, configure all `DEMO_*` variables in `backend/.env`. The pipeline must already exist in your sandbox and must be fixed to the trusted `DEMO_REPOSITORY`. The UI cannot substitute another repository or YAML for it. Each signed-in account can start up to three demo runs in 24 hours.

For model recommendations, configure `MODEL_API_KEY` and the matching `MODEL_API_URL`/`MODEL_NAME`. Only `api.deepseek.com` and `api.openai.com` are accepted. If the model is absent or fails, the rule-based analysis still works.

## Security and scope

- Email/password uses bcrypt. Session and optional remember cookies are HttpOnly, SameSite Strict, and Secure on HTTPS. Session idle timeout is 30 minutes; each session expires absolutely after 8 hours. A remember cookie can create a new session for up to 14 days, but does not count as fresh password verification.
- Saving/removing a Harness connection and creating/triggering a pipeline require password verification within the last 5 minutes. State-changing requests enforce exact Origin and CSRF token checks.
- Per-user sessions, encrypted Harness credentials, and run records are stored in `backend/data/app.json` with mode 0600. `APP_ENCRYPTION_KEY` must remain stable across restarts. This JSON store is intended for one local backend process, not multiple server replicas.
- The app supports public GitHub Go and Node repositories. Private repos, production multi-instance storage, registration email verification, Harness log streaming, and Harness CD deployment are outside this version. The app never claims a deploy placeholder is a real release.
- Real Harness API calls and model calls require your own credentials and outbound network access. The repository contains tests with fake transports; no real account has been exercised by the automated suite.

## Checks

```bash
cd backend && go test ./...
cd ../frontend && npm run build && npm run lint
```

See [architecture](docs/architecture.md) for the request flows and [interview notes](docs/interview-notes.md) for the decisions behind the three modes.
