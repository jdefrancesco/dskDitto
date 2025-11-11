package ui

import (
	"fmt"
	"math/rand" //gosec:
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"
	"unicode"

	"ditto/internal/dfs"
	"ditto/internal/dmap"
	"ditto/internal/dsklog"

	"ditto/pkg/utils"

	tea "github.com/charmbracelet/bubbletea"
)

// LaunchTUI builds and runs the Bubble Tea program that visualizes duplicate files.
func LaunchTUI(dMap *dmap.Dmap) {
	if dMap == nil {
		dsklog.Dlogger.Warn("nil duplicate map supplied to LaunchTUI")
		return
	}

	program := tea.NewProgram(newModel(dMap), tea.WithAltScreen())
	setCurrentProgram(program)
	defer clearCurrentProgram(program)

	if err := program.Start(); err != nil {
		panic(err)
	}
}

// StopTUI signals the currently running Bubble Tea program (if any) to quit.
func StopTUI() {
	programMu.Lock()
	defer programMu.Unlock()
	if currentProgram != nil {
		currentProgram.Quit()
	}
}

var (
	programMu      sync.Mutex
	currentProgram *tea.Program
)

func setCurrentProgram(p *tea.Program) {
	programMu.Lock()
	defer programMu.Unlock()
	currentProgram = p
}

func clearCurrentProgram(p *tea.Program) {
	programMu.Lock()
	defer programMu.Unlock()
	if currentProgram == p {
		currentProgram = nil
	}
}

type viewMode int

const (
	modeTree viewMode = iota
	modeConfirm
)

type nodeType int

const (
	nodeGroup nodeType = iota
	nodeFile
)

type fileStatus int

const (
	fileStatusPending fileStatus = iota
	fileStatusDeleted
	fileStatusError
)

type fileEntry struct {
	Path    string
	Marked  bool
	Status  fileStatus
	Message string
}

type duplicateGroup struct {
	Hash     dmap.SHA256Hash
	Title    string
	Files    []*fileEntry
	Expanded bool
}

type nodeRef struct {
	typ   nodeType
	group int
	file  int
}

type model struct {
	groups  []*duplicateGroup
	visible []nodeRef
	cursor  int

	mode viewMode

	confirmCode  string
	confirmInput string
	confirmError string

	deleteResult string

	width  int
	height int
}

var _ tea.Model = (*model)(nil)

func newModel(dMap *dmap.Dmap) *model {
	m := &model{
		mode: modeTree,
	}

	for hash, files := range dMap.GetMap() {
		if len(files) <= 1 {
			continue
		}

		group := &duplicateGroup{
			Hash:     hash,
			Title:    formatGroupTitle(hash, files),
			Expanded: true,
		}

		for _, file := range files {
			group.Files = append(group.Files, &fileEntry{Path: file})
		}

		autoMarkGroup(group)
		m.groups = append(m.groups, group)
	}

	m.rebuildVisibleNodes()
	return m
}

func (m *model) Init() tea.Cmd {
	return nil
}

func (m *model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.KeyMsg:
		if m.mode == modeConfirm {
			return m.handleConfirmKeys(msg)
		}
		return m.handleTreeKeys(msg)
	case tea.WindowSizeMsg:
		m.width = msg.Width
		m.height = msg.Height
	}
	return m, nil
}

func (m *model) View() string {
	if m.mode == modeConfirm {
		return m.renderConfirmView()
	}
	return m.renderTreeView()
}

func (m *model) handleTreeKeys(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch msg.String() {
	case "ctrl+c", "esc", "q":
		return m, tea.Quit
	case "up", "k":
		m.moveCursor(-1)
	case "down", "j":
		m.moveCursor(1)
	case "left", "h":
		m.collapseCurrentGroup()
	case "right", "l":
		m.expandCurrentGroup()
	case "enter":
		m.toggleCurrentGroup()
	case "m":
		m.toggleCurrentFileMark()
	case "d":
		m.startConfirmationPrompt()
	}
	return m, nil
}

