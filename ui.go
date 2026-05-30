package main

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"github.com/charmbracelet/bubbles/textinput"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/mattn/go-runewidth"
)

type mode int

const (
	modeBrowse mode = iota
	modeSearch
	modeDetail
	modeCopy
	modeDeleteConfirm
)

type columnFocus int

const (
	focusProject columnFocus = iota
	focusProvider
	focusSession
	focusTargetProvider
)

type UIModel struct {
	allSessions     []Session
	projects        []ProjectSummary
	providers       []string
	configProviders []string
	root            string
	fork            ForkOptions
	trash           TrashOptions
	search          textinput.Model
	mode            mode
	focus           columnFocus
	width           int
	height          int

	projectIdx        int
	providerIdx       int
	sessionIdx        int
	targetProviderIdx int

	pendingCopy   *Session
	pendingDelete *Session
	detail        *Session
	detailMsgs    []UserMessage
	statusMsg     string
}

var (
	titleStyle    = lipgloss.NewStyle().Bold(true)
	headerStyle   = lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("39"))
	statusStyle   = lipgloss.NewStyle().Foreground(lipgloss.Color("8"))
	selectedStyle = lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("39"))
	dimStyle      = lipgloss.NewStyle().Foreground(lipgloss.Color("8"))
	keyStyle      = lipgloss.NewStyle().Foreground(lipgloss.Color("8"))
)

func NewUIModel(sessions []Session, root string, filters FilterOptions, configProviders []string) UIModel {
	projects := ProjectSummaries(sessions)
	projectIdx := firstProjectIndex(projects, filters.CWD)
	providers := providersForProject(sessions, projectCWD(projects, projectIdx))
	providerIdx := providerIndex(providers, filters.Provider)
	filtered := sessionsForSelection(sessions, projects, projectIdx, providers, providerIdx, filters.Query)
	forkProviders := MergeProviders(configProviders, UniqueProviders(sessions))

	search := textinput.New()
	search.Placeholder = "search sessions"
	search.SetValue(filters.Query)

	return UIModel{
		allSessions:     sessions,
		projects:        projects,
		providers:       providers,
		configProviders: forkProviders,
		root:            root,
		fork:            ForkOptions{},
		search:          search,
		projectIdx:      projectIdx,
		providerIdx:     providerIdx,
		sessionIdx:      clampIndex(0, len(filtered)),
		focus:           focusProject,
	}
}

func (m UIModel) Init() tea.Cmd {
	return nil
}

func (m UIModel) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.width = msg.Width
		m.height = msg.Height
	case forkDoneMsg:
		if msg.err != nil {
			m.statusMsg = "copy failed: " + msg.err.Error()
		} else {
			m.statusMsg = "copy command completed"
		}
	case trashDoneMsg:
		if msg.err != nil {
			m.statusMsg = "delete failed: " + msg.err.Error()
		} else {
			m.removeSessionByPath(msg.sourcePath)
			m.refreshSelectionAfterDelete(msg.selection)
			m.statusMsg = "moved to trash: " + msg.path
		}
	case detailLoadedMsg:
		if msg.err != nil {
			m.statusMsg = "failed to read detail messages: " + msg.err.Error()
		} else {
			m.detailMsgs = msg.messages
		}
	case tea.KeyMsg:
		return m.handleKey(msg)
	}
	return m, nil
}

