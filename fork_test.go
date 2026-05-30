package main

import (
	"reflect"
	"strings"
	"testing"
)

func TestBuildForkCommandWithWorkDir(t *testing.T) {
	got, err := BuildForkCommand("session-id", ForkOptions{
		CodexBin:       "codex",
		TargetProvider: "zaixiao_s2a",
		Model:          "gpt-5",
		WorkDir:        "/Users/lula/code/Tools/session-browser",
	})
	if err != nil {
		t.Fatal(err)
	}
	want := []string{
		"codex",
		"fork",
		"-c",
		"model_provider=zaixiao_s2a",
		"-m",
		"gpt-5",
		"-C",
		"/Users/lula/code/Tools/session-browser",
		"session-id",
	}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("BuildForkCommand() = %#v, want %#v", got, want)
	}
}

func TestBuildForkCommandWithLast(t *testing.T) {
	got, err := BuildForkCommand("", ForkOptions{
		CodexBin:       "codex",
		TargetProvider: "zaixiao_s2a",
		WorkDir:        "/tmp/project",
		Last:           true,
	})
	if err != nil {
		t.Fatal(err)
	}
	want := []string{
		"codex",
		"fork",
		"-c",
		"model_provider=zaixiao_s2a",
		"-C",
		"/tmp/project",
		"--last",
	}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("BuildForkCommand() = %#v, want %#v", got, want)
	}
}

func TestBuildForkCommandRejectsLastWithSessionID(t *testing.T) {
	_, err := BuildForkCommand("session-id", ForkOptions{Last: true})
	if err == nil {
		t.Fatal("BuildForkCommand() error = nil, want error")
	}
	if !strings.Contains(err.Error(), "either a session id or --last") {
		t.Fatalf("error = %q, want --last conflict", err)
	}
}

func TestCoalesceWorkDirRejectsConflicts(t *testing.T) {
	_, err := coalesceWorkDir("/tmp/a", "", "/tmp/b")
	if err == nil {
		t.Fatal("coalesceWorkDir() error = nil, want error")
	}
	if !strings.Contains(err.Error(), "only one target directory") {
		t.Fatalf("error = %q, want directory conflict", err)
	}
}
