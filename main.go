package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"time"

	tea "github.com/charmbracelet/bubbletea"
)

type commandConfig struct {
	root             string
	config           string
	provider         string
	cwd              string
	source           string
	query            string
	limit            int
	json             bool
	includeSubagents bool
}

func main() {
	if err := run(os.Args[1:]); err != nil {
		fmt.Fprintln(os.Stderr, "error:", err)
		os.Exit(1)
	}
}

func run(args []string) error {
	if len(args) == 0 {
		args = []string{"ui"}
	}
	switch args[0] {
	case "ui":
		return runUI(args[1:])
	case "list":
		return runList(args[1:])
	case "providers":
		return runProviders(args[1:])
	case "projects":
		return runProjects(args[1:])
	case "inspect":
		return runInspect(args[1:])
	case "fork":
		return runFork(args[1:])
	case "trash":
		return runTrash(args[1:])
	case "recover":
		return runRecover(args[1:])
	case "render":
		return runRender(args[1:])
	case "help", "-h", "--help":
		printUsage()
		return nil
	default:
		return fmt.Errorf("unknown command %q\n\nrun `session-manager help`", args[0])
	}
}

func baseFlagSet(name string, cfg *commandConfig) *flag.FlagSet {
	fs := flag.NewFlagSet(name, flag.ContinueOnError)
	fs.StringVar(&cfg.root, "root", DefaultSessionsRoot(), "Codex sessions directory to read")
	fs.StringVar(&cfg.config, "config", DefaultConfigPath(), "Codex config.toml to read providers from")
	fs.StringVar(&cfg.provider, "provider", "", "filter by model provider id")
	fs.StringVar(&cfg.cwd, "cwd", "", "filter by working directory")
	fs.StringVar(&cfg.source, "source", "", "filter by session source")
	fs.StringVar(&cfg.query, "query", "", "substring search across id, cwd, provider, branch, and preview")
	fs.IntVar(&cfg.limit, "limit", 25, "maximum rows to print")
	fs.BoolVar(&cfg.json, "json", false, "print JSON")
	fs.BoolVar(&cfg.includeSubagents, "include-subagents", false, "include subagent/internal non-root sessions")
	return fs
}

func runUI(args []string) error {
	cfg := commandConfig{}
	fs := baseFlagSet("ui", &cfg)
	targetProvider := fs.String("to-provider", "", "target provider for interactive fork")
	modelOverride := fs.String("model", "", "target model override for interactive fork")
	codexBin := fs.String("codex", "codex", "codex executable")
	dryRun := fs.Bool("dry-run", false, "print fork command instead of running codex")
	if err := fs.Parse(args); err != nil {
		return err
	}
	sessions, err := ScanSessions(ScanOptions{Root: cfg.root})
	if err != nil {
		return err
	}
	sessions = FilterSessions(sessions, FilterOptions{
		Source:           cfg.source,
		IncludeSubagents: cfg.includeSubagents,
	})
	configProviders, _, err := ReadConfigProviders(cfg.config)
	if err != nil {
		return err
	}
	model := NewUIModel(sessions, cfg.root, FilterOptions{
		Provider:         cfg.provider,
		CWD:              cfg.cwd,
		Source:           cfg.source,
		Query:            cfg.query,
		IncludeSubagents: cfg.includeSubagents,
	}, configProviders)
	model.fork = ForkOptions{
		CodexBin:       *codexBin,
		TargetProvider: *targetProvider,
		Model:          *modelOverride,
		DryRun:         *dryRun,
	}
	model.trash = TrashOptions{
		Root: cfg.root,
	}
	_, err = tea.NewProgram(model, tea.WithAltScreen()).Run()
	return err
}

func runList(args []string) error {
	cfg := commandConfig{}
	fs := baseFlagSet("list", &cfg)
	if err := fs.Parse(args); err != nil {
		return err
	}
	sessions, err := scanAndFilter(cfg)
	if err != nil {
		return err
	}
	if cfg.limit > 0 && len(sessions) > cfg.limit {
		sessions = sessions[:cfg.limit]
	}
	if cfg.json {
		return writeJSON(sessions)
	}
	printSessionTable(sessions)
	return nil
}

