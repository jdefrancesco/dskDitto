package ui

import (
	"fmt"
	"math/rand"
	"os"
	"path/filepath"
	"strings"
	"time"

	"ditto/internal/dfs"
	"ditto/internal/dmap"
	"ditto/internal/dsklog"
	"ditto/pkg/utils"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
)

// Styles using Lip Gloss
var (
	titleStyle = lipgloss.NewStyle().
			Foreground(lipgloss.Color("86")).
			Bold(true).
			Padding(0, 1)

	borderStyle = lipgloss.NewStyle().
			Border(lipgloss.RoundedBorder()).
			BorderForeground(lipgloss.Color("35")).
			Padding(0, 1)

	normalFileStyle = lipgloss.NewStyle().
			Foreground(lipgloss.Color("252"))

	markedFileStyle = lipgloss.NewStyle().
			Foreground(lipgloss.Color("196")).
			Bold(true)

	selectedStyle = lipgloss.NewStyle().
			Background(lipgloss.Color("240")).
			Bold(true)

	headerStyle = lipgloss.NewStyle().
			Foreground(lipgloss.Color("220")).
			Bold(true)

	helpStyle = lipgloss.NewStyle().
			Foreground(lipgloss.Color("241")).
			Italic(true)
)

// TreeNode represents a node in the tree
type TreeNode struct {
	Text       string
	Children   []*TreeNode
	Expanded   bool
	Level      int
	IsFile     bool
	FilePath   string
	IsMarked   bool
	Parent     *TreeNode
}

// Model holds the state of the TUI
type Model struct {
	dMap          *dmap.Dmap
	root          *TreeNode
	flatList      []*TreeNode
	cursor        int
	markedFiles   map[string]*TreeNode
	showingDialog bool
	dialogInput   string
	dialogCode    string
	dialogError   string
	quitting      bool
}

// Program instance to allow stopping from main
var Program *tea.Program

// LaunchTUI launches the TUI using Bubble Tea
func LaunchTUI(dMap *dmap.Dmap) {
	m := initialModel(dMap)
	Program = tea.NewProgram(m, tea.WithAltScreen())

	if _, err := Program.Run(); err != nil {
		fmt.Fprintf(os.Stderr, "Error running program: %v\n", err)
		os.Exit(1)
	}
}

// initialModel creates the initial model for the TUI
func initialModel(dMap *dmap.Dmap) Model {
	root := buildTree(dMap)
	markedFiles := make(map[string]*TreeNode)
	
	// Auto-mark all but the first file in each duplicate group
	autoMarkFiles(root, markedFiles)
	
	m := Model{
		dMap:        dMap,
		root:        root,
		markedFiles: markedFiles,
		cursor:      0,
	}
	
	// Build flat list for navigation
	m.rebuildFlatList()
	
	return m
}

// Init is called when the program starts
func (m Model) Init() tea.Cmd {
	return nil
}

// Update handles messages and updates the model
func (m Model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	if m.showingDialog {
		return m.updateDialog(msg)
	}

	switch msg := msg.(type) {
	case tea.KeyMsg:
		switch msg.String() {
		case "q", "esc":
			m.quitting = true
			return m, tea.Quit

		case "up", "k":
			if m.cursor > 0 {
				m.cursor--
			}

		case "down", "j":
			if m.cursor < len(m.flatList)-1 {
				m.cursor++
			}

		case "enter", " ":
			// Toggle expand/collapse
			if m.cursor < len(m.flatList) {
				node := m.flatList[m.cursor]
				if !node.IsFile && len(node.Children) > 0 {
					node.Expanded = !node.Expanded
					m.rebuildFlatList()
				}
			}

		case "m":
			// Mark/unmark file
			if m.cursor < len(m.flatList) {
				node := m.flatList[m.cursor]
				if node.IsFile {
					if _, ok := m.markedFiles[node.FilePath]; ok {
						delete(m.markedFiles, node.FilePath)
						node.IsMarked = false
					} else {
						m.markedFiles[node.FilePath] = node
						node.IsMarked = true
					}
				}
			}

		case "d":
			// Show delete confirmation
			if len(m.markedFiles) > 0 {
				m.showingDialog = true
				m.dialogCode = GenConfirmationCode()
				m.dialogInput = ""
				m.dialogError = ""
			}
		}
	}

	return m, nil
}

