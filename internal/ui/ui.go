package ui

import (
	"fmt"
	"math/rand"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"time"
	"unicode"

	"github.com/jdefrancesco/dskDitto/internal/dfs"
	"github.com/jdefrancesco/dskDitto/internal/dmap"
	"github.com/jdefrancesco/dskDitto/internal/dsklog"

	"github.com/jdefrancesco/dskDitto/pkg/utils"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	runewidth "github.com/mattn/go-runewidth"
)

// LaunchTUI builds and runs the Bubble Tea program that visualizes duplicate files.
func LaunchTUI(dMap *dmap.Dmap) {
	if dMap == nil {
		dsklog.Dlogger.Warn("nil duplicate map supplied to LaunchTUI")
		return
	}

	program := tea.NewProgram(newModel(dMap), tea.WithAltScreen(), tea.WithMouseCellMotion())
	setCurrentProgram(program)
	defer clearCurrentProgram(program)

	if _, err := program.Run(); err != nil {
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

// Color scheme to make things look good!
var (
	titleStyle = lipgloss.NewStyle().
			Foreground(lipgloss.Color("#A6E22E")).
			Bold(true).
			PaddingBottom(0)

	dividerStyle        = lipgloss.NewStyle().Foreground(lipgloss.Color("#3F3F46"))
	cursorActiveStyle   = lipgloss.NewStyle().Foreground(lipgloss.Color("#FF5555")).Bold(true)
	cursorInactiveStyle = lipgloss.NewStyle().Foreground(lipgloss.Color("#5C6370"))
	groupStyle          = lipgloss.NewStyle().Foreground(lipgloss.Color("#FFD866")).Bold(false)
	groupCollapsedStyle = lipgloss.NewStyle().Foreground(lipgloss.Color("#FFD866"))
	fileStyle           = lipgloss.NewStyle().Foreground(lipgloss.Color("#E5E5E5"))
	// selectedLineStyle   = lipgloss.NewStyle().Background(lipgloss.Color("#1F2937"))
	markedStyle        = lipgloss.NewStyle().Foreground(lipgloss.Color("#50FA7B")).Bold(true)
	unmarkedStyle      = lipgloss.NewStyle().Foreground(lipgloss.Color("#4B5563"))
	statusDeletedStyle = lipgloss.NewStyle().Foreground(lipgloss.Color("#50FA7B")).Bold(true)
	statusErrorStyle   = lipgloss.NewStyle().Foreground(lipgloss.Color("#FF5555")).Bold(true)
	statusInfoStyle    = lipgloss.NewStyle().Foreground(lipgloss.Color("#7F848E"))
	footerStyle        = lipgloss.NewStyle().Foreground(lipgloss.Color("#9CA3AF"))
	resultStyle        = lipgloss.NewStyle().Foreground(lipgloss.Color("#FFB86C"))
	emptyStateStyle    = lipgloss.NewStyle().Foreground(lipgloss.Color("#6C7086")).Italic(true)

	confirmPanelStyle = lipgloss.NewStyle().
				Border(lipgloss.RoundedBorder()).
				BorderForeground(lipgloss.Color("#86fb71ff")).
				Padding(1, 2)

	confirmCodeStyle  = lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("#FBBF24"))
	confirmInputStyle = lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("#E2E8F0"))
	errorTextStyle    = lipgloss.NewStyle().Foreground(lipgloss.Color("#F87171")).Bold(true)
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
	fileStatusLinked
	fileStatusError
)

type confirmAction int

const (
	confirmDelete confirmAction = iota
	confirmLink
)

// fileEntry represents a file tracked by the UI, capturing its path, marked state,
// status, and any associated message.
type fileEntry struct {
	Path    string
	Marked  bool
	Status  fileStatus
	Message string
}

type sortMode int

const (
	sortByTotalSize sortMode = iota
	sortByCount
)

const sortModeCount = 2

// These are batches of file dups
type duplicateGroup struct {
	Hash     dmap.Digest
	Title    string
	Files    []*fileEntry
	Expanded bool
	TotalSz  uint64
}

type nodeRef struct {
	// typ tracks the classification of the current node within the UI layer.
	typ   nodeType
	group int
	file  int
}

