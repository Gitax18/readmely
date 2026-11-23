package ingest

import (
	"encoding/json"
	"fmt"
	"path"
	"regexp"
	"slices"
	"sort"
	"strings"
)

// Facts is what the project page shows about a repository. Its shape is ProjectFacts in
// contracts/core-public.yaml; it is stored as one JSON document.
type Facts struct {
	Languages       []LanguageStat `json:"languages"`
	Frameworks      []string       `json:"frameworks"`
	PackageManagers []string       `json:"package_managers"`
	EntryPoints     []string       `json:"entry_points"`
	Commands        []Command      `json:"commands"`
	EnvVars         []string       `json:"env_vars"`
	Services        []string       `json:"services"`
	License         *string        `json:"license"`
	HasReadme       bool           `json:"has_readme"`
	HasTests        bool           `json:"has_tests"`
	HasCI           bool           `json:"has_ci"`
	HasDocker       bool           `json:"has_docker"`
	Totals          Totals         `json:"totals"`
	Directories     []DirStat      `json:"directories"`
	Secrets         SecretSummary  `json:"secrets"`
	Skipped         SkippedSummary `json:"skipped"`
}

type LanguageStat struct {
	Name    string  `json:"name"`
	Files   int     `json:"files"`
	Lines   int     `json:"lines"`
	Percent float64 `json:"percent"`
}

type Command struct {
	Name   string `json:"name"`
	Source string `json:"source"`
}

type Totals struct {
	Files       int `json:"files"`
	SourceFiles int `json:"source_files"`
	Lines       int `json:"lines"`
	Bytes       int `json:"bytes"`
}

type DirStat struct {
	Path  string `json:"path"`
	Files int    `json:"files"`
}

type SecretSummary struct {
	Count     int            `json:"count"`
	ByRule    map[string]int `json:"by_rule"`
	Locations []Finding      `json:"locations"`
}

type SkippedSummary struct {
	Count    int            `json:"count"`
	ByReason map[string]int `json:"by_reason"`
}

const (
	maxLocations   = 50
	maxFrameworks  = 24
	maxEnvVars     = 60
	maxEntryPoints = 10
	maxCommands    = 20
	maxDirectories = 15
	maxLanguages   = 8
)

// Languages that describe the project's code. Data and prose are counted in the totals only.
var notCode = map[string]bool{"": true, "Markdown": true, "JSON": true, "YAML": true, "TOML": true, "Text": true}

// deps holds the dependency names the manifests declare, per ecosystem.
type deps struct {
	js, py, goMods map[string]bool
	goModules      map[string]string // directory of a go.mod -> its module path
}