// updateDialog handles updates when the delete confirmation dialog is shown
func (m Model) updateDialog(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.KeyMsg:
		switch msg.String() {
		case "esc":
			m.showingDialog = false
			m.dialogInput = ""
			m.dialogError = ""

		case "enter":
			if m.dialogInput == m.dialogCode {
				// Perform deletion
				performDeletion(m.markedFiles)
				m.showingDialog = false
				m.dialogInput = ""
				m.dialogError = ""
				// Rebuild flat list to show deleted files
				m.rebuildFlatList()
			} else {
				m.dialogError = "Incorrect code. Try again."
				m.dialogInput = ""
			}

		case "backspace":
			if len(m.dialogInput) > 0 {
				m.dialogInput = m.dialogInput[:len(m.dialogInput)-1]
			}

		default:
			// Add character to input if it's alphanumeric and within length
			if len(msg.String()) == 1 && len(m.dialogInput) < len(m.dialogCode) {
				ch := msg.String()[0]
				if (ch >= 'A' && ch <= 'Z') || (ch >= 'a' && ch <= 'z') || (ch >= '0' && ch <= '9') {
					m.dialogInput += msg.String()
				}
			}
		}
	}

	return m, nil
}

// View renders the UI
func (m Model) View() string {
	if m.quitting {
		return ""
	}

	if m.showingDialog {
		return m.renderDialog()
	}

	var b strings.Builder

	// Title
	title := titleStyle.Render("dskDitto: Interactive Duplicate Management")
	help := helpStyle.Render("[m=mark, d=delete, q=quit, ↑↓=navigate, enter=expand/collapse]")
	b.WriteString(title + "\n")
	b.WriteString(help + "\n\n")

	// Render tree
	for i, node := range m.flatList {
		b.WriteString(m.renderNode(node, i == m.cursor))
		b.WriteString("\n")
	}

	// Footer with marked count
	if len(m.markedFiles) > 0 {
		footer := helpStyle.Render(fmt.Sprintf("\n%d file(s) marked for deletion", len(m.markedFiles)))
		b.WriteString(footer)
	}

	return borderStyle.Render(b.String())
}

// renderNode renders a single tree node
func (m Model) renderNode(node *TreeNode, selected bool) string {
	indent := strings.Repeat("  ", node.Level)
	
	var prefix string
	if !node.IsFile {
		if node.Expanded {
			prefix = "▼ "
		} else {
			prefix = "▶ "
		}
	} else {
		prefix = "  "
	}

	text := indent + prefix + node.Text

	var style lipgloss.Style
	if node.IsFile && node.IsMarked {
		style = markedFileStyle
	} else if node.IsFile {
		style = normalFileStyle
	} else {
		style = headerStyle
	}

	if selected {
		style = style.Inherit(selectedStyle)
	}

	return style.Render(text)
}

// renderDialog renders the delete confirmation dialog
func (m Model) renderDialog() string {
	var b strings.Builder

	dialogStyle := lipgloss.NewStyle().
		Border(lipgloss.DoubleBorder()).
		BorderForeground(lipgloss.Color("196")).
		Padding(1, 2).
		Width(60)

	b.WriteString(fmt.Sprintf("Type the confirmation code below to delete %d file(s):\n\n", len(m.markedFiles)))
	b.WriteString(lipgloss.NewStyle().Foreground(lipgloss.Color("220")).Bold(true).Render(m.dialogCode))
	b.WriteString("\n\n")
	b.WriteString(fmt.Sprintf("Code: %s\n", m.dialogInput))

	if m.dialogError != "" {
		b.WriteString("\n")
		b.WriteString(lipgloss.NewStyle().Foreground(lipgloss.Color("196")).Render(m.dialogError))
	}

	b.WriteString("\n\n")
	b.WriteString(helpStyle.Render("[enter=confirm, esc=cancel]"))

	return lipgloss.Place(
		80, 24,
		lipgloss.Center, lipgloss.Center,
		dialogStyle.Render(b.String()),
	)
}

