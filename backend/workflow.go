package main

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"sort"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
	"gopkg.in/yaml.v3"
)

func (a *application) routes(router *gin.Engine) {
	router.POST("/api/auth/register", func(c *gin.Context) { a.auth(c, true) })
	router.POST("/api/auth/login", func(c *gin.Context) { a.auth(c, false) })
	router.GET("/api/auth/me", a.me)
	router.POST("/api/auth/reauth", a.requireSession(false), a.reauthenticate)
	router.POST("/api/auth/logout", a.requireSession(false), a.logout)
	router.POST("/api/projects/analyze", a.analyzeLive)
	router.POST("/api/pipelines/generate", a.generateLive)
	router.GET("/api/harness/connection", a.requireSession(false), a.connectionStatus)
	router.POST("/api/harness/connection", a.requireSession(true), a.saveConnection)
	router.DELETE("/api/harness/connection", a.requireSession(true), a.deleteConnection)
	router.GET("/api/harness/resources", a.requireSession(false), a.resources)
	router.POST("/api/pipelines/create", a.requireSession(true), a.createPipeline)
	router.GET("/api/runs", a.requireSession(false), a.listRuns)
	router.POST("/api/runs/:id/trigger", a.requireSession(true), a.triggerRun)
	router.GET("/api/runs/:id", a.requireSession(false), a.runStatus)
	router.GET("/api/demo/config", a.demoConfig)
	router.POST("/api/demo/run", a.requireSession(true), a.demoRun)
}

func (a *application) llmRecommendations(signals repoSignals) ([]string, string) {
	token := os.Getenv("MODEL_API_KEY")
	if token == "" {
		return nil, "rules"
	}
	endpoint := getenv("MODEL_API_URL", "https://api.deepseek.com/chat/completions")
	parsed, err := url.Parse(endpoint)
	if err != nil || parsed.Scheme != "https" || (parsed.Host != "api.deepseek.com" && parsed.Host != "api.openai.com") {
		return []string{"Model endpoint is invalid; using repository signals."}, "rules"
	}
	input := map[string]any{"model": getenv("MODEL_NAME", "deepseek-chat"), "temperature": 0.2, "max_tokens": 250,
		"messages": []map[string]string{
			{"role": "system", "content": "You are a CI advisor. Return JSON only: {\"recommendations\":[\"...\"]}. Give up to three concise English recommendations based only on metadata. Do not emit commands, YAML, secrets, or claims about files not listed."},
			{"role": "user", "content": fmt.Sprintf("Language: %s; workdir: %s; frameworks: %s; Dockerfile: %t; tests: %t; lockfile: %t; Kubernetes: %t", signals.Language, signals.Workdir, strings.Join(signals.Frameworks, ","), signals.HasDockerfile, signals.HasTests, signals.HasLockfile, signals.HasKubernetes)},
		}}
	raw, _ := json.Marshal(input)
	req, err := http.NewRequest(http.MethodPost, endpoint, bytes.NewReader(raw))
	if err != nil {
		return nil, "rules"
	}
	req.Header.Set("Authorization", "Bearer "+token)
	req.Header.Set("Content-Type", "application/json")
	response, err := a.httpClient.Do(req)
	if err != nil {
		return []string{"Model service unavailable; using repository signals."}, "rules"
	}
	defer response.Body.Close()
	if response.StatusCode != 200 {
		return []string{"Model service returned an error; using repository signals."}, "rules"
	}
	var result struct {
		Choices []struct {
			Message struct {
				Content string `json:"content"`
			} `json:"message"`
		} `json:"choices"`
	}
	if json.NewDecoder(io.LimitReader(response.Body, 32<<10)).Decode(&result) != nil || len(result.Choices) == 0 {
		return nil, "rules"
	}
	content := strings.TrimSpace(result.Choices[0].Message.Content)
	content = strings.TrimPrefix(strings.TrimSuffix(content, "```"), "```json")
	var recommendations struct {
		Items []string `json:"recommendations"`
	}
	if json.Unmarshal([]byte(strings.TrimSpace(content)), &recommendations) != nil {
		return []string{"Model response was invalid; using repository signals."}, "rules"
	}
	if len(recommendations.Items) > 3 {
		recommendations.Items = recommendations.Items[:3]
	}
	var clean []string
	for _, item := range recommendations.Items {
		if len(item) > 250 {
			continue
		}
		clean = append(clean, item)
	}
	return clean, "model"
}

