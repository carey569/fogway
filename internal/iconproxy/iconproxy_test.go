package iconproxy

import (
	"net/url"
	"testing"
)

func TestURLCarriesThePictureWhole(t *testing.T) {
	const base = "https://icons.example/api/icon"
	t.Setenv("FOGWAY_ICON_PROXY", base)
	pic := "https://github.com/octocat.png?size=96&x=a b#c"
	got := URL(pic)
	u, err := url.Parse(got)
	if err != nil {
		t.Fatal(err)
	}
	if u.Scheme+"://"+u.Host+u.Path != base {
		t.Fatalf("URL(%q) = %q, not on %s", pic, got, base)
	}
	if q := u.Query(); len(q) != 1 || q.Get("url") != pic {
		t.Fatalf("URL(%q) = %q: query %v", pic, got, q)
	}
}

func TestURLWithoutAProxyIsThePicture(t *testing.T) {
	t.Setenv("FOGWAY_ICON_PROXY", "")
	if pic := "https://example.com/x.png"; URL(pic) != pic {
		t.Fatalf("URL(%q) = %q", pic, URL(pic))
	}
}
