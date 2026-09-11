package main

import (
	"net/http"
	"net/url"
	"os"
	"regexp"
	"strings"

	"github.com/gin-gonic/gin"
	"gopkg.in/yaml.v3"
)

type pipelineRequest struct {
	ServiceName          string `json:"serviceName" binding:"required"`
	RepoURL              string `json:"repoUrl" binding:"required"`
	ProjectDescription   string `json:"projectDescription"`
	Branch               string `json:"branch"`
	ImageName            string `json:"imageName" binding:"required"`
	Mode                 string `json:"mode"`
	HarnessAccountID     string `json:"harnessAccountId"`
	HasHarnessToken      bool   `json:"hasHarnessToken"`
	OrgIdentifier        string `json:"orgIdentifier"`
	ProjectIdentifier    string `json:"projectIdentifier"`
	CodebaseConnectorRef string `json:"codebaseConnectorRef"`
	DockerConnectorRef   string `json:"dockerConnectorRef"`
	IncludeDeploy        bool   `json:"includeDeploy"`
	K8sConnectorRef      string `json:"k8sConnectorRef"`
	Namespace            string `json:"namespace"`
}

type pipelineNode struct {
	ID          string `json:"id"`
	Label       string `json:"label"`
	Kind        string `json:"kind"`
	Description string `json:"description"`
	Command     string `json:"command,omitempty"`
}

type pipelineResponse struct {
	PipelineIdentifier string         `json:"pipelineIdentifier"`
	YAML               string         `json:"yaml"`
	Nodes              []pipelineNode `json:"nodes"`
	Analysis           []string       `json:"analysis"`
	Recommendations    []string       `json:"recommendations"`
	MissingSetup       []setupItem    `json:"missingSetup"`
	Mode               string         `json:"mode"`
	Notes              []string       `json:"notes"`
}

type projectAnalyzeRequest struct {
	RepoURL          string `json:"repoUrl" binding:"required"`
	Branch           string `json:"branch"`
	Mode             string `json:"mode"`
	HarnessAccountID string `json:"harnessAccountId"`
	HasHarnessToken  bool   `json:"hasHarnessToken"`
}

type projectAnalyzeResponse struct {
	Defaults        pipelineRequest `json:"defaults"`
	Analysis        []string        `json:"analysis"`
	Recommendations []string        `json:"recommendations"`
	MissingSetup    []setupItem     `json:"missingSetup"`
	Mode            string          `json:"mode"`
}

type setupItem struct {
	ID          string `json:"id"`
	Label       string `json:"label"`
	Description string `json:"description"`
	RequiredFor string `json:"requiredFor"`
	Status      string `json:"status"`
}

type repositoryMetadata struct {
	Provider string
	Owner    string
	Name     string
	IsGitHub bool
}

var (
	identifierCleaner = regexp.MustCompile(`[^a-zA-Z0-9_]+`)
	imagePartCleaner  = regexp.MustCompile(`[^a-z0-9._-]+`)
	serviceCleaner    = regexp.MustCompile(`[^a-z0-9-]+`)
)

func main() {
	router := gin.Default()
	router.Use(cors())

	router.GET("/api/health", func(c *gin.Context) {
		c.JSON(http.StatusOK, gin.H{"status": "ok"})
	})

	router.POST("/api/projects/analyze", analyzeRepository)
	router.POST("/api/pipelines/generate", generatePipeline)

	port := getenv("PORT", "8080")
	if err := router.Run(":" + port); err != nil {
		panic(err)
	}
}