func pipelineFor(req pipelineRequest, s repoSignals) (string, []pipelineNode, error) {
	if len(req.ServiceName) > 100 || !safeRepoPart.MatchString(req.ServiceName) {
		return "", nil, errors.New("service name must use letters, numbers, dots, underscores or hyphens")
	}
	if req.IncludeDeploy {
		return "", nil, errors.New("deployment requires a Harness CD service and environment; this generator currently creates CI pipelines")
	}
	if !safeRef.MatchString(req.Branch) {
		return "", nil, errors.New("invalid branch")
	}
	if req.Mode != "connected" {
		if req.OrgIdentifier == "" {
			req.OrgIdentifier = "default"
		}
		if req.ProjectIdentifier == "" {
			req.ProjectIdentifier = "YOUR_PROJECT"
		}
		if req.CodebaseConnectorRef == "" {
			req.CodebaseConnectorRef = "YOUR_GIT_CONNECTOR"
		}
		if req.K8sConnectorRef == "" {
			req.K8sConnectorRef = "YOUR_CI_INFRA_CONNECTOR"
		}
	}
	if req.OrgIdentifier == "" || req.ProjectIdentifier == "" || req.CodebaseConnectorRef == "" {
		return "", nil, errors.New("org, project, and Git connector are required")
	}
	if req.K8sConnectorRef == "" {
		return "", nil, errors.New("CI infrastructure connector is required")
	}
	if !safeRepoPart.MatchString(req.OrgIdentifier) || !safeRepoPart.MatchString(req.ProjectIdentifier) || !safeRepoPart.MatchString(req.CodebaseConnectorRef) {
		return "", nil, errors.New("invalid Harness identifier")
	}
	if s.HasDockerfile && req.DockerConnectorRef != "" && !safeRepoPart.MatchString(req.DockerConnectorRef) {
		return "", nil, errors.New("invalid Docker connector")
	}
	commandPrefix := ""
	if s.Workdir != "" {
		commandPrefix = "cd " + s.Workdir + " && "
	}
	tests, build, image := "go test ./...", "go build ./...", "golang:1.24"
	if s.Language == "node" {
		install := "npm install"
		if s.HasLockfile {
			install = "npm ci"
		}
		tests, build, image = install+" && CI=true npm test", install, "node:22"
		if s.HasBuild {
			build += " && npm run build"
		}
	}
	nodes := []pipelineNode{{ID: "clone", Label: "Clone", Kind: "source", Description: "Harness checks out the repository."}}
	steps := []any{}
	if s.HasTests {
		nodes = append(nodes, pipelineNode{ID: "test", Label: "Test", Kind: "test", Description: "Run the repository test suite.", Command: commandPrefix + tests})
		steps = append(steps, map[string]any{"step": map[string]any{"type": "Run", "name": "Test", "identifier": "test", "spec": map[string]any{"shell": "Sh", "image": image, "command": commandPrefix + tests}}})
	}
	nodes = append(nodes, pipelineNode{ID: "build", Label: "Build", Kind: "build", Description: "Compile or build the project.", Command: commandPrefix + build})
	steps = append(steps, map[string]any{"step": map[string]any{"type": "Run", "name": "Build", "identifier": "build", "spec": map[string]any{"shell": "Sh", "image": image, "command": commandPrefix + build}}})
	if s.HasDockerfile && req.DockerConnectorRef != "" {
		nodes = append(nodes, pipelineNode{ID: "docker", Label: "Docker Push", Kind: "package", Description: "Build and push the Docker image."})
		steps = append(steps, map[string]any{"step": map[string]any{"type": "BuildAndPushDockerRegistry", "name": "Docker Build and Push", "identifier": "docker_push", "spec": map[string]any{"connectorRef": req.DockerConnectorRef, "repo": req.ImageName, "tags": []string{"<+pipeline.sequenceId>"}, "dockerfile": s.DockerfilePath, "context": "."}}})
	}
	owner, name, _ := githubLocation(req.RepoURL)
	pipelineID := sanitizeIdentifier(req.ServiceName + "_ci")
	doc := map[string]any{"pipeline": map[string]any{
		"name": req.ServiceName + " CI", "identifier": pipelineID, "projectIdentifier": req.ProjectIdentifier, "orgIdentifier": req.OrgIdentifier,
		"properties": map[string]any{"ci": map[string]any{"codebase": map[string]any{"connectorRef": req.CodebaseConnectorRef, "repoName": owner + "/" + name, "build": map[string]any{"type": "branch", "spec": map[string]any{"branch": req.Branch}}}}},
		"stages": []any{map[string]any{"stage": map[string]any{
			"name": "Build", "identifier": "build", "type": "CI", "spec": map[string]any{
				"cloneCodebase":  true,
				"infrastructure": map[string]any{"type": "KubernetesDirect", "spec": map[string]any{"connectorRef": req.K8sConnectorRef, "namespace": req.Namespace, "automountServiceAccountToken": true, "os": "Linux"}},
				"execution":      map[string]any{"steps": steps},
			},
		}}},
	}}
	encoded, err := yaml.Marshal(doc)
	return string(encoded), nodes, err
}

