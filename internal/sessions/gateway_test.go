package sessions

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func gatewayHome(t *testing.T) {
	t.Helper()
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	if err := SetGatewayRecording(false, true); err != nil {
		t.Fatal(err)
	}
}

func TestGatewayConversationClearAndReenableRejectsOldRequest(t *testing.T) {
	gatewayHome(t)
	if err := SetGatewayRecording(true, false); err != nil {
		t.Fatal(err)
	}
	generation := GatewayRecordingGeneration()
	if err := SetGatewayRecording(false, true); err != nil {
		t.Fatal(err)
	}
	if err := SetGatewayRecording(true, false); err != nil {
		t.Fatal(err)
	}
	if err := SaveGatewayTurnGeneration(GatewayTurn{Agent: "pi", Session: "s", Time: time.Now()}, generation); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(gatewayDir()); !os.IsNotExist(err) {
		t.Fatal("old request resurrected cleared content")
	}
}

func TestGatewayConversationOversizedLine(t *testing.T) {
	gatewayHome(t)
	if err := SetGatewayRecording(true, false); err != nil {
		t.Fatal(err)
	}
	dir := filepath.Join(gatewayDir(), time.Now().UTC().Format(time.DateOnly), gatewayIdentity("pi", "s"))
	if err := os.MkdirAll(dir, 0700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "turns.jsonl"), []byte(strings.Repeat(" ", gatewayTurnMax+1)+"\n"), 0600); err != nil {
		t.Fatal(err)
	}
	if _, err := GatewayTranscript("pi", "s"); err == nil {
		t.Fatal("oversized corrupt line reported as an empty transcript")
	}
}