func analyzeRepository(c *gin.Context) {
	var req projectAnalyzeRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	defaults := buildDefaultsFromRepository(req)
	analysis, recommendations := analyzeProject(defaults)
	metadata := parseRepositoryURL(defaults.RepoURL)
	missingSetup := buildMissingSetup(defaults)

	analysis = append([]string{
		"Repository URL was parsed before pipeline generation.",
		"Inferred service name: " + defaults.ServiceName + ".",
		"Inferred Docker image: " + defaults.ImageName + ".",
		"Live repository file scanning is not enabled in this milestone.",
	}, analysis...)

	if metadata.Provider != "" {
		analysis = append([]string{"Detected repository provider: " + metadata.Provider + "."}, analysis...)
	}

	c.JSON(http.StatusOK, projectAnalyzeResponse{
		Defaults:        defaults,
		Analysis:        analysis,
		Recommendations: recommendations,
		MissingSetup:    missingSetup,
		Mode:            defaults.Mode,
	})
}

func generatePipeline(c *gin.Context) {
	var req pipelineRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	normalizeRequest(&req)
	analysis, recommendations := analyzeProject(req)
	pipelineID := sanitizeIdentifier(req.ServiceName + "_ci")
	nodes := buildNodes(req.IncludeDeploy)
	doc := buildHarnessPipeline(req, pipelineID)
	missingSetup := buildMissingSetup(req)

	encoded, err := yaml.Marshal(doc)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to generate pipeline yaml"})
		return
	}

	c.JSON(http.StatusOK, pipelineResponse{
		PipelineIdentifier: pipelineID,
		YAML:               string(encoded),
		Nodes:              nodes,
		Analysis:           analysis,
		Recommendations:    recommendations,
		MissingSetup:       missingSetup,
		Mode:               req.Mode,
		Notes: []string{
			"v1 uses a deterministic agent rule set for Go services.",
			"Next milestone: call Harness APIs to create and trigger the generated pipeline.",
		},
	})
}

func buildDefaultsFromRepository(req projectAnalyzeRequest) pipelineRequest {
	metadata := parseRepositoryURL(req.RepoURL)
	serviceName := serviceNameFromRepo(metadata.Name)
	imageName := imageNameFromRepo(metadata.Owner, serviceName)
	branch := strings.TrimSpace(req.Branch)
	if branch == "" {
		branch = "main"
	}

	defaults := pipelineRequest{
		ServiceName:          serviceName,
		RepoURL:              strings.TrimSpace(req.RepoURL),
		ProjectDescription:   inferredProjectDescription(metadata),
		Branch:               branch,
		ImageName:            imageName,
		Mode:                 normalizeMode(req.Mode),
		HarnessAccountID:     strings.TrimSpace(req.HarnessAccountID),
		HasHarnessToken:      req.HasHarnessToken,
		OrgIdentifier:        "default",
		ProjectIdentifier:    "harness_agent_studio",
		CodebaseConnectorRef: "github_connector",
		DockerConnectorRef:   "dockerhub_connector",
		IncludeDeploy:        false,
		K8sConnectorRef:      "k8s_connector",
		Namespace:            "default",
	}
	normalizeRequest(&defaults)

	return defaults
}

