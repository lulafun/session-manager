package main

import (
	"bufio"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"
)

const (
	defaultMaxPreviewLines = 500
	rolloutPrefix          = "rollout-"
	rolloutSuffix          = ".jsonl"
)

type Session struct {
	ID            string
	ForkedFromID  string
	Path          string
	CreatedAt     time.Time
	UpdatedAt     time.Time
	CWD           string
	Originator    string
	CLIVersion    string
	Source        string
	SourceSubtype string
	ThreadSource  string
	NonRootAgent  bool
	ModelProvider string
	GitBranch     string
	GitSHA        string
	GitOriginURL  string
	Preview       string
	LineCount     int
	ParseWarning  string
}

type ScanOptions struct {
	Root            string
	MaxPreviewLines int
}

type FilterOptions struct {
	Provider         string
	CWD              string
	Source           string
	Query            string
	IncludeSubagents bool
}

type ProjectSummary struct {
	CWD   string
	Label string
	Count int
}

type ProviderSummary struct {
	Provider string
	Count    int
}

type rawLine struct {
	Type      string          `json:"type"`
	Timestamp string          `json:"timestamp"`
	Payload   json.RawMessage `json:"payload"`
}

type sessionMetaPayload struct {
	ID            string `json:"id"`
	ForkedFromID  string `json:"forked_from_id"`
	Timestamp     string `json:"timestamp"`
	CWD           string `json:"cwd"`
	Originator    string `json:"originator"`
	CLIVersion    string `json:"cli_version"`
	Source        any    `json:"source"`
	ThreadSource  string `json:"thread_source"`
	ModelProvider string `json:"model_provider"`
	Git           *struct {
		Branch        string `json:"branch"`
		CommitHash    string `json:"commit_hash"`
		RepositoryURL string `json:"repository_url"`
	} `json:"git"`
}

type eventPayload struct {
	Type    string `json:"type"`
	Message string `json:"message"`
}

type UserMessage struct {
	Line    int
	Message string
}

func (s Session) DisplaySource() string {
	if s.SourceSubtype != "" {
		return s.SourceSubtype
	}
	if s.Source != "" {
		return s.Source
	}
	return "(missing)"
}

func DefaultSessionsRoot() string {
	home, err := os.UserHomeDir()
	if err != nil {
		return filepath.Join(".codex", "sessions")
	}
	return filepath.Join(home, ".codex", "sessions")
}

func ScanSessions(opts ScanOptions) ([]Session, error) {
	root := opts.Root
	if root == "" {
		root = DefaultSessionsRoot()
	}
	maxPreviewLines := opts.MaxPreviewLines
	if maxPreviewLines <= 0 {
		maxPreviewLines = defaultMaxPreviewLines
	}

	var sessions []Session
	err := filepath.WalkDir(root, func(path string, d os.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() {
			return nil
		}
		name := d.Name()
		if !strings.HasPrefix(name, rolloutPrefix) || !strings.HasSuffix(name, rolloutSuffix) {
			return nil
		}
		session, err := readSession(path, maxPreviewLines)
		if err != nil {
			session = Session{
				Path:         path,
				ParseWarning: err.Error(),
			}
		}
		sessions = append(sessions, session)
		return nil
	})
	if err != nil {
		return nil, err
	}

	sort.SliceStable(sessions, func(i, j int) bool {
		return sessions[i].UpdatedAt.After(sessions[j].UpdatedAt)
	})
	return sessions, nil
}

func readSession(path string, maxPreviewLines int) (Session, error) {
	file, err := os.Open(path)
	if err != nil {
		return Session{}, err
	}
	defer file.Close()

	info, statErr := file.Stat()
	var updatedAt time.Time
	if statErr == nil {
		updatedAt = info.ModTime()
	}

	session := Session{
		Path:      path,
		UpdatedAt: updatedAt,
	}
	if ts, id, ok := parseFilename(path); ok {
		session.CreatedAt = ts
		session.ID = id
	}

	reader := bufio.NewReader(file)
	var metaSeen bool
	for {
		line, err := reader.ReadBytes('\n')
		if len(line) > 0 {
			session.LineCount++
			var raw rawLine
			if jsonErr := json.Unmarshal(line, &raw); jsonErr != nil {
				if session.ParseWarning == "" {
					session.ParseWarning = fmt.Sprintf("line %d: %v", session.LineCount, jsonErr)
				}
			} else {
				switch raw.Type {
				case "session_meta":
					if !metaSeen {
						metaSeen = true
						applySessionMeta(&session, raw.Payload)
					}
				case "event_msg":
					if session.Preview == "" && session.LineCount <= maxPreviewLines {
						session.Preview = previewFromEvent(raw.Payload)
					}
				}
			}
		}
		if err != nil {
			if errors.Is(err, io.EOF) {
				break
			}
			return session, err
		}
	}

	if !metaSeen && session.ParseWarning == "" {
		session.ParseWarning = "missing session_meta"
	}
	if session.Preview == "" {
		session.Preview = "(no user message preview)"
	}
	return session, nil
}