func (a *application) generateLive(c *gin.Context) {
	var req pipelineRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(400, gin.H{"error": "invalid pipeline form"})
		return
	}
	normalizeRequest(&req)
	signals, err := a.scanRepository(req.RepoURL, req.Branch)
	if err != nil {
		c.JSON(422, gin.H{"error": err.Error()})
		return
	}
	if req.ServiceName == "" {
		req.ServiceName = serviceNameFromRepo(signals.Name)
	}
	if req.ImageName == "" {
		req.ImageName = imageNameFromRepo(signals.Owner, req.ServiceName)
	}
	output, nodes, err := pipelineFor(req, signals)
	if err != nil {
		c.JSON(422, gin.H{"error": err.Error()})
		return
	}
	analysis, recommendations := analysisForSignals(signals)
	c.JSON(200, pipelineResponse{PipelineIdentifier: sanitizeIdentifier(req.ServiceName + "_ci"), YAML: output, Nodes: nodes, Analysis: analysis, Recommendations: recommendations, MissingSetup: setupForSignals(req, signals), Mode: req.Mode, Notes: []string{"Generated from scanned GitHub repository metadata."}})
}

func setupForSignals(req pipelineRequest, s repoSignals) []setupItem {
	items := []setupItem{}
	add := func(id, label, description, requiredFor, status string) {
		items = append(items, setupItem{ID: id, Label: label, Description: description, RequiredFor: requiredFor, Status: status})
	}
	if req.Mode == "preview" {
		add("harness-account", "Harness Account", "Connect your account before creating a real pipeline.", "Connected Mode", "missing")
		add("harness-token", "Harness API Token", "Stored by the backend after you connect.", "Connected Mode", "missing")
	}
	if req.Mode == "connected" {
		if req.OrgIdentifier == "" {
			add("org", "Organization", "Choose an organization from your Harness account.", "Pipeline creation", "missing")
		}
		if req.ProjectIdentifier == "" {
			add("project", "Project", "Choose a project from your Harness account.", "Pipeline creation", "missing")
		}
		if req.CodebaseConnectorRef == "" {
			add("git-connector", "Git connector", "Choose a connector with repository read access.", "Pipeline creation", "missing")
		}
		if req.K8sConnectorRef == "" {
			add("ci-infra", "CI infrastructure connector", "Choose the Kubernetes connector used to run this CI stage.", "Pipeline run", "missing")
		}
	} else if req.Mode == "preview" {
		add("git-connector", "Git connector", "Harness needs repository read access.", "Pipeline creation", "missing")
		add("ci-infra", "CI infrastructure connector", "The CI stage needs a Kubernetes build environment.", "Pipeline run", "missing")
	}
	if s.HasDockerfile && req.DockerConnectorRef == "" {
		add("docker-connector", "Docker registry connector", "Select this to build and publish the detected Dockerfile.", "Image publishing", "optional")
	}
	return items
}