func buildMissingSetup(req pipelineRequest) []setupItem {
	switch normalizeMode(req.Mode) {
	case "connected":
		items := []setupItem{}
		if strings.TrimSpace(req.HarnessAccountID) == "" {
			items = append(items, setupItem{
				ID:          "harness-account",
				Label:       "Harness Account ID",
				Description: "Required before the backend can query the user's Harness resources.",
				RequiredFor: "Connected Mode",
				Status:      "missing",
			})
		}
		if !req.HasHarnessToken {
			items = append(items, setupItem{
				ID:          "harness-token",
				Label:       "Harness API Token",
				Description: "Required to list orgs, projects, connectors, create pipelines, and trigger runs.",
				RequiredFor: "Connected Mode",
				Status:      "missing",
			})
		}
		items = append(items, setupItem{
			ID:          "resource-discovery",
			Label:       "Harness Resource Discovery",
			Description: "Next milestone: replace typed placeholders with org, project, and connector selectors from Harness.",
			RequiredFor: "Create Pipeline",
			Status:      "next",
		})
		return items
	case "demo":
		return []setupItem{
			{
				ID:          "demo-sandbox",
				Label:       "Demo Harness Sandbox",
				Description: "A backend-owned Harness sandbox is needed for safe demo runs.",
				RequiredFor: "Demo Mode",
				Status:      "next",
			},
			{
				ID:          "demo-whitelist",
				Label:       "Whitelisted Demo Repository",
				Description: "Demo mode should only run trusted sample repositories, not arbitrary user code.",
				RequiredFor: "Demo Mode",
				Status:      "next",
			},
		}
	default:
		return []setupItem{
			{
				ID:          "harness-account",
				Label:       "Harness Account ID",
				Description: "Needed when the user wants this pipeline created in their Harness account.",
				RequiredFor: "Connected Mode",
				Status:      "missing",
			},
			{
				ID:          "harness-token",
				Label:       "Harness API Token",
				Description: "Needed for org, project, connector, pipeline, and execution API calls.",
				RequiredFor: "Connected Mode",
				Status:      "missing",
			},
			{
				ID:          "git-connector",
				Label:       "GitHub Connector",
				Description: "Harness needs a connector that can read the target GitHub repository.",
				RequiredFor: "Pipeline Creation",
				Status:      "missing",
			},
			{
				ID:          "docker-connector",
				Label:       "Docker Registry Connector",
				Description: "Harness needs a connector for pushing the service image.",
				RequiredFor: "Image Publishing",
				Status:      "missing",
			},
			{
				ID:          "docker-secret",
				Label:       "Docker Registry Secret",
				Description: "Registry credentials should be stored as Harness secrets, not embedded in YAML.",
				RequiredFor: "Image Publishing",
				Status:      "missing",
			},
			{
				ID:          "k8s-connector",
				Label:       "Kubernetes Connector",
				Description: "Only needed when deployment is enabled after the CI path is stable.",
				RequiredFor: "Deployment",
				Status:      "optional",
			},
		}
	}
}

func normalizeMode(mode string) string {
	switch strings.ToLower(strings.TrimSpace(mode)) {
	case "demo":
		return "demo"
	case "connected":
		return "connected"
	default:
		return "preview"
	}
}

func parseRepositoryURL(repoURL string) repositoryMetadata {
	cleaned := strings.TrimSpace(repoURL)
	cleaned = strings.TrimSuffix(cleaned, ".git")

	metadata := repositoryMetadata{
		Provider: "Git",
		Name:     "go-service",
	}

	lower := strings.ToLower(cleaned)
	if strings.Contains(lower, "github.com") {
		metadata.Provider = "GitHub"
		metadata.IsGitHub = true
	}

	if strings.HasPrefix(cleaned, "git@") {
		parseSSHRepositoryURL(cleaned, &metadata)
		return metadata
	}

	if parsed, err := url.Parse(cleaned); err == nil && parsed.Host != "" {
		parts := strings.Split(strings.Trim(parsed.Path, "/"), "/")
		if len(parts) >= 2 {
			metadata.Owner = strings.TrimSpace(parts[0])
			metadata.Name = strings.TrimSuffix(strings.TrimSpace(parts[1]), ".git")
		}
		return metadata
	}

	parts := strings.Split(strings.Trim(cleaned, "/"), "/")
	if len(parts) >= 2 {
		metadata.Owner = strings.TrimSpace(parts[len(parts)-2])
		metadata.Name = strings.TrimSuffix(strings.TrimSpace(parts[len(parts)-1]), ".git")
	}

	return metadata
}

func parseSSHRepositoryURL(repoURL string, metadata *repositoryMetadata) {
	withoutPrefix := strings.TrimPrefix(repoURL, "git@")
	hostAndPath := strings.SplitN(withoutPrefix, ":", 2)
	if len(hostAndPath) != 2 {
		return
	}

	host := strings.ToLower(hostAndPath[0])
	if strings.Contains(host, "github.com") {
		metadata.Provider = "GitHub"
		metadata.IsGitHub = true
	}

	parts := strings.Split(strings.Trim(hostAndPath[1], "/"), "/")
	if len(parts) >= 2 {
		metadata.Owner = strings.TrimSpace(parts[0])
		metadata.Name = strings.TrimSuffix(strings.TrimSpace(parts[1]), ".git")
	}
}

