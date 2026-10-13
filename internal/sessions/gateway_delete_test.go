package sessions

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

// A session fogway recorded nothing of is trashed with an empty list of
// items, not null: the Trash page reads the list (found in a real try, where
// null broke the page).
func TestGatewayDeleteNothingRecorded(t *testing.T) {
	gatewayHome(t)
	last := time.Now().Add(-time.Hour)
	if _, err := DeleteGateway("claude", "s3", "s3", last); err != nil {
		t.Fatal(err)
	}
	list := Trash()
	if len(list) != 1 || list[0].Items == nil || !list[0].Gateway {
		t.Fatalf("trash %+v", list)
	}
	b, _ := json.Marshal(list[0])
	if !strings.Contains(string(b), `"items":[]`) {
		t.Fatalf("items not a list: %s", b)
	}
}

// A trash note can't send Restore's moves outside the gateway store.
func TestGatewayRestoreRefusesForeignItems(t *testing.T) {
	gatewayHome(t)
	elsewhere := filepath.Join(t.TempDir(), "x")
	dir := filepath.Join(TrashDir(), "gateway", "n")
	os.MkdirAll(filepath.Join(dir, "files", "2026-10-10"), 0o700)
	writeManifest(dir, Trashed{Agent: "claude", ID: "s", Gateway: true, Items: []moved{{From: elsewhere, Name: "2026-10-10"}}})
	if _, err := Restore("gateway/n"); err == nil {
		t.Fatal("restored to a path outside the gateway store")
	}
	if _, err := os.Stat(elsewhere); !os.IsNotExist(err) {
		t.Fatal("something was moved out")
	}
}
