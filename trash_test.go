package main

import (
	"os"
	"path/filepath"
	"testing"
)

func TestMoveSessionToTrashAndRecoverPreservesRelativePath(t *testing.T) {
	temp := t.TempDir()
	root := filepath.Join(temp, "sessions")
	trashRoot := filepath.Join(temp, "trash")
	path := filepath.Join(root, "2026", "05", "31", "rollout-2026-05-31T01-42-12-019e79fa-b106-7a20-9fec-b71f632fa38a.jsonl")
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte("test\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	session := Session{
		ID:            "019e79fa-b106-7a20-9fec-b71f632fa38a",
		Path:          path,
		ModelProvider: "zaixiao_s2a",
		CWD:           "/tmp/project",
	}

	record, err := MoveSessionToTrash(session, TrashOptions{Root: root, TrashRoot: trashRoot})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(path); !os.IsNotExist(err) {
		t.Fatalf("source exists after trash: %v", err)
	}
	if _, err := os.Stat(record.TrashPath); err != nil {
		t.Fatalf("trash path missing: %v", err)
	}
	if record.RelativePath != "2026/05/31/rollout-2026-05-31T01-42-12-019e79fa-b106-7a20-9fec-b71f632fa38a.jsonl" {
		t.Fatalf("RelativePath = %q", record.RelativePath)
	}

	trashed := session
	trashed.Path = record.TrashPath
	recovered, err := RecoverSessionFromTrash(trashed, TrashOptions{Root: root, TrashRoot: trashRoot})
	if err != nil {
		t.Fatal(err)
	}
	if recovered.OriginalPath != filepath.Clean(path) {
		t.Fatalf("OriginalPath = %q, want %q", recovered.OriginalPath, filepath.Clean(path))
	}
	if _, err := os.Stat(path); err != nil {
		t.Fatalf("restored path missing: %v", err)
	}
}

func TestSafeRelativePathRejectsOutsideRoot(t *testing.T) {
	root := filepath.Join(t.TempDir(), "root")
	outside := filepath.Join(t.TempDir(), "outside.jsonl")
	if _, err := safeRelativePath(root, outside); err == nil {
		t.Fatal("safeRelativePath accepted path outside root")
	}
}
