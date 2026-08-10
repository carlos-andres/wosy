package cli

import (
	"bytes"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"wosy.local/brain/internal/store"
)

// seedTask plants a minimal-valid task in a fresh sqlite store and returns the path.
func seedTask(t *testing.T, id string) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "t.db")
	s, err := store.Open(path)
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	defer s.Close()
	rec := map[string]any{
		"id": id, "type": "task", "status": "open", "updated": "2026-05-26",
		"title":     "t",
		"context":   "c",
		"todo":      []any{map[string]any{"id": "a", "desc": "do a", "status": "open"}},
		"scope":     map[string]any{"in_scope": []any{"x"}},
		"done_when": []any{"x done"},
	}
	if err := s.Put(rec); err != nil {
		t.Fatalf("seed put: %v", err)
	}
	return path
}

// captureStdout runs fn with os.Stdout redirected to a buffer and returns the captured bytes.
// Shared helper — relocated here from cli_doctor_test.go when III-A4b deleted that file.
func captureStdout(t *testing.T, fn func()) string {
	t.Helper()
	orig := os.Stdout
	r, w, err := os.Pipe()
	if err != nil {
		t.Fatalf("pipe: %v", err)
	}
	os.Stdout = w
	done := make(chan struct{})
	var buf bytes.Buffer
	go func() {
		io.Copy(&buf, r)
		close(done)
	}()
	defer func() {
		w.Close()
		<-done
		os.Stdout = orig
	}()
	fn()
	w.Close()
	<-done
	os.Stdout = orig
	return buf.String()
}

// captureStderr runs fn with os.Stderr redirected to a buffer and returns the captured bytes.
func captureStderr(t *testing.T, fn func()) string {
	t.Helper()
	orig := os.Stderr
	r, w, err := os.Pipe()
	if err != nil {
		t.Fatalf("pipe: %v", err)
	}
	os.Stderr = w
	done := make(chan struct{})
	var buf bytes.Buffer
	go func() {
		io.Copy(&buf, r)
		close(done)
	}()
	defer func() {
		w.Close()
		<-done
		os.Stderr = orig
	}()
	fn()
	w.Close()
	<-done
	os.Stderr = orig
	return buf.String()
}

// Sanity: valid section still passes (no regression on the happy path).
func TestSet_ValidSection_Passes(t *testing.T) {
	path := seedTask(t, "k1")
	code := Run([]string{"set", "--store=" + path, "--task=k1", "--section=context", "--input=fresh"})
	if code != 0 {
		t.Fatalf("set valid section exit=%d, want 0", code)
	}
	s, _ := store.Open(path)
	defer s.Close()
	rec, _ := s.Get("k1")
	if rec["context"] != "fresh" {
		t.Fatalf("context not updated: %v", rec["context"])
	}
}

// G3: set with an unknown section MUST be rejected, with a non-zero exit and
// a stderr message that names the section + lists the allowed ones. Store untouched.
func TestSet_UnknownSection_Rejected_G3(t *testing.T) {
	path := seedTask(t, "k2")
	var code int
	msg := captureStderr(t, func() {
		code = Run([]string{"set", "--store=" + path, "--task=k2", "--section=fnord", "--input=probe"})
	})
	if code == 0 {
		t.Fatalf("expected non-zero exit on unknown section; got 0")
	}
	if !strings.Contains(msg, "fnord") {
		t.Fatalf("stderr should name the rejected section, got: %q", msg)
	}
	if !strings.Contains(strings.ToLower(msg), "allowed") {
		t.Fatalf("stderr should surface the allow-list, got: %q", msg)
	}
	// Store untouched: no `fnord` key on the record.
	s, _ := store.Open(path)
	defer s.Close()
	rec, _ := s.Get("k2")
	if _, present := rec["fnord"]; present {
		t.Fatalf("rejected write polluted the doc: %v", rec)
	}
}