func (m UIModel) handleKey(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	if m.mode == modeSearch {
		switch msg.String() {
		case "esc", "enter":
			m.mode = modeBrowse
			m.search.Blur()
			m.sessionIdx = 0
			return m, nil
		}
		var cmd tea.Cmd
		m.search, cmd = m.search.Update(msg)
		m.sessionIdx = clampIndex(m.sessionIdx, len(m.filteredSessions()))
		return m, cmd
	}

	switch m.mode {
	case modeDetail:
		switch msg.String() {
		case "esc", "left", "h", "q", "backspace":
			m.mode = modeBrowse
			m.detail = nil
			m.detailMsgs = nil
		case "up", "k":
			return m.moveDetailSelection(-1)
		case "down", "j":
			return m.moveDetailSelection(1)
		case "f", "F":
			return m.openCopy()
		case "d", "D":
			return m.openDeleteConfirm()
		}
		return m, nil
	case modeCopy:
		switch msg.String() {
		case "esc", "left", "h", "j", "q", "backspace":
			m.mode = modeBrowse
			m.pendingCopy = nil
		case "up", "k":
			m.targetProviderIdx = wrapIndex(m.targetProviderIdx-1, len(m.configProviders))
		case "down", "l":
			m.targetProviderIdx = wrapIndex(m.targetProviderIdx+1, len(m.configProviders))
		case "enter":
			return m.runCopy()
		}
		return m, nil
	case modeDeleteConfirm:
		switch msg.String() {
		case "esc", "n", "N", "q", "backspace":
			m.mode = modeBrowse
			m.pendingDelete = nil
			m.statusMsg = "delete cancelled"
		case "enter", "y", "Y":
			return m.runDelete()
		}
		return m, nil
	}

	switch msg.String() {
	case "q", "ctrl+c":
		return m, tea.Quit
	case "/":
		m.mode = modeSearch
		m.search.Focus()
		return m, textinput.Blink
	case "left", "h":
		m.moveFocus(-1)
	case "right", "l":
		m.moveFocus(1)
	case "up", "k":
		m.moveSelection(-1)
	case "down", "j":
		m.moveSelection(1)
	case "enter":
		if m.focus == focusSession {
			return m.openDetail()
		}
	case "f", "F":
		if m.focus == focusSession {
			return m.openCopy()
		}
	case "d", "D":
		if m.focus == focusSession {
			return m.openDeleteConfirm()
		}
	case "c":
		m.projectIdx = 0
		m.providers = providersForProject(m.allSessions, "")
		m.providerIdx = 0
		m.sessionIdx = 0
		m.search.SetValue("")
		m.statusMsg = "filters cleared"
	}
	return m, nil
}

func (m *UIModel) moveFocus(delta int) {
	next := int(m.focus) + delta
	if next < int(focusProject) {
		next = int(focusProject)
	}
	if next > int(focusSession) {
		next = int(focusSession)
	}
	m.focus = columnFocus(next)
}

func (m *UIModel) moveSelection(delta int) {
	switch m.focus {
	case focusProject:
		m.projectIdx = wrapIndex(m.projectIdx+delta, len(m.projects)+1)
		m.providers = providersForProject(m.allSessions, projectCWD(m.projects, m.projectIdx))
		m.providerIdx = 0
		m.sessionIdx = 0
	case focusProvider:
		m.providerIdx = wrapIndex(m.providerIdx+delta, len(m.providers)+1)
		m.sessionIdx = 0
	case focusSession:
		m.sessionIdx = wrapIndex(m.sessionIdx+delta, len(m.filteredSessions()))
	}
}

func (m UIModel) View() string {
	if m.width == 0 {
		m.width = 110
	}
	if m.height == 0 {
		m.height = 28
	}
	header := headerStyle.Render("Codex Sessions")
	status := statusStyle.Render(fmt.Sprintf(
		"root=%s  project=%s  provider=%s  query=%q  shown=%d/%d",
		m.root,
		filterLabel(projectCWD(m.projects, m.projectIdx)),
		selectedProvider(m.providers, m.providerIdx),
		m.search.Value(),
		len(m.filteredSessions()),
		len(m.allSessions),
	))
	help := "HJKL/arrows navigate | Enter detail | F copy/fork | D delete | / search | c clear | q quit"
	switch m.mode {
	case modeSearch:
		help = m.search.View()
	case modeDetail:
		help = "detail: j/k move sessions | f/F copy/fork | ESC/left/h/q return"
	case modeCopy:
		help = "copy: choose target provider with up/down or k/l, Enter copy, ESC/left/h/j return"
	case modeDeleteConfirm:
		help = "delete: y/Enter move to trash, n/Esc cancel"
	}
	if m.statusMsg != "" && m.mode == modeBrowse {
		help = m.statusMsg
	}
	body := m.browseView()
	switch m.mode {
	case modeDetail:
		body = m.detailView()
	case modeCopy:
		body = m.copyView()
	case modeDeleteConfirm:
		body = lipgloss.JoinVertical(lipgloss.Left, m.browseView(), "", m.deleteConfirmView())
	}
	return lipgloss.JoinVertical(lipgloss.Left, header, status, statusStyle.Render(help), "", body)
}