func runProviders(args []string) error {
	cfg := commandConfig{}
	fs := baseFlagSet("providers", &cfg)
	if err := fs.Parse(args); err != nil {
		return err
	}
	sessions, err := ScanSessions(ScanOptions{Root: cfg.root})
	if err != nil {
		return err
	}
	if cfg.cwd != "" || cfg.source != "" || cfg.query != "" || !cfg.includeSubagents {
		sessions = FilterSessions(sessions, FilterOptions{
			CWD:              cfg.cwd,
			Source:           cfg.source,
			Query:            cfg.query,
			IncludeSubagents: cfg.includeSubagents,
		})
	}
	sessionSummaries := ProviderSummaries(sessions)
	configProviders, currentProvider, err := ReadConfigProviders(cfg.config)
	if err != nil {
		return err
	}
	summaries := mergeProviderOutput(configProviders, sessionSummaries)
	if cfg.json {
		return writeJSON(struct {
			Current   string        `json:"current"`
			Providers []providerRow `json:"providers"`
		}{Current: currentProvider, Providers: summaries})
	}
	for _, summary := range summaries {
		marker := "sessions"
		if summary.FromConfig {
			marker = "config"
		}
		current := ""
		if summary.Provider == currentProvider {
			current = "*"
		}
		fmt.Printf("%-1s %-24s %5d  %s\n", current, summary.Provider, summary.Count, marker)
	}
	return nil
}

type providerRow struct {
	Provider   string `json:"provider"`
	Count      int    `json:"count"`
	FromConfig bool   `json:"fromConfig"`
}

func mergeProviderOutput(configProviders []string, sessionSummaries []ProviderSummary) []providerRow {
	counts := map[string]int{}
	for _, summary := range sessionSummaries {
		counts[summary.Provider] = summary.Count
	}
	rows := make([]providerRow, 0, len(configProviders)+len(sessionSummaries))
	seen := map[string]bool{}
	for _, provider := range configProviders {
		if provider == "" || seen[provider] {
			continue
		}
		rows = append(rows, providerRow{
			Provider:   provider,
			Count:      counts[provider],
			FromConfig: true,
		})
		seen[provider] = true
	}
	for _, summary := range sessionSummaries {
		if seen[summary.Provider] {
			continue
		}
		rows = append(rows, providerRow{
			Provider: summary.Provider,
			Count:    summary.Count,
		})
	}
	return rows
}

func runProjects(args []string) error {
	subcommand := "list"
	subArgs := args
	if len(args) > 0 && !strings.HasPrefix(args[0], "-") {
		switch args[0] {
		case "list", "ls", "sessions", "providers":
			subcommand = args[0]
			subArgs = args[1:]
		case "help", "-h", "--help":
			printProjectsUsage()
			return nil
		default:
			return fmt.Errorf("unknown projects subcommand %q\n\nrun `session-manager projects help`", args[0])
		}
	}

	switch subcommand {
	case "list", "ls":
		return runProjectsList(subArgs)
	case "sessions":
		return runProjectSessions(subArgs)
	case "providers":
		return runProjectProviders(subArgs)
	default:
		return fmt.Errorf("unknown projects subcommand %q", subcommand)
	}
}

func runProjectsList(args []string) error {
	cfg := commandConfig{}
	fs := baseFlagSet("projects list", &cfg)
	if err := fs.Parse(args); err != nil {
		return err
	}
	sessions, err := scanAndFilter(cfg)
	if err != nil {
		return err
	}
	rows := projectRowsFromSessions(sessions)
	if cfg.limit > 0 && len(rows) > cfg.limit {
		rows = rows[:cfg.limit]
	}
	if cfg.json {
		return writeJSON(rows)
	}
	printProjectTable(rows)
	return nil
}

