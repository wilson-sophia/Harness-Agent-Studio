package main

import (
	"archive/tar"
	"bytes"
	"compress/gzip"
	"encoding/base64"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
)

type roundTripFunc func(*http.Request) (*http.Response, error)

func (fn roundTripFunc) RoundTrip(req *http.Request) (*http.Response, error) { return fn(req) }
func fakeResponse(status int, body string) *http.Response {
	return &http.Response{StatusCode: status, Body: io.NopCloser(strings.NewReader(body)), Header: http.Header{"Content-Type": []string{"application/json"}}}
}

func archiveFixture(t *testing.T) []byte {
	t.Helper()
	var buffer bytes.Buffer
	gzipWriter := gzip.NewWriter(&buffer)
	tarWriter := tar.NewWriter(gzipWriter)
	for name, content := range map[string]string{
		"sample-main/go.mod":       "module sample\nrequire github.com/gin-gonic/gin v1.0.0",
		"sample-main/main_test.go": "package main",
		"sample-main/Dockerfile":   "FROM scratch",
	} {
		if err := tarWriter.WriteHeader(&tar.Header{Name: name, Mode: 0600, Size: int64(len(content))}); err != nil {
			t.Fatal(err)
		}
		if _, err := tarWriter.Write([]byte(content)); err != nil {
			t.Fatal(err)
		}
	}
	if err := tarWriter.Close(); err != nil {
		t.Fatal(err)
	}
	if err := gzipWriter.Close(); err != nil {
		t.Fatal(err)
	}
	return buffer.Bytes()
}

func testApp(t *testing.T) (*application, *gin.Engine) {
	t.Helper()
	t.Setenv("DATA_FILE", filepath.Join(t.TempDir(), "app.json"))
	t.Setenv("APP_ENCRYPTION_KEY", base64.StdEncoding.EncodeToString(bytes.Repeat([]byte{7}, 32)))
	app, err := newApplication()
	if err != nil {
		t.Fatal(err)
	}
	fixture := archiveFixture(t)
	app.httpClient.Transport = roundTripFunc(func(req *http.Request) (*http.Response, error) {
		switch {
		case req.URL.Host == "codeload.github.com":
			return &http.Response{StatusCode: 200, Body: io.NopCloser(bytes.NewReader(fixture)), Header: http.Header{"Content-Type": []string{"application/x-gzip"}}}, nil
		case req.URL.Host == "app.harness.io" && req.Method == "GET" && strings.HasSuffix(req.URL.Path, "/v1/orgs"):
			return fakeResponse(200, `{"data":{"content":[{"organization":{"identifier":"default","name":"Default"}}]}}`), nil
		case req.URL.Host == "app.harness.io" && req.Method == "POST" && strings.HasSuffix(req.URL.Path, "/execute"):
			return fakeResponse(200, `{"execution_details":{"execution_id":"execution-1","status":"RUNNING"}}`), nil
		case req.URL.Host == "app.harness.io" && req.Method == "POST":
			return fakeResponse(201, `{"identifier":"sample_ci"}`), nil
		case req.URL.Host == "app.harness.io" && req.Method == "GET":
			return fakeResponse(200, `{"data":{"status":"RUNNING"}}`), nil
		default:
			return fakeResponse(404, `{}`), nil
		}
	})
	gin.SetMode(gin.TestMode)
	router := gin.New()
	router.Use(app.sameOrigin())
	app.routes(router)
	return app, router
}

func send(t *testing.T, router *gin.Engine, method, path string, body any, cookie *http.Cookie, csrf string) *httptest.ResponseRecorder {
	t.Helper()
	var reader io.Reader
	if body != nil {
		raw, err := json.Marshal(body)
		if err != nil {
			t.Fatal(err)
		}
		reader = bytes.NewReader(raw)
	}
	req := httptest.NewRequest(method, path, reader)
	req.Header.Set("Origin", "http://127.0.0.1:5173")
	req.Header.Set("Content-Type", "application/json")
	if csrf != "" {
		req.Header.Set("X-CSRF-Token", csrf)
	}
	if cookie != nil {
		req.AddCookie(cookie)
	}
	recorder := httptest.NewRecorder()
	router.ServeHTTP(recorder, req)
	return recorder
}