func (m UIModel) deleteConfirmView() string {
	if m.pendingDelete == nil {
		return ""
	}
	session := *m.pendingDelete
	lines := []string{
		headerStyle.Render("Delete session?"),
		"This moves the rollout JSONL to trash and keeps its relative path for recovery.",
		field("ID", session.ID),
		field("Provider", session.ModelProvider),
		field("Project", HomeRelativePath(session.CWD)),
		field("Trash", filepath.Join(TrashSessionsRoot(m.trash.TrashRoot), mustRelativePath(m.root, session.Path))),
		"",
		"Press y or Enter to delete, n or Esc to cancel.",
	}
	width := min(max(60, m.width-4), m.width)
	return lipgloss.NewStyle().
		Width(width).
		Border(lipgloss.NormalBorder()).
		BorderForeground(lipgloss.Color("196")).
		Render(strings.Join(lines, "\n"))
}

func (m UIModel) browseView() string {
	innerHeight := max(6, m.height-5)
	projectW := max(24, m.width/4)
	providerW := max(18, m.width/5)
	sessionW := max(30, m.width-projectW-providerW-6)
	return lipgloss.JoinHorizontal(
		lipgloss.Top,
		m.column("Project", m.projectRows(), int(m.focus) == int(focusProject), projectW, innerHeight),
		"  ",
		m.column("Provider", m.providerRows(), int(m.focus) == int(focusProvider), providerW, innerHeight),
		"  ",
		m.column("Session", m.sessionRows(m.filteredSessions()), int(m.focus) == int(focusSession), sessionW, innerHeight),
	)
}

func (m UIModel) copyView() string {
	innerHeight := max(6, m.height-5)
	providerW := max(22, m.width/4)
	sessionW := max(30, m.width/2)
	targetW := max(22, m.width-providerW-sessionW-6)
	sourceProvider := ""
	if m.pendingCopy != nil {
		sourceProvider = m.pendingCopy.ModelProvider
	}
	return lipgloss.JoinHorizontal(
		lipgloss.Top,
		m.column("Provider", []row{{Text: sourceProvider, Selected: true}}, false, providerW, innerHeight),
		"  ",
		m.column("Session", m.sessionRowsForCopy(), false, sessionW, innerHeight),
		"  ",
		m.column("Target Provider", m.targetProviderRows(), true, targetW, innerHeight),
	)
}

func (m UIModel) detailView() string {
	innerHeight := max(6, m.height-5)
	leftW := max(38, m.width/2-2)
	rightW := max(38, m.width-leftW-4)
	return lipgloss.JoinHorizontal(
		lipgloss.Top,
		m.column("Session", m.sessionRows(m.filteredSessions()), true, leftW, innerHeight),
		"  ",
		m.detailColumn(rightW, innerHeight),
	)
}

type row struct {
	Text     string
	Selected bool
	Dim      bool
}

func (m UIModel) projectRows() []row {
	rows := []row{{Text: fmt.Sprintf("All projects (%d)", len(m.allSessions)), Selected: m.projectIdx == 0}}
	for i, project := range m.projects {
		rows = append(rows, row{
			Text:     fmt.Sprintf("%s (%d)", project.Label, project.Count),
			Selected: i+1 == m.projectIdx,
		})
	}
	return rows
}

func (m UIModel) providerRows() []row {
	rows := []row{{Text: fmt.Sprintf("All providers (%d)", len(sessionsForSelection(m.allSessions, m.projects, m.projectIdx, nil, 0, ""))), Selected: m.providerIdx == 0}}
	for i, provider := range m.providers {
		count := len(FilterSessions(m.allSessions, FilterOptions{CWD: projectCWD(m.projects, m.projectIdx), Provider: provider, IncludeSubagents: true}))
		rows = append(rows, row{Text: fmt.Sprintf("%s (%d)", provider, count), Selected: i+1 == m.providerIdx})
	}
	return rows
}