func buildFacts(files []File, parsed map[string]Parsed, findings []Finding) (Facts, deps) {
	byPath := make(map[string]*File, len(files))
	for i := range files {
		byPath[files[i].Path] = &files[i]
	}
	d := deps{js: map[string]bool{}, py: map[string]bool{}, goMods: map[string]bool{}, goModules: map[string]string{}}
	f := Facts{
		Languages: []LanguageStat{}, Frameworks: []string{}, PackageManagers: []string{}, EntryPoints: []string{},
		Commands: []Command{}, EnvVars: []string{}, Services: []string{}, Directories: []DirStat{},
		Secrets: SecretSummary{ByRule: map[string]int{}, Locations: []Finding{}},
		Skipped: SkippedSummary{ByReason: map[string]int{}},
	}
	frameworks := map[string]bool{}
	managers := map[string]bool{}
	envVars := map[string]bool{}

	// Totals, languages, directories, skipped, tests, CI, Docker, README.
	type agg struct{ files, lines int }
	langs := map[string]*agg{}
	dirs := map[string]int{}
	totalCodeLines := 0
	for i := range files {
		fl := &files[i]
		f.Totals.Files++
		f.Totals.Bytes += int(fl.Size)
		top := "."
		if i := strings.IndexByte(fl.Path, '/'); i >= 0 {
			top = fl.Path[:i]
		}
		dirs[top]++
		if fl.Skipped != "" {
			f.Skipped.Count++
			f.Skipped.ByReason[fl.Skipped]++
		} else {
			f.Totals.Lines += fl.Lines
			if !notCode[fl.Language] {
				f.Totals.SourceFiles++
				a := langs[fl.Language]
				if a == nil {
					a = &agg{}
					langs[fl.Language] = a
				}
				a.files++
				a.lines += fl.Lines
				totalCodeLines += fl.Lines
			}
		}
		lower := strings.ToLower(fl.Path)
		base := path.Base(lower)
		switch {
		case isTestPath(lower, base):
			f.HasTests = true
		}
		if strings.HasPrefix(lower, ".github/workflows/") || base == ".gitlab-ci.yml" || strings.HasPrefix(lower, ".circleci/") || base == "jenkinsfile" {
			f.HasCI = true
		}
		if base == "dockerfile" || strings.HasPrefix(base, "dockerfile.") || strings.HasPrefix(base, "docker-compose") || base == "compose.yaml" || base == "compose.yml" {
			f.HasDocker = true
		}
		if fl.Path == "README.md" || fl.Path == "README" || strings.EqualFold(fl.Path, "readme.rst") || strings.EqualFold(fl.Path, "readme.md") {
			f.HasReadme = true
		}
	}
	for name, a := range langs {
		pct := 0.0
		if totalCodeLines > 0 {
			pct = float64(int(float64(a.lines)/float64(totalCodeLines)*1000+0.5)) / 10
		}
		f.Languages = append(f.Languages, LanguageStat{Name: name, Files: a.files, Lines: a.lines, Percent: pct})
	}
	sort.Slice(f.Languages, func(i, j int) bool {
		if f.Languages[i].Lines != f.Languages[j].Lines {
			return f.Languages[i].Lines > f.Languages[j].Lines
		}
		return f.Languages[i].Name < f.Languages[j].Name
	})
	if len(f.Languages) > maxLanguages {
		f.Languages = f.Languages[:maxLanguages]
	}
	for p, n := range dirs {
		f.Directories = append(f.Directories, DirStat{Path: p, Files: n})
	}
	sort.Slice(f.Directories, func(i, j int) bool {
		if f.Directories[i].Files != f.Directories[j].Files {
			return f.Directories[i].Files > f.Directories[j].Files
		}
		return f.Directories[i].Path < f.Directories[j].Path
	})
	if len(f.Directories) > maxDirectories {
		f.Directories = f.Directories[:maxDirectories]
	}

	// Secrets.
	f.Secrets.Count = len(findings)
	for _, fd := range findings {
		f.Secrets.ByRule[fd.Rule]++
	}
	f.Secrets.Locations = append(f.Secrets.Locations, findings[:min(len(findings), maxLocations)]...)

	// Manifests.
	paths := make([]string, 0, len(byPath))
	for p := range byPath {
		paths = append(paths, p)
	}
	sort.Strings(paths)
	for _, p := range paths {
		fl := byPath[p]
		if fl.Skipped != "" || fl.Content == nil {
			continue
		}
		base := path.Base(p)
		dir := path.Dir(p)
		switch {
		case base == "package.json":
			readPackageJSON(p, fl.Content, &f, d, managers, frameworks)
		case base == "go.mod":
			readGoMod(dir, fl.Content, d)
			managers["go"] = true
		case base == "pyproject.toml":
			readPyproject(fl.Content, d)
			managers["pip"] = true
		case strings.HasPrefix(base, "requirements") && strings.HasSuffix(base, ".txt"):
			readRequirements(fl.Content, d)
			managers["pip"] = true
		case base == "Makefile" || base == "makefile":
			readMakefile(p, fl.Content, &f)
		case base == "docker-compose.yml" || base == "docker-compose.yaml" || base == "compose.yml" || base == "compose.yaml":
			f.Services = append(f.Services, composeServices(fl.Content)...)
			frameworks["Docker Compose"] = true
		case base == "Dockerfile" || strings.HasPrefix(base, "Dockerfile."):
			frameworks["Docker"] = true
		case isLicense(p):
			if f.License == nil {
				f.License = detectLicense(fl.Content)
			}
		}
		switch base {
		case "Cargo.toml":
			managers["cargo"] = true
		case "pom.xml":
			managers["maven"] = true
		case "build.gradle", "build.gradle.kts":
			managers["gradle"] = true
		case "Gemfile":
			managers["bundler"] = true
		case "composer.json":
			managers["composer"] = true
		}
	}
	for _, lock := range []struct{ file, name string }{
		{"pnpm-lock.yaml", "pnpm"}, {"yarn.lock", "yarn"}, {"bun.lockb", "bun"}, {"uv.lock", "uv"}, {"poetry.lock", "poetry"},
	} {
		for p := range byPath {
			if path.Base(p) == lock.file {
				managers[lock.name] = true
				delete(managers, "npm")
				break
			}
		}
	}
	if managers["pip"] && (managers["uv"] || managers["poetry"]) {
		delete(managers, "pip")
	}
	for p := range byPath {
		if path.Base(p) == "package-lock.json" && !managers["pnpm"] && !managers["yarn"] && !managers["bun"] {
			managers["npm"] = true
		}
	}
	if !managers["npm"] && !managers["pnpm"] && !managers["yarn"] && !managers["bun"] {
		for p := range byPath {
			if path.Base(p) == "package.json" {
				managers["npm"] = true
				break
			}
		}
	}

	// Frameworks from dependencies.
	for name := range d.js {
		if label, ok := jsFrameworks[name]; ok {
			frameworks[label] = true
		}
	}
	for name := range d.py {
		if label, ok := pyFrameworks[name]; ok {
			frameworks[label] = true
		}
	}
	for mod := range d.goMods {
		for prefix, label := range goFrameworks {
			if mod == prefix || strings.HasPrefix(mod, prefix+"/") {
				frameworks[label] = true
			}
		}
	}
	for p := range byPath {
		if strings.HasPrefix(strings.ToLower(p), ".github/workflows/") {
			frameworks["GitHub Actions"] = true
			break
		}
	}
	for p := range byPath {
		if b := path.Base(p); b == "Makefile" || b == "makefile" {
			frameworks["Make"] = true
			break
		}
	}

	// Entry points.
	for _, p := range paths {
		if pr, ok := parsed[p]; ok && pr.IsMain {
			f.EntryPoints = append(f.EntryPoints, p)
		}
		if fl := byPath[p]; fl.Skipped == "" {
			depth := strings.Count(p, "/")
			switch path.Base(p) {
			case "__main__.py", "main.py", "app.py", "manage.py", "wsgi.py", "asgi.py":
				if depth <= 3 {
					f.EntryPoints = append(f.EntryPoints, p)
				}
			case "index.js", "index.ts", "server.js", "server.ts", "main.ts", "main.js":
				if depth <= 2 && !strings.Contains(p, "node_modules") {
					f.EntryPoints = append(f.EntryPoints, p)
				}
			}
		}
	}
	// Prefer cmd/ and shallow paths when there are many.
	sort.SliceStable(f.EntryPoints, func(i, j int) bool {
		a, b := f.EntryPoints[i], f.EntryPoints[j]
		ca, cb := strings.Contains(a, "/cmd/") || strings.HasPrefix(a, "cmd/"), strings.Contains(b, "/cmd/") || strings.HasPrefix(b, "cmd/")
		if ca != cb {
			return ca
		}
		return strings.Count(a, "/") < strings.Count(b, "/")
	})
	f.EntryPoints = capStrings(dedupe(f.EntryPoints), maxEntryPoints)

	// Environment variables read by the code, and listed in example files.
	for _, p := range paths {
		fl := byPath[p]
		if fl.Skipped != "" || fl.Content == nil {
			continue
		}
		base := strings.ToLower(path.Base(p))
		if strings.HasPrefix(base, ".env.") || base == ".env" {
			for _, m := range envFileKey.FindAllSubmatch(fl.Content, -1) {
				envVars[string(m[1])] = true
			}
			continue
		}
		switch fl.Language {
		case "Go", "Python", "JavaScript", "TypeScript":
			for _, re := range envUse {
				for _, m := range re.FindAllSubmatch(fl.Content, -1) {
					envVars[string(m[1])] = true
				}
			}
		}
	}

	f.Frameworks = capStrings(sortedKeys(frameworks), maxFrameworks)
	f.PackageManagers = sortedKeys(managers)
	f.EnvVars = capStrings(sortedKeys(envVars), maxEnvVars)
	f.Services = dedupe(f.Services)
	if len(f.Commands) > maxCommands {
		f.Commands = f.Commands[:maxCommands]
	}
	return f, d
}

