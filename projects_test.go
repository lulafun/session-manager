package main

import (
	"bytes"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestProjectRowsFromSessionsGroupsAndSortsByLatestUpdate(t *testing.T) {
	oldTime := time.Date(2026, 5, 30, 10, 0, 0, 0, time.UTC)
	newTime := time.Date(2026, 5, 31, 10, 0, 0, 0, time.UTC)
	sessions := []Session{
		{ID: "a1", CWD: "/repo/a", ModelProvider: "openai", UpdatedAt: oldTime},
		{ID: "a2", CWD: "/repo/a", ModelProvider: "zaixiao_s2a", UpdatedAt: newTime},
		{ID: "b1", CWD: "/repo/b", ModelProvider: "openai", UpdatedAt: oldTime.Add(time.Hour)},
		{ID: "missing-cwd", ModelProvider: "openai", UpdatedAt: newTime.Add(time.Hour)},
	}

	rows := projectRowsFromSessions(sessions)
	if len(rows) != 2 {
		t.Fatalf("len(rows) = %d, want 2", len(rows))
	}
	if rows[0].CWD != "/repo/a" {
		t.Fatalf("rows[0].CWD = %q, want /repo/a", rows[0].CWD)
	}
	if rows[0].Count != 2 {
		t.Fatalf("rows[0].Count = %d, want 2", rows[0].Count)
	}
	if got := formatProviderCounts(rows[0].Providers); got != "openai:1, zaixiao_s2a:1" {
		t.Fatalf("providers = %q, want openai:1, zaixiao_s2a:1", got)
	}
}

func TestResolveProjectCWDAcceptsHomeRelativeLabelAndRejectsAmbiguousSubstring(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	sessions := []Session{
		{ID: "a", CWD: filepath.Join(home, "Code", "alpha"), ModelProvider: "openai"},
		{ID: "b", CWD: filepath.Join(home, "Code", "alpha-tools"), ModelProvider: "openai"},
	}

	cwd, err := resolveProjectCWD(sessions, "~/Code/alpha")
	if err != nil {
		t.Fatal(err)
	}
	if cwd != filepath.Join(home, "Code", "alpha") {
		t.Fatalf("cwd = %q, want alpha cwd", cwd)
	}

	_, err = resolveProjectCWD(sessions, "alpha")
	if err == nil {
		t.Fatal("resolveProjectCWD ambiguous substring returned nil error")
	}
	if !strings.Contains(err.Error(), "ambiguous") {
		t.Fatalf("error = %q, want ambiguous", err)
	}
}

func TestRunProjectsListJSONHidesSubagentsByDefault(t *testing.T) {
	root := t.TempDir()
	project := filepath.Join(t.TempDir(), "project")
	writeRollout(t, root, "2026/05/30/rollout-2026-05-30T10-00-00-019e8000-0000-7000-8000-000000000001.jsonl", `{"timestamp":"2026-05-30T10:00:00Z","type":"session_meta","payload":{"id":"019e8000-0000-7000-8000-000000000001","timestamp":"2026-05-30T10:00:00Z","cwd":`+quote(project)+`,"source":"cli","thread_source":"user","model_provider":"openai"}}
{"timestamp":"2026-05-30T10:00:01Z","type":"event_msg","payload":{"type":"user_message","message":"root"}}
`)
	writeRollout(t, root, "2026/05/30/rollout-2026-05-30T11-00-00-019e8000-0000-7000-8000-000000000002.jsonl", `{"timestamp":"2026-05-30T11:00:00Z","type":"session_meta","payload":{"id":"019e8000-0000-7000-8000-000000000002","timestamp":"2026-05-30T11:00:00Z","cwd":`+quote(project)+`,"source":{"subagent":"thread_spawn"},"thread_source":"subagent","model_provider":"openai"}}
{"timestamp":"2026-05-30T11:00:01Z","type":"event_msg","payload":{"type":"user_message","message":"child"}}
`)

	output := captureStdout(t, func() {
		if err := run([]string{"projects", "-root", root, "-json"}); err != nil {
			t.Fatal(err)
		}
	})

	if !strings.Contains(output, `"count": 1`) {
		t.Fatalf("output = %s, want one visible root session", output)
	}
	if strings.Contains(output, "child") {
		t.Fatalf("output = %s, want subagent hidden", output)
	}
}

func TestRunProjectSessionsResolvesProjectLabel(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	root := t.TempDir()
	project := filepath.Join(home, "Code", "project")
	writeRollout(t, root, "2026/05/30/rollout-2026-05-30T10-00-00-019e8000-0000-7000-8000-000000000003.jsonl", `{"timestamp":"2026-05-30T10:00:00Z","type":"session_meta","payload":{"id":"019e8000-0000-7000-8000-000000000003","timestamp":"2026-05-30T10:00:00Z","cwd":`+quote(project)+`,"source":"cli","thread_source":"user","model_provider":"openai","git":{"branch":"main"}}}
{"timestamp":"2026-05-30T10:00:01Z","type":"event_msg","payload":{"type":"user_message","message":"project session"}}
`)

	output := captureStdout(t, func() {
		if err := run([]string{"projects", "sessions", "-root", root, "~/Code/project"}); err != nil {
			t.Fatal(err)
		}
	})

	if !strings.Contains(output, "019e8000-0000-7000-8000-000000000003") {
		t.Fatalf("output = %s, want session id", output)
	}
	if !strings.Contains(output, "project session") {
		t.Fatalf("output = %s, want preview", output)
	}
}

func TestRunProjectProvidersAcceptsFlagsAfterProjectSelector(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	root := t.TempDir()
	project := filepath.Join(home, "Code", "project")
	writeRollout(t, root, "2026/05/30/rollout-2026-05-30T10-00-00-019e8000-0000-7000-8000-000000000004.jsonl", `{"timestamp":"2026-05-30T10:00:00Z","type":"session_meta","payload":{"id":"019e8000-0000-7000-8000-000000000004","timestamp":"2026-05-30T10:00:00Z","cwd":`+quote(project)+`,"source":"cli","thread_source":"user","model_provider":"zaixiao_s2a"}}
{"timestamp":"2026-05-30T10:00:01Z","type":"event_msg","payload":{"type":"user_message","message":"project session"}}
`)

	output := captureStdout(t, func() {
		if err := run([]string{"projects", "providers", "-root", root, "~/Code/project", "-json"}); err != nil {
			t.Fatal(err)
		}
	})

	if !strings.Contains(output, `"provider": "zaixiao_s2a"`) {
		t.Fatalf("output = %s, want JSON provider", output)
	}
	if strings.Contains(output, "provider                  count") {
		t.Fatalf("output = %s, want JSON not table", output)
	}
}

func writeRollout(t *testing.T, root, relativePath, content string) {
	t.Helper()
	path := filepath.Join(root, filepath.FromSlash(relativePath))
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
		t.Fatal(err)
	}
}

func quote(text string) string {
	return `"` + strings.ReplaceAll(text, `\`, `\\`) + `"`
}

func captureStdout(t *testing.T, fn func()) string {
	t.Helper()
	original := os.Stdout
	readEnd, writeEnd, err := os.Pipe()
	if err != nil {
		t.Fatal(err)
	}
	os.Stdout = writeEnd
	defer func() {
		os.Stdout = original
	}()

	fn()

	if err := writeEnd.Close(); err != nil {
		t.Fatal(err)
	}
	var buf bytes.Buffer
	if _, err := io.Copy(&buf, readEnd); err != nil {
		t.Fatal(err)
	}
	return buf.String()
}