func inferredProjectDescription(metadata repositoryMetadata) string {
	source := metadata.Provider
	if source == "" {
		source = "Git"
	}

	return "Go backend inferred from " + source + " repository metadata. The current analyzer assumes Go modules, a Dockerfile-based image build, and CI-first delivery before real deployment."
}

func serviceNameFromRepo(repoName string) string {
	name := strings.ToLower(strings.TrimSpace(repoName))
	name = serviceCleaner.ReplaceAllString(name, "-")
	name = strings.Trim(name, "-")
	if name == "" {
		return "go-service"
	}

	return name
}

func imageNameFromRepo(owner string, serviceName string) string {
	cleanOwner := sanitizeImagePart(owner)
	cleanService := sanitizeImagePart(serviceName)
	if cleanService == "" {
		cleanService = "go-service"
	}
	if cleanOwner == "" {
		return cleanService
	}

	return cleanOwner + "/" + cleanService
}

func sanitizeImagePart(value string) string {
	part := strings.ToLower(strings.TrimSpace(value))
	if part == "" {
		return ""
	}

	part = imagePartCleaner.ReplaceAllString(part, "-")
	part = strings.Trim(part, "-._")

	return part
}

func analyzeProject(req pipelineRequest) ([]string, []string) {
	description := strings.ToLower(req.ProjectDescription)
	analysis := []string{
		"Detected Go service workflow from the selected project template.",
		"Using Go modules friendly commands: go test ./... and go build.",
	}

	if description == "" {
		analysis = append(analysis, "No project description was provided, so analysis falls back to form fields only.")
	} else {
		analysis = append(analysis, "Detected context is used as structured input for the rule-based agent analysis.")
	}

	if containsAny(description, "gin") {
		analysis = append(analysis, "Detected Gin as the likely Go HTTP framework.")
	}

	if containsAny(description, "fiber", "echo") {
		analysis = append(analysis, "Detected a Go web framework from the project description.")
	}

	if containsAny(description, "postgres", "postgresql") {
		analysis = append(analysis, "Detected PostgreSQL as a likely backing database.")
	}

	if containsAny(description, "redis") {
		analysis = append(analysis, "Detected Redis, which may be used for caching, rate limits, or distributed locks.")
	}

	if containsAny(description, "dockerfile") {
		analysis = append(analysis, "Dockerfile is mentioned, so the pipeline can reuse the repository build definition.")
	} else if containsAny(description, "docker") {
		analysis = append(analysis, "Docker is mentioned, but Dockerfile availability should be verified before running CI.")
	}

	if containsAny(description, "kubernetes", "k8s") {
		analysis = append(analysis, "Detected Kubernetes as the likely deployment target.")
	}

	if containsAny(description, "monorepo") {
		analysis = append(analysis, "Detected monorepo context, so path-based pipeline triggers may be useful later.")
	}

	if strings.Contains(strings.ToLower(req.RepoURL), "github.com") {
		analysis = append(analysis, "Repository source looks like GitHub, so a GitHub codebase connector is expected.")
	} else {
		analysis = append(analysis, "Repository source is not recognized as GitHub; connector configuration may need adjustment.")
	}

	if req.ImageName != "" {
		analysis = append(analysis, "Docker image publishing is configured through the image name field.")
	}

	if req.IncludeDeploy {
		analysis = append(analysis, "Deployment placeholder is enabled, but real CD should be added after the Kubernetes connector is verified.")
	} else {
		analysis = append(analysis, "Deployment is disabled, so the generated pipeline focuses on CI and image publishing.")
	}

	recommendations := []string{
		"Run tests before building the binary to fail fast on code issues.",
		"Use the Harness pipeline sequence id as one Docker tag so every run has a traceable image.",
		"Keep final YAML generation in Go so future LLM output can be validated before reaching Harness.",
	}

	if description == "" {
		recommendations = append(recommendations, "Add a short project description to get more specific analysis before connecting an LLM provider.")
	}

	if containsAny(description, "gin", "fiber", "echo") {
		recommendations = append(recommendations, "Keep a lightweight health endpoint so Harness can later add deployment verification.")
	}

	if containsAny(description, "postgres", "postgresql", "redis") {
		recommendations = append(recommendations, "Keep database and cache credentials in Harness secrets instead of hardcoding them in YAML.")
	}

	if containsAny(description, "dockerfile") {
		recommendations = append(recommendations, "Use the repository Dockerfile for image builds and keep runtime configuration outside the image.")
	}

	if containsAny(description, "monorepo") {
		recommendations = append(recommendations, "Add path filters in a later milestone so unchanged services do not rebuild unnecessarily.")
	}

	if req.IncludeDeploy {
		recommendations = append(recommendations, "Replace the deploy placeholder with Harness CD or GitOps once the Kubernetes service model is ready.")
	} else {
		if containsAny(description, "kubernetes", "k8s") {
			recommendations = append(recommendations, "Enable the deploy stage after the Kubernetes connector and namespace are confirmed.")
		} else {
			recommendations = append(recommendations, "Add deployment as milestone 2 after the CI path is stable.")
		}
	}

	return analysis, recommendations
}

