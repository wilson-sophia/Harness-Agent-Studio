package main

import (
	"net/http"
	"os"
	"regexp"
	"strings"

	"github.com/gin-gonic/gin"
)

type pipelineRequest struct {
	ServiceName          string `json:"serviceName" binding:"required"`
	RepoURL              string `json:"repoUrl" binding:"required"`
	ProjectDescription   string `json:"projectDescription"`
	Branch               string `json:"branch"`
	ImageName            string `json:"imageName" binding:"required"`
	Mode                 string `json:"mode"`
	OrgIdentifier        string `json:"orgIdentifier"`
	ProjectIdentifier    string `json:"projectIdentifier"`
	CodebaseConnectorRef string `json:"codebaseConnectorRef"`
	DockerConnectorRef   string `json:"dockerConnectorRef"`
	IncludeDeploy        bool   `json:"includeDeploy"`
	K8sConnectorRef      string `json:"k8sConnectorRef"`
	Namespace            string `json:"namespace"`
}

type projectAnalyzeRequest struct {
	RepoURL string `json:"repoUrl" binding:"required"`
	Branch  string `json:"branch"`
	Mode    string `json:"mode"`
}

type pipelineNode struct {
	ID          string `json:"id"`
	Label       string `json:"label"`
	Kind        string `json:"kind"`
	Description string `json:"description"`
	Command     string `json:"command,omitempty"`
}

type setupItem struct {
	ID          string `json:"id"`
	Label       string `json:"label"`
	Description string `json:"description"`
	RequiredFor string `json:"requiredFor"`
	Status      string `json:"status"`
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

var identifierCleaner = regexp.MustCompile(`[^a-zA-Z0-9_]+`)
var imagePartCleaner = regexp.MustCompile(`[^a-z0-9._-]+`)
var serviceCleaner = regexp.MustCompile(`[^a-z0-9-]+`)

func main() {
	app, err := newApplication()
	if err != nil {
		panic(err)
	}
	router := gin.Default()
	if err = router.SetTrustedProxies(nil); err != nil {
		panic(err)
	}
	router.Use(app.securityHeaders(), app.sameOrigin())
	router.GET("/api/health", func(c *gin.Context) { c.JSON(http.StatusOK, gin.H{"status": "ok"}) })
	app.routes(router)
	if err = router.Run(getenv("LISTEN_ADDR", "127.0.0.1:8080")); err != nil {
		panic(err)
	}
}

func buildDefaultsFromRepository(req projectAnalyzeRequest) pipelineRequest {
	owner, name, _ := githubLocation(req.RepoURL)
	service := serviceNameFromRepo(name)
	branch := strings.TrimSpace(req.Branch)
	if branch == "" {
		branch = "main"
	}
	return pipelineRequest{
		ServiceName: service, RepoURL: strings.TrimSpace(req.RepoURL), Branch: branch,
		ImageName: imageNameFromRepo(owner, service), Mode: normalizeMode(req.Mode),
		Namespace: "default",
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

func normalizeRequest(req *pipelineRequest) {
	req.ServiceName = strings.TrimSpace(req.ServiceName)
	req.RepoURL = strings.TrimSpace(req.RepoURL)
	req.ImageName = strings.TrimSpace(req.ImageName)
	req.Branch = strings.TrimSpace(req.Branch)
	if req.Branch == "" {
		req.Branch = "main"
	}
	req.Mode = normalizeMode(req.Mode)
	req.OrgIdentifier = strings.TrimSpace(req.OrgIdentifier)
	req.ProjectIdentifier = strings.TrimSpace(req.ProjectIdentifier)
	req.CodebaseConnectorRef = strings.TrimSpace(req.CodebaseConnectorRef)
	req.DockerConnectorRef = strings.TrimSpace(req.DockerConnectorRef)
	req.K8sConnectorRef = strings.TrimSpace(req.K8sConnectorRef)
	req.Namespace = strings.TrimSpace(req.Namespace)
	if req.Namespace == "" {
		req.Namespace = "default"
	}
}

func serviceNameFromRepo(name string) string {
	value := strings.ToLower(strings.TrimSpace(name))
	value = serviceCleaner.ReplaceAllString(value, "-")
	value = strings.Trim(value, "-")
	if value == "" {
		return "service"
	}
	return value
}

func imageNameFromRepo(owner, name string) string {
	owner = sanitizeImagePart(owner)
	name = sanitizeImagePart(name)
	if owner == "" {
		return name
	}
	return owner + "/" + name
}

func sanitizeImagePart(value string) string {
	value = strings.ToLower(strings.TrimSpace(value))
	value = imagePartCleaner.ReplaceAllString(value, "-")
	return strings.Trim(value, "-._")
}

func sanitizeIdentifier(value string) string {
	value = identifierCleaner.ReplaceAllString(strings.TrimSpace(value), "_")
	value = strings.ToLower(strings.Trim(value, "_"))
	if value == "" {
		return "service_ci"
	}
	if value[0] >= '0' && value[0] <= '9' {
		value = "p_" + value
	}
	if len(value) > 128 {
		value = value[:128]
	}
	return value
}

func getenv(key, fallback string) string {
	if value := os.Getenv(key); value != "" {
		return value
	}
	return fallback
}
