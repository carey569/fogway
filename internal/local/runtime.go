package local

import (
	"archive/tar"
	"archive/zip"
	"compress/gzip"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"runtime"
	"slices"
	"strings"
)

// The llama.cpp builds come from its GitHub releases: one per build
// (b11583), each with an archive per platform and backend, and the SHA-256
// of each in the release's asset list (its digest).
const releasesAPI = "https://api.github.com/repos/ggml-org/llama.cpp/releases"

type ghAsset struct {
	Name   string `json:"name"`
	Size   int64  `json:"size"`
	Digest string `json:"digest"` // sha256:…
	URL    string `json:"browser_download_url"`
}

type ghRelease struct {
	Tag    string    `json:"tag_name"`
	Assets []ghAsset `json:"assets"`
}

// assetNames are the archives that run on goos/goarch with backend, best
// first: a CUDA build needs its runtime beside it on Windows (cudart-…).
func assetNames(tag, goos, goarch, backend string) (want []string, extra string) {
	arch := map[string]string{"amd64": "x64", "arm64": "arm64"}[goarch]
	switch goos {
	case "darwin":
		return []string{"llama-" + tag + "-bin-macos-" + arch + ".tar.gz"}, ""
	case "windows":
		switch backend {
		case "cuda":
			return []string{"llama-" + tag + "-bin-win-cuda-12.4-" + arch + ".zip"}, "cudart-llama-bin-win-cuda-12.4-" + arch + ".zip"
		case "vulkan":
			return []string{"llama-" + tag + "-bin-win-vulkan-" + arch + ".zip", "llama-" + tag + "-bin-win-cpu-" + arch + ".zip"}, ""
		}
		return []string{"llama-" + tag + "-bin-win-cpu-" + arch + ".zip"}, ""
	case "linux":
		switch backend {
		case "cuda":
			return []string{"llama-" + tag + "-bin-ubuntu-cuda-12.8-" + arch + ".tar.gz", "llama-" + tag + "-bin-ubuntu-vulkan-" + arch + ".tar.gz"}, ""
		case "vulkan":
			return []string{"llama-" + tag + "-bin-ubuntu-vulkan-" + arch + ".tar.gz", "llama-" + tag + "-bin-ubuntu-" + arch + ".tar.gz"}, ""
		}
		return []string{"llama-" + tag + "-bin-ubuntu-" + arch + ".tar.gz"}, ""
	}
	return nil, ""
}

// release is the build tag's release, or the newest build's when tag is
// "" (llama.cpp tags each build as a prerelease: the latest release is
// not a build).
func release(ctx context.Context, tag string) (ghRelease, error) {
	u := releasesAPI + "?per_page=5"
	if tag != "" {
		u = releasesAPI + "/tags/" + tag
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, u, nil)
	if err != nil {
		return ghRelease{}, err
	}
	req.Header.Set("Accept", "application/vnd.github+json")
	if t := os.Getenv("GITHUB_TOKEN"); t != "" {
		req.Header.Set("Authorization", "Bearer "+t)
	}
	res, err := http.DefaultClient.Do(req)
	if err != nil {
		return ghRelease{}, fmt.Errorf("asking GitHub for llama.cpp's builds: %w", err)
	}
	defer res.Body.Close()
	if res.StatusCode != http.StatusOK {
		return ghRelease{}, fmt.Errorf("GitHub answered %s for llama.cpp's builds", res.Status)
	}
	if tag != "" {
		var r ghRelease
		return r, json.NewDecoder(res.Body).Decode(&r)
	}
	var rs []ghRelease
	if err := json.NewDecoder(res.Body).Decode(&rs); err != nil {
		return ghRelease{}, err
	}
	for _, r := range rs {
		if strings.HasPrefix(r.Tag, "b") && len(r.Assets) > 0 {
			return r, nil
		}
	}
	return ghRelease{}, fmt.Errorf("GitHub lists no llama.cpp build")
}

// RuntimeDir is where a build is unpacked.
func RuntimeDir(tag, backend string) string {
	return filepath.Join(Dir(), "llama.cpp", tag+"-"+backend)
}

// serverName is llama-server's file name here.
func serverName() string {
	if runtime.GOOS == "windows" {
		return "llama-server.exe"
	}
	return "llama-server"
}

// installed is a llama-server unpacked before, the newest build first, or
// "" when there is none of backend.
func installed(backend string) (path, tag string) {
	dirs, _ := filepath.Glob(filepath.Join(Dir(), "llama.cpp", "b*-"+backend))
	slices.SortFunc(dirs, func(a, b string) int { return buildNumber(b) - buildNumber(a) })
	for _, d := range dirs {
		if p := findServer(d); p != "" {
			return p, strings.TrimSuffix(filepath.Base(d), "-"+backend)
		}
	}
	return "", ""
}

func buildNumber(dir string) int {
	n := 0
	fmt.Sscanf(strings.TrimPrefix(filepath.Base(dir), "b"), "%d", &n)
	return n
}

// findServer is llama-server under dir, at whatever depth the archive put
// it (the macOS and Linux ones in a folder of their build, Windows' flat).
func findServer(dir string) string {
	var found string
	filepath.WalkDir(dir, func(p string, d os.DirEntry, err error) error {
		if err == nil && !d.IsDir() && d.Name() == serverName() {
			found = p
			return filepath.SkipAll
		}
		return nil
	})
	return found
}