var (
	envFileKey = regexp.MustCompile(`(?m)^\s*(?:export\s+)?([A-Z][A-Z0-9_]{2,})\s*=`)
	envUse     = []*regexp.Regexp{
		regexp.MustCompile(`process\.env\.([A-Z][A-Z0-9_]{2,})`),
		regexp.MustCompile(`process\.env\[['"]([A-Z][A-Z0-9_]{2,})['"]\]`),
		regexp.MustCompile(`os\.Getenv\("([A-Z][A-Z0-9_]{2,})"\)`),
		regexp.MustCompile(`os\.environ(?:\.get)?[\[(]\s*['"]([A-Z][A-Z0-9_]{2,})['"]`),
		regexp.MustCompile(`os\.getenv\(\s*['"]([A-Z][A-Z0-9_]{2,})['"]`),
	}
)

func isTestPath(lower, base string) bool {
	if strings.HasSuffix(base, "_test.go") || strings.HasPrefix(base, "test_") || strings.HasSuffix(base, "_test.py") ||
		strings.Contains(base, ".test.") || strings.Contains(base, ".spec.") {
		return true
	}
	// A root-level test.js, test.ts, tests.py and the like.
	if stem, _, ok := strings.Cut(base, "."); ok && (stem == "test" || stem == "tests") && !strings.Contains(lower, "/") {
		return true
	}
	for _, seg := range strings.Split(lower, "/") {
		if seg == "tests" || seg == "__tests__" || seg == "test" || seg == "spec" {
			return true
		}
	}
	return false
}