func runProjectSessions(args []string) error {
	cfg := commandConfig{}
	fs := baseFlagSet("projects sessions", &cfg)
	target, err := parseTargetCommandArgs(fs, args)
	if err != nil {
		return err
	}
	if cfg.cwd != "" {
		return fmt.Errorf("projects sessions uses the project argument; do not pass -cwd")
	}
	if target == "" {
		return fmt.Errorf("projects sessions requires a project cwd or label")
	}
	allSessions, err := ScanSessions(ScanOptions{Root: cfg.root})
	if err != nil {
		return err
	}
	matchBase := FilterSessions(allSessions, FilterOptions{
		Source:           cfg.source,
		Query:            cfg.query,
		IncludeSubagents: cfg.includeSubagents,
	})
	cwd, err := resolveProjectCWD(matchBase, target)
	if err != nil {
		return err
	}
	sessions := FilterSessions(allSessions, FilterOptions{
		Provider:         cfg.provider,
		CWD:              cwd,
		Source:           cfg.source,
		Query:            cfg.query,
		IncludeSubagents: cfg.includeSubagents,
	})
	if cfg.limit > 0 && len(sessions) > cfg.limit {
		sessions = sessions[:cfg.limit]
	}
	if cfg.json {
		return writeJSON(sessions)
	}
	printSessionTable(sessions)
	return nil
}

func runProjectProviders(args []string) error {
	cfg := commandConfig{}
	fs := baseFlagSet("projects providers", &cfg)
	target, err := parseTargetCommandArgs(fs, args)
	if err != nil {
		return err
	}
	if cfg.cwd != "" {
		return fmt.Errorf("projects providers uses the project argument; do not pass -cwd")
	}
	if target == "" {
		return fmt.Errorf("projects providers requires a project cwd or label")
	}
	allSessions, err := ScanSessions(ScanOptions{Root: cfg.root})
	if err != nil {
		return err
	}
	matchBase := FilterSessions(allSessions, FilterOptions{
		Source:           cfg.source,
		Query:            cfg.query,
		IncludeSubagents: cfg.includeSubagents,
	})
	cwd, err := resolveProjectCWD(matchBase, target)
	if err != nil {
		return err
	}
	sessions := FilterSessions(allSessions, FilterOptions{
		Provider:         cfg.provider,
		CWD:              cwd,
		Source:           cfg.source,
		Query:            cfg.query,
		IncludeSubagents: cfg.includeSubagents,
	})
	row := projectRowsFromSessions(sessions)
	if len(row) == 0 {
		return fmt.Errorf("project %q has no sessions matching the filters", target)
	}
	if cfg.json {
		return writeJSON(row[0])
	}
	printProviderTable(row[0].Providers)
	return nil
}

type projectRow struct {
	CWD       string               `json:"cwd"`
	Label     string               `json:"label"`
	Count     int                  `json:"count"`
	Providers []projectProviderRow `json:"providers"`
	UpdatedAt time.Time            `json:"updatedAt"`
}

type projectProviderRow struct {
	Provider string `json:"provider"`
	Count    int    `json:"count"`
}

func projectRowsFromSessions(sessions []Session) []projectRow {
	grouped := map[string][]Session{}
	for _, session := range sessions {
		if session.CWD == "" {
			continue
		}
		grouped[session.CWD] = append(grouped[session.CWD], session)
	}

	rows := make([]projectRow, 0, len(grouped))
	for cwd, projectSessions := range grouped {
		rows = append(rows, projectRow{
			CWD:       cwd,
			Label:     HomeRelativePath(cwd),
			Count:     len(projectSessions),
			Providers: projectProviderRows(projectSessions),
			UpdatedAt: latestUpdatedAt(projectSessions),
		})
	}
	sort.Slice(rows, func(i, j int) bool {
		if rows[i].UpdatedAt.Equal(rows[j].UpdatedAt) {
			return rows[i].Label < rows[j].Label
		}
		return rows[i].UpdatedAt.After(rows[j].UpdatedAt)
	})
	return rows
}

func projectProviderRows(sessions []Session) []projectProviderRow {
	summaries := ProviderSummaries(sessions)
	rows := make([]projectProviderRow, 0, len(summaries))
	for _, summary := range summaries {
		rows = append(rows, projectProviderRow{
			Provider: summary.Provider,
			Count:    summary.Count,
		})
	}
	return rows
}

func latestUpdatedAt(sessions []Session) time.Time {
	var latest time.Time
	for _, session := range sessions {
		if session.UpdatedAt.After(latest) {
			latest = session.UpdatedAt
		}
	}
	return latest
}