func (a *application) harnessCall(method, path, accountID, token string, body any) (json.RawMessage, error) {
	base := strings.TrimRight(getenv("HARNESS_API_BASE", "https://app.harness.io/gateway"), "/")
	parsed, err := url.Parse(base)
	if err != nil || parsed.Scheme != "https" || (parsed.Host != "app.harness.io" && parsed.Host != "app2.harness.io" && parsed.Host != "app3.harness.io") {
		return nil, errors.New("invalid HARNESS_API_BASE")
	}
	var reader io.Reader
	if body != nil {
		raw, _ := json.Marshal(body)
		reader = bytes.NewReader(raw)
	}
	req, err := http.NewRequest(method, base+path, reader)
	if err != nil {
		return nil, err
	}
	req.Header.Set("x-api-key", token)
	req.Header.Set("Harness-Account", accountID)
	req.Header.Set("Accept", "application/json")
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	response, err := a.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("Harness is unavailable: %w", err)
	}
	defer response.Body.Close()
	raw, err := io.ReadAll(io.LimitReader(response.Body, 256<<10))
	if err != nil {
		return nil, err
	}
	if response.StatusCode < 200 || response.StatusCode >= 300 {
		return nil, fmt.Errorf("Harness returned %d; check permissions and configured resources", response.StatusCode)
	}
	return raw, nil
}

func (a *application) credentialFor(userID string) (harnessCredential, string, error) {
	a.mu.Lock()
	credential := a.data.Credentials[userID]
	a.mu.Unlock()
	if credential.AccountID == "" {
		return credential, "", errors.New("connect your Harness account first")
	}
	token, err := a.decryptToken(credential.EncryptedToken)
	return credential, token, err
}

func (a *application) connectionStatus(c *gin.Context) {
	a.mu.Lock()
	credential := a.data.Credentials[sessionFrom(c).UserID]
	a.mu.Unlock()
	c.JSON(200, gin.H{"connected": credential.AccountID != "", "accountId": credential.AccountID, "encryptionConfigured": len(a.encryptionKey) == 32})
}

func (a *application) saveConnection(c *gin.Context) {
	var req struct {
		AccountID string `json:"accountId"`
		APIToken  string `json:"apiToken"`
	}
	if c.ShouldBindJSON(&req) != nil || !safeRepoPart.MatchString(req.AccountID) || len(req.APIToken) < 10 || len(req.APIToken) > 4096 {
		c.JSON(400, gin.H{"error": "valid account ID and API token required"})
		return
	}
	encrypted, err := a.encryptToken(req.APIToken)
	if err != nil {
		c.JSON(503, gin.H{"error": err.Error()})
		return
	}
	if _, err = a.harnessCall(http.MethodGet, "/v1/orgs", req.AccountID, req.APIToken, nil); err != nil {
		c.JSON(422, gin.H{"error": err.Error()})
		return
	}
	a.mu.Lock()
	a.data.Credentials[sessionFrom(c).UserID] = harnessCredential{AccountID: req.AccountID, EncryptedToken: encrypted}
	err = a.saveLocked()
	a.mu.Unlock()
	if err != nil {
		c.JSON(500, gin.H{"error": "connection could not be saved"})
		return
	}
	c.JSON(200, gin.H{"connected": true, "accountId": req.AccountID})
}

func (a *application) deleteConnection(c *gin.Context) {
	a.mu.Lock()
	delete(a.data.Credentials, sessionFrom(c).UserID)
	err := a.saveLocked()
	a.mu.Unlock()
	if err != nil {
		c.JSON(500, gin.H{"error": "connection could not be removed"})
		return
	}
	c.Status(204)
}

