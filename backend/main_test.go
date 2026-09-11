package main

import (
	"strings"
	"testing"
)

func TestAnalyzeProjectReflectsDeployChoice(t *testing.T) {
	req := pipelineRequest{
		ServiceName:   "go-shortlink",
		RepoURL:       "https://github.com/wilson-sophia/go-shortlink",
		Branch:        "main",
		ImageName:     "wilson-sophia/go-shortlink",
		IncludeDeploy: true,
	}

	analysis, recommendations := analyzeProject(req)

	if !containsText(analysis, "GitHub") {
		t.Fatalf("expected GitHub analysis, got %v", analysis)
	}

	if !containsText(analysis, "Deployment placeholder is enabled") {
		t.Fatalf("expected deploy analysis, got %v", analysis)
	}

	if !containsText(recommendations, "Replace the deploy placeholder") {
		t.Fatalf("expected deploy recommendation, got %v", recommendations)
	}
}

func TestAnalyzeProjectUsesProjectDescription(t *testing.T) {
	req := pipelineRequest{
		ServiceName:        "go-shortlink",
		RepoURL:            "https://github.com/wilson-sophia/go-shortlink",
		ProjectDescription: "Go backend using Gin, PostgreSQL, Redis, Dockerfile, and Kubernetes.",
		Branch:             "main",
		ImageName:          "wilson-sophia/go-shortlink",
		IncludeDeploy:      false,
	}

	analysis, recommendations := analyzeProject(req)

	for _, want := range []string{"Gin", "PostgreSQL", "Redis", "Dockerfile", "Kubernetes"} {
		if !containsText(analysis, want) {
			t.Fatalf("expected %q analysis, got %v", want, analysis)
		}
	}

	if !containsText(recommendations, "Harness secrets") {
		t.Fatalf("expected secrets recommendation, got %v", recommendations)
	}

	if !containsText(recommendations, "Kubernetes connector") {
		t.Fatalf("expected Kubernetes deploy recommendation, got %v", recommendations)
	}
}

func TestBuildDefaultsFromRepository(t *testing.T) {
	defaults := buildDefaultsFromRepository(projectAnalyzeRequest{
		RepoURL: "https://github.com/wilson-sophia/go-shortlink.git",
		Branch:  "develop",
	})

	if defaults.ServiceName != "go-shortlink" {
		t.Fatalf("ServiceName = %q, want go-shortlink", defaults.ServiceName)
	}

	if defaults.ImageName != "wilson-sophia/go-shortlink" {
		t.Fatalf("ImageName = %q, want wilson-sophia/go-shortlink", defaults.ImageName)
	}

	if defaults.Branch != "develop" {
		t.Fatalf("Branch = %q, want develop", defaults.Branch)
	}

	if !strings.Contains(defaults.ProjectDescription, "Go backend inferred") {
		t.Fatalf("expected inferred project description, got %q", defaults.ProjectDescription)
	}
}

func TestParseSSHRepositoryURL(t *testing.T) {
	metadata := parseRepositoryURL("git@github.com:wilson-sophia/go-shortlink.git")

	if metadata.Provider != "GitHub" {
		t.Fatalf("Provider = %q, want GitHub", metadata.Provider)
	}

	if metadata.Owner != "wilson-sophia" {
		t.Fatalf("Owner = %q, want wilson-sophia", metadata.Owner)
	}

	if metadata.Name != "go-shortlink" {
		t.Fatalf("Name = %q, want go-shortlink", metadata.Name)
	}
}

func TestBuildMissingSetupForPreviewMode(t *testing.T) {
	items := buildMissingSetup(pipelineRequest{Mode: "preview"})

	if !hasSetupItem(items, "harness-token") {
		t.Fatalf("expected Harness token setup item, got %v", items)
	}

	if !hasSetupItem(items, "docker-secret") {
		t.Fatalf("expected Docker secret setup item, got %v", items)
	}
}

func TestBuildMissingSetupForDemoMode(t *testing.T) {
	items := buildMissingSetup(pipelineRequest{Mode: "demo"})

	if !hasSetupItem(items, "demo-sandbox") {
		t.Fatalf("expected demo sandbox setup item, got %v", items)
	}

	if !hasSetupItem(items, "demo-whitelist") {
		t.Fatalf("expected demo whitelist setup item, got %v", items)
	}
}

func TestBuildMissingSetupForConnectedMode(t *testing.T) {
	items := buildMissingSetup(pipelineRequest{
		Mode:             "connected",
		HarnessAccountID: "abc",
		HasHarnessToken:  true,
	})

	if hasSetupItem(items, "harness-token") {
		t.Fatalf("did not expect token setup item when token is present, got %v", items)
	}

	if !hasSetupItem(items, "resource-discovery") {
		t.Fatalf("expected resource discovery next step, got %v", items)
	}
}

func TestSanitizeIdentifier(t *testing.T) {
	got := sanitizeIdentifier("Go Shortlink CI!")
	want := "go_shortlink_ci"

	if got != want {
		t.Fatalf("sanitizeIdentifier() = %q, want %q", got, want)
	}
}

func containsText(items []string, needle string) bool {
	for _, item := range items {
		if strings.Contains(item, needle) {
			return true
		}
	}

	return false
}

func hasSetupItem(items []setupItem, id string) bool {
	for _, item := range items {
		if item.ID == id {
			return true
		}
	}

	return false
}
