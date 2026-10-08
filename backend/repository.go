package main

import (
	"archive/tar"
	"compress/gzip"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"path"
	"regexp"
	"sort"
	"strings"

	"github.com/gin-gonic/gin"
)

type repoSignals struct {
	Owner          string   `json:"owner"`
	Name           string   `json:"name"`
	Branch         string   `json:"branch"`
	Language       string   `json:"language"`
	Workdir        string   `json:"workdir"`
	Frameworks     []string `json:"frameworks"`
	HasDockerfile  bool     `json:"hasDockerfile"`
	DockerfilePath string   `json:"dockerfilePath"`
	HasTests       bool     `json:"hasTests"`
	HasLockfile    bool     `json:"hasLockfile"`
	HasBuild       bool     `json:"hasBuild"`
	HasKubernetes  bool     `json:"hasKubernetes"`
	Files          []string `json:"files"`
}

var safeRef = regexp.MustCompile(`^[a-zA-Z0-9][a-zA-Z0-9._/-]{0,150}$`)
var safeRepoPart = regexp.MustCompile(`^[a-zA-Z0-9_.-]{1,100}$`)
var safeWorkdir = regexp.MustCompile(`^[a-zA-Z0-9_./-]+$`)

func githubLocation(raw string) (string, string, error) {
	raw = strings.TrimSpace(raw)
	if strings.HasPrefix(raw, "git@github.com:") {
		raw = "https://github.com/" + strings.TrimPrefix(raw, "git@github.com:")
	}
	parsed, err := url.Parse(raw)
	if err != nil || parsed.Scheme != "https" || !strings.EqualFold(parsed.Host, "github.com") || parsed.User != nil || parsed.RawQuery != "" || parsed.Fragment != "" {
		return "", "", errors.New("use a public github.com repository URL")
	}
	parts := strings.Split(strings.TrimSuffix(strings.Trim(parsed.Path, "/"), ".git"), "/")
	if len(parts) != 2 || !safeRepoPart.MatchString(parts[0]) || !safeRepoPart.MatchString(parts[1]) {
		return "", "", errors.New("repository URL must be github.com/owner/repo")
	}
	return parts[0], parts[1], nil
}

func (a *application) scanRepository(repoURL, branch string) (repoSignals, error) {
	owner, name, err := githubLocation(repoURL)
	if err != nil {
		return repoSignals{}, err
	}
	if branch == "" {
		branch = "main"
	}
	if !safeRef.MatchString(branch) || strings.Contains(branch, "..") || strings.Contains(branch, "//") {
		return repoSignals{}, errors.New("invalid branch")
	}
	endpoint := "https://codeload.github.com/" + url.PathEscape(owner) + "/" + url.PathEscape(name) + "/tar.gz/refs/heads/" + url.PathEscape(branch)
	req, err := http.NewRequest(http.MethodGet, endpoint, nil)
	if err != nil {
		return repoSignals{}, err
	}
	response, err := a.httpClient.Do(req)
	if err != nil {
		return repoSignals{}, fmt.Errorf("GitHub archive unavailable: %w", err)
	}
	defer response.Body.Close()
	if response.StatusCode != 200 {
		return repoSignals{}, fmt.Errorf("GitHub archive returned %d; check that the repository and branch are public", response.StatusCode)
	}
	gzipReader, err := gzip.NewReader(io.LimitReader(response.Body, 10<<20))
	if err != nil {
		return repoSignals{}, errors.New("invalid GitHub archive")
	}
	defer gzipReader.Close()
	tarReader := tar.NewReader(gzipReader)
	files := map[string]bool{}
	manifests := map[string][]byte{}
	var total int64
	for {
		header, err := tarReader.Next()
		if err == io.EOF {
			break
		}
		if err != nil {
			return repoSignals{}, errors.New("GitHub archive is incomplete or too large")
		}
		if header.Typeflag != tar.TypeReg && header.Typeflag != tar.TypeRegA {
			continue
		}
		if header.Size < 0 || header.Size > 40<<20 || total+header.Size > 40<<20 || len(files) >= 3000 {
			return repoSignals{}, errors.New("repository is too large for a bounded scan")
		}
		total += header.Size
		_, relative, ok := strings.Cut(header.Name, "/")
		if !ok || relative == "" || path.IsAbs(relative) || strings.HasPrefix(relative, "../") || strings.Contains(relative, "/../") {
			continue
		}
		relative = path.Clean(relative)
		files[relative] = true
		base := path.Base(relative)
		if (base == "go.mod" || base == "package.json") && header.Size <= 512<<10 {
			content, err := io.ReadAll(io.LimitReader(tarReader, 512<<10))
			if err == nil {
				manifests[relative] = content
			}
		}
	}
	return deriveSignals(owner, name, branch, files, manifests)
}

