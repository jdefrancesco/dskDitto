package ui

import (
	"fmt"
	"os"
	"path/filepath"

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

	tree := tview.NewTreeView().
		SetRoot(tview.NewTreeNode("Duplicates").
			SetColor(tcell.ColorGreen)).
		SetTopLevel(0)

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
	autoMarkDuplicates(tree, markedItems)

	root := tree.GetRoot()
	if root != nil && len(root.GetChildren()) > 0 {
		// We want arrow keys to be able to nacigate through the tree.
		tree.SetCurrentNode(root.GetChildren()[0])
	}

	 // Key bindings for user actions
	 tree.SetInputCapture(func(event *tcell.EventKey) *tcell.EventKey {
	 	switch event.Key() {
	 	case tcell.KeyEsc:
	 		App.Stop()
	 	case tcell.KeyRune:
	 		switch event.Rune() {
	 		case 'q':
	 			App.Stop()

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

	tree.SetSelectedFunc(func(node *tview.TreeNode) {
		// Expand or collapse the node.
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
	// BUG(jdefr): Map seems to be getting an empty hash somewhere.
	for hash, files := range dMap.GetMap() {

		var zeroHash dmap.SHA256Hash
		if hash == zeroHash {
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

// showDeleteConfirmation displays a modal dialog asking for confirmation before deleting files
func showDeleteConfirmation(markedItems map[string]*tview.TreeNode, tree *tview.TreeView) {

	// Create a modal window with the list of files to be deleted
	fileList := ""
	for path := range markedItems {
		fileList += fmt.Sprintf("• %s\n", filepath.Base(path))
	}

	message := fmt.Sprintf("Are you sure you want to delete %d file(s)?\n\n%s\nPress 'y' to confirm, any other key to cancel.",
		len(markedItems), fileList)
	modal := tview.NewModal().
		SetText(message).
		AddButtons([]string{"Cancel", "Delete"}).
		SetDoneFunc(func(buttonIndex int, buttonLabel string) {
			App.SetRoot(tree, true).SetFocus(tree)

			// If user clicked "Delete" button (index 1)
			if buttonIndex == 1 {
				performDeletion(markedItems, tree)
			}
		})

	// Set up key capture for the modal to handle 'y' key and Escape
	modal.SetInputCapture(func(event *tcell.EventKey) *tcell.EventKey {
		switch event.Key() {
		case tcell.KeyEscape:
			App.SetRoot(tree, true).SetFocus(tree)
			return nil
		}

		switch event.Rune() {
		case 'y', 'Y':
			App.SetRoot(tree, true).SetFocus(tree)
			performDeletion(markedItems, tree)
			return nil
		case 'n', 'N':
			App.SetRoot(tree, true).SetFocus(tree)
			return nil
		}
		// For any other key, let the modal handle it normally (like Tab, Enter)
		return event
	})

	App.SetRoot(modal, true).SetFocus(modal)
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