func (m UIModel) sessionRows(sessions []Session) []row {
	rows := make([]row, 0, len(sessions))
	for i, session := range sessions {
		rows = append(rows, row{
			Text:     fmt.Sprintf("%s  %s  %s", shortProvider(session.ModelProvider), formatTime(session.UpdatedAt), truncWidth(session.Preview, 90)),
			Selected: i == m.sessionIdx,
		})
	}
	if len(rows) == 0 {
		rows = append(rows, row{Text: "No sessions", Dim: true})
	}
	return rows
}

func (m UIModel) sessionRowsForCopy() []row {
	if m.pendingCopy == nil {
		return []row{{Text: "No session selected", Dim: true}}
	}
	return []row{{Text: fmt.Sprintf("%s  %s", m.pendingCopy.ID, truncWidth(m.pendingCopy.Preview, 120)), Selected: true}}
}

func (m UIModel) targetProviderRows() []row {
	rows := make([]row, 0, len(m.configProviders))
	for i, provider := range m.configProviders {
		rows = append(rows, row{Text: provider, Selected: i == m.targetProviderIdx})
	}
	if len(rows) == 0 {
		rows = append(rows, row{Text: "No providers", Dim: true})
	}
	return rows
}

func (m UIModel) column(title string, rows []row, focused bool, width int, height int) string {
	style := lipgloss.NewStyle().Width(width).Height(height)
	if focused {
		style = style.Border(lipgloss.NormalBorder()).BorderForeground(lipgloss.Color("39"))
	} else {
		style = style.Border(lipgloss.NormalBorder()).BorderForeground(lipgloss.Color("8"))
	}
	visible := max(1, height-3)
	start := selectedStart(rows, visible)
	lines := []string{headerStyle.Render(title)}
	for _, row := range rows[start:min(len(rows), start+visible)] {
		text := truncWidth(row.Text, max(4, width-4))
		switch {
		case row.Selected:
			lines = append(lines, selectedStyle.Render("> "+text))
		case row.Dim:
			lines = append(lines, dimStyle.Render("  "+text))
		default:
			lines = append(lines, "  "+text)
		}
	}
	return style.Render(strings.Join(lines, "\n"))
}

func selectedStart(rows []row, visible int) int {
	selected := 0
	for i, row := range rows {
		if row.Selected {
			selected = i
			break
		}
	}
	if selected < visible {
		return 0
	}
	return selected - visible + 1
}

func (m UIModel) detailColumn(width int, height int) string {
	var lines []string
	if m.detail == nil {
		return ""
	}
	s := *m.detail
	lines = append(lines,
		headerStyle.Render("Details"),
		field("ID", s.ID),
		field("Forked", s.ForkedFromID),
		field("Provider", s.ModelProvider),
		field("Source", s.DisplaySource()),
		field("Thread", s.ThreadSource),
		field("Non-root", fmt.Sprintf("%t", s.NonRootAgent)),
		field("Created", formatTime(s.CreatedAt)),
		field("Updated", formatTime(s.UpdatedAt)),
		field("Project", HomeRelativePath(s.CWD)),
		field("Path", s.Path),
		"",
		headerStyle.Render("Last 10 user messages"),
	)
	if len(m.detailMsgs) == 0 {
		lines = append(lines, dimStyle.Render("No user messages found"))
	}
	for _, msg := range m.detailMsgs {
		lines = append(lines, fmt.Sprintf("L%d  %s", msg.Line, msg.Message))
	}
	return lipgloss.NewStyle().
		Width(width).
		Height(height).
		Border(lipgloss.NormalBorder()).
		BorderForeground(lipgloss.Color("8")).
		Render(strings.Join(lines, "\n"))
}

func (m UIModel) openDetail() (UIModel, tea.Cmd) {
	sessions := m.filteredSessions()
	if len(sessions) == 0 {
		m.statusMsg = "no session selected"
		return m, nil
	}
	session := sessions[clampIndex(m.sessionIdx, len(sessions))]
	m.detail = &session
	m.mode = modeDetail
	m.statusMsg = ""
	return m, func() tea.Msg {
		msgs, err := ReadLastUserMessages(session.Path, 10)
		return detailLoadedMsg{messages: msgs, err: err}
	}
}

