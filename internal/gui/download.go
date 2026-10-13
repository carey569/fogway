package gui

import (
	"fmt"
	"io"
	"os"
	"path/filepath"
)

// downloads is where an export is saved: the Downloads folder, or home
// when there is none.
func downloads() string {
	home, _ := os.UserHomeDir()
	if d := filepath.Join(home, "Downloads"); isDir(d) {
		return d
	}
	return home
}

func isDir(p string) bool {
	fi, err := os.Stat(p)
	return err == nil && fi.IsDir()
}

// saveDownload writes what r reads to dir/<stem><ext>, or <stem>-2<ext>…
// when that is there: whole, or not at all.
func saveDownload(dir, stem, ext string, r io.Reader) (string, error) {
	tmp, err := os.CreateTemp(dir, "."+stem+"-*.part")
	if err != nil {
		return "", err
	}
	_, err = io.Copy(tmp, r)
	if cerr := tmp.Close(); err == nil {
		err = cerr
	}
	if err != nil {
		os.Remove(tmp.Name())
		return "", err
	}
	name := filepath.Join(dir, stem+ext)
	for i := 2; ; i++ { // never over an earlier one
		if _, err := os.Stat(name); err != nil {
			break
		}
		name = filepath.Join(dir, fmt.Sprintf("%s-%d%s", stem, i, ext))
	}
	if err := os.Rename(tmp.Name(), name); err != nil {
		os.Remove(tmp.Name())
		return "", err
	}
	return name, nil
}
