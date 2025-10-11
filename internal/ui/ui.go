package ui

import (
	"fmt"
	"math/rand"
	"os"
	"path/filepath"
	"time"

	"ditto/internal/dfs"
	"ditto/internal/dmap"
	"ditto/internal/dsklog"

	"ditto/pkg/utils"

	"github.com/gdamore/tcell/v2"
	"github.com/rivo/tview"
)

// Global app instance. We need this to launch the TUI and may also want other
// TUI components in main.
var App *tview.Application = tview.NewApplication()

// LaunchTUI launches the TUI.
func LaunchTUI(dMap *dmap.Dmap) {

	// Create the tree view.
	tree := tview.NewTreeView().
		SetRoot(tview.NewTreeNode("Duplicates").
			SetColor(tcell.ColorGreen)).
		SetTopLevel(0)

	// Border styling and title
	tree.SetBorder(true).
		SetBorderPadding(0, 0, 1, 1).
		SetTitleColor(tcell.ColorDeepSkyBlue).
		SetTitle("dskDitto: Interactive Duplicate Management [m=mark, d=delete, q=quit]").
		SetBorderColor(tcell.ColorGreen)

	// Add the nodes to the tree.
	addTreeData(tree, dMap)

	// Map to keep track of marked items
	markedItems := make(map[string]*tview.TreeNode)

	// Auto-mark all but the first file in each duplicate group for deletion
	// This is supposed to be a UX convenience feature.
	autoMarkDuplicates(tree, markedItems)

	root := tree.GetRoot()
	if root != nil && len(root.GetChildren()) > 0 {
		// We want arrow keys to be able to navigate through the tree.
		tree.SetCurrentNode(root.GetChildren()[0])
	}

	// Key bindings for user actions
	tree.SetInputCapture(func(event *tcell.EventKey) *tcell.EventKey {
		switch event.Key() {
		// Stop the app. Quit view.
		case tcell.KeyEsc:
			App.Stop()
		case tcell.KeyRune:
			switch event.Rune() {
			case 'q':
				App.Stop()

			// mark/unmark current file for deletion
			case 'm':
				currentNode := tree.GetCurrentNode()
				if currentNode.GetLevel() != 2 {
					goto Skip
				}
				filePath := currentNode.GetText()
				if _, ok := markedItems[filePath]; !ok {
					markedItems[filePath] = currentNode
					currentNode.SetColor(tcell.ColorRed)
				} else {
					currentNode.SetColor(tcell.ColorWhite)
					delete(markedItems, filePath)
				}

			case 'd':
				if len(markedItems) == 0 {
					goto Skip
				}
				showDeleteConfirmation(markedItems, tree)
			}
		}
	Skip:
		return event
	})

	// Expand or collapse the node.
	tree.SetSelectedFunc(func(node *tview.TreeNode) {
		if node.IsExpanded() {
			node.Collapse()
		} else {
			node.Expand()
		}
	})

	// Launch the TUI.
	if err := App.SetRoot(tree, true).
		EnableMouse(true).
		SetFocus(tree).
		Run(); err != nil {
		panic(err)
	}
}

// addTreeData adds the duplicate file information to the tree.
func addTreeData(tree *tview.TreeView, dMap *dmap.Dmap) {

	// Ensure tree and dMap are not nil
	if tree == nil || dMap == nil {
		dsklog.Dlogger.Debug("tree or dMap is nil")
	}

	if tree.GetRoot() == nil {
		tree.SetRoot(tview.NewTreeNode("Root"))
	}

	// Add the hash as root node and the files as children.
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
			dupSet := tview.NewTreeNode(header).SetSelectable(true)
			// Add our children under header.
			for _, file := range files {
				dupSet.AddChild(tview.NewTreeNode(file)).
					SetColor(tcell.ColorLightGreen)
			}
			tree.GetRoot().AddChild(dupSet)
		}
	}

}