func normalizeRequest(req *pipelineRequest) {
	req.ServiceName = strings.TrimSpace(req.ServiceName)
	req.RepoURL = strings.TrimSpace(req.RepoURL)
	req.ProjectDescription = strings.TrimSpace(req.ProjectDescription)
	req.ImageName = strings.TrimSpace(req.ImageName)
	req.Mode = normalizeMode(req.Mode)
	req.HarnessAccountID = strings.TrimSpace(req.HarnessAccountID)

	if req.Branch == "" {
		req.Branch = "main"
	}
	if req.OrgIdentifier == "" {
		req.OrgIdentifier = "default"
	}
	if req.ProjectIdentifier == "" {
		req.ProjectIdentifier = "harness_agent_studio"
	}
	if req.CodebaseConnectorRef == "" {
		req.CodebaseConnectorRef = "github_connector"
	}
	if req.DockerConnectorRef == "" {
		req.DockerConnectorRef = "dockerhub_connector"
	}
	if req.K8sConnectorRef == "" {
		req.K8sConnectorRef = "k8s_connector"
	}
	if req.Namespace == "" {
		req.Namespace = "default"
	}
}

func containsAny(value string, keywords ...string) bool {
	for _, keyword := range keywords {
		if strings.Contains(value, keyword) {
			return true
		}
	}

	return false
}

func buildNodes(includeDeploy bool) []pipelineNode {
	nodes := []pipelineNode{
		{
			ID:          "clone",
			Label:       "Clone",
			Kind:        "source",
			Description: "Fetch repository code from GitHub.",
		},
		{
			ID:          "test",
			Label:       "Go Test",
			Kind:        "test",
			Description: "Run the Go test suite before building.",
			Command:     "go test ./...",
		},
		{
			ID:          "build",
			Label:       "Go Build",
			Kind:        "build",
			Description: "Compile the Go service binary.",
			Command:     "go build -o bin/app ./...",
		},
		{
			ID:          "docker",
			Label:       "Docker Push",
			Kind:        "package",
			Description: "Build and push the runtime image.",
		},
	}

	if includeDeploy {
		nodes = append(nodes, pipelineNode{
			ID:          "deploy",
			Label:       "Deploy",
			Kind:        "deploy",
			Description: "Placeholder Kubernetes deploy step for the next milestone.",
		})
	}

	return nodes
}

