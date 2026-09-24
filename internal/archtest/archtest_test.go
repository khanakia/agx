// Package archtest enforces agx's package layering (spec §8.1) by parsing
// every non-test Go file's imports with go/parser — never by matching text,
// so multi-line import blocks and aliases are seen exactly as the compiler
// sees them.
package archtest

import (
	"go/parser"
	"go/token"
	"io/fs"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
)

const module = "github.com/khanakia/agx"

// Imports that mark a package as framework-bound.
const (
	cobraPath      = "github.com/spf13/cobra"
	versioncmdPath = "github.com/khanakia/voltkit/versioncmd"
)

// repoRoot is two levels up from internal/archtest.
func repoRoot(t *testing.T) string {
	t.Helper()
	wd, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	return filepath.Clean(filepath.Join(wd, "..", ".."))
}

// packageImports maps each package dir (relative, "." for root) to the set
// of paths its non-test files import.
func packageImports(t *testing.T, root string) map[string]map[string]bool {
	t.Helper()
	out := map[string]map[string]bool{}
	fset := token.NewFileSet()
	err := filepath.WalkDir(root, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() {
			switch d.Name() {
			case ".git", "bin", "testdata", "docsi", "brand":
				return filepath.SkipDir
			}
			return nil
		}
		if !strings.HasSuffix(path, ".go") || strings.HasSuffix(path, "_test.go") {
			return nil
		}
		f, err := parser.ParseFile(fset, path, nil, parser.ImportsOnly)
		if err != nil {
			return err
		}
		rel, err := filepath.Rel(root, filepath.Dir(path))
		if err != nil {
			return err
		}
		if out[rel] == nil {
			out[rel] = map[string]bool{}
		}
		for _, imp := range f.Imports {
			p, err := strconv.Unquote(imp.Path.Value)
			if err != nil {
				return err
			}
			out[rel][p] = true
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	return out
}

func isStdlib(path string) bool {
	first, _, _ := strings.Cut(path, "/")
	return !strings.Contains(first, ".")
}

func TestLayering(t *testing.T) {
	t.Parallel()
	pkgs := packageImports(t, repoRoot(t))
	if len(pkgs) < 8 {
		t.Fatalf("walked only %d packages — the guard is not looking at the repo", len(pkgs))
	}
	for dir, imps := range pkgs {
		for imp := range imps {
			// 1. The domain model imports only the standard library.
			if dir == "provider" && !isStdlib(imp) {
				t.Errorf("provider imports %s; it must stay stdlib-only", imp)
			}
			// 2. Libraries never import internal packages or the framework.
			isLibrary := strings.HasPrefix(dir, "provider") || dir == "config" || dir == "secret" || dir == "pick" || dir == "sessions"
			if isLibrary && (strings.HasPrefix(imp, module+"/internal/") || imp == cobraPath || strings.HasPrefix(imp, "github.com/khanakia/voltkit/")) {
				t.Errorf("library %s imports %s", dir, imp)
			}
			// 3. Only internal/cli touches cobra and voltkit command modules.
			if (imp == cobraPath || imp == versioncmdPath) && dir != filepath.Join("internal", "cli") {
				t.Errorf("%s imports %s; only internal/cli may", dir, imp)
			}
			// 4. main wires the process and nothing else.
			if dir == "." && !isStdlib(imp) && imp != module+"/internal/cli" && imp != module+"/internal/appmeta" {
				t.Errorf("main imports %s; it may only import internal/cli and internal/appmeta", imp)
			}
		}
	}
}
