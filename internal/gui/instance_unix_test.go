//go:build !windows

package gui

import (
	"os"
	"os/exec"
	"path/filepath"
	"testing"
)

// A second fogway on the same config finds the first one's lock held, a
// copy elsewhere (fogway-dev) has its own, and a lock whose holder ended,
// as when the Mac shuts down with fogway running, is free again (Crispin
// on Discord: two fogways after each boot).
func TestOneFogwayAtATime(t *testing.T) {
	dir := t.TempDir()
	if os.Getenv("FOGWAY_HOLD_LOCK") != "" {
		if held, err := holdInstance(os.Getenv("FOGWAY_HOLD_LOCK")); err != nil || !held {
			os.Exit(3)
		}
		os.Stdout.WriteString("held\n")
		select {}
	}
	lock := instanceLock(dir, "/Applications/Fogway.app/Contents/MacOS/fogway")
	if dev := instanceLock(dir, "/src/fogway/fogway-dev"); dev == lock {
		t.Fatal("fogway-dev shares the app's lock")
	}
	// every installed copy shares the lock: another fogway.app (one the
	// Mac reopens, one run translocated from Downloads) and Homebrew's of
	// any version (inaction on Discord: two birds after a restart)
	for _, other := range []string{
		"/Users/u/Downloads/fogway.app/Contents/MacOS/fogway",
		"/private/var/folders/x/T/AppTranslocation/1F2E/d/fogway.app/Contents/MacOS/fogway",
		"/opt/homebrew/Cellar/fogway/0.1.900/bin/fogway",
	} {
		if instanceLock(dir, other) != lock {
			t.Errorf("%s has a lock of its own", other)
		}
	}
	if filepath.Dir(lock) != dir {
		t.Fatalf("lock %s not in the config dir", lock)
	}

	// the first fogway: another process, killed as a shutdown would
	cmd := exec.Command(os.Args[0], "-test.run=^TestOneFogwayAtATime$")
	cmd.Env = append(os.Environ(), "FOGWAY_HOLD_LOCK="+lock)
	out, _ := cmd.StdoutPipe()
	if err := cmd.Start(); err != nil {
		t.Fatal(err)
	}
	buf := make([]byte, 5)
	if _, err := out.Read(buf); err != nil || string(buf[:4]) != "held" {
		cmd.Process.Kill()
		t.Fatalf("the first fogway didn't take the lock: %q %v", buf, err)
	}
	if held, err := holdInstance(lock); err != nil || held {
		cmd.Process.Kill()
		t.Fatalf("a second fogway took the held lock: %v %v", held, err)
	}
	cmd.Process.Kill()
	cmd.Wait()
	if held, err := holdInstance(lock); err != nil || !held {
		t.Fatalf("the lock of a fogway that was killed stayed held: %v %v", held, err)
	}
	heldInstance.Close()
	heldInstance = nil
}
