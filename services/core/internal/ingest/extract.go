package ingest

import (
	"archive/tar"
	"bytes"
	"compress/gzip"
	"errors"
	"fmt"
	"io"
	"path"
	"strings"
	"unicode/utf8"
)

// Limits bound what one archive can make the worker do. They are constants, not settings:
// changing one is a security decision.
const (
	maxEntries      = 100_000
	maxUncompressed = 600 << 20
	maxFileBytes    = 256 << 10
	maxContentTotal = 25 << 20
	maxStoredFiles  = 20_000
	maxPathLen      = 400
)

// ErrTooLarge means the archive is bigger than the limits allow.
var ErrTooLarge = errors.New("repository is too large to analyse")

// Reasons a file's content is not kept (they match the files_skipped_reason_check constraint).
const (
	skipBinary    = "binary"
	skipTooLarge  = "too_large"
	skipSensitive = "sensitive"
	skipGenerated = "generated"
	skipVendored  = "vendored"
	skipBudget    = "budget"
)

// File is one file of the repository. Content is nil when Skipped is set.
type File struct {
	Path     string
	Size     int64
	Language string
	Lines    int
	Content  []byte
	Skipped  string
}

// ExtractStats says what the archive held, including what was not kept.
type ExtractStats struct {
	Entries      int
	Uncompressed int64
	Dropped      int // regular files beyond maxStoredFiles
	UnsafePaths  int
}

// Extract reads a gzipped tarball into memory. Every entry is untrusted: only regular files are
// read, never written anywhere, so a link or a hostile path cannot touch the file system.
func Extract(r io.Reader) ([]File, ExtractStats, error) {
	var stats ExtractStats
	gz, err := gzip.NewReader(r)
	if err != nil {
		return nil, stats, fmt.Errorf("archive is not gzip: %w", err)
	}
	defer func() { _ = gz.Close() }()
	tr := tar.NewReader(gz)

	var files []File
	contentTotal := 0
	for {
		hdr, err := tr.Next()
		if errors.Is(err, io.EOF) {
			break
		}
		if err != nil {
			return nil, stats, fmt.Errorf("archive is damaged: %w", err)
		}
		stats.Entries++
		if stats.Entries > maxEntries {
			return nil, stats, ErrTooLarge
		}
		if hdr.Typeflag != tar.TypeReg {
			continue // directories, links, devices and the global header: nothing to read
		}
		stats.Uncompressed += hdr.Size
		if hdr.Size < 0 || stats.Uncompressed > maxUncompressed {
			return nil, stats, ErrTooLarge
		}
		rel, ok := relativePath(hdr.Name)
		if !ok {
			stats.UnsafePaths++
			continue
		}
		if len(files) >= maxStoredFiles {
			stats.Dropped++
			continue
		}
		f := File{Path: rel, Size: hdr.Size, Language: languageOf(rel)}
		switch reason := skipReason(rel); {
		case reason != "":
			f.Skipped = reason
		case hdr.Size > maxFileBytes:
			f.Skipped = skipTooLarge
		default:
			data, err := io.ReadAll(io.LimitReader(tr, hdr.Size))
			if err != nil {
				return nil, stats, fmt.Errorf("archive is damaged: %w", err)
			}
			switch {
			case looksBinary(data):
				f.Skipped = skipBinary
			case contentTotal+len(data) > maxContentTotal:
				f.Skipped = skipBudget
			default:
				f.Content = data
				f.Lines = countLines(data)
				contentTotal += len(data)
			}
		}
		files = append(files, f)
	}
	return files, stats, nil
}

// relativePath drops the archive's single top directory and refuses anything that is not a plain
// relative path: empty parts, dot parts, backslashes, control characters, or too long.
func relativePath(name string) (string, bool) {
	_, rel, found := strings.Cut(name, "/")
	if !found || rel == "" || len(rel) > maxPathLen || strings.HasPrefix(rel, "/") {
		return "", false
	}
	if strings.ContainsRune(rel, '\\') || !utf8.ValidString(rel) {
		return "", false
	}
	for _, r := range rel {
		if r < ' ' || r == 0x7f {
			return "", false
		}
	}
	if path.Clean(rel) != rel {
		return "", false
	}
	for _, seg := range strings.Split(rel, "/") {
		if seg == "" || seg == "." || seg == ".." {
			return "", false
		}
	}
	return rel, true
}

func looksBinary(data []byte) bool {
	head := data
	if len(head) > 8000 {
		head = head[:8000]
	}
	return bytes.IndexByte(head, 0) >= 0 || !utf8.Valid(data)
}

func countLines(data []byte) int {
	if len(data) == 0 {
		return 0
	}
	n := bytes.Count(data, []byte{'\n'})
	if data[len(data)-1] != '\n' {
		n++
	}
	return n
}