func isLicense(p string) bool {
	if strings.Contains(p, "/") {
		return false
	}
	b := strings.ToLower(p)
	return b == "license" || b == "license.md" || b == "license.txt" || b == "copying" || b == "licence"
}

func detectLicense(content []byte) *string {
	head := strings.ToLower(string(content[:min(len(content), 3000)]))
	var id string
	switch {
	case strings.Contains(head, "mit license") || strings.Contains(head, "permission is hereby granted, free of charge"):
		id = "MIT"
	case strings.Contains(head, "apache license") && strings.Contains(head, "version 2.0"):
		id = "Apache-2.0"
	case strings.Contains(head, "gnu affero general public license"):
		id = "AGPL-3.0"
	case strings.Contains(head, "gnu lesser general public license"):
		id = "LGPL"
	case strings.Contains(head, "gnu general public license") && strings.Contains(head, "version 3"):
		id = "GPL-3.0"
	case strings.Contains(head, "gnu general public license") && strings.Contains(head, "version 2"):
		id = "GPL-2.0"
	case strings.Contains(head, "mozilla public license"):
		id = "MPL-2.0"
	case strings.Contains(head, "isc license"):
		id = "ISC"
	case strings.Contains(head, "unlicense"):
		id = "Unlicense"
	case strings.Contains(head, "redistribution and use in source and binary forms"):
		id = "BSD"
	default:
		id = "Other"
	}
	return &id
}

type packageJSON struct {
	Scripts         map[string]string `json:"scripts"`
	Dependencies    map[string]string `json:"dependencies"`
	DevDependencies map[string]string `json:"devDependencies"`
	Main            string            `json:"main"`
}