// showDeleteConfirmation displays a confirmation prompt requiring a typed code before deleting files.
func showDeleteConfirmation(markedItems map[string]*tview.TreeNode, tree *tview.TreeView) {

	// Get code user must type
	code := GenConfirmationCode()

	// var fileListBuilder strings.Builder
	// for path := range markedItems {
	// 	fileListBuilder.WriteString("• ")
	// 	fileListBuilder.WriteString(filepath.Base(path))
	// 	fileListBuilder.WriteByte('\n')
	// }
	// fileList := strings.TrimSuffix(fileListBuilder.String(), "\n")

	infoText := fmt.Sprintf(
		"[white]Type the confirmation code below to delete [yellow]%d[white] file(s):\n\n[yellow]%s[white]\n",
		len(markedItems),
		code,
	)
	// if fileList != "" {
	// 	infoText += fmt.Sprintf("\nMarked files:\n[gray]%s[white]", fileList)
	// }

	// Information view
	infoView := tview.NewTextView().
		SetDynamicColors(true).
		SetWrap(true).
		SetScrollable(false).
		SetText(infoText)

	infoView.SetLabel("")
	infoView.SetDisabled(true)

	// Error view
	errorView := tview.NewTextView().
		SetDynamicColors(true).
		SetWrap(true).
		SetScrollable(false).
		SetTextAlign(tview.AlignCenter).
		SetText("")

	errorView.SetLabel("")
	errorView.SetDisabled(true)
	errorView.SetSize(1, 0)

	inputField := tview.NewInputField().
		SetLabel("Code: ").
		SetFieldWidth(len(code)).
		SetAcceptanceFunc(func(text string, ch rune) bool {
			if ch == 0 {
				return true
			}
			if len(text) >= len(code) {
				return false
			}
			if (ch >= 'A' && ch <= 'Z') || (ch >= 'a' && ch <= 'z') || (ch >= '0' && ch <= '9') {
				return true
			}
			return false
		})

	returnToTree := func() {
		App.SetRoot(tree, true).SetFocus(tree)
	}

	confirmDeletion := func() {
		if inputField.GetText() != code {
			errorView.SetText("[red]Incorrect code. Try again.")
			inputField.SetText("")
			App.SetFocus(inputField)
			App.Draw()
			return
		}
		returnToTree()
		performDeletion(markedItems, tree)
	}

	inputField.SetDoneFunc(func(key tcell.Key) {
		switch key {
		case tcell.KeyEnter:
			confirmDeletion()
		case tcell.KeyEscape:
			returnToTree()
		}
	})

	form := tview.NewForm()
	form.AddFormItem(infoView).
		AddFormItem(errorView).
		AddFormItem(inputField).
		AddButton("Delete", confirmDeletion).
		AddButton("Cancel", returnToTree).
		SetButtonsAlign(tview.AlignCenter)
	form.SetBorder(true)
	form.SetTitle("Confirm Deletion")
	form.SetCancelFunc(returnToTree)
	form.SetFocus(2) // focus the code field when the form gains focus

	layout := tview.NewGrid().
		SetRows(0, 0, 0).
		SetColumns(0, 60, 0).
		AddItem(form, 1, 1, 1, 1, 0, 0, true)

	layout.SetInputCapture(func(event *tcell.EventKey) *tcell.EventKey {
		if event.Key() == tcell.KeyEscape {
			returnToTree()
			return nil
		}
		return event
	})

	App.SetRoot(layout, true).SetFocus(form)
}

// performDeletion actually deletes the marked files
func performDeletion(markedItems map[string]*tview.TreeNode, tree *tview.TreeView) {
	for path, node := range markedItems {
		err := os.Remove(path)
		if err != nil {
			dsklog.Dlogger.Errorf("Failed to delete file %s: %v", path, err)
			node.SetColor(tcell.ColorGray).SetText("[ERROR] " + filepath.Base(path))
		} else {
			dsklog.Dlogger.Infof("Successfully deleted file: %s", path)
			node.SetColor(tcell.ColorGray).SetText("[DELETED] " + filepath.Base(path))
		}
	}
	App.Draw()
}

// autoMarkDuplicates automatically marks all but the first file in each duplicate group for deletion
func autoMarkDuplicates(tree *tview.TreeView, markedItems map[string]*tview.TreeNode) {
	root := tree.GetRoot()
	if root == nil {
		return
	}

	for _, groupNode := range root.GetChildren() {
		children := groupNode.GetChildren()
		if len(children) <= 1 {
			continue
		}
		for i, childNode := range children {
			if i == 0 {
				continue
			}
			filePath := childNode.GetText()
			markedItems[filePath] = childNode
			childNode.SetColor(tcell.ColorRed)
			dsklog.Dlogger.Infof("Auto-marked file for deletion: %s", filePath)
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