// model struct for Bubble Tea. This core structure
// holds bulk of what is needed to render TUI.
// See Bubble Tea github page for tutorial.
type model struct {
	groups        []*duplicateGroup
	sortMode      sortMode
	visible       []nodeRef
	cursor        int
	scroll        int
	minDuplicates uint

	// double-click tracking
	lastClickIdx int
	lastClickAt  time.Time
	// Sticky group info tracking: last group header cursor was on.
	lastGroupIdx int

	mode   viewMode
	action confirmAction

	confirmCode  string
	confirmInput string
	confirmError string

	deleteResult string

	width  int
	height int
}

var _ tea.Model = (*model)(nil)

// newModel constructs a model initialized with duplicate groups derived from the provided dmap,
// filtering out groups below the minimum duplicate threshold, preparing file entries, and
// rebuilding the visible UI nodes before returning the result.
func newModel(dMap *dmap.Dmap) *model {
	m := &model{
		mode:          modeTree,
		sortMode:      sortByTotalSize,
		minDuplicates: dMap.MinDuplicates(),
		lastGroupIdx:  -1,
	}

	for hash, files := range dMap.GetMap() {
		if uint(len(files)) < m.minDuplicates {
			continue
		}

		totalSize := estimateGroupTotalSize(files)

		group := &duplicateGroup{
			Hash:     hash,
			Title:    formatGroupTitle(hash, len(files), totalSize),
			Expanded: true,
			TotalSz:  totalSize,
		}

		for _, file := range files {
			group.Files = append(group.Files, &fileEntry{Path: file})
		}

		autoMarkGroup(group)
		m.groups = append(m.groups, group)
	}

	m.sortGroups()
	m.rebuildVisibleNodes()
	return m
}

func (m *model) Init() tea.Cmd {
	return nil
}