// rebuildFlatList rebuilds the flat list of visible nodes for navigation
func (m *Model) rebuildFlatList() {
	m.flatList = nil
	m.flattenTree(m.root)
	
	// Ensure cursor is within bounds
	if m.cursor >= len(m.flatList) {
		m.cursor = len(m.flatList) - 1
	}
	if m.cursor < 0 {
		m.cursor = 0
	}
}

// flattenTree recursively flattens the tree into a list
func (m *Model) flattenTree(node *TreeNode) {
	if node == nil {
		return
	}
	
	// Don't add the root node itself
	if node.Level >= 0 && node.Text != "Root" {
		m.flatList = append(m.flatList, node)
	}
	
	if node.Expanded || node.Level < 0 {
		for _, child := range node.Children {
			m.flattenTree(child)
		}
	}
}

// buildTree builds the tree structure from the duplicate map
func buildTree(dMap *dmap.Dmap) *TreeNode {
	root := &TreeNode{
		Text:     "Root",
		Level:    -1,
		Expanded: true,
	}

	if dMap == nil {
		dsklog.Dlogger.Debug("dMap is nil")
		return root
	}

	// Add the hash as parent node and the files as children
	for hash, files := range dMap.GetMap() {
		if len(hash) == 0 {
			continue
		}

		if len(files) > 1 {
			var fmt_str = "%s - %d Duplicates - (Using %s of storage total)"
			fSize := dfs.GetFileSize(files[0])
			totalSize := uint64(fSize) * uint64(len(files))
			// Create header with relevant information - display first 8 characters of hex hash
			hashHex := fmt.Sprintf("%x", hash[:4]) // Show first 4 bytes as 8 hex chars
			header := fmt.Sprintf(fmt_str, hashHex, len(files), utils.DisplaySize(totalSize))
			
			groupNode := &TreeNode{
				Text:     header,
				Level:    0,
				Expanded: true,
				IsFile:   false,
				Parent:   root,
			}

			// Add files as children
			for _, file := range files {
				fileNode := &TreeNode{
					Text:     file,
					Level:    1,
					IsFile:   true,
					FilePath: file,
					Parent:   groupNode,
				}
				groupNode.Children = append(groupNode.Children, fileNode)
			}

			root.Children = append(root.Children, groupNode)
		}
	}

	return root
}

// performDeletion actually deletes the marked files
func performDeletion(markedFiles map[string]*TreeNode) {
	for path, node := range markedFiles {
		err := os.Remove(path)
		if err != nil {
			dsklog.Dlogger.Errorf("Failed to delete file %s: %v", path, err)
			node.Text = "[ERROR] " + filepath.Base(path)
		} else {
			dsklog.Dlogger.Infof("Successfully deleted file: %s", path)
			node.Text = "[DELETED] " + filepath.Base(path)
		}
	}
}

// autoMarkFiles automatically marks all but the first file in each duplicate group for deletion
func autoMarkFiles(root *TreeNode, markedFiles map[string]*TreeNode) {
	if root == nil {
		return
	}

	for _, groupNode := range root.Children {
		if len(groupNode.Children) <= 1 {
			continue
		}
		for i, fileNode := range groupNode.Children {
			if i == 0 {
				continue
			}
			markedFiles[fileNode.FilePath] = fileNode
			fileNode.IsMarked = true
			dsklog.Dlogger.Infof("Auto-marked file for deletion: %s", fileNode.FilePath)
		}
	}
}

// GenConfirmationCode generates a random alphanumeric confirmation code
// user will need to type to confirm the deletion of files.
func GenConfirmationCode() string {

	const kAlnum = "ABCDEFGHIJKLMNOPQRSTUVWXYZabcdefghijklmnopqrstuvwxyz0123456789"
	// #nosec G404 -- used intentionally. Not being used for crypto just UX.
	r := rand.New(rand.NewSource(time.Now().UnixNano()))

	length := r.Intn(4) + 5 // Random length between 5 and 8
	code := make([]byte, length)

	for i := range code {
		code[i] = kAlnum[r.Intn(len(kAlnum))]
	}

	return string(code)

}