func readPackageJSON(p string, content []byte, f *Facts, d deps, managers, frameworks map[string]bool) {
	var pj packageJSON
	if json.Unmarshal(content, &pj) != nil {
		return
	}
	for n := range pj.Dependencies {
		d.js[n] = true
	}
	for n := range pj.DevDependencies {
		d.js[n] = true
	}
	if strings.Count(p, "/") > 2 { // deep manifests are fixtures and examples
		return
	}
	prefix := ""
	if dir := path.Dir(p); dir != "." {
		prefix = dir + ": "
	}
	for _, name := range []string{"dev", "start", "build", "test", "lint", "typecheck", "format"} {
		if _, ok := pj.Scripts[name]; ok {
			f.Commands = append(f.Commands, Command{Name: prefix + "npm run " + name, Source: p})
		}
	}
	_ = managers
	_ = frameworks
}

var goRequire = regexp.MustCompile(`(?m)^\s*(?:require\s+)?([A-Za-z0-9._\-]+\.[A-Za-z]{2,}/[A-Za-z0-9._\-/]+)\s+v[0-9]`)
var goModule = regexp.MustCompile(`(?m)^module\s+(\S+)`)

func readGoMod(dir string, content []byte, d deps) {
	if m := goModule.FindSubmatch(content); m != nil {
		d.goModules[dir] = string(m[1])
	}
	for _, m := range goRequire.FindAllSubmatch(content, -1) {
		d.goMods[string(m[1])] = true
	}
}

var pyName = regexp.MustCompile(`(?m)^\s*["']?([A-Za-z0-9][A-Za-z0-9_.\-]*)`)

func normPy(name string) string { return strings.ToLower(strings.ReplaceAll(name, "_", "-")) }

func readRequirements(content []byte, d deps) {
	for _, line := range strings.Split(string(content), "\n") {
		line = strings.TrimSpace(line)
		if line == "" || strings.HasPrefix(line, "#") || strings.HasPrefix(line, "-") {
			continue
		}
		if m := pyName.FindStringSubmatch(line); m != nil {
			d.py[normPy(m[1])] = true
		}
	}
}

var pyDepsBlock = regexp.MustCompile(`(?s)\bdependencies\s*=\s*\[(.*?)\]`)
var pyQuoted = regexp.MustCompile(`["']([A-Za-z0-9][A-Za-z0-9_.\-]*)`)
var poetryDeps = regexp.MustCompile(`(?ms)^\[tool\.poetry(?:\.[a-z\-]+)?\.dependencies\]\s*\n(.*?)(?:^\[|\z)`)

func readPyproject(content []byte, d deps) {
	for _, b := range pyDepsBlock.FindAllSubmatch(content, -1) {
		for _, m := range pyQuoted.FindAllSubmatch(b[1], -1) {
			d.py[normPy(string(m[1]))] = true
		}
	}
	for _, b := range poetryDeps.FindAllSubmatch(content, -1) {
		for _, line := range strings.Split(string(b[1]), "\n") {
			if m := pyName.FindStringSubmatch(line); m != nil {
				d.py[normPy(m[1])] = true
			}
		}
	}
}

var makeTarget = regexp.MustCompile(`(?m)^([a-zA-Z][a-zA-Z0-9_.\-]*):(?:[^=\n]*?##\s*(.*))?$`)

func readMakefile(p string, content []byte, f *Facts) {
	prefix := ""
	if dir := path.Dir(p); dir != "." {
		prefix = dir + ": "
	}
	n := 0
	for _, m := range makeTarget.FindAllSubmatch(content, -1) {
		name := string(m[1])
		if len(m[2]) == 0 && name != "test" && name != "build" && name != "run" && name != "dev" && name != "lint" {
			continue // only documented targets and the usual ones
		}
		f.Commands = append(f.Commands, Command{Name: prefix + "make " + name, Source: p})
		if n++; n >= 10 {
			return
		}
	}
}

