package agent

import (
	"go/ast"
	"go/parser"
	"go/token"
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"testing"
)

// i18nTables reads the GUI's i18n.js: each language's table, by its name,
// and the NOTICES list tNotice matches notices against.
func i18nTables(t *testing.T) (map[string]map[string]string, []string) {
	t.Helper()
	b, err := os.ReadFile(filepath.Join("..", "gui", "assets", "i18n.js"))
	if err != nil {
		t.Fatal(err)
	}
	src := string(b)
	lang := regexp.MustCompile(`^  (\w+|"[\w-]+"): \{$`)
	entry := regexp.MustCompile(`^    ("(?:[^"\\]|\\.)*"): ("(?:[^"\\]|\\.)*"),?$`)
	unq := func(s string) string {
		u, err := strconv.Unquote(s)
		if err != nil {
			t.Fatalf("%s: %v", s, err)
		}
		return u
	}
	tables := map[string]map[string]string{}
	var cur map[string]string
	inTables := false
	for _, line := range strings.Split(src, "\n") {
		switch {
		case line == "const I18N = {":
			inTables = true
		case inTables && line == "};":
			inTables = false
		case inTables && lang.MatchString(line):
			name := strings.Trim(lang.FindStringSubmatch(line)[1], `"`)
			cur = map[string]string{}
			tables[name] = cur
		case inTables && cur != nil:
			if m := entry.FindStringSubmatch(line); m != nil {
				cur[unq(m[1])] = unq(m[2])
			}
		}
	}
	start := strings.Index(src, "\nconst NOTICES = [\n")
	if start < 0 {
		t.Fatal("i18n.js has no NOTICES list")
	}
	var list []string
	for _, line := range strings.Split(src[start+len("\nconst NOTICES = [\n"):], "\n") {
		if line == "];" {
			break
		}
		list = append(list, unq(strings.TrimSuffix(strings.TrimSpace(line), ",")))
	}
	return tables, list
}

// A notice is written as a newNotice template, never as English in the
// code that gives it, so a new agent's advice can't miss the translation
// TestEveryNoticeIsTranslated asks for.
func TestNoticesAreTemplates(t *testing.T) {
	files, _ := filepath.Glob("*.go")
	prose := regexp.MustCompile(`[A-Za-z']+ [A-Za-z']+ [A-Za-z']+`)
	fset := token.NewFileSet()
	checked := 0
	for _, name := range files {
		if strings.HasSuffix(name, "_test.go") || name == "notice.go" {
			continue
		}
		f, err := parser.ParseFile(fset, name, nil, 0)
		if err != nil {
			t.Fatal(err)
		}
		check := func(n ast.Node) {
			checked++
			ast.Inspect(n, func(n ast.Node) bool {
				// newNotice is where a template is written
				if c, ok := n.(*ast.CallExpr); ok {
					if id, ok := c.Fun.(*ast.Ident); ok && id.Name == "newNotice" {
						return false
					}
				}
				if l, ok := n.(*ast.BasicLit); ok && l.Kind == token.STRING && prose.MatchString(l.Value) {
					t.Errorf("%s: a notice written in place, not as a newNotice: %s", fset.Position(l.Pos()), l.Value)
				}
				return true
			})
		}
		ast.Inspect(f, func(n ast.Node) bool {
			switch n := n.(type) {
			case *ast.KeyValueExpr:
				if k, ok := n.Key.(*ast.Ident); ok && k.Name == "Notice" {
					check(n.Value)
				}
			case *ast.AssignStmt:
				for i, l := range n.Lhs {
					if s, ok := l.(*ast.SelectorExpr); ok && s.Sel.Name == "Notice" && i < len(n.Rhs) {
						check(n.Rhs[i])
					}
				}
			case *ast.FuncDecl:
				// the helpers a Notice says through
				if strings.Contains(strings.ToLower(n.Name.Name), "notice") || n.Name.Name == "notMirrored" {
					check(n)
				}
			}
			return true
		})
	}
	if checked < 12 {
		t.Fatalf("checked only %d notices", checked)
	}
}