func resolveProjectCWD(sessions []Session, selector string) (string, error) {
	selector = strings.TrimSpace(selector)
	if selector == "" {
		return "", fmt.Errorf("project selector is empty")
	}
	rows := projectRowsFromSessions(sessions)
	if len(rows) == 0 {
		return "", fmt.Errorf("no projects found")
	}

	expanded := expandHomePath(selector)
	var exact []projectRow
	for _, row := range rows {
		if row.CWD == selector || row.Label == selector {
			exact = append(exact, row)
			continue
		}
		if selectorLooksLikePath(selector) && samePath(row.CWD, expanded) {
			exact = append(exact, row)
		}
	}
	if len(exact) == 1 {
		return exact[0].CWD, nil
	}
	if len(exact) > 1 {
		return "", ambiguousProjectError(selector, exact)
	}

	query := strings.ToLower(selector)
	var matches []projectRow
	for _, row := range rows {
		if strings.Contains(strings.ToLower(row.CWD), query) ||
			strings.Contains(strings.ToLower(row.Label), query) {
			matches = append(matches, row)
		}
	}
	if len(matches) == 1 {
		return matches[0].CWD, nil
	}
	if len(matches) > 1 {
		return "", ambiguousProjectError(selector, matches)
	}
	return "", fmt.Errorf("project %q not found", selector)
}

func expandHomePath(path string) string {
	if path == "~" {
		home, err := os.UserHomeDir()
		if err == nil {
			return home
		}
		return path
	}
	if strings.HasPrefix(path, "~/") {
		home, err := os.UserHomeDir()
		if err == nil {
			return filepath.Join(home, strings.TrimPrefix(path, "~/"))
		}
	}
	return path
}

func selectorLooksLikePath(selector string) bool {
	return selector == "~" ||
		strings.HasPrefix(selector, "~/") ||
		strings.HasPrefix(selector, "/") ||
		strings.HasPrefix(selector, ".") ||
		strings.Contains(selector, string(os.PathSeparator))
}

func ambiguousProjectError(selector string, matches []projectRow) error {
	var labels []string
	for i, match := range matches {
		if i >= 8 {
			labels = append(labels, "...")
			break
		}
		labels = append(labels, match.Label)
	}
	return fmt.Errorf("project %q is ambiguous; matches: %s", selector, strings.Join(labels, ", "))
}

func parseTargetCommandArgs(fs *flag.FlagSet, args []string) (string, error) {
	var flagArgs []string
	var positionals []string
	for i := 0; i < len(args); i++ {
		arg := args[i]
		if arg == "--" {
			positionals = append(positionals, args[i+1:]...)
			break
		}
		if !strings.HasPrefix(arg, "-") || arg == "-" {
			positionals = append(positionals, arg)
			continue
		}
		flagArgs = append(flagArgs, arg)
		name := strings.TrimLeft(arg, "-")
		if before, _, ok := strings.Cut(name, "="); ok {
			name = before
		}
		flagDef := fs.Lookup(name)
		if flagDef == nil || strings.Contains(arg, "=") {
			continue
		}
		if boolFlag, ok := flagDef.Value.(interface{ IsBoolFlag() bool }); ok && boolFlag.IsBoolFlag() {
			continue
		}
		if i+1 >= len(args) {
			return "", fmt.Errorf("flag needs an argument: -%s", name)
		}
		i++
		flagArgs = append(flagArgs, args[i])
	}
	if err := fs.Parse(flagArgs); err != nil {
		return "", err
	}
	if len(positionals) > 1 {
		return "", fmt.Errorf("expected one project selector, got %d", len(positionals))
	}
	if len(positionals) == 0 {
		return "", nil
	}
	return positionals[0], nil
}

func runInspect(args []string) error {
	cfg := commandConfig{}
	fs := flag.NewFlagSet("inspect", flag.ContinueOnError)
	fs.StringVar(&cfg.root, "root", DefaultSessionsRoot(), "Codex sessions directory to read")
	fs.BoolVar(&cfg.json, "json", false, "print JSON")
	if err := fs.Parse(args); err != nil {
		return err
	}
	target := fs.Arg(0)
	if target == "" {
		return fmt.Errorf("inspect requires a session id or rollout path")
	}
	session, err := findSession(cfg.root, target)
	if err != nil {
		return err
	}
	if cfg.json {
		return writeJSON(session)
	}
	printSessionDetail(session)
	return nil
}