func (m *model) handleConfirmKeys(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch msg.Type {
	case tea.KeyEsc:
		m.mode = modeTree
		m.confirmError = ""
		m.confirmInput = ""
	case tea.KeyEnter:
		if m.confirmInput == m.confirmCode {
			m.processDeletion()
		} else {
			m.confirmError = "Incorrect code. Try again."
			m.confirmInput = ""
		}
	case tea.KeyBackspace:
		if len(m.confirmInput) > 0 {
			m.confirmInput = m.confirmInput[:len(m.confirmInput)-1]
		}
	case tea.KeyRunes:
		if len(m.confirmInput) >= len(m.confirmCode) {
			return m, nil
		}
		for _, r := range msg.Runes {
			if isAlphaNumeric(r) {
				m.confirmInput += string(r)
			}
		}
	}
	return m, nil
}

func (m *model) renderTreeView() string {
	var b strings.Builder

	title := "dskDitto: Interactive Duplicate Management [enter=expand/collapse, m=mark, d=delete, q=quit]"
	b.WriteString(title)
	b.WriteString("\n")
	b.WriteString(strings.Repeat("-", min(len(title), 80)))
	b.WriteString("\n\n")

	if len(m.visible) == 0 {
		b.WriteString("No duplicate groups found. Press q to exit.\n")
		return b.String()
	}

	for i, ref := range m.visible {
		selected := i == m.cursor
		switch ref.typ {
		case nodeGroup:
			group := m.groups[ref.group]
			indicator := "[+]"
			if group.Expanded {
				indicator = "[-]"
			}
			cursor := " "
			if selected {
				cursor = ">"
			}
			fmt.Fprintf(&b, "%s %s %s\n", cursor, indicator, group.Title)
		case nodeFile:
			entry := m.groups[ref.group].Files[ref.file]
			cursor := " "
			if selected {
				cursor = ">"
			}
			mark := "[ ]"
			if entry.Marked {
				mark = "[x]"
			}
			fmt.Fprintf(&b, "%s     %s %s%s\n", cursor, mark, entry.Path, formatFileStatus(entry))
		}
	}

	fmt.Fprintf(&b, "\nMarked files: %d", m.countMarked())
	if m.deleteResult != "" {
		fmt.Fprintf(&b, "\n%s", m.deleteResult)
	}
	b.WriteString("\nPress Esc or q to exit.")

	return b.String()
}

func (m *model) renderConfirmView() string {
	var b strings.Builder

	b.WriteString("Confirm Deletion\n")
	b.WriteString(strings.Repeat("-", 80))
	b.WriteString("\n\n")
	fmt.Fprintf(&b, "You are about to delete %d file(s).\n", m.countMarked())
	fmt.Fprintf(&b, "Confirmation code: %s\n", m.confirmCode)
	fmt.Fprintf(&b, "Input: %s\n", m.confirmInput)
	if m.confirmError != "" {
		fmt.Fprintf(&b, "\n%s\n", m.confirmError)
	}
	b.WriteString("\nEnter to confirm, Esc to cancel.")

	return b.String()
}

func (m *model) moveCursor(delta int) {
	if len(m.visible) == 0 {
		m.cursor = 0
		return
	}

	m.cursor += delta
	if m.cursor < 0 {
		m.cursor = 0
	}
	if m.cursor >= len(m.visible) {
		m.cursor = len(m.visible) - 1
	}
}

func (m *model) currentNode() *nodeRef {
	if len(m.visible) == 0 || m.cursor < 0 || m.cursor >= len(m.visible) {
		return nil
	}
	return &m.visible[m.cursor]
}

func (m *model) collapseCurrentGroup() {
	node := m.currentNode()
	if node == nil || node.typ != nodeGroup {
		return
	}
	group := m.groups[node.group]
	if group.Expanded {
		group.Expanded = false
		m.rebuildVisibleNodes()
	}
}

func (m *model) expandCurrentGroup() {
	node := m.currentNode()
	if node == nil || node.typ != nodeGroup {
		return
	}
	group := m.groups[node.group]
	if !group.Expanded {
		group.Expanded = true
		m.rebuildVisibleNodes()
	}
}

func (m *model) toggleCurrentGroup() {
	node := m.currentNode()
	if node == nil || node.typ != nodeGroup {
		return
	}
	group := m.groups[node.group]
	group.Expanded = !group.Expanded
	m.rebuildVisibleNodes()
}

func (m *model) toggleCurrentFileMark() {
	node := m.currentNode()
	if node == nil || node.typ != nodeFile {
		return
	}

	entry := m.groups[node.group].Files[node.file]
	if entry.Status == fileStatusDeleted {
		return
	}

	entry.Marked = !entry.Marked
	m.deleteResult = ""
}

