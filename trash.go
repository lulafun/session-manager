package main

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"
)

const trashSessionsDir = "sessions"

type TrashOptions struct {
	Root      string
	TrashRoot string
	DryRun    bool
}

type TrashRecord struct {
	ID            string    `json:"id"`
	ForkedFromID  string    `json:"forkedFromId,omitempty"`
	ModelProvider string    `json:"modelProvider"`
	CWD           string    `json:"cwd"`
	OriginalPath  string    `json:"originalPath"`
	TrashPath     string    `json:"trashPath"`
	RelativePath  string    `json:"relativePath"`
	DeletedAt     time.Time `json:"deletedAt"`
}

func DefaultTrashRoot() string {
	home, err := os.UserHomeDir()
	if err != nil {
		return filepath.Join(".codex", "session-manager-trash")
	}
	return filepath.Join(home, ".codex", "session-manager-trash")
}

func TrashSessionsRoot(trashRoot string) string {
	if trashRoot == "" {
		trashRoot = DefaultTrashRoot()
	}
	return filepath.Join(trashRoot, trashSessionsDir)
}

func MoveSessionToTrash(session Session, opts TrashOptions) (TrashRecord, error) {
	root := opts.Root
	if root == "" {
		root = DefaultSessionsRoot()
	}
	trashRoot := opts.TrashRoot
	if trashRoot == "" {
		trashRoot = DefaultTrashRoot()
	}
	rel, err := safeRelativePath(root, session.Path)
	if err != nil {
		return TrashRecord{}, err
	}
	dest := filepath.Join(TrashSessionsRoot(trashRoot), rel)
	record := TrashRecord{
		ID:            session.ID,
		ForkedFromID:  session.ForkedFromID,
		ModelProvider: session.ModelProvider,
		CWD:           session.CWD,
		OriginalPath:  filepath.Clean(session.Path),
		TrashPath:     filepath.Clean(dest),
		RelativePath:  filepath.ToSlash(rel),
		DeletedAt:     time.Now(),
	}
	if opts.DryRun {
		return record, nil
	}
	if _, err := os.Stat(dest); err == nil {
		return TrashRecord{}, fmt.Errorf("trash destination already exists: %s", dest)
	} else if !os.IsNotExist(err) {
		return TrashRecord{}, err
	}
	if err := os.MkdirAll(filepath.Dir(dest), 0o700); err != nil {
		return TrashRecord{}, err
	}
	if err := os.Rename(session.Path, dest); err != nil {
		return TrashRecord{}, err
	}
	if err := appendTrashRecord(trashRoot, record); err != nil {
		return record, err
	}
	return record, nil
}

func RecoverSessionFromTrash(session Session, opts TrashOptions) (TrashRecord, error) {
	root := opts.Root
	if root == "" {
		root = DefaultSessionsRoot()
	}
	trashRoot := opts.TrashRoot
	if trashRoot == "" {
		trashRoot = DefaultTrashRoot()
	}
	rel, err := safeRelativePath(TrashSessionsRoot(trashRoot), session.Path)
	if err != nil {
		return TrashRecord{}, err
	}
	dest := filepath.Join(root, rel)
	record := TrashRecord{
		ID:            session.ID,
		ForkedFromID:  session.ForkedFromID,
		ModelProvider: session.ModelProvider,
		CWD:           session.CWD,
		OriginalPath:  filepath.Clean(dest),
		TrashPath:     filepath.Clean(session.Path),
		RelativePath:  filepath.ToSlash(rel),
		DeletedAt:     time.Now(),
	}
	if opts.DryRun {
		return record, nil
	}
	if _, err := os.Stat(dest); err == nil {
		return TrashRecord{}, fmt.Errorf("restore destination already exists: %s", dest)
	} else if !os.IsNotExist(err) {
		return TrashRecord{}, err
	}
	if err := os.MkdirAll(filepath.Dir(dest), 0o700); err != nil {
		return TrashRecord{}, err
	}
	if err := os.Rename(session.Path, dest); err != nil {
		return TrashRecord{}, err
	}
	return record, nil
}

func appendTrashRecord(trashRoot string, record TrashRecord) error {
	if err := os.MkdirAll(trashRoot, 0o700); err != nil {
		return err
	}
	file, err := os.OpenFile(filepath.Join(trashRoot, "manifest.jsonl"), os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0o600)
	if err != nil {
		return err
	}
	defer file.Close()
	encoded, err := json.Marshal(record)
	if err != nil {
		return err
	}
	if _, err := file.Write(append(encoded, '\n')); err != nil {
		return err
	}
	return file.Sync()
}

func safeRelativePath(root, path string) (string, error) {
	rootAbs, err := filepath.Abs(root)
	if err != nil {
		return "", err
	}
	pathAbs, err := filepath.Abs(path)
	if err != nil {
		return "", err
	}
	rel, err := filepath.Rel(rootAbs, pathAbs)
	if err != nil {
		return "", err
	}
	if rel == "." || strings.HasPrefix(rel, ".."+string(os.PathSeparator)) || rel == ".." || filepath.IsAbs(rel) {
		return "", fmt.Errorf("path %s is not under root %s", path, root)
	}
	return rel, nil
}