func (m UIModel) moveDetailSelection(delta int) (UIModel, tea.Cmd) {
	sessions := m.filteredSessions()
	if len(sessions) == 0 {
		m.detail = nil
		m.detailMsgs = nil
		return m, nil
	}
	m.sessionIdx = wrapIndex(m.sessionIdx+delta, len(sessions))
	session := sessions[m.sessionIdx]
	m.detail = &session
	m.detailMsgs = nil
	m.statusMsg = ""
	return m, func() tea.Msg {
		msgs, err := ReadLastUserMessages(session.Path, 10)
		return detailLoadedMsg{messages: msgs, err: err}
	}
}

type detailLoadedMsg struct {
	messages []UserMessage
	err      error
}

func (m UIModel) openCopy() (UIModel, tea.Cmd) {
	sessions := m.filteredSessions()
	if len(sessions) == 0 {
		m.statusMsg = "no session selected"
		return m, nil
	}
	session := sessions[clampIndex(m.sessionIdx, len(sessions))]
	m.pendingCopy = &session
	m.mode = modeCopy
	m.targetProviderIdx = firstProviderIndex(m.configProviders, session.ModelProvider)
	m.statusMsg = ""
	if m.fork.TargetProvider != "" {
		return m.runCopyForProvider(m.fork.TargetProvider)
	}
	return m, nil
}

func (m UIModel) openDeleteConfirm() (UIModel, tea.Cmd) {
	sessions := m.filteredSessions()
	if len(sessions) == 0 {
		m.statusMsg = "no session selected"
		return m, nil
	}
	session := sessions[clampIndex(m.sessionIdx, len(sessions))]
	m.pendingDelete = &session
	m.mode = modeDeleteConfirm
	m.statusMsg = ""
	return m, nil
}

func (m UIModel) runDelete() (UIModel, tea.Cmd) {
	if m.pendingDelete == nil {
		m.statusMsg = "no session selected"
		m.mode = modeBrowse
		return m, nil
	}
	session := *m.pendingDelete
	opts := m.trash
	m.mode = modeBrowse
	m.pendingDelete = nil
	if opts.DryRun {
		record, err := MoveSessionToTrash(session, opts)
		if err != nil {
			m.statusMsg = err.Error()
			return m, nil
		}
		m.statusMsg = "would move to trash: " + record.TrashPath
		return m, nil
	}
	m.statusMsg = "moving to trash: " + session.ID
	selection := selectionSnapshot{
		projectCWD: projectCWD(m.projects, m.projectIdx),
		provider:   selectedProviderValue(m.providers, m.providerIdx),
		focus:      m.focus,
	}
	return m, func() tea.Msg {
		record, err := MoveSessionToTrash(session, opts)
		return trashDoneMsg{path: record.TrashPath, sourcePath: session.Path, selection: selection, err: err}
	}
}

type selectionSnapshot struct {
	projectCWD string
	provider   string
	focus      columnFocus
}

type trashDoneMsg struct {
	path       string
	sourcePath string
	selection  selectionSnapshot
	err        error
}

func (m UIModel) runCopy() (UIModel, tea.Cmd) {
	if len(m.configProviders) == 0 {
		m.statusMsg = "no target providers"
		m.mode = modeBrowse
		return m, nil
	}
	return m.runCopyForProvider(m.configProviders[m.targetProviderIdx])
}

func (m UIModel) runCopyForProvider(provider string) (UIModel, tea.Cmd) {
	if m.pendingCopy == nil {
		m.statusMsg = "no session selected"
		return m, nil
	}
	opts := m.fork
	opts.TargetProvider = provider
	args, err := BuildForkCommand(m.pendingCopy.ID, opts)
	if err != nil {
		m.statusMsg = err.Error()
		m.mode = modeBrowse
		return m, nil
	}
	m.mode = modeBrowse
	m.pendingCopy = nil
	if opts.DryRun {
		m.statusMsg = shellQuote(args)
		return m, nil
	}
	m.statusMsg = "running: " + shellQuote(args)
	return m, tea.ExecProcess(commandForArgs(args), func(err error) tea.Msg {
		return forkDoneMsg{err: err}
	})
}

type forkDoneMsg struct {
	err error
}

func commandForArgs(args []string) *exec.Cmd {
	cmd := exec.Command(args[0], args[1:]...)
	cmd.Stdin = os.Stdin
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	return cmd
}

