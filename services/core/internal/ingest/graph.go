package ingest

import (
	"path"
	"sort"
	"strings"
)

// Node kinds and edge limits (kinds match the graph_nodes_kind_check constraint).
const (
	nodeFile     = "file"
	nodePackage  = "package"
	nodeExternal = "external"

	maxNodes = 4000
	maxEdges = 12000
)

// Node is a source file, a package directory, or an external dependency.
type Node struct {
	Kind string
	Key  string
}

// Edge says From imports To.
type Edge struct{ From, To Node }

var jsExts = []string{".ts", ".tsx", ".js", ".jsx", ".mjs", ".cjs"}

var pyStdlib = map[string]bool{}

func init() {
	for _, m := range strings.Fields(`abc argparse array ast asyncio base64 binascii bisect builtins bz2 calendar cgi cmath cmd codecs collections colorsys concurrent configparser contextlib contextvars copy csv ctypes dataclasses datetime decimal difflib dis email enum errno fcntl filecmp fnmatch fractions ftplib functools gc getopt getpass gettext glob gzip hashlib heapq hmac html http imaplib importlib inspect io ipaddress itertools json keyword linecache locale logging lzma math mimetypes multiprocessing numbers operator os pathlib pickle pkgutil platform pprint queue random re readline reprlib resource sched secrets select selectors shelve shlex shutil signal site smtplib socket socketserver sqlite3 ssl stat statistics string struct subprocess sys sysconfig tempfile textwrap threading time timeit tkinter token tokenize traceback types typing unicodedata unittest urllib uuid venv warnings weakref xml zipfile zlib zoneinfo __future__`) {
		pyStdlib[m] = true
	}
}

// buildGraph resolves each source file's imports into edges. Imports that resolve to a file or
// package of the repository become edges between them; the rest become external dependencies,
// except the standard library, which is not interesting and is left out.
func buildGraph(files []File, parsed map[string]Parsed, d deps) []Edge {
	have := make(map[string]bool, len(files))
	dirs := map[string]bool{}
	for _, f := range files {
		have[f.Path] = true
		for dir := path.Dir(f.Path); dir != "." && !dirs[dir]; dir = path.Dir(dir) {
			dirs[dir] = true
		}
	}
	pyIndex := buildPyIndex(files)

	paths := make([]string, 0, len(parsed))
	for p := range parsed {
		paths = append(paths, p)
	}
	sort.Strings(paths)

	var edges []Edge
	seen := map[Edge]bool{}
	add := func(e Edge) {
		if len(edges) < maxEdges && e.From != e.To && !seen[e] {
			seen[e] = true
			edges = append(edges, e)
		}
	}
	langOf := map[string]string{}
	for _, f := range files {
		langOf[f.Path] = f.Language
	}
	for _, p := range paths {
		from := Node{nodeFile, p}
		for _, imp := range parsed[p].Imports {
			var to *Node
			switch langOf[p] {
			case "Go":
				to = resolveGo(imp, dirs, d)
			case "JavaScript", "TypeScript":
				to = resolveJS(p, imp, have)
			case "Python":
				to = resolvePy(p, imp, pyIndex, have)
			}
			if to != nil {
				add(Edge{from, *to})
			}
		}
	}
	return edges
}

func resolveGo(imp string, dirs map[string]bool, d deps) *Node {
	for dir, mod := range d.goModules {
		if imp == mod || strings.HasPrefix(imp, mod+"/") {
			rest := strings.TrimPrefix(strings.TrimPrefix(imp, mod), "/")
			target := path.Join(dir, rest)
			if target == "." {
				target = ""
			}
			if target != "" && dirs[target] {
				return &Node{nodePackage, target}
			}
			return nil
		}
	}
	first, _, _ := strings.Cut(imp, "/")
	if !strings.Contains(first, ".") {
		return nil // standard library
	}
	parts := strings.Split(imp, "/")
	if len(parts) > 3 {
		parts = parts[:3]
	}
	return &Node{nodeExternal, strings.Join(parts, "/")}
}

func resolveJS(from, imp string, have map[string]bool) *Node {
	switch {
	case strings.HasPrefix(imp, "."):
		return jsFile(path.Join(path.Dir(from), imp), have)
	case strings.HasPrefix(imp, "@/") || strings.HasPrefix(imp, "~/"):
		segs := strings.Split(path.Dir(from), "/")
		for i := len(segs); i > 0; i-- {
			if segs[i-1] == "src" {
				return jsFile(path.Join(strings.Join(segs[:i], "/"), imp[2:]), have)
			}
		}
		return nil
	case strings.HasPrefix(imp, "node:"):
		return nil
	}
	name := imp
	if strings.HasPrefix(imp, "@") {
		parts := strings.SplitN(imp, "/", 3)
		if len(parts) >= 2 {
			name = parts[0] + "/" + parts[1]
		}
	} else if i := strings.IndexByte(imp, '/'); i >= 0 {
		name = imp[:i]
	}
	if name == "" {
		return nil
	}
	return &Node{nodeExternal, name}
}

func jsFile(base string, have map[string]bool) *Node {
	if base == "." || base == "" {
		return nil
	}
	cands := []string{base}
	for _, e := range jsExts {
		cands = append(cands, base+e)
	}
	for _, e := range jsExts {
		cands = append(cands, base+"/index"+e)
	}
	for _, c := range cands {
		if have[c] {
			return &Node{nodeFile, c}
		}
	}
	return nil
}

// buildPyIndex maps every dotted suffix of a module's path to the file, or "" when ambiguous, so
// "app.db" finds services/ai/app/db.py wherever the Python root happens to be.
func buildPyIndex(files []File) map[string]string {
	idx := map[string]string{}
	for _, f := range files {
		if f.Language != "Python" {
			continue
		}
		mod := strings.TrimSuffix(f.Path, ".py")
		mod = strings.TrimSuffix(mod, "/__init__")
		segs := strings.Split(mod, "/")
		for i := range segs {
			key := strings.Join(segs[i:], ".")
			if old, ok := idx[key]; ok && old != f.Path {
				idx[key] = ""
			} else {
				idx[key] = f.Path
			}
		}
	}
	return idx
}

func resolvePy(from, imp string, idx map[string]string, have map[string]bool) *Node {
	if strings.HasPrefix(imp, ".") {
		dots := len(imp) - len(strings.TrimLeft(imp, "."))
		base := path.Dir(from)
		for i := 1; i < dots; i++ {
			base = path.Dir(base)
		}
		rest := strings.ReplaceAll(strings.TrimLeft(imp, "."), ".", "/")
		target := path.Join(base, rest)
		for _, c := range []string{target + ".py", target + "/__init__.py"} {
			if have[c] {
				return &Node{nodeFile, c}
			}
		}
		return nil
	}
	parts := strings.Split(imp, ".")
	for n := len(parts); n >= 1; n-- {
		if p := idx[strings.Join(parts[:n], ".")]; p != "" {
			if p == from {
				return nil
			}
			return &Node{nodeFile, p}
		}
	}
	if pyStdlib[parts[0]] || parts[0] == "" {
		return nil
	}
	return &Node{nodeExternal, strings.ToLower(parts[0])}
}
