package cli

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// touch creates an empty file plus its parent dirs.
func touch(t *testing.T, path string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatalf("mkdir %s: %v", path, err)
	}
	if err := os.WriteFile(path, nil, 0o644); err != nil {
		t.Fatalf("touch %s: %v", path, err)
	}
}

// writeHubPointer plants <root>/.devwork/wosy.yml pointing at hub.
func writeHubPointer(t *testing.T, root, hub string) {
	t.Helper()
	content := "hub: " + hub + "\nproject: test-proj\n"
	pointer := filepath.Join(root, ".devwork", "wosy.yml")
	if err := os.MkdirAll(filepath.Dir(pointer), 0o755); err != nil {
		t.Fatalf("mkdir .devwork: %v", err)
	}
	if err := os.WriteFile(pointer, []byte(content), 0o644); err != nil {
		t.Fatalf("write wosy.yml: %v", err)
	}
}

func TestResolveStore_HubPointer_SingleTeamDB(t *testing.T) {
	t.Setenv("BRAIN_STORE", "")
	tmp := t.TempDir()
	hub := filepath.Join(tmp, "hubws", ".devwork")
	dbPath := filepath.Join(hub, "platform", "brain.db")
	touch(t, dbPath)
	sat := filepath.Join(tmp, "proj", "sub", "dir")
	if err := os.MkdirAll(sat, 0o755); err != nil {
		t.Fatalf("mkdir satellite: %v", err)
	}
	writeHubPointer(t, filepath.Join(tmp, "proj"), hub)

	path, explicit, err := resolveStore(args{}, sat)
	if err != nil {
		t.Fatalf("resolveStore: %v", err)
	}
	if path != dbPath {
		t.Errorf("path=%q, want %q", path, dbPath)
	}
	if !explicit {
		t.Errorf("hub-resolved path should be explicit")
	}
}

func TestResolveStore_HubPointer_TwoTeamDBs_ErrorsListingCandidates(t *testing.T) {
	t.Setenv("BRAIN_STORE", "")
	tmp := t.TempDir()
	hub := filepath.Join(tmp, "hubws", ".devwork")
	db1 := filepath.Join(hub, "core", "brain.db")
	db2 := filepath.Join(hub, "platform", "brain.db")
	touch(t, db1)
	touch(t, db2)
	root := filepath.Join(tmp, "proj")
	if err := os.MkdirAll(root, 0o755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	writeHubPointer(t, root, hub)

	_, _, err := resolveStore(args{}, root)
	if err == nil {
		t.Fatalf("expected error with two team stores")
	}
	if !strings.Contains(err.Error(), db1) || !strings.Contains(err.Error(), db2) {
		t.Errorf("error should list both candidates, got: %v", err)
	}
}

func TestResolveStore_HubLevelDBWinsOverTeamGlob(t *testing.T) {
	t.Setenv("BRAIN_STORE", "")
	tmp := t.TempDir()
	hub := filepath.Join(tmp, "hubws", ".devwork")
	hubDB := filepath.Join(hub, "brain.db")
	touch(t, hubDB)
	touch(t, filepath.Join(hub, "platform", "brain.db"))
	root := filepath.Join(tmp, "proj")
	if err := os.MkdirAll(root, 0o755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	writeHubPointer(t, root, hub)

	path, _, err := resolveStore(args{}, root)
	if err != nil {
		t.Fatalf("resolveStore: %v", err)
	}
	if path != hubDB {
		t.Errorf("path=%q, want workspace-level %q", path, hubDB)
	}
}

func TestResolveStore_EnvWinsOverWalkUp(t *testing.T) {
	tmp := t.TempDir()
	hub := filepath.Join(tmp, "hubws", ".devwork")
	touch(t, filepath.Join(hub, "platform", "brain.db"))
	root := filepath.Join(tmp, "proj")
	if err := os.MkdirAll(root, 0o755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	writeHubPointer(t, root, hub)
	envDB := filepath.Join(tmp, "env-brain.db")
	t.Setenv("BRAIN_STORE", envDB)

	path, explicit, err := resolveStore(args{}, root)
	if err != nil {
		t.Fatalf("resolveStore: %v", err)
	}
	if path != envDB || !explicit {
		t.Errorf("path=%q explicit=%v, want env path %q explicit", path, explicit, envDB)
	}
}

func TestResolveStore_FlagWinsOverEnv(t *testing.T) {
	t.Setenv("BRAIN_STORE", "/somewhere/env.db")
	path, explicit, err := resolveStore(args{store: "/flagged/brain.db"}, t.TempDir())
	if err != nil {
		t.Fatalf("resolveStore: %v", err)
	}
	if path != "/flagged/brain.db" || !explicit {
		t.Errorf("path=%q explicit=%v, want flag path explicit", path, explicit)
	}
}

func TestResolveStore_NoPointer_LegacyFallback(t *testing.T) {
	t.Setenv("BRAIN_STORE", "")
	path, explicit, err := resolveStore(args{}, t.TempDir())
	if err != nil {
		t.Fatalf("resolveStore: %v", err)
	}
	if path != ".brain/store.db" || explicit {
		t.Errorf("path=%q explicit=%v, want legacy implicit fallback", path, explicit)
	}
}