func registerTestUser(t *testing.T, router *gin.Engine, email string) (*http.Cookie, string) {
	t.Helper()
	response := send(t, router, "POST", "/api/auth/register", map[string]any{"email": email, "password": "long-password-123"}, nil, "")
	if response.Code != 200 {
		t.Fatalf("register: %d %s", response.Code, response.Body.String())
	}
	var payload struct {
		CSRF string `json:"csrfToken"`
	}
	if json.Unmarshal(response.Body.Bytes(), &payload) != nil {
		t.Fatal("invalid auth response")
	}
	for _, cookie := range response.Result().Cookies() {
		if cookie.Name == "has_session" {
			return cookie, payload.CSRF
		}
	}
	t.Fatal("missing HttpOnly session cookie")
	return nil, ""
}

func TestRepositoryURLRejectsOtherHosts(t *testing.T) {
	for _, raw := range []string{"https://github.com.evil.test/o/r", "http://github.com/o/r", "https://github.com@evil.test/o/r", "https://github.com/o/r/extra"} {
		if _, _, err := githubLocation(raw); err == nil {
			t.Errorf("accepted %q", raw)
		}
	}
}

func TestAuthenticatedConnectionAndRunAreIsolated(t *testing.T) {
	app, router := testApp(t)
	alice, aliceCSRF := registerTestUser(t, router, "alice@example.com")
	bob, bobCSRF := registerTestUser(t, router, "bob@example.com")
	connection := send(t, router, "POST", "/api/harness/connection", map[string]string{"accountId": "accountA", "apiToken": "secret-token-alice"}, alice, aliceCSRF)
	if connection.Code != 200 {
		t.Fatalf("connection: %d %s", connection.Code, connection.Body.String())
	}
	bobView := send(t, router, "GET", "/api/harness/connection", nil, bob, bobCSRF)
	if !strings.Contains(bobView.Body.String(), `"connected":false`) {
		t.Fatalf("bob saw a connection: %s", bobView.Body.String())
	}
	req := map[string]any{"serviceName": "sample", "repoUrl": "https://github.com/example/sample", "branch": "main", "imageName": "example/sample", "mode": "connected", "orgIdentifier": "default", "projectIdentifier": "projectA", "codebaseConnectorRef": "git_connector", "k8sConnectorRef": "k8s_connector"}
	created := send(t, router, "POST", "/api/pipelines/create", req, alice, aliceCSRF)
	if created.Code != 201 {
		t.Fatalf("create: %d %s", created.Code, created.Body.String())
	}
	var run runRecord
	if json.Unmarshal(created.Body.Bytes(), &run) != nil {
		t.Fatal("invalid run")
	}
	blocked := send(t, router, "POST", "/api/runs/"+run.ID+"/trigger", map[string]any{}, bob, bobCSRF)
	if blocked.Code != 404 {
		t.Fatalf("bob could operate on Alice's run: %d", blocked.Code)
	}
	missingCSRF := send(t, router, "POST", "/api/runs/"+run.ID+"/trigger", map[string]any{}, alice, "")
	if missingCSRF.Code != 403 {
		t.Fatalf("missing CSRF accepted: %d", missingCSRF.Code)
	}
	triggered := send(t, router, "POST", "/api/runs/"+run.ID+"/trigger", map[string]any{}, alice, aliceCSRF)
	if triggered.Code != 200 {
		t.Fatalf("trigger: %d %s", triggered.Code, triggered.Body.String())
	}
	repeated := send(t, router, "POST", "/api/runs/"+run.ID+"/trigger", map[string]any{}, alice, aliceCSRF)
	if repeated.Code != 200 {
		t.Fatalf("repeat: %d %s", repeated.Code, repeated.Body.String())
	}
	var repeatedPayload struct {
		Run runRecord `json:"run"`
	}
	if json.Unmarshal(repeated.Body.Bytes(), &repeatedPayload) != nil || repeatedPayload.Run.ID == run.ID {
		t.Fatal("repeat run overwrote previous record")
	}
	raw, err := os.ReadFile(app.dataFile)
	if err != nil {
		t.Fatal(err)
	}
	if bytes.Contains(raw, []byte("secret-token-alice")) {
		t.Fatal("token persisted in plaintext")
	}
}