func (a *application) resources(c *gin.Context) {
	credential, token, err := a.credentialFor(sessionFrom(c).UserID)
	if err != nil {
		c.JSON(400, gin.H{"error": err.Error()})
		return
	}
	kind := c.Query("kind")
	org := c.Query("org")
	project := c.Query("project")
	path := ""
	switch kind {
	case "orgs":
		path = "/v1/orgs"
	case "projects":
		if !safeRepoPart.MatchString(org) {
			c.JSON(400, gin.H{"error": "org required"})
			return
		}
		path = "/v1/orgs/" + url.PathEscape(org) + "/projects"
	case "connectors":
		if !safeRepoPart.MatchString(org) || !safeRepoPart.MatchString(project) {
			c.JSON(400, gin.H{"error": "org and project required"})
			return
		}
		path = "/v1/orgs/" + url.PathEscape(org) + "/projects/" + url.PathEscape(project) + "/connectors"
	default:
		c.JSON(400, gin.H{"error": "invalid resource kind"})
		return
	}
	raw, err := a.harnessCall(http.MethodGet, path, credential.AccountID, token, nil)
	if err != nil {
		c.JSON(502, gin.H{"error": err.Error()})
		return
	}
	c.Data(200, "application/json", raw)
}

func (a *application) createPipeline(c *gin.Context) {
	var req pipelineRequest
	if c.ShouldBindJSON(&req) != nil {
		c.JSON(400, gin.H{"error": "invalid pipeline form"})
		return
	}
	normalizeRequest(&req)
	if req.Mode != "connected" {
		c.JSON(400, gin.H{"error": "connected mode required"})
		return
	}
	if req.ServiceName == "" || req.ImageName == "" {
		c.JSON(400, gin.H{"error": "analyze the repository before creating a pipeline"})
		return
	}
	signals, err := a.scanRepository(req.RepoURL, req.Branch)
	if err != nil {
		c.JSON(422, gin.H{"error": err.Error()})
		return
	}
	output, _, err := pipelineFor(req, signals)
	if err != nil {
		c.JSON(422, gin.H{"error": err.Error()})
		return
	}
	credential, token, err := a.credentialFor(sessionFrom(c).UserID)
	if err != nil {
		c.JSON(400, gin.H{"error": err.Error()})
		return
	}
	identifier := sanitizeIdentifier(req.ServiceName + "_ci")
	path := "/v1/orgs/" + url.PathEscape(req.OrgIdentifier) + "/projects/" + url.PathEscape(req.ProjectIdentifier) + "/pipelines"
	_, err = a.harnessCall(http.MethodPost, path, credential.AccountID, token, map[string]any{"pipeline_yaml": output, "identifier": identifier, "name": req.ServiceName + " CI"})
	if err != nil {
		c.JSON(502, gin.H{"error": err.Error()})
		return
	}
	record := runRecord{ID: randomSecret(), UserID: sessionFrom(c).UserID, Mode: "connected", AccountID: credential.AccountID, OrgID: req.OrgIdentifier, ProjectID: req.ProjectIdentifier, PipelineID: identifier, RepoURL: req.RepoURL, CreatedAt: time.Now()}
	a.mu.Lock()
	a.data.Runs[record.ID] = record
	err = a.saveLocked()
	a.mu.Unlock()
	if err != nil {
		c.JSON(500, gin.H{"error": "pipeline created but local record failed"})
		return
	}
	c.JSON(201, record)
}

func (a *application) listRuns(c *gin.Context) {
	userID := sessionFrom(c).UserID
	a.mu.Lock()
	records := []runRecord{}
	for _, record := range a.data.Runs {
		if record.UserID == userID {
			records = append(records, record)
		}
	}
	a.mu.Unlock()
	sort.Slice(records, func(i, j int) bool { return records[i].CreatedAt.After(records[j].CreatedAt) })
	c.JSON(200, records)
}