func buildHarnessPipeline(req pipelineRequest, pipelineID string) map[string]any {
	steps := []any{
		map[string]any{
			"step": map[string]any{
				"type":       "Run",
				"name":       "Go Test",
				"identifier": "go_test",
				"spec": map[string]any{
					"shell":   "Sh",
					"command": "go test ./...",
				},
			},
		},
		map[string]any{
			"step": map[string]any{
				"type":       "Run",
				"name":       "Go Build",
				"identifier": "go_build",
				"spec": map[string]any{
					"shell":   "Sh",
					"command": "go build -o bin/app ./...",
				},
			},
		},
		map[string]any{
			"step": map[string]any{
				"type":       "BuildAndPushDockerRegistry",
				"name":       "Docker Build and Push",
				"identifier": "docker_build_and_push",
				"spec": map[string]any{
					"connectorRef": req.DockerConnectorRef,
					"repo":         req.ImageName,
					"tags": []string{
						"<+pipeline.sequenceId>",
						req.Branch,
					},
					"dockerfile": "Dockerfile",
				},
			},
		},
	}

	if req.IncludeDeploy {
		steps = append(steps, map[string]any{
			"step": map[string]any{
				"type":       "Run",
				"name":       "Deploy Placeholder",
				"identifier": "deploy_placeholder",
				"spec": map[string]any{
					"shell": "Sh",
					"command": strings.Join([]string{
						"echo \"Deploy " + req.ImageName + " to namespace " + req.Namespace + "\"",
						"echo \"Replace this placeholder with Harness CD or GitOps in milestone 2.\"",
					}, "\n"),
				},
			},
		})
	}

	return map[string]any{
		"pipeline": map[string]any{
			"name":              req.ServiceName + " CI",
			"identifier":        pipelineID,
			"projectIdentifier": req.ProjectIdentifier,
			"orgIdentifier":     req.OrgIdentifier,
			"tags": map[string]string{
				"generated_by": "harness-agent-studio",
				"language":     "go",
			},
			"properties": map[string]any{
				"ci": map[string]any{
					"codebase": map[string]any{
						"connectorRef": req.CodebaseConnectorRef,
						"repoName":     req.RepoURL,
						"build": map[string]any{
							"type": "branch",
							"spec": map[string]any{
								"branch": req.Branch,
							},
						},
					},
				},
			},
			"stages": []any{
				map[string]any{
					"stage": map[string]any{
						"name":       "Build and Publish",
						"identifier": "build_and_publish",
						"type":       "CI",
						"spec": map[string]any{
							"cloneCodebase": true,
							"infrastructure": map[string]any{
								"type": "KubernetesDirect",
								"spec": map[string]any{
									"connectorRef":                 req.K8sConnectorRef,
									"namespace":                    req.Namespace,
									"automountServiceAccountToken": true,
									"nodeSelector":                 map[string]string{},
									"os":                           "Linux",
								},
							},
							"execution": map[string]any{
								"steps": steps,
							},
						},
					},
				},
			},
		},
	}
}

func sanitizeIdentifier(value string) string {
	identifier := identifierCleaner.ReplaceAllString(strings.TrimSpace(value), "_")
	identifier = strings.Trim(identifier, "_")
	identifier = strings.ToLower(identifier)
	if identifier == "" {
		return "go_service_ci"
	}
	return identifier
}

func getenv(key string, fallback string) string {
	value := os.Getenv(key)
	if value == "" {
		return fallback
	}
	return value
}

func cors() gin.HandlerFunc {
	return func(c *gin.Context) {
		c.Header("Access-Control-Allow-Origin", "*")
		c.Header("Access-Control-Allow-Methods", "GET,POST,OPTIONS")
		c.Header("Access-Control-Allow-Headers", "Content-Type,Authorization")

		if c.Request.Method == http.MethodOptions {
			c.AbortWithStatus(http.StatusNoContent)
			return
		}

		c.Next()
	}
}