func (m *UIModel) removeSessionByPath(path string) {
	filtered := m.allSessions[:0]
	for _, session := range m.allSessions {
		if filepath.Clean(session.Path) != filepath.Clean(path) {
			filtered = append(filtered, session)
		}
	}
	m.allSessions = filtered
}

func (m *UIModel) refreshSelectionAfterDelete(selection selectionSnapshot) {
	m.projects = ProjectSummaries(m.allSessions)
	m.projectIdx = firstProjectIndex(m.projects, selection.projectCWD)
	m.providers = providersForProject(m.allSessions, projectCWD(m.projects, m.projectIdx))
	m.providerIdx = providerIndex(m.providers, selection.provider)
	switch {
	case selection.projectCWD != "" && m.projectIdx == 0:
		m.focus = focusProject
		m.providerIdx = 0
	case selection.provider != "" && m.providerIdx == 0:
		m.focus = focusProject
	default:
		m.focus = selection.focus
	}
	m.sessionIdx = clampIndex(m.sessionIdx, len(m.filteredSessions()))
}

func (m UIModel) filteredSessions() []Session {
	return sessionsForSelection(m.allSessions, m.projects, m.projectIdx, m.providers, m.providerIdx, m.search.Value())
}

func sessionsForSelection(all []Session, projects []ProjectSummary, projectIdx int, providers []string, providerIdx int, query string) []Session {
	provider := selectedProviderValue(providers, providerIdx)
	return FilterSessions(all, FilterOptions{
		CWD:              projectCWD(projects, projectIdx),
		Provider:         provider,
		Query:            query,
		IncludeSubagents: true,
	})
}

func providersForProject(sessions []Session, cwd string) []string {
	filtered := sessions
	if cwd != "" {
		filtered = FilterSessions(sessions, FilterOptions{CWD: cwd, IncludeSubagents: true})
	}
	return UniqueProviders(filtered)
}

func projectCWD(projects []ProjectSummary, idx int) string {
	if idx <= 0 || idx > len(projects) {
		return ""
	}
	return projects[idx-1].CWD
}

func selectedProvider(providers []string, idx int) string {
	value := selectedProviderValue(providers, idx)
	if value == "" {
		return "all"
	}
	return value
}

func selectedProviderValue(providers []string, idx int) string {
	if idx <= 0 || idx > len(providers) {
		return ""
	}
	return providers[idx-1]
}

func field(key, value string) string {
	return keyStyle.Render(fmt.Sprintf("%-10s", key+":")) + value
}

func firstProjectIndex(projects []ProjectSummary, preferred string) int {
	if preferred == "" {
		return 0
	}
	for i, project := range projects {
		if samePath(project.CWD, preferred) {
			return i + 1
		}
	}
	return 0
}

func firstProviderIndex(providers []string, preferred string) int {
	if len(providers) == 0 {
		return 0
	}
	if preferred != "" {
		for i, provider := range providers {
			if provider == preferred {
				return i
			}
		}
	}
	return 0
}

func providerIndex(providers []string, preferred string) int {
	if preferred == "" {
		return 0
	}
	for i, provider := range providers {
		if provider == preferred {
			return i + 1
		}
	}
	return 0
}

func filterLabel(value string) string {
	if strings.TrimSpace(value) == "" {
		return "all"
	}
	return HomeRelativePath(value)
}

func clampIndex(idx int, length int) int {
	if length <= 0 {
		return 0
	}
	if idx < 0 {
		return 0
	}
	if idx >= length {
		return length - 1
	}
	return idx
}

func wrapIndex(idx int, length int) int {
	if length <= 0 {
		return 0
	}
	for idx < 0 {
		idx += length
	}
	return idx % length
}

func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}

func max(a, b int) int {
	if a > b {
		return a
	}
	return b
}

func shortProvider(value string) string {
	if value == "" {
		return "(missing)"
	}
	return trunc(value, 18)
}

func mustRelativePath(root, path string) string {
	rel, err := safeRelativePath(root, path)
	if err != nil {
		return filepath.Base(path)
	}
	return rel
}

func truncWidth(text string, width int) string {
	if width <= 0 {
		return ""
	}
	if runewidth.StringWidth(text) <= width {
		return text
	}
	if width <= 3 {
		return runewidth.Truncate(text, width, "")
	}
	return runewidth.Truncate(text, width, "...")
}