func (m *model) startConfirmationPrompt() {
	if m.countMarked() == 0 {
		return
	}
	m.confirmCode = GenConfirmationCode()
	m.confirmInput = ""
	m.confirmError = ""
	m.deleteResult = ""
	m.mode = modeConfirm
}

func (m *model) processDeletion() {
	m.mode = modeTree
	m.confirmInput = ""
	m.confirmError = ""

	if len(m.groups) == 0 {
		return
	}

	var deleted, failures int
	for _, entry := range m.markedEntries() {
		err := os.Remove(entry.Path)
		if err != nil {
			entry.Status = fileStatusError
			entry.Message = err.Error()
			dsklog.Dlogger.Errorf("Failed to delete file %s: %v", entry.Path, err)
			failures++
		} else {
			entry.Status = fileStatusDeleted
			entry.Message = fmt.Sprintf("deleted (%s)", filepath.Base(entry.Path))
			dsklog.Dlogger.Infof("Successfully deleted file: %s", entry.Path)
			deleted++
		}
		entry.Marked = false
	}

	switch {
	case deleted == 0 && failures == 0:
		m.deleteResult = "No files were deleted."
	case failures == 0:
		m.deleteResult = fmt.Sprintf("Deleted %d file(s).", deleted)
	case deleted == 0:
		m.deleteResult = fmt.Sprintf("Failed to delete %d file(s).", failures)
	default:
		m.deleteResult = fmt.Sprintf("Deleted %d file(s); %d error(s) occurred.", deleted, failures)
	}
}

func (m *model) markedEntries() []*fileEntry {
	var entries []*fileEntry
	for _, group := range m.groups {
		for _, entry := range group.Files {
			if entry.Marked {
				entries = append(entries, entry)
			}
		}
	}
	return entries
}

func (m *model) countMarked() int {
	count := 0
	for _, group := range m.groups {
		for _, entry := range group.Files {
			if entry.Marked {
				count++
			}
		}
	}
	return count
}

func (m *model) rebuildVisibleNodes() {
	m.visible = m.visible[:0]
	for gi, group := range m.groups {
		m.visible = append(m.visible, nodeRef{typ: nodeGroup, group: gi})
		if group.Expanded {
			for fi := range group.Files {
				m.visible = append(m.visible, nodeRef{typ: nodeFile, group: gi, file: fi})
			}
		}
	}
	if len(m.visible) == 0 {
		m.cursor = 0
		return
	}
	if m.cursor >= len(m.visible) {
		m.cursor = len(m.visible) - 1
	}
	if m.cursor < 0 {
		m.cursor = 0
	}
}

func formatFileStatus(entry *fileEntry) string {
	switch entry.Status {
	case fileStatusDeleted:
		return " [DELETED]"
	case fileStatusError:
		if entry.Message != "" {
			return " [ERROR: " + entry.Message + "]"
		}
		return " [ERROR]"
	default:
		return ""
	}
}

func formatGroupTitle(hash dmap.SHA256Hash, files []string) string {
	if len(files) == 0 {
		return "Empty group"
	}

	const tmpl = "%s - %d duplicates - (Using %s total)"
	fileSize := dfs.GetFileSize(files[0])
	totalSize := uint64(fileSize) * uint64(len(files))
	hashHex := fmt.Sprintf("%x", hash[:4])
	return fmt.Sprintf(tmpl, hashHex, len(files), utils.DisplaySize(totalSize))
}

func autoMarkGroup(group *duplicateGroup) {
	if group == nil {
		return
	}
	for i, entry := range group.Files {
		if i == 0 {
			continue
		}
		entry.Marked = true
		dsklog.Dlogger.Infof("Auto-marked file for deletion: %s", entry.Path)
	}
}

func isAlphaNumeric(r rune) bool {
	return unicode.IsLetter(r) || unicode.IsDigit(r)
}

// GenConfirmationCode generates a random alphanumeric confirmation code.
func GenConfirmationCode() string {
	const charset = "ABCDEFGHIJKLMNOPQRSTUVWXYZabcdefghijklmnopqrstuvwxyz0123456789"
	// #nosec G404
	r := rand.New(rand.NewSource(time.Now().UnixNano()))

	codeLen := r.Intn(4) + 5 // between 5 and 8 characters
	code := make([]byte, codeLen)
	for i := range code {
		code[i] = charset[r.Intn(len(charset))]
	}
	return string(code)
}

func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}