func deriveSignals(owner, name, branch string, files map[string]bool, manifests map[string][]byte) (repoSignals, error) {
	signals := repoSignals{Owner: owner, Name: name, Branch: branch, Frameworks: []string{}, Files: []string{}}
	goDirs := map[string]int{}
	nodeDirs := map[string]int{}
	for file := range files {
		base, dir := path.Base(file), path.Dir(file)
		if base == "go.mod" {
			goDirs[dir] = 3
		}
		if base == "package.json" {
			nodeDirs[dir] = 3
		}
		if strings.Contains(strings.ToLower(file), "k8s/") || strings.Contains(strings.ToLower(file), "kubernetes/") {
			signals.HasKubernetes = true
		}
	}
	workdir := bestDir(goDirs)
	if workdir != "" {
		signals.Language = "go"
	} else if len(nodeDirs) > 0 {
		workdir = bestDir(nodeDirs)
		signals.Language = "node"
	}
	if signals.Language == "" {
		return repoSignals{}, errors.New("no go.mod or package.json found; this version supports Go and Node projects")
	}
	if workdir == "." {
		workdir = ""
	}
	if workdir != "" && (!safeWorkdir.MatchString(workdir) || strings.Contains(workdir, "..")) {
		return repoSignals{}, errors.New("repository directory contains unsupported characters")
	}
	signals.Workdir = workdir
	for file := range files {
		if workdir != "" && !strings.HasPrefix(file, workdir+"/") {
			continue
		}
		base := path.Base(file)
		if strings.HasSuffix(base, "_test.go") || strings.HasSuffix(base, ".test.ts") || strings.HasSuffix(base, ".test.tsx") || strings.HasSuffix(base, ".spec.ts") {
			signals.HasTests = true
		}
	}
	signals.HasLockfile = files[path.Join(workdir, "package-lock.json")] || files[path.Join(workdir, "pnpm-lock.yaml")] || files[path.Join(workdir, "yarn.lock")]
	if files[path.Join(workdir, "Dockerfile")] {
		signals.HasDockerfile = true
		signals.DockerfilePath = path.Join(workdir, "Dockerfile")
	} else if files["Dockerfile"] {
		signals.HasDockerfile = true
		signals.DockerfilePath = "Dockerfile"
	}
	for _, filename := range []string{"go.mod", "package.json", "Dockerfile", "package-lock.json", "pnpm-lock.yaml", "yarn.lock"} {
		file := path.Join(workdir, filename)
		if files[file] {
			signals.Files = append(signals.Files, file)
		}
	}
	sort.Strings(signals.Files)
	if signals.Language == "go" {
		content := strings.ToLower(string(manifests[path.Join(workdir, "go.mod")]))
		for _, framework := range []string{"gin-gonic/gin", "gofiber/fiber", "labstack/echo"} {
			if strings.Contains(content, framework) {
				signals.Frameworks = append(signals.Frameworks, framework)
			}
		}
	} else {
		var pkg struct {
			Dependencies    map[string]any `json:"dependencies"`
			DevDependencies map[string]any `json:"devDependencies"`
			Scripts         map[string]any `json:"scripts"`
		}
		if json.Unmarshal(manifests[path.Join(workdir, "package.json")], &pkg) == nil {
			for _, framework := range []string{"react", "next", "vite", "vue", "express", "@nestjs/core"} {
				if pkg.Dependencies[framework] != nil || pkg.DevDependencies[framework] != nil {
					signals.Frameworks = append(signals.Frameworks, framework)
				}
			}
			signals.HasTests = pkg.Scripts["test"] != nil || signals.HasTests
			signals.HasBuild = pkg.Scripts["build"] != nil
		}
	}
	return signals, nil
}

func bestDir(dirs map[string]int) string {
	best := ""
	for dir := range dirs {
		if best == "" || len(dir) < len(best) || (len(dir) == len(best) && dir < best) {
			best = dir
		}
	}
	return best
}

func analysisForSignals(s repoSignals) ([]string, []string) {
	directory := s.Workdir
	if directory == "" {
		directory = "repository root"
	}
	analysis := []string{
		"Scanned public GitHub repository " + s.Owner + "/" + s.Name + " at branch " + s.Branch + ".",
		"Detected " + strings.ToUpper(s.Language) + " project in " + directory + ".",
	}
	if len(s.Frameworks) > 0 {
		analysis = append(analysis, "Detected dependencies: "+strings.Join(s.Frameworks, ", ")+".")
	}
	if s.HasDockerfile {
		analysis = append(analysis, "Dockerfile exists in the repository.")
	}
	if s.HasTests {
		analysis = append(analysis, "Test files or a test script were found.")
	}
	recommendations := []string{"Run tests before building and keep credentials in Harness connectors or secrets."}
	if !s.HasTests {
		recommendations = append(recommendations, "No test suite was detected; add tests before relying on CI checks.")
	}
	if !s.HasDockerfile {
		recommendations = append(recommendations, "Add a Dockerfile before enabling image publishing.")
	}
	if s.HasKubernetes {
		recommendations = append(recommendations, "Kubernetes manifests were found; review deployment separately from CI.")
	}
	return analysis, recommendations
}

func (a *application) analyzeLive(c *gin.Context) {
	var req projectAnalyzeRequest
	if c.ShouldBindJSON(&req) != nil {
		c.JSON(400, gin.H{"error": "repository URL required"})
		return
	}
	signals, err := a.scanRepository(req.RepoURL, req.Branch)
	if err != nil {
		c.JSON(422, gin.H{"error": err.Error()})
		return
	}
	defaults := buildDefaultsFromRepository(req)
	directory := signals.Workdir
	if directory == "" {
		directory = "repository root"
	}
	defaults.ProjectDescription = strings.ToUpper(signals.Language) + " project in " + directory
	analysis, recommendations := analysisForSignals(signals)
	llmNotes, source := a.llmRecommendations(signals)
	recommendations = append(recommendations, llmNotes...)
	c.JSON(200, gin.H{"defaults": defaults, "signals": signals, "analysis": analysis, "recommendations": recommendations, "missingSetup": setupForSignals(defaults, signals), "mode": defaults.Mode, "analysisSource": source})
}