func (a *application) ownedRun(c *gin.Context) (runRecord, bool) {
	a.mu.Lock()
	record, ok := a.data.Runs[c.Param("id")]
	a.mu.Unlock()
	if !ok || record.UserID != sessionFrom(c).UserID {
		c.JSON(404, gin.H{"error": "run not found"})
		return runRecord{}, false
	}
	return record, true
}

func (a *application) triggerRun(c *gin.Context) {
	record, ok := a.ownedRun(c)
	if !ok {
		return
	}
	if !a.beginOperation("trigger:" + record.ID) {
		c.JSON(409, gin.H{"error": "a trigger is already in progress"})
		return
	}
	defer a.endOperation("trigger:" + record.ID)
	if record.Mode != "connected" {
		c.JSON(409, gin.H{"error": "only connected pipelines can be triggered here"})
		return
	}
	credential, token, err := a.credentialFor(record.UserID)
	if err != nil || credential.AccountID != record.AccountID {
		c.JSON(403, gin.H{"error": "matching Harness connection required"})
		return
	}
	path := "/v1/orgs/" + url.PathEscape(record.OrgID) + "/projects/" + url.PathEscape(record.ProjectID) + "/pipelines/" + url.PathEscape(record.PipelineID) + "/execute?module=CI"
	raw, err := a.harnessCall(http.MethodPost, path, record.AccountID, token, map[string]any{"inputs_yaml": ""})
	if err != nil {
		c.JSON(502, gin.H{"error": err.Error()})
		return
	}
	var response struct {
		ExecutionDetails struct {
			ID     string `json:"execution_id"`
			Status string `json:"status"`
		} `json:"execution_details"`
	}
	if json.Unmarshal(raw, &response) != nil || response.ExecutionDetails.ID == "" {
		c.JSON(502, gin.H{"error": "Harness returned an unexpected execution response"})
		return
	}
	if record.ExecutionID != "" {
		record.ID = randomSecret()
		record.CreatedAt = time.Now()
	}
	record.ExecutionID = response.ExecutionDetails.ID
	a.mu.Lock()
	a.data.Runs[record.ID] = record
	err = a.saveLocked()
	a.mu.Unlock()
	if err != nil {
		c.JSON(500, gin.H{"error": "run started but local record failed"})
		return
	}
	c.JSON(200, gin.H{"run": record, "status": response.ExecutionDetails.Status})
}

func (a *application) runStatus(c *gin.Context) {
	record, ok := a.ownedRun(c)
	if !ok {
		return
	}
	if record.ExecutionID == "" {
		c.JSON(200, gin.H{"run": record, "status": "NOT_STARTED"})
		return
	}
	var token string
	var err error
	if record.Mode == "demo" {
		token = os.Getenv("DEMO_API_TOKEN")
	} else {
		credential, selected, lookupErr := a.credentialFor(record.UserID)
		if lookupErr != nil || credential.AccountID != record.AccountID {
			c.JSON(403, gin.H{"error": "matching Harness connection required"})
			return
		}
		token = selected
	}
	path := "/pipeline/api/pipelines/execution/v2/" + url.PathEscape(record.ExecutionID) + "?accountIdentifier=" + url.QueryEscape(record.AccountID)
	raw, err := a.harnessCall(http.MethodGet, path, record.AccountID, token, nil)
	if err != nil {
		c.JSON(502, gin.H{"error": err.Error()})
		return
	}
	var response map[string]any
	if json.Unmarshal(raw, &response) != nil {
		c.JSON(502, gin.H{"error": "invalid Harness response"})
		return
	}
	status := extractStatus(response)
	c.JSON(200, gin.H{"run": record, "status": status, "details": response})
}

func extractStatus(value map[string]any) string {
	if status, ok := value["status"].(string); ok && status != "SUCCESS" {
		return status
	}
	for _, key := range []string{"data", "execution_details", "pipelineExecutionSummary"} {
		if nested, ok := value[key].(map[string]any); ok {
			if status := extractStatus(nested); status != "" {
				return status
			}
		}
	}
	return ""
}

