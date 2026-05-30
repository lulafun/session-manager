package main

import "testing"

func TestRefreshSelectionAfterDeleteKeepsProjectAndProviderWhenAvailable(t *testing.T) {
	model := testUIModelForDelete()
	model.projectIdx = firstProjectIndex(model.projects, "/repo/a")
	model.providers = providersForProject(model.allSessions, projectCWD(model.projects, model.projectIdx))
	model.providerIdx = providerIndex(model.providers, "p1")
	model.focus = focusSession
	snapshot := selectionSnapshot{
		projectCWD: projectCWD(model.projects, model.projectIdx),
		provider:   selectedProviderValue(model.providers, model.providerIdx),
		focus:      model.focus,
	}

	model.removeSessionByPath("/sessions/a-p1-1.jsonl")
	model.refreshSelectionAfterDelete(snapshot)

	if got := projectCWD(model.projects, model.projectIdx); got != "/repo/a" {
		t.Fatalf("project = %q, want /repo/a", got)
	}
	if got := selectedProviderValue(model.providers, model.providerIdx); got != "p1" {
		t.Fatalf("provider = %q, want p1", got)
	}
	if model.focus != focusSession {
		t.Fatalf("focus = %v, want focusSession", model.focus)
	}
}

func TestRefreshSelectionAfterDeleteFallsBackToProjectWhenProviderGone(t *testing.T) {
	model := testUIModelForDelete()
	model.projectIdx = firstProjectIndex(model.projects, "/repo/a")
	model.providers = providersForProject(model.allSessions, projectCWD(model.projects, model.projectIdx))
	model.providerIdx = providerIndex(model.providers, "p1")
	model.focus = focusSession
	snapshot := selectionSnapshot{
		projectCWD: projectCWD(model.projects, model.projectIdx),
		provider:   selectedProviderValue(model.providers, model.providerIdx),
		focus:      model.focus,
	}

	model.removeSessionByPath("/sessions/a-p1-1.jsonl")
	model.removeSessionByPath("/sessions/a-p1-2.jsonl")
	model.refreshSelectionAfterDelete(snapshot)

	if got := projectCWD(model.projects, model.projectIdx); got != "/repo/a" {
		t.Fatalf("project = %q, want /repo/a", got)
	}
	if got := selectedProviderValue(model.providers, model.providerIdx); got != "" {
		t.Fatalf("provider = %q, want all providers fallback", got)
	}
	if model.focus != focusProject {
		t.Fatalf("focus = %v, want focusProject", model.focus)
	}
}

func TestRefreshSelectionAfterDeleteDoesNotKeepMissingProject(t *testing.T) {
	model := testUIModelForDelete()
	model.projectIdx = firstProjectIndex(model.projects, "/repo/b")
	model.providers = providersForProject(model.allSessions, projectCWD(model.projects, model.projectIdx))
	model.providerIdx = providerIndex(model.providers, "p1")
	model.focus = focusSession
	snapshot := selectionSnapshot{
		projectCWD: projectCWD(model.projects, model.projectIdx),
		provider:   selectedProviderValue(model.providers, model.providerIdx),
		focus:      model.focus,
	}

	model.removeSessionByPath("/sessions/b-p1.jsonl")
	model.refreshSelectionAfterDelete(snapshot)

	if got := projectCWD(model.projects, model.projectIdx); got == "/repo/b" {
		t.Fatalf("project = %q, want missing project not retained", got)
	}
	if model.focus != focusProject {
		t.Fatalf("focus = %v, want focusProject", model.focus)
	}
}

func testUIModelForDelete() UIModel {
	return NewUIModel([]Session{
		{ID: "a-p1-1", Path: "/sessions/a-p1-1.jsonl", CWD: "/repo/a", ModelProvider: "p1"},
		{ID: "a-p1-2", Path: "/sessions/a-p1-2.jsonl", CWD: "/repo/a", ModelProvider: "p1"},
		{ID: "a-p2", Path: "/sessions/a-p2.jsonl", CWD: "/repo/a", ModelProvider: "p2"},
		{ID: "b-p1", Path: "/sessions/b-p1.jsonl", CWD: "/repo/b", ModelProvider: "p1"},
	}, "/sessions", FilterOptions{}, nil)
}