var vendoredDirs = map[string]bool{
	"node_modules": true, "vendor": true, "dist": true, "build": true, ".next": true, "target": true,
	"__pycache__": true, ".venv": true, "venv": true, ".git": true, "bower_components": true,
	".gradle": true, "coverage": true, ".idea": true, ".vscode": true, ".terraform": true, "out": true,
}

var generatedNames = map[string]bool{
	"package-lock.json": true, "pnpm-lock.yaml": true, "yarn.lock": true, "go.sum": true,
	"poetry.lock": true, "cargo.lock": true, "uv.lock": true, "composer.lock": true, "gemfile.lock": true,
	"bun.lockb": true, ".ds_store": true,
}

var binaryExt = map[string]bool{
	".png": true, ".jpg": true, ".jpeg": true, ".gif": true, ".webp": true, ".ico": true, ".bmp": true, ".tiff": true,
	".pdf": true, ".zip": true, ".gz": true, ".tgz": true, ".tar": true, ".7z": true, ".rar": true, ".bz2": true, ".xz": true,
	".jar": true, ".war": true, ".class": true, ".so": true, ".dylib": true, ".dll": true, ".exe": true, ".a": true, ".o": true,
	".woff": true, ".woff2": true, ".ttf": true, ".otf": true, ".eot": true,
	".mp3": true, ".mp4": true, ".mov": true, ".avi": true, ".wav": true, ".ogg": true, ".webm": true,
	".wasm": true, ".bin": true, ".dat": true, ".db": true, ".sqlite": true, ".pyc": true, ".psd": true,
}

// skipReason says why a file is not read at all, judged from its path alone ("" means read it).
func skipReason(rel string) string {
	segs := strings.Split(rel, "/")
	for _, s := range segs[:len(segs)-1] {
		if vendoredDirs[s] {
			return skipVendored
		}
	}
	base := segs[len(segs)-1]
	lower := strings.ToLower(base)
	if sensitiveName(lower) {
		return skipSensitive
	}
	if generatedNames[lower] || strings.HasSuffix(lower, ".min.js") || strings.HasSuffix(lower, ".min.css") ||
		strings.HasSuffix(lower, ".map") || strings.HasSuffix(lower, ".pb.go") || strings.HasSuffix(lower, "_generated.go") ||
		strings.HasSuffix(lower, ".gen.go") || strings.HasSuffix(lower, ".snap") {
		return skipGenerated
	}
	if binaryExt[strings.ToLower(path.Ext(base))] {
		return skipBinary
	}
	return ""
}

// sensitiveName is true for files that usually hold credentials. They are never read: their
// existence is reported, their content is not kept.
func sensitiveName(lower string) bool {
	switch lower {
	case ".env", "id_rsa", "id_dsa", "id_ecdsa", "id_ed25519", "credentials", "credentials.json", "secrets.json",
		"secrets.yml", "secrets.yaml", ".npmrc", ".pypirc", ".netrc", "terraform.tfvars", ".htpasswd":
		return true
	}
	if strings.HasPrefix(lower, ".env.") {
		switch strings.TrimPrefix(lower, ".env.") {
		case "example", "sample", "template", "dist", "defaults":
			return false
		}
		return true
	}
	for _, ext := range []string{".pem", ".key", ".p12", ".pfx", ".jks", ".keystore", ".tfstate", ".kdbx"} {
		if strings.HasSuffix(lower, ext) {
			return true
		}
	}
	return strings.HasPrefix(lower, "service-account") && strings.HasSuffix(lower, ".json")
}

var languageByExt = map[string]string{
	".go": "Go", ".py": "Python", ".js": "JavaScript", ".jsx": "JavaScript", ".mjs": "JavaScript", ".cjs": "JavaScript",
	".ts": "TypeScript", ".tsx": "TypeScript", ".java": "Java", ".kt": "Kotlin", ".rs": "Rust", ".rb": "Ruby",
	".php": "PHP", ".cs": "C#", ".c": "C", ".h": "C", ".cpp": "C++", ".cc": "C++", ".hpp": "C++", ".swift": "Swift",
	".sh": "Shell", ".bash": "Shell", ".sql": "SQL", ".html": "HTML", ".css": "CSS", ".scss": "CSS", ".vue": "Vue",
	".svelte": "Svelte", ".dart": "Dart", ".scala": "Scala", ".lua": "Lua", ".r": "R", ".ex": "Elixir", ".exs": "Elixir",
	".md": "Markdown", ".mdx": "Markdown", ".json": "JSON", ".yaml": "YAML", ".yml": "YAML", ".toml": "TOML", ".txt": "Text",
}

func languageOf(rel string) string {
	base := path.Base(rel)
	if strings.EqualFold(base, "Dockerfile") || strings.HasPrefix(base, "Dockerfile.") {
		return "Docker"
	}
	if base == "Makefile" || base == "makefile" {
		return "Make"
	}
	return languageByExt[strings.ToLower(path.Ext(base))]
}