func runFork(args []string) error {
	cfg := commandConfig{}
	fs := flag.NewFlagSet("fork", flag.ContinueOnError)
	fs.StringVar(&cfg.root, "root", DefaultSessionsRoot(), "Codex sessions directory to read")
	targetProvider := fs.String("to-provider", "", "target model provider id for the fork")
	model := fs.String("model", "", "target model override")
	codexBin := fs.String("codex", "codex", "codex executable")
	dryRun := fs.Bool("dry-run", false, "print the codex command without running it")
	if err := fs.Parse(args); err != nil {
		return err
	}
	target := fs.Arg(0)
	if target == "" {
		return fmt.Errorf("fork requires a session id or rollout path")
	}
	session, err := findSession(cfg.root, target)
	if err != nil {
		return err
	}
	if *targetProvider == "" {
		return fmt.Errorf("-to-provider is required")
	}
	return RunForkCommand(session.ID, ForkOptions{
		CodexBin:       *codexBin,
		TargetProvider: *targetProvider,
		Model:          *model,
		DryRun:         *dryRun,
	})
}

func runTrash(args []string) error {
	cfg := commandConfig{}
	fs := flag.NewFlagSet("trash", flag.ContinueOnError)
	fs.StringVar(&cfg.root, "root", DefaultSessionsRoot(), "Codex sessions directory to read")
	trashRoot := fs.String("trash-root", DefaultTrashRoot(), "directory where deleted sessions are moved")
	dryRun := fs.Bool("dry-run", false, "print the planned move without moving the file")
	if err := fs.Parse(args); err != nil {
		return err
	}
	target := fs.Arg(0)
	if target == "" {
		return fmt.Errorf("trash requires a session id or rollout path")
	}
	session, err := findSession(cfg.root, target)
	if err != nil {
		return err
	}
	record, err := MoveSessionToTrash(session, TrashOptions{
		Root:      cfg.root,
		TrashRoot: *trashRoot,
		DryRun:    *dryRun,
	})
	if err != nil {
		return err
	}
	if *dryRun {
		fmt.Printf("would move %s -> %s\n", record.OriginalPath, record.TrashPath)
		return nil
	}
	fmt.Printf("moved %s -> %s\n", record.OriginalPath, record.TrashPath)
	return nil
}

func runRecover(args []string) error {
	cfg := commandConfig{}
	fs := flag.NewFlagSet("recover", flag.ContinueOnError)
	fs.StringVar(&cfg.root, "root", DefaultSessionsRoot(), "Codex sessions directory to restore into")
	trashRoot := fs.String("trash-root", DefaultTrashRoot(), "directory where deleted sessions were moved")
	dryRun := fs.Bool("dry-run", false, "print the planned restore without moving the file")
	if err := fs.Parse(args); err != nil {
		return err
	}
	target := fs.Arg(0)
	if target == "" {
		return fmt.Errorf("recover requires a session id or trashed rollout path")
	}
	session, err := findSession(TrashSessionsRoot(*trashRoot), target)
	if err != nil {
		return err
	}
	record, err := RecoverSessionFromTrash(session, TrashOptions{
		Root:      cfg.root,
		TrashRoot: *trashRoot,
		DryRun:    *dryRun,
	})
	if err != nil {
		return err
	}
	if *dryRun {
		fmt.Printf("would restore %s -> %s\n", record.TrashPath, record.OriginalPath)
		return nil
	}
	fmt.Printf("restored %s -> %s\n", record.TrashPath, record.OriginalPath)
	return nil
}