// Update our Bubble Tea view. Handle key presses and mouse
// activity as well.
func (m *model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {

	switch msg := msg.(type) {

	case tea.KeyMsg:
		if m.mode == modeConfirm {
			return m.handleConfirmKeys(msg)
		}
		return m.handleTreeKeys(msg)

	case tea.MouseMsg:
		return m.handleMouse(msg)

	case tea.WindowSizeMsg:
		m.width = msg.Width
		m.height = msg.Height
		m.adjustScroll()
	}

	return m, nil
}

// View is a primary method used by Bubble Tea. Similiar to MVC.
func (m *model) View() string {
	// Modal window that pops up when user triggers file removal
	// process.
	if m.mode == modeConfirm {
		return m.renderConfirmView()
	}

	// Interactive tree view for viewing results and selecting course of action.
	return m.renderTreeView()
}

// handleTreeKeys allows user to navigate the TUI.
// It wraps the finer detail handling so we can keep our Update
// method clean. We provide some vim key binding too!
func (m *model) handleTreeKeys(msg tea.KeyMsg) (tea.Model, tea.Cmd) {

	// Prefer string-based matching for common keys.
	switch msg.String() {

	// Quit application
	case "ctrl+c", "esc", "q":
		return m, tea.Quit

	// Move cursor up with arrow and vim like binding
	case "up", "k":
		m.moveCursor(-1)

	case "down", "j":
		m.moveCursor(1)

	case "left", "h":
		m.collapseCurrentGroup()

	case "right", "l":
		m.expandCurrentGroup()

	case "pgup":
		m.pageMove(-1)

	case "pgdown", "pgdn":
		m.pageMove(1)

	case "ctrl+u":
		m.halfPageMove(-1)

	case "ctrl+d":
		m.halfPageMove(1)

	case "a", "A", "ctrl+a":
		m.markAllFiles()

	case "u", "U":
		m.unmarkAllFiles()

	case "enter":
		m.toggleCurrentGroup()

	// m and space bar will mark a file.
	case "m", " ":
		m.toggleCurrentFileMark()

	case "d":
		m.startConfirmationPrompt(confirmDelete)

	case "L":
		m.startConfirmationPrompt(confirmLink)

	case "1":
		m.setSortMode(sortByTotalSize)

	case "2":
		m.setSortMode(sortByCount)

	case "s", "S":
		m.cycleSortMode()
	}

	// Also catch PageUp/PageDown by key type for wider terminal support.
	switch msg.Type {

	case tea.KeyPgUp:
		m.pageMove(-1)

	case tea.KeyPgDown:
		m.pageMove(1)
	}

	return m, nil
}

// handleMouse supports scroll wheel and selecting a row by clicking.
// TODO: Refactor and remove any deprecated Bubble Tea types.
func (m *model) handleMouse(msg tea.MouseMsg) (tea.Model, tea.Cmd) {

	if m.mode != modeTree {
		return m, nil
	}

	// Refactor later as some of these methods seem to be deprecated.
	switch msg.Action {
	case tea.MouseActionPress:
		switch msg.Button {
		case tea.MouseButtonWheelUp:
			// Scroll up a few lines per tick.
			m.moveCursor(-3)
			return m, nil
		case tea.MouseButtonWheelDown:
			m.moveCursor(3)
			return m, nil
		case tea.MouseButtonLeft:
			// Map Y position to list row.
			row := msg.Y - m.listTopOffset()
			if row >= 0 && row < m.listAreaHeight() {
				idx := m.scroll + row
				if idx >= 0 && idx < len(m.visible) {
					// Detect double-click on the same row within a short threshold
					const dbl = 350 * time.Millisecond
					now := time.Now()
					if idx == m.lastClickIdx && now.Sub(m.lastClickAt) <= dbl {
						// Double-click: toggle group if clicking on a group header
						ref := m.visible[idx]
						if ref.typ == nodeGroup {
							// Keep cursor on the group and toggle expansion
							m.cursor = idx
							m.toggleCurrentGroup()
						}
						// Reset to avoid repeated toggles on subsequent events
						m.lastClickIdx = -1
						m.lastClickAt = time.Time{}
					} else {
						// Single click: move cursor and record for potential double-click
						m.cursor = idx
						m.adjustScroll()
						m.lastClickIdx = idx
						m.lastClickAt = now
					}
				}
			}
		}
	}

	return m, nil
}

// handleConfirmKeys ensures the user doesn't shoot themselves in the foot. The files will
// be removed only if they type the code correctly.
func (m *model) handleConfirmKeys(msg tea.KeyMsg) (tea.Model, tea.Cmd) {

	switch msg.Type {

	case tea.KeyEsc:
		m.mode = modeTree
		m.confirmError = ""
		m.confirmInput = ""

	case tea.KeyEnter:
		if m.confirmInput == m.confirmCode {
			switch m.action {
			case confirmLink:
				m.processLinking()
			default:
				m.processDeletion()
			}
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

// renderTreeView provides View with primary tree based TUI the user interacts with.
func (m *model) renderTreeView() string {

	width := m.effectiveWidth()
	divider := dividerStyle.Render(strings.Repeat("─", width))

	var sections []string

	// Title at top of view
	title := "dskDitto • Interactive Results"
	sections = append(sections,
		titleStyle.Width(width).Render(runewidth.Truncate(title, width, "…")))
	sections = append(sections, divider)

	if len(m.visible) == 0 {
		sections = append(sections, emptyStateStyle.Render("No duplicate groups found. Press q to exit."))
	} else {
		// Render only the portion of the list that fits in the viewport.
		contentH := max(m.listAreaHeight(), 1)
		start := max(m.scroll, 0)
		end := min(start+contentH, len(m.visible))
		for i := start; i < end; i++ {
			ref := m.visible[i]
			sections = append(sections, m.renderNodeLine(ref, i == m.cursor))
		}
	}

	sections = append(sections, divider)
	countStr := fmt.Sprintf("%d", m.countMarked())
	markedLabel := "marked files: "
	countStyled := lipgloss.NewStyle().Foreground(lipgloss.Color("#50FA7B")).Render(countStr)
	sections = append(sections,
		footerStyle.Render(markedLabel)+countStyled,
	)
	if m.deleteResult != "" {
		sections = append(sections, resultStyle.Render(m.deleteResult))
	}
	// Navigation instructions.
	sections = append(sections, m.instructionsFooter(width))

	return strings.Join(sections, "\n")
}

// renderConfirmView is our modal box that prevents the user from "shooting themelves in the foot"
// In order to delete files they have selected, they must first enter small code. Dunno how far or useful
// this type of thing really is but it satisfies my OCD for time being.
func (m *model) renderConfirmView() string {
	width := m.effectiveWidth()
	title := "Confirm Deletion"
	verb := "delete"
	if m.action == confirmLink {
		title = "Confirm Symlink Conversion"
		verb = "convert to symlinks"
	}
	content := []string{
		titleStyle.Render(title),
		statusInfoStyle.Render(fmt.Sprintf("You are about to %s %d file(s).", verb, m.countMarked())),
		"",
		fmt.Sprintf("Confirmation code: %s", confirmCodeStyle.Render(m.confirmCode)),
		fmt.Sprintf("Your input: %s", confirmInputStyle.Render(m.confirmInput)),
	}

	if m.confirmError != "" {
		content = append(content, "", errorTextStyle.Render(m.confirmError))
	}

	content = append(content, "", footerStyle.Render("Enter confirms • Esc cancels"))
	panel := confirmPanelStyle.Width(min(width, 80)).Render(strings.Join(content, "\n"))
	renderWidth := max(width, lipgloss.Width(panel))
	return lipgloss.Place(renderWidth, lipgloss.Height(panel), lipgloss.Center, lipgloss.Center, panel)
}

// moveCursor moves the indicator on the left of the listed items.
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
	m.adjustScroll()
}

// pageMove moves the cursor up or down by one viewport height and adjusts scroll.
func (m *model) pageMove(direction int) {
	if len(m.visible) == 0 {
		return
	}
	amount := max(m.listAreaHeight(), 1)
	if direction < 0 {
		m.cursor -= amount
	} else {
		m.cursor += amount
	}
	if m.cursor < 0 {
		m.cursor = 0
	}
	if m.cursor >= len(m.visible) {
		m.cursor = len(m.visible) - 1
	}
	m.adjustScroll()
}

// halfPageMove moves the cursor by half the viewport height.
// Ctrl+D/U will let the user navigate by half page up or down.
func (m *model) halfPageMove(direction int) {
	if len(m.visible) == 0 {
		return
	}
	amount := max(m.listAreaHeight()/2, 1)
	if direction < 0 {
		m.cursor -= amount
	} else {
		m.cursor += amount
	}
	if m.cursor < 0 {
		m.cursor = 0
	}
	if m.cursor >= len(m.visible) {
		m.cursor = len(m.visible) - 1
	}
	m.adjustScroll()
}

func (m *model) currentNode() *nodeRef {
	if len(m.visible) == 0 || m.cursor < 0 || m.cursor >= len(m.visible) {
		return nil
	}
	return &m.visible[m.cursor]
}

// collapseCurrentGroup collapses the currently selected group node,
// ensuring its expanded state is false and refreshing the visible node list when needed.
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

// expandCurrentGroup marks the currently selected group as expanded.
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

// toggleCurrentGroup toggles between collapsed and expanded view.
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

// markAllFiles marks every non-deleted file in all groups.
func (m *model) markAllFiles() {
	for _, group := range m.groups {
		for _, entry := range group.Files {
			if entry.Status == fileStatusDeleted {
				continue
			}
			entry.Marked = true
		}
	}
	m.deleteResult = ""
}

// unmarkAllFiles clears the marked flag for every file.
func (m *model) unmarkAllFiles() {
	for _, group := range m.groups {
		for _, entry := range group.Files {
			entry.Marked = false
		}
	}
	m.deleteResult = ""
}

// startConfirmationPrompt is modal window to tell user what is about to happen
// and asking them to confirm moving forward with file removal
func (m *model) startConfirmationPrompt(action confirmAction) {
	if m.countMarked() == 0 {
		return
	}
	m.action = action
	m.confirmCode = GenConfirmationCode()
	m.confirmInput = ""
	m.confirmError = ""
	m.deleteResult = ""
	m.mode = modeConfirm
}

// processDeletion actually removes the duplicate files.
// TODO: Factor out logic that removes file.
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

func (m *model) processLinking() {
	m.mode = modeTree
	m.confirmInput = ""
	m.confirmError = ""

	if len(m.groups) == 0 {
		return
	}

	var linked, failures int
	for _, group := range m.groups {
		var target *fileEntry
		for _, entry := range group.Files {
			if entry.Status == fileStatusDeleted {
				continue
			}
			if !entry.Marked {
				target = entry
				break
			}
		}
		if target == nil {
			// No unmarked survivor to point to; mark all selected in this group as errors.
			for _, entry := range group.Files {
				if !entry.Marked {
					continue
				}
				entry.Status = fileStatusError
				entry.Message = "no unmarked file to link to"
				entry.Marked = false
				failures++
			}
			continue
		}

		for _, entry := range group.Files {
			if !entry.Marked {
				continue
			}
			if entry.Status == fileStatusDeleted {
				entry.Marked = false
				continue
			}

			if err := os.Remove(entry.Path); err != nil {
				entry.Status = fileStatusError
				entry.Message = err.Error()
				dsklog.Dlogger.Errorf("Failed to remove file for relink %s: %v", entry.Path, err)
				failures++
				entry.Marked = false
				continue
			}
			if err := os.Symlink(target.Path, entry.Path); err != nil {
				entry.Status = fileStatusError
				entry.Message = err.Error()
				dsklog.Dlogger.Errorf("Failed to create symlink %s -> %s: %v", entry.Path, target.Path, err)
				failures++
				entry.Marked = false
				continue
			}
			entry.Status = fileStatusLinked
			entry.Message = fmt.Sprintf("linked -> %s", filepath.Base(target.Path))
			dsklog.Dlogger.Infof("Converted duplicate to symlink: %s -> %s", entry.Path, target.Path)
			linked++
			entry.Marked = false
		}
	}

	switch {
	case linked == 0 && failures == 0:
		m.deleteResult = "No files were converted."
	case failures == 0:
		m.deleteResult = fmt.Sprintf("Converted %d file(s) to symlinks.", linked)
	case linked == 0:
		m.deleteResult = fmt.Sprintf("Failed to convert %d file(s) to symlinks.", failures)
	default:
		m.deleteResult = fmt.Sprintf("Converted %d file(s); %d error(s) occurred.", linked, failures)
	}
}

// markedEntries return a slice of files selected (marked) for removal.
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
		m.scroll = 0
		m.lastGroupIdx = -1
		return
	}
	if m.cursor >= len(m.visible) {
		m.cursor = len(m.visible) - 1
	}
	if m.cursor < 0 {
		m.cursor = 0
	}
	m.adjustScroll()
}

// renderNodeLine renders the visual line for the provided node reference, applying the appropriate
// cursor state, group or file styling, truncation, and status formatting based on the available width
// and whether the node is currently selected.
func (m *model) renderNodeLine(ref nodeRef, selected bool) string {
	cursor := cursorInactiveStyle.Render("  ")
	if selected {
		cursor = cursorActiveStyle.Render("▸ ")
	}

	var content string
	width := m.effectiveWidth()
	avail := width - lipgloss.Width(cursor)
	switch ref.typ {
	case nodeGroup:
		group := m.groups[ref.group]
		indicator := groupCollapsedStyle.Render("▸")
		if group.Expanded {
			indicator = groupStyle.Render("▾")
		}
		// Truncate group title to avoid line wrapping.
		// Reserve 2 for indicator + space.
		titleMax := max(avail-(lipgloss.Width(indicator)+1), 0)
		title := group.Title
		if runewidth.StringWidth(title) > titleMax {
			title = runewidth.Truncate(title, titleMax, "…")
		}
		body := lipgloss.JoinHorizontal(lipgloss.Left, indicator, " ", groupStyle.Render(title))
		content = body

	case nodeFile:
		entry := m.groups[ref.group].Files[ref.file]
		mark := unmarkedStyle.Render("□")
		if entry.Marked {
			mark = markedStyle.Render("■")
		}
		// Add a bit of space between the checkbox and filename for readability.
		markStr := "  " + mark + "  "

		// First, estimate a status width budget as a third of available after mark.
		markW := lipgloss.Width(markStr)
		baseAvail := max(avail-markW, 1)
		// Make sure we have enough room to display "DELETED"
		statusBudget := max(baseAvail/3, 8)
		statusStr := formatFileStatus(entry, statusBudget)

		// Now compute remaining width for the path and recompute status if needed.
		used := lipgloss.Width(markStr) + lipgloss.Width(statusStr)
		pathMax := max(avail-used, 1)
		path := entry.Path
		// If this path is currently a symlink on disk, annotate it so the user
		// can distinguish converted duplicates.
		if fi, err := os.Lstat(entry.Path); err == nil && fi.Mode()&os.ModeSymlink != 0 {
			path += " [symlink]"
		}
		if runewidth.StringWidth(path) > pathMax {
			path = runewidth.Truncate(path, pathMax, "…")
		}

		// Recompute status with the final remaining width after mark+path (in case status was too big).
		usedAfterPath := lipgloss.Width(markStr) + lipgloss.Width(fileStyle.Render(path))
		rem := max(avail-usedAfterPath, 0)
		statusStr = formatFileStatus(entry, rem)
		body := lipgloss.JoinHorizontal(lipgloss.Left,
			markStr,
			fileStyle.Render(path),
			statusStr,
		)
		content = body
	}

	line := lipgloss.JoinHorizontal(lipgloss.Left, cursor, content)
	return line
}

func formatFileStatus(entry *fileEntry, maxWidth int) string {
	if maxWidth <= 0 {
		return ""
	}
	switch entry.Status {
	case fileStatusDeleted:
		text := "DELETED"
		if runewidth.StringWidth(text) > maxWidth {
			text = runewidth.Truncate(text, maxWidth, "…")
		}
		return " " + statusDeletedStyle.Render(text)
	case fileStatusLinked:
		text := "LINKED"
		if runewidth.StringWidth(text) > maxWidth {
			text = runewidth.Truncate(text, maxWidth, "…")
		}
		return " " + statusDeletedStyle.Render(text)
	case fileStatusError:
		text := "ERROR"
		if entry.Message != "" {
			text = "ERROR: " + entry.Message
		}
		if runewidth.StringWidth(text) > maxWidth {
			text = runewidth.Truncate(text, maxWidth, "…")
		}
		return " " + statusErrorStyle.Render(text)
	default:
		return ""
	}
}

// effectiveWidth returns the model's width constrained between 80 and 120,
// defaulting to 80 when the current width is non-positive.
func (m *model) effectiveWidth() int {
	switch {
	case m.width <= 0:
		return 80
	case m.width > 120:
		return 120
	default:
		return m.width
	}
}

// listAreaHeight returns how many rows are available to render the list
// given the current terminal height and static header/footer rows.
func (m *model) listAreaHeight() int {
	h := m.height
	if h <= 0 {
		h = 24
	}
	width := m.effectiveWidth()
	// Static rows: title (1) + top divider (1) + bottom divider (1) + marked footer (1)
	reserved := 4
	reserved += m.instructionsLineCount(width)
	if m.deleteResult != "" {
		reserved++ // extra line when we show the deletion result
	}
	return max(1, h-reserved)
}

func (m *model) instructionsText() string {
	return "enter exp/fold • arrows nav • m toggle • a mark all • u clear • d delete marked • L link marked • q exit"
}

func (m *model) sortHotkeysText() string {
	return "1 sort by total size; by 2 dup count • s cycle"
}

func wrapText(text string, width int) string {
	if width <= 0 {
		return text
	}

	words := strings.Fields(text)
	if len(words) == 0 {
		return ""
	}

	lines := make([]string, 0, 4)
	line := ""
	lineW := 0

	for _, word := range words {
		wordW := runewidth.StringWidth(word)
		if wordW > width && width > 1 {
			word = runewidth.Truncate(word, width, "…")
			wordW = runewidth.StringWidth(word)
		}

		if line == "" {
			line = word
			lineW = wordW
			continue
		}

		// +1 accounts for the space.
		if lineW+1+wordW <= width {
			line += " " + word
			lineW += 1 + wordW
			continue
		}

		lines = append(lines, line)
		line = word
		lineW = wordW
	}

	if line != "" {
		lines = append(lines, line)
	}

	return strings.Join(lines, "\n")
}

func (m *model) instructionsFooter(width int) string {
	nav := m.instructionsText()
	sort := m.sortHotkeysText()

	if width <= 0 {
		return footerStyle.Render(nav + "\n" + sort)
	}

	wrappedNav := wrapText(nav, width)
	wrappedSort := wrapText(sort, width)
	return footerStyle.Render(wrappedNav + "\n" + wrappedSort)
}

func (m *model) instructionsLineCount(width int) int {
	text := m.instructionsText() + "\n" + m.sortHotkeysText()
	if width <= 0 {
		return max(1, lipgloss.Height(text))
	}
	return max(1, lipgloss.Height(wrapText(text, width)))
}

// listTopOffset returns the number of rows occupied above the list.
func (m *model) listTopOffset() int {
	// title (1) + top divider (1)
	return 2
}

// adjustScroll ensures the scroll offset keeps the cursor within the viewport
// and clamps both cursor and scroll to valid ranges.
// TODO: Refactor to use min/max
func (m *model) adjustScroll() {
	if len(m.visible) == 0 || m.mode != modeTree {
		m.scroll = 0
		return
	}
	contentH := max(m.listAreaHeight(), 1)

	// Clamp cursor to valid range.
	if m.cursor < 0 {
		m.cursor = 0
	}
	if m.cursor >= len(m.visible) {
		m.cursor = len(m.visible) - 1
	}

	// Clamp scroll to [0, maxScroll].
	maxScroll := max(len(m.visible)-contentH, 0)
	if m.scroll < 0 {
		m.scroll = 0
	}
	if m.scroll > maxScroll {
		m.scroll = maxScroll
	}

	// Ensure cursor is visible inside [scroll, scroll+contentH-1].
	if m.cursor < m.scroll {
		m.scroll = m.cursor
	} else if m.cursor >= m.scroll+contentH {
		m.scroll = max(m.cursor-contentH+1, 0)
		m.scroll = min(m.scroll, maxScroll)
	}

	m.recordGroupFocus()
}

func (m *model) setSortMode(mode sortMode) {
	if mode < 0 || int(mode) >= sortModeCount {
		return
	}
	if m.sortMode == mode {
		return
	}
	m.sortMode = mode
	m.sortGroups()
	m.rebuildVisibleNodes()
}

func (m *model) cycleSortMode() {
	next := sortMode((int(m.sortMode) + 1) % sortModeCount)
	m.setSortMode(next)
}

func (m *model) sortModeLabel() string {
	switch m.sortMode {
	case sortByTotalSize:
		return "Total size"
	case sortByCount:
		return "Duplicate count"
	default:
		return "Total size"
	}
}

func (m *model) sortGroups() {
	if len(m.groups) == 0 {
		return
	}
	switch m.sortMode {
	case sortByTotalSize:
		sort.SliceStable(m.groups, func(i, j int) bool {
			if m.groups[i].TotalSz == m.groups[j].TotalSz {
				return len(m.groups[i].Files) > len(m.groups[j].Files)
			}
			return m.groups[i].TotalSz > m.groups[j].TotalSz
		})
	case sortByCount:
		sort.SliceStable(m.groups, func(i, j int) bool {
			si := len(m.groups[i].Files)
			sj := len(m.groups[j].Files)
			if si == sj {
				return m.groups[i].TotalSz > m.groups[j].TotalSz
			}
			return si > sj
		})
	default:
		m.sortMode = sortByTotalSize
		m.sortGroups()
	}
}

func (m *model) recordGroupFocus() {
	node := m.currentNode()
	if node == nil || node.typ != nodeGroup {
		return
	}
	if node.group < 0 || node.group >= len(m.groups) {
		return
	}
	m.lastGroupIdx = node.group
}

func (m *model) hasSelectedGroupInfo() bool {
	return m.lastGroupIdx >= 0 && m.lastGroupIdx < len(m.groups)
}

func (m *model) selectedGroupInfoLine() string {
	if !m.hasSelectedGroupInfo() {
		return ""
	}
	group := m.groups[m.lastGroupIdx]
	if group == nil {
		return ""
	}
	count := len(group.Files)
	if count == 0 {
		return ""
	}
	prefix := fmt.Sprintf("Selected group: %d files • Total size: ", count)
	size := utils.DisplaySize(group.TotalSz)
	// Make the size pop using an existing bold highlight style.
	return footerStyle.Render(prefix) + confirmCodeStyle.Render(size)
}

func estimateGroupTotalSize(files []string) uint64 {
	if len(files) == 0 {
		return 0
	}
	perFile := dfs.GetFileSize(files[0])
	return perFile * uint64(len(files))
}

// formatGroupTitle constructs a descriptive label for a digest-based group, summarizing its hash, file count, and approximate total size.
func formatGroupTitle(hash dmap.Digest, count int, totalSize uint64) string {
	if count == 0 {
		return "Empty group"
	}

	const tmpl = "%s - %d files - (approx. size %s)"
	// Show 16 hex chars of SHA-256 hash.
	hashHex := fmt.Sprintf("%x", hash[:16])
	return fmt.Sprintf(tmpl, hashHex, count, utils.DisplaySize(totalSize))
}

// autoMarkGroup marks all but one in the duplicate group. For UX, assumes users will want
// to probably keep at least one of the files.
func autoMarkGroup(group *duplicateGroup) {
	if group == nil {
		return
	}
	for i, entry := range group.Files {
		if i == 0 {
			continue
		}
		entry.Marked = true
		dsklog.Dlogger.Debugf("Auto-marked file for deletion: %s", entry.Path)
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