func applySessionMeta(session *Session, payload json.RawMessage) {
	var meta sessionMetaPayload
	if err := json.Unmarshal(payload, &meta); err != nil {
		session.ParseWarning = err.Error()
		return
	}
	if meta.ID != "" {
		session.ID = meta.ID
	}
	session.ForkedFromID = meta.ForkedFromID
	if ts, ok := parseCodexTime(meta.Timestamp); ok {
		session.CreatedAt = ts
	}
	session.CWD = meta.CWD
	session.Originator = meta.Originator
	session.CLIVersion = meta.CLIVersion
	session.Source = sourceToString(meta.Source)
	session.SourceSubtype = sourceSubtype(meta.Source)
	session.ThreadSource = meta.ThreadSource
	session.NonRootAgent = isNonRootAgent(meta.Source, meta.ThreadSource)
	session.ModelProvider = meta.ModelProvider
	if session.ModelProvider == "" {
		session.ModelProvider = "(missing)"
	}
	if meta.Git != nil {
		session.GitBranch = meta.Git.Branch
		session.GitSHA = meta.Git.CommitHash
		session.GitOriginURL = meta.Git.RepositoryURL
	}
}

func previewFromEvent(payload json.RawMessage) string {
	var event eventPayload
	if err := json.Unmarshal(payload, &event); err != nil {
		return ""
	}
	if event.Type != "user_message" {
		return ""
	}
	return cleanPreview(event.Message)
}

func ReadLastUserMessages(path string, limit int) ([]UserMessage, error) {
	if limit <= 0 {
		limit = 10
	}
	file, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer file.Close()

	var messages []UserMessage
	scanner := bufio.NewScanner(file)
	buf := make([]byte, 0, 64*1024)
	scanner.Buffer(buf, 4*1024*1024)
	lineNo := 0
	for scanner.Scan() {
		lineNo++
		var raw rawLine
		if err := json.Unmarshal(scanner.Bytes(), &raw); err != nil || raw.Type != "event_msg" {
			continue
		}
		var event eventPayload
		if err := json.Unmarshal(raw.Payload, &event); err != nil || event.Type != "user_message" {
			continue
		}
		messages = append(messages, UserMessage{
			Line:    lineNo,
			Message: cleanPreview(event.Message),
		})
	}
	if err := scanner.Err(); err != nil {
		return nil, err
	}
	for i, j := 0, len(messages)-1; i < j; i, j = i+1, j-1 {
		messages[i], messages[j] = messages[j], messages[i]
	}
	if len(messages) > limit {
		messages = messages[:limit]
	}
	return messages, nil
}

func cleanPreview(text string) string {
	text = strings.ReplaceAll(text, "<user_message>", "")
	text = strings.ReplaceAll(text, "</user_message>", "")
	text = strings.ReplaceAll(text, "\n", " ")
	text = strings.Join(strings.Fields(text), " ")
	if len([]rune(text)) > 160 {
		runes := []rune(text)
		text = string(runes[:160]) + "..."
	}
	return text
}

func sourceToString(value any) string {
	switch v := value.(type) {
	case string:
		return v
	case map[string]any:
		if len(v) == 1 {
			for key, val := range v {
				if s, ok := val.(string); ok {
					return key + ":" + s
				}
				return key
			}
		}
		bytes, err := json.Marshal(v)
		if err == nil {
			return string(bytes)
		}
	}
	return ""
}

func sourceSubtype(value any) string {
	switch v := value.(type) {
	case string:
		return v
	case map[string]any:
		if _, ok := v["internal"]; ok {
			return sourceObjectSubtype("internal", v["internal"])
		}
		if _, ok := v["subagent"]; ok {
			return sourceObjectSubtype("subagent", v["subagent"])
		}
		if len(v) == 1 {
			for key, val := range v {
				return sourceObjectSubtype(key, val)
			}
		}
	}
	return ""
}

func sourceObjectSubtype(prefix string, value any) string {
	switch v := value.(type) {
	case string:
		if v == "" {
			return prefix
		}
		return prefix + ":" + v
	case map[string]any:
		if len(v) == 1 {
			for key, val := range v {
				if s, ok := val.(string); ok && s != "" {
					return prefix + ":" + key + ":" + s
				}
				return prefix + ":" + key
			}
		}
		return prefix
	}
	return prefix
}

func isNonRootAgent(source any, threadSource string) bool {
	switch strings.ToLower(strings.TrimSpace(threadSource)) {
	case "subagent", "memory_consolidation":
		return true
	}
	switch v := source.(type) {
	case string:
		normalized := strings.ToLower(strings.TrimSpace(v))
		return strings.HasPrefix(normalized, "subagent") ||
			strings.HasPrefix(normalized, "internal_") ||
			normalized == "memory_consolidation"
	case map[string]any:
		_, subagent := v["subagent"]
		_, internal := v["internal"]
		return subagent || internal
	}
	return false
}