func runRender(args []string) error {
	cfg := commandConfig{}
	fs := baseFlagSet("render", &cfg)
	width := fs.Int("width", 100, "render width for debug preview")
	height := fs.Int("height", 28, "render height for debug preview")
	view := fs.String("view", "browse", "debug view: browse, copy, detail, delete")
	targetProvider := fs.String("to-provider", "", "target provider shown in interactive fork hint")
	modelOverride := fs.String("model", "", "model shown in interactive fork hint")
	dryRun := fs.Bool("dry-run", true, "render with dry-run fork mode")
	if err := fs.Parse(args); err != nil {
		return err
	}
	sessions, err := scanAndFilter(cfg)
	if err != nil {
		return err
	}
	configProviders, _, err := ReadConfigProviders(cfg.config)
	if err != nil {
		return err
	}
	model := NewUIModel(sessions, cfg.root, FilterOptions{
		Provider:         cfg.provider,
		CWD:              cfg.cwd,
		Source:           cfg.source,
		Query:            cfg.query,
		IncludeSubagents: cfg.includeSubagents,
	}, configProviders)
	model.fork = ForkOptions{
		TargetProvider: *targetProvider,
		Model:          *modelOverride,
		DryRun:         *dryRun,
	}
	model.trash = TrashOptions{
		Root:   cfg.root,
		DryRun: true,
	}
	model.width = *width
	model.height = *height
	switch *view {
	case "copy":
		sessions := model.filteredSessions()
		if len(sessions) > 0 {
			session := sessions[0]
			model.pendingCopy = &session
		}
		model.mode = modeCopy
	case "detail":
		sessions := model.filteredSessions()
		if len(sessions) > 0 {
			session := sessions[0]
			model.detail = &session
			messages, err := ReadLastUserMessages(session.Path, 10)
			if err == nil {
				model.detailMsgs = messages
			}
		}
		model.mode = modeDetail
	case "delete":
		sessions := model.filteredSessions()
		if len(sessions) > 0 {
			session := sessions[0]
			model.pendingDelete = &session
		}
		model.mode = modeDeleteConfirm
	case "browse":
	default:
		return fmt.Errorf("unknown render view %q", *view)
	}
	fmt.Print(model.View())
	return nil
}

func scanAndFilter(cfg commandConfig) ([]Session, error) {
	sessions, err := ScanSessions(ScanOptions{Root: cfg.root})
	if err != nil {
		return nil, err
	}
	return FilterSessions(sessions, FilterOptions{
		Provider:         cfg.provider,
		CWD:              cfg.cwd,
		Source:           cfg.source,
		Query:            cfg.query,
		IncludeSubagents: cfg.includeSubagents,
	}), nil
}

func findSession(root, target string) (Session, error) {
	if strings.Contains(target, string(os.PathSeparator)) || strings.HasSuffix(target, rolloutSuffix) {
		return readSession(target, defaultMaxPreviewLines)
	}
	sessions, err := ScanSessions(ScanOptions{Root: root})
	if err != nil {
		return Session{}, err
	}
	for _, session := range sessions {
		if session.ID == target || strings.HasPrefix(session.ID, target) {
			return session, nil
		}
	}
	return Session{}, fmt.Errorf("session %q not found under %s", target, root)
}

func printSessionTable(sessions []Session) {
	fmt.Printf("%-19s  %-16s  %-18s  %-12s  %-36s  %s\n", "updated", "provider", "source", "branch", "id", "preview")
	for _, session := range sessions {
		fmt.Printf("%-19s  %-16s  %-18s  %-12s  %-36s  %s\n",
			formatTime(session.UpdatedAt),
			trunc(session.ModelProvider, 16),
			trunc(session.DisplaySource(), 18),
			trunc(session.GitBranch, 12),
			trunc(session.ID, 36),
			trunc(session.Preview, 80),
		)
	}
}

func printProjectTable(rows []projectRow) {
	fmt.Printf("%-19s  %5s  %-28s  %-48s  %s\n", "updated", "count", "providers", "label", "cwd")
	for _, row := range rows {
		fmt.Printf("%-19s  %5d  %-28s  %-48s  %s\n",
			formatTime(row.UpdatedAt),
			row.Count,
			trunc(formatProviderCounts(row.Providers), 28),
			trunc(row.Label, 48),
			row.CWD,
		)
	}
}

func printProviderTable(rows []projectProviderRow) {
	fmt.Printf("%-24s  %5s\n", "provider", "count")
	for _, row := range rows {
		fmt.Printf("%-24s  %5d\n", row.Provider, row.Count)
	}
}

func formatProviderCounts(rows []projectProviderRow) string {
	parts := make([]string, 0, len(rows))
	for _, row := range rows {
		parts = append(parts, fmt.Sprintf("%s:%d", row.Provider, row.Count))
	}
	return strings.Join(parts, ", ")
}