func (a *application) demoConfig(c *gin.Context) {
	configured := os.Getenv("DEMO_API_TOKEN") != "" && os.Getenv("DEMO_ACCOUNT_ID") != "" && os.Getenv("DEMO_ORG_ID") != "" && os.Getenv("DEMO_PROJECT_ID") != "" && os.Getenv("DEMO_PIPELINE_ID") != "" && os.Getenv("DEMO_REPOSITORY") != ""
	c.JSON(200, gin.H{"configured": configured, "repository": os.Getenv("DEMO_REPOSITORY"), "pipelineIdentifier": os.Getenv("DEMO_PIPELINE_ID")})
}

func (a *application) demoRun(c *gin.Context) {
	userID := sessionFrom(c).UserID
	if !a.beginOperation("demo:" + userID) {
		c.JSON(429, gin.H{"error": "a demo run is already in progress"})
		return
	}
	defer a.endOperation("demo:" + userID)
	repo := os.Getenv("DEMO_REPOSITORY")
	if repo == "" || os.Getenv("DEMO_API_TOKEN") == "" || os.Getenv("DEMO_ACCOUNT_ID") == "" || os.Getenv("DEMO_ORG_ID") == "" || os.Getenv("DEMO_PROJECT_ID") == "" || os.Getenv("DEMO_PIPELINE_ID") == "" {
		c.JSON(503, gin.H{"error": "demo sandbox is not configured"})
		return
	}
	a.mu.Lock()
	count := 0
	for _, record := range a.data.Runs {
		if record.UserID == sessionFrom(c).UserID && record.Mode == "demo" && time.Since(record.CreatedAt) < 24*time.Hour {
			count++
		}
	}
	a.mu.Unlock()
	if count >= 3 {
		c.JSON(429, gin.H{"error": "demo limit is three runs per day"})
		return
	}
	record := runRecord{ID: randomSecret(), UserID: sessionFrom(c).UserID, Mode: "demo", AccountID: os.Getenv("DEMO_ACCOUNT_ID"), OrgID: os.Getenv("DEMO_ORG_ID"), ProjectID: os.Getenv("DEMO_PROJECT_ID"), PipelineID: os.Getenv("DEMO_PIPELINE_ID"), RepoURL: repo, CreatedAt: time.Now()}
	path := "/v1/orgs/" + url.PathEscape(record.OrgID) + "/projects/" + url.PathEscape(record.ProjectID) + "/pipelines/" + url.PathEscape(record.PipelineID) + "/execute?module=CI"
	raw, err := a.harnessCall(http.MethodPost, path, record.AccountID, os.Getenv("DEMO_API_TOKEN"), map[string]any{"inputs_yaml": ""})
	if err != nil {
		c.JSON(502, gin.H{"error": err.Error()})
		return
	}
	var response struct {
		ExecutionDetails struct {
			ID     string `json:"execution_id"`
			Status string `json:"status"`
		} `json:"execution_details"`
	}
	if json.Unmarshal(raw, &response) != nil || response.ExecutionDetails.ID == "" {
		c.JSON(502, gin.H{"error": "unexpected Harness execution response"})
		return
	}
	record.ExecutionID = response.ExecutionDetails.ID
	a.mu.Lock()
	a.data.Runs[record.ID] = record
	err = a.saveLocked()
	a.mu.Unlock()
	if err != nil {
		c.JSON(500, gin.H{"error": "run started but local record failed"})
		return
	}
	c.JSON(201, gin.H{"run": record, "status": response.ExecutionDetails.Status})
}

func (a *application) beginOperation(key string) bool {
	a.mu.Lock()
	defer a.mu.Unlock()
	if a.inflight[key] {
		return false
	}
	a.inflight[key] = true
	return true
}

func (a *application) endOperation(key string) {
	a.mu.Lock()
	delete(a.inflight, key)
	a.mu.Unlock()
}