func parseFilename(path string) (time.Time, string, bool) {
	name := filepath.Base(path)
	name = strings.TrimSuffix(strings.TrimPrefix(name, rolloutPrefix), rolloutSuffix)
	if len(name) < len("2006-01-02T15-04-05-")+36 {
		return time.Time{}, "", false
	}
	tsText := name[:len("2006-01-02T15-04-05")]
	id := strings.TrimPrefix(name[len("2006-01-02T15-04-05"):], "-")
	ts, err := time.ParseInLocation("2006-01-02T15-04-05", tsText, time.Local)
	if err != nil {
		return time.Time{}, "", false
	}
	return ts, id, true
}

func parseCodexTime(text string) (time.Time, bool) {
	if text == "" {
		return time.Time{}, false
	}
	for _, layout := range []string{
		time.RFC3339Nano,
		time.RFC3339,
		"2006-01-02T15-04-05",
	} {
		if ts, err := time.Parse(layout, text); err == nil {
			return ts, true
		}
		if ts, err := time.ParseInLocation(layout, text, time.Local); err == nil {
			return ts, true
		}
	}
	return time.Time{}, false
}

func FilterSessions(sessions []Session, filters FilterOptions) []Session {
	var out []Session
	query := strings.ToLower(strings.TrimSpace(filters.Query))
	for _, session := range sessions {
		if !filters.IncludeSubagents && session.NonRootAgent {
			continue
		}
		if filters.Provider != "" && session.ModelProvider != filters.Provider {
			continue
		}
		if filters.CWD != "" && !samePath(session.CWD, filters.CWD) {
			continue
		}
		if filters.Source != "" && session.Source != filters.Source {
			continue
		}
		if query != "" && !sessionMatchesQuery(session, query) {
			continue
		}
		out = append(out, session)
	}
	return out
}

func sessionMatchesQuery(session Session, query string) bool {
	haystack := strings.ToLower(strings.Join([]string{
		session.ID,
		session.Path,
		session.CWD,
		session.Source,
		session.ModelProvider,
		session.GitBranch,
		session.Preview,
	}, "\n"))
	return strings.Contains(haystack, query)
}

func samePath(a, b string) bool {
	if a == b {
		return true
	}
	cleanA, errA := filepath.Abs(a)
	cleanB, errB := filepath.Abs(b)
	if errA == nil && errB == nil && filepath.Clean(cleanA) == filepath.Clean(cleanB) {
		return true
	}
	return filepath.Clean(a) == filepath.Clean(b)
}

func ProviderSummaries(sessions []Session) []ProviderSummary {
	counts := map[string]int{}
	for _, session := range sessions {
		provider := session.ModelProvider
		if provider == "" {
			provider = "(missing)"
		}
		counts[provider]++
	}
	summaries := make([]ProviderSummary, 0, len(counts))
	for provider, count := range counts {
		summaries = append(summaries, ProviderSummary{Provider: provider, Count: count})
	}
	sort.Slice(summaries, func(i, j int) bool {
		if summaries[i].Count == summaries[j].Count {
			return summaries[i].Provider < summaries[j].Provider
		}
		return summaries[i].Count > summaries[j].Count
	})
	return summaries
}

func UniqueProviders(sessions []Session) []string {
	summaries := ProviderSummaries(sessions)
	providers := make([]string, 0, len(summaries))
	for _, summary := range summaries {
		providers = append(providers, summary.Provider)
	}
	return providers
}

func ProjectSummaries(sessions []Session) []ProjectSummary {
	counts := map[string]int{}
	for _, session := range sessions {
		if session.CWD == "" {
			continue
		}
		counts[session.CWD]++
	}
	summaries := make([]ProjectSummary, 0, len(counts))
	for cwd, count := range counts {
		summaries = append(summaries, ProjectSummary{
			CWD:   cwd,
			Label: HomeRelativePath(cwd),
			Count: count,
		})
	}
	sort.Slice(summaries, func(i, j int) bool {
		if summaries[i].Count == summaries[j].Count {
			return summaries[i].Label < summaries[j].Label
		}
		return summaries[i].Count > summaries[j].Count
	})
	return summaries
}

func HomeRelativePath(path string) string {
	home, err := os.UserHomeDir()
	if err != nil || home == "" {
		return path
	}
	cleanHome := filepath.Clean(home)
	cleanPath := filepath.Clean(path)
	if cleanPath == cleanHome {
		return "~"
	}
	if rel, err := filepath.Rel(cleanHome, cleanPath); err == nil && rel != "." && !strings.HasPrefix(rel, "..") {
		return filepath.Join("~", rel)
	}
	return path
}
