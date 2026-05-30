package main

import (
	"os"
	"path/filepath"
	"testing"
)

func TestFilterSessionsHidesNonRootAgentsByDefault(t *testing.T) {
	sessions := []Session{
		{ID: "root", Source: "cli"},
		{ID: "thread-source", Source: "cli", ThreadSource: "subagent", NonRootAgent: true},
		{ID: "subagent-source", Source: "subagent", SourceSubtype: "subagent:thread_spawn", NonRootAgent: true},
		{ID: "internal-source", Source: "internal", SourceSubtype: "internal:memory_consolidation", NonRootAgent: true},
	}

	got := FilterSessions(sessions, FilterOptions{})
	want := []Session{{ID: "root", Source: "cli"}}
	if len(got) != len(want) || got[0].ID != want[0].ID {
		t.Fatalf("FilterSessions() = %#v, want only root session", got)
	}

	got = FilterSessions(sessions, FilterOptions{IncludeSubagents: true})
	if len(got) != len(sessions) {
		t.Fatalf("FilterSessions(include) returned %d sessions, want %d", len(got), len(sessions))
	}
}

func TestSourceSubtypeAndNonRootAgent(t *testing.T) {
	source := map[string]any{
		"subagent": map[string]any{
			"thread_spawn": map[string]any{
				"parent_thread_id": "parent",
				"depth":            float64(1),
			},
		},
	}

	if got := sourceSubtype(source); got != "subagent:thread_spawn" {
		t.Fatalf("sourceSubtype() = %q, want %q", got, "subagent:thread_spawn")
	}
	if !isNonRootAgent(source, "") {
		t.Fatal("isNonRootAgent() = false, want true for subagent source")
	}
	if !isNonRootAgent("cli", "subagent") {
		t.Fatal("isNonRootAgent() = false, want true for subagent thread_source")
	}
	if isNonRootAgent("cli", "user") {
		t.Fatal("isNonRootAgent() = true, want false for root user thread")
	}
}

func TestReadSessionUsesFirstSessionMeta(t *testing.T) {
	path := filepath.Join(t.TempDir(), "rollout-2026-05-31T01-42-12-019e79fa-b106-7a20-9fec-b71f632fa38a.jsonl")
	content := `{"timestamp":"2026-05-30T17:42:13.315Z","type":"session_meta","payload":{"id":"019e79fa-b106-7a20-9fec-b71f632fa38a","forked_from_id":"019e646a-2204-7241-a812-b6a2c1c26c39","timestamp":"2026-05-30T17:42:12.998Z","cwd":"/tmp/project","originator":"codex-tui","cli_version":"0.135.0","source":"cli","thread_source":"user","model_provider":"zaixiao_s2a"}}
{"timestamp":"2026-05-30T17:42:13.315Z","type":"session_meta","payload":{"id":"019e646a-2204-7241-a812-b6a2c1c26c39","timestamp":"2026-05-26T13:12:17.668Z","cwd":"/tmp/project","originator":"Codex Desktop","cli_version":"0.133.0-alpha.1","source":"vscode","thread_source":"user","model_provider":"openai"}}
`
	if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
		t.Fatal(err)
	}

	session, err := readSession(path, defaultMaxPreviewLines)
	if err != nil {
		t.Fatal(err)
	}

	if session.ID != "019e79fa-b106-7a20-9fec-b71f632fa38a" {
		t.Fatalf("ID = %q, want fork id", session.ID)
	}
	if session.ForkedFromID != "019e646a-2204-7241-a812-b6a2c1c26c39" {
		t.Fatalf("ForkedFromID = %q, want source id", session.ForkedFromID)
	}
	if session.ModelProvider != "zaixiao_s2a" {
		t.Fatalf("ModelProvider = %q, want zaixiao_s2a", session.ModelProvider)
	}
}
