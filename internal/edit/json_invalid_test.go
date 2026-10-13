package edit

import (
	"os"
	"testing"
)

func TestJSONItemEditsPreserveInvalidFiles(t *testing.T) {
	where := map[string]string{"name": "fogway"}
	for _, input := range []string{
		`[{"name":"fogway"}, {"name":]`,
		`[{"name":"fogway"} {"name":"keep"}]`,
		`[{"name":"fogway"}] [{"name":"keep"}]`,
	} {
		for name, edit := range map[string]func(string) error{
			"set": func(p string) error {
				return SetJSONItem(p, where, map[string]string{"name": "fogway", "model": "new"})
			},
			"delete": func(p string) error { return DelJSONItem(p, where) },
		} {
			t.Run(name+"/"+input, func(t *testing.T) {
				p := tmpFile(t, "chatLanguageModels.json", input)
				if err := edit(p); err == nil {
					t.Error("editing invalid JSON array succeeded")
				}
				if got, err := os.ReadFile(p); err != nil || string(got) != input {
					t.Errorf("invalid config was changed: %q, %v", got, err)
				}
			})
		}
	}
}