// ensureRuntime is llama-server's path: the build the setup names, or the
// newest one unpacked already, else the newest from GitHub, downloaded
// and unpacked now.
func ensureRuntime(ctx context.Context, c Config, backend string, tell progressFn) (string, error) {
	if c.Build == "" {
		if p, _ := installed(backend); p != "" {
			return p, nil
		}
	} else if p := findServer(RuntimeDir(c.Build, backend)); p != "" {
		return p, nil
	}
	r, err := release(ctx, c.Build)
	if err != nil {
		return "", err
	}
	want, extra := assetNames(r.Tag, runtime.GOOS, runtime.GOARCH, backend)
	var pick *ghAsset
	for _, name := range want {
		if i := slices.IndexFunc(r.Assets, func(a ghAsset) bool { return a.Name == name }); i >= 0 {
			pick = &r.Assets[i]
			break
		}
	}
	if pick == nil {
		return "", fmt.Errorf("llama.cpp %s has no build for %s/%s (%s)", r.Tag, runtime.GOOS, runtime.GOARCH, backend)
	}
	dir := RuntimeDir(r.Tag, backend)
	tmp := dir + ".unpacking"
	os.RemoveAll(tmp)
	for _, a := range []string{pick.Name, extra} {
		if a == "" {
			continue
		}
		i := slices.IndexFunc(r.Assets, func(x ghAsset) bool { return x.Name == a })
		if i < 0 {
			return "", fmt.Errorf("llama.cpp %s has no %s", r.Tag, a)
		}
		asset := r.Assets[i]
		archive := filepath.Join(Dir(), "downloads", asset.Name)
		if err := fetch(ctx, mirrored(c, asset.URL), archive, asset.Size, strings.TrimPrefix(asset.Digest, "sha256:"), "llama.cpp "+r.Tag, tell, nil); err != nil {
			return "", err
		}
		if err := unpack(archive, tmp); err != nil {
			return "", fmt.Errorf("unpacking %s: %w", asset.Name, err)
		}
		os.Remove(archive)
	}
	os.RemoveAll(dir)
	if err := os.Rename(tmp, dir); err != nil {
		return "", err
	}
	p := findServer(dir)
	if p == "" {
		return "", fmt.Errorf("llama.cpp %s's archive has no %s", r.Tag, serverName())
	}
	return p, nil
}

// mirrored is a github.com download address through the mirror set, as
// fogway's own updates take one.
func mirrored(c Config, u string) string {
	m := strings.TrimSpace(c.GitHubMirror)
	if m == "" {
		return u
	}
	return strings.TrimRight(m, "/") + "/" + u
}

// unpack puts a .tar.gz or .zip archive's files under dir, refusing any
// that would land outside it.
func unpack(archive, dir string) error {
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return err
	}
	inside := func(name string) (string, error) {
		p := filepath.Join(dir, filepath.FromSlash(name))
		if p != dir && !strings.HasPrefix(p, dir+string(filepath.Separator)) {
			return "", fmt.Errorf("%s leads out of the archive", name)
		}
		return p, nil
	}
	if strings.HasSuffix(archive, ".zip") {
		z, err := zip.OpenReader(archive)
		if err != nil {
			return err
		}
		defer z.Close()
		for _, f := range z.File {
			p, err := inside(f.Name)
			if err != nil {
				return err
			}
			if f.FileInfo().IsDir() {
				os.MkdirAll(p, 0o755)
				continue
			}
			if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
				return err
			}
			r, err := f.Open()
			if err != nil {
				return err
			}
			err = writeFile(p, r, f.Mode()|0o600)
			r.Close()
			if err != nil {
				return err
			}
		}
		return nil
	}
	f, err := os.Open(archive)
	if err != nil {
		return err
	}
	defer f.Close()
	gz, err := gzip.NewReader(f)
	if err != nil {
		return err
	}
	tr := tar.NewReader(gz)
	for {
		h, err := tr.Next()
		if err == io.EOF {
			return nil
		}
		if err != nil {
			return err
		}
		p, err := inside(h.Name)
		if err != nil {
			return err
		}
		switch h.Typeflag {
		case tar.TypeDir:
			os.MkdirAll(p, 0o755)
		case tar.TypeReg:
			if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
				return err
			}
			if err := writeFile(p, tr, os.FileMode(h.Mode)|0o600); err != nil {
				return err
			}
		case tar.TypeSymlink:
			// the dylibs' version names: a link within the folder only
			if filepath.IsAbs(h.Linkname) || strings.Contains(h.Linkname, "..") {
				return fmt.Errorf("%s links out of the archive", h.Name)
			}
			os.MkdirAll(filepath.Dir(p), 0o755)
			os.Remove(p)
			if err := os.Symlink(h.Linkname, p); err != nil {
				return err
			}
		}
	}
}

func writeFile(p string, r io.Reader, mode os.FileMode) error {
	f, err := os.OpenFile(p, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, mode&0o777)
	if err != nil {
		return err
	}
	if _, err := io.Copy(f, r); err != nil {
		f.Close()
		return err
	}
	return f.Close()
}
