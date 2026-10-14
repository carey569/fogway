package agent

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/carey569/fogway/internal/provider"
)

// Every model iFlytek Spark's plans offer shows its maker's logo: Spark's
// own, or the maker of one the Token Plan serves under an id of its own
// (xopglm52 is GLM-5.2), and the logo is one fogway bundles.
func TestIFlytekModelIcons(t *testing.T) {
	for id, want := range map[string]string{
		"spark-x": "spark-color", "spark-x2.5": "spark-color", "xsparkx2flash": "spark-color", "astron-code-latest": "spark-color",
		"xopglm52": "zhipu-color", "xopglmv47flash": "zhipu-color", "xopdeepseekv4pro": "deepseek-color", "xopkimik26": "kimi",
		"xminimaxm25": "minimax-color", "xopqwen35397b": "qwen-color", "xop3qwencodernext": "qwen-color",
	} {
		if got := ModelIcon(id); got != want {
			t.Errorf("%s: %q, want %q", id, got, want)
		}
	}
	for _, r := range provider.Preset("iflytek").Regions {
		for _, id := range r.Models {
			ic := ModelIcon(id)
			if ic == "" {
				t.Errorf("%s (%s) has no logo", id, r.ID)
				continue
			}
			if _, err := os.Stat(filepath.Join("..", "gui", "assets", "icons", ic+".svg")); err != nil {
				t.Errorf("%s: %v", id, err)
			}
		}
	}
}