var composeTop = regexp.MustCompile(`^[A-Za-z_]`)
var composeService = regexp.MustCompile(`^  ([A-Za-z0-9][A-Za-z0-9_.\-]*):\s*(?:#.*)?$`)

func composeServices(content []byte) []string {
	var out []string
	in := false
	for _, line := range strings.Split(string(content), "\n") {
		switch {
		case strings.HasPrefix(line, "services:"):
			in = true
		case composeTop.MatchString(line):
			in = false
		case in:
			if m := composeService.FindStringSubmatch(line); m != nil {
				out = append(out, m[1])
			}
		}
	}
	return out
}

var jsFrameworks = map[string]string{
	"next": "Next.js", "react": "React", "vue": "Vue", "svelte": "Svelte", "@sveltejs/kit": "SvelteKit", "express": "Express",
	"fastify": "Fastify", "@nestjs/core": "NestJS", "tailwindcss": "Tailwind CSS", "vite": "Vite", "astro": "Astro", "nuxt": "Nuxt",
	"@angular/core": "Angular", "electron": "Electron", "@prisma/client": "Prisma", "drizzle-orm": "Drizzle", "jest": "Jest",
	"vitest": "Vitest", "@playwright/test": "Playwright", "cypress": "Cypress", "typescript": "TypeScript", "zod": "Zod",
	"@reduxjs/toolkit": "Redux Toolkit", "graphql": "GraphQL", "socket.io": "Socket.IO", "mongoose": "Mongoose",
	"react-native": "React Native", "expo": "Expo", "hono": "Hono", "remix": "Remix", "@remix-run/react": "Remix",
}

var pyFrameworks = map[string]string{
	"fastapi": "FastAPI", "flask": "Flask", "django": "Django", "sqlalchemy": "SQLAlchemy", "pydantic": "Pydantic", "pytest": "pytest",
	"celery": "Celery", "uvicorn": "Uvicorn", "numpy": "NumPy", "pandas": "pandas", "torch": "PyTorch", "tensorflow": "TensorFlow",
	"scikit-learn": "scikit-learn", "langchain": "LangChain", "openai": "OpenAI SDK", "anthropic": "Anthropic SDK",
	"streamlit": "Streamlit", "gradio": "Gradio", "httpx": "HTTPX", "alembic": "Alembic", "psycopg": "PostgreSQL (psycopg)",
	"psycopg2": "PostgreSQL (psycopg)", "redis": "Redis", "aiohttp": "aiohttp", "scrapy": "Scrapy", "ruff": "Ruff",
}

var goFrameworks = map[string]string{
	"github.com/go-chi/chi": "Chi", "github.com/gin-gonic/gin": "Gin", "github.com/labstack/echo": "Echo",
	"github.com/gofiber/fiber": "Fiber", "gorm.io/gorm": "GORM", "github.com/jackc/pgx": "PostgreSQL (pgx)",
	"github.com/lib/pq": "PostgreSQL", "github.com/redis/go-redis": "Redis", "github.com/go-redis/redis": "Redis",
	"github.com/spf13/cobra": "Cobra", "google.golang.org/grpc": "gRPC", "github.com/stretchr/testify": "Testify",
	"github.com/aws/aws-sdk-go": "AWS SDK", "go.uber.org/zap": "Zap", "github.com/gorilla/mux": "Gorilla Mux",
}

func sortedKeys(m map[string]bool) []string {
	out := make([]string, 0, len(m))
	for k := range m {
		out = append(out, k)
	}
	sort.Strings(out)
	return out
}

func dedupe(in []string) []string {
	seen := map[string]bool{}
	out := make([]string, 0, len(in))
	for _, s := range in {
		if !seen[s] {
			seen[s] = true
			out = append(out, s)
		}
	}
	return out
}

func capStrings(in []string, n int) []string {
	if len(in) > n {
		return in[:n]
	}
	return in
}

var _ = fmt.Sprintf
var _ = slices.Contains[[]string]