func TestFreshAuthExpiresAndDemoUsesFixedPipeline(t *testing.T) {
	app, router := testApp(t)
	t.Setenv("DEMO_REPOSITORY", "https://github.com/trusted/demo")
	t.Setenv("DEMO_API_TOKEN", "demo-secret")
	t.Setenv("DEMO_ACCOUNT_ID", "demoAccount")
	t.Setenv("DEMO_ORG_ID", "demoOrg")
	t.Setenv("DEMO_PROJECT_ID", "demoProject")
	t.Setenv("DEMO_PIPELINE_ID", "trusted_pipeline")
	cookie, csrf := registerTestUser(t, router, "demo@example.com")
	app.mu.Lock()
	key := secretHash(cookie.Value)
	session := app.data.Sessions[key]
	session.VerifiedAt = time.Now().Add(-6 * time.Minute)
	app.data.Sessions[key] = session
	app.mu.Unlock()
	blocked := send(t, router, "POST", "/api/demo/run", map[string]any{"repoUrl": "https://github.com/attacker/repo"}, cookie, csrf)
	if blocked.Code != 403 {
		t.Fatalf("stale auth accepted: %d", blocked.Code)
	}
	reauth := send(t, router, "POST", "/api/auth/reauth", map[string]string{"password": "long-password-123"}, cookie, csrf)
	if reauth.Code != 200 {
		t.Fatalf("reauth: %d %s", reauth.Code, reauth.Body.String())
	}
	started := send(t, router, "POST", "/api/demo/run", map[string]any{"repoUrl": "https://github.com/attacker/repo"}, cookie, csrf)
	if started.Code != 201 {
		t.Fatalf("demo: %d %s", started.Code, started.Body.String())
	}
	if strings.Contains(started.Body.String(), "attacker") {
		t.Fatal("demo used untrusted repo")
	}
	if !strings.Contains(started.Body.String(), "trusted_pipeline") {
		t.Fatal("demo did not use fixed pipeline")
	}
}

func TestPipelineForNodeWithoutBuildScript(t *testing.T) {
	req := pipelineRequest{ServiceName: "ui", RepoURL: "https://github.com/example/ui", Branch: "main", ImageName: "example/ui", Mode: "preview"}
	output, _, err := pipelineFor(req, repoSignals{Language: "node", Name: "ui", Owner: "example"})
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(output, "npm run build") {
		t.Fatal("invented build script")
	}
	if !strings.Contains(output, "npm install") {
		t.Fatal("missing dependency install")
	}
}

func TestAnalyzeAndGenerateUseScannedFiles(t *testing.T) {
	_, router := testApp(t)
	analyzed := send(t, router, "POST", "/api/projects/analyze", map[string]any{"repoUrl": "https://github.com/example/sample", "branch": "main", "mode": "preview"}, nil, "")
	if analyzed.Code != 200 {
		t.Fatalf("analyze: %d %s", analyzed.Code, analyzed.Body.String())
	}
	if !strings.Contains(analyzed.Body.String(), `"language":"go"`) || !strings.Contains(analyzed.Body.String(), "Dockerfile exists") {
		t.Fatalf("scan signals missing: %s", analyzed.Body.String())
	}
	request := map[string]any{"serviceName": "sample", "repoUrl": "https://github.com/example/sample", "branch": "main", "imageName": "example/sample", "mode": "preview"}
	generated := send(t, router, "POST", "/api/pipelines/generate", request, nil, "")
	if generated.Code != 200 {
		t.Fatalf("generate: %d %s", generated.Code, generated.Body.String())
	}
	if !strings.Contains(generated.Body.String(), "go test ./...") || !strings.Contains(generated.Body.String(), "YOUR_GIT_CONNECTOR") {
		t.Fatalf("generated pipeline does not match scanned Go project: %s", generated.Body.String())
	}
}