func printSessionDetail(session Session) {
	fmt.Println("ID:             " + session.ID)
	fmt.Println("Forked from:    " + session.ForkedFromID)
	fmt.Println("Provider:       " + session.ModelProvider)
	fmt.Println("Source:         " + session.DisplaySource())
	fmt.Println("Thread source:  " + session.ThreadSource)
	fmt.Println("Non-root:       " + strconv.FormatBool(session.NonRootAgent))
	fmt.Println("Created:        " + formatTime(session.CreatedAt))
	fmt.Println("Updated:        " + formatTime(session.UpdatedAt))
	fmt.Println("CWD:            " + session.CWD)
	fmt.Println("Git branch:     " + session.GitBranch)
	fmt.Println("Git SHA:        " + session.GitSHA)
	fmt.Println("Git origin:     " + session.GitOriginURL)
	fmt.Println("CLI version:    " + session.CLIVersion)
	fmt.Println("Line count:     " + strconv.Itoa(session.LineCount))
	fmt.Println("Path:           " + filepath.Clean(session.Path))
	if session.ParseWarning != "" {
		fmt.Println("Warning:        " + session.ParseWarning)
	}
	fmt.Println()
	fmt.Println(session.Preview)
}

func writeJSON(value any) error {
	encoder := json.NewEncoder(os.Stdout)
	encoder.SetIndent("", "  ")
	return encoder.Encode(value)
}

func printUsage() {
	fmt.Println(`session-manager browses Codex rollout sessions. Delete/recover move files only when those commands are used.

Usage:
  session-manager ui [flags]
  session-manager list [flags]
  session-manager providers [flags]
  session-manager projects [list] [flags]
  session-manager projects sessions [flags] <project-cwd-or-label>
  session-manager projects providers [flags] <project-cwd-or-label>
  session-manager inspect [flags] <session-id-or-path>
  session-manager fork -to-provider <provider> [flags] <session-id-or-path>
  session-manager trash [flags] <session-id-or-path>
  session-manager recover [flags] <session-id-or-path>
  session-manager render [flags]

Useful flags:
  -root       sessions directory (default ~/.codex/sessions)
  -provider   model provider id filter
  -cwd        working directory filter
  -source     session source filter
  -include-subagents
              show subagent/internal non-root sessions; hidden by default
  -query      substring search
  -json       machine-readable output for list/providers/projects/inspect
  -to-provider target provider for fork and interactive fork
  -model      optional model override for forked sessions
  -dry-run    print fork command instead of running it
  -trash-root directory for reversible deletes (default ~/.codex/session-manager-trash)

Examples:
  session-manager providers
  session-manager projects -json
  session-manager projects sessions -provider openai "~/Code/project"
  session-manager projects providers "~/Code/project"
  session-manager list -provider openai -limit 10
  session-manager fork -to-provider zaixiao_s2a -dry-run <session-id>
  session-manager render -provider openai -width 120 -height 32
  session-manager ui -root ~/.codex/sessions`)
}

func printProjectsUsage() {
	fmt.Println(`session-manager projects lists and expands project-level session groups.

Usage:
  session-manager projects [list] [flags]
  session-manager projects sessions [flags] <project-cwd-or-label>
  session-manager projects providers [flags] <project-cwd-or-label>

Subcommands:
  list       list projects by most recent visible session
  sessions   list sessions under one project
  providers  list provider counts under one project

Project selectors accept an absolute cwd, a ~-relative label, or an unambiguous substring.

Useful flags:
  -root       sessions directory (default ~/.codex/sessions)
  -provider   model provider id filter
  -source     session source filter
  -include-subagents
              show subagent/internal non-root sessions; hidden by default
  -query      substring search
  -limit      maximum rows to print
  -json       machine-readable output`)
}

func formatTime(t time.Time) string {
	if t.IsZero() {
		return ""
	}
	return t.Local().Format("2006-01-02 15:04:05")
}

func trunc(text string, width int) string {
	runes := []rune(text)
	if len(runes) <= width {
		return text
	}
	if width <= 3 {
		return string(runes[:width])
	}
	return string(runes[:width-3]) + "..."
}
