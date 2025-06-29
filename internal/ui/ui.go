package ui

import (
	"fmt"
	"os"
	"path/filepath"

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

	root := tree.GetRoot()
	if root != nil && len(root.GetChildren()) > 0 {
		// We want arrow keys to be able to nacigate through the tree.
		tree.SetCurrentNode(root.GetChildren()[0])
	}

	// Map to keep track of marked items and their original colors
	markedItems := make(map[string]*tview.TreeNode)
	originalColors := make(map[string]tcell.Color)

	// Key binding to quit.
	tree.SetInputCapture(func(event *tcell.EventKey) *tcell.EventKey {
		switch event.Key() {
		case tcell.KeyEsc:
			App.Stop()
		case tcell.KeyRune:
			switch event.Rune() {
			case 'q':
				App.Stop()

			// Handle marking items for deletion
			case 'm':
				dsklog.Dlogger.Info("Marking item")
				currentNode := tree.GetCurrentNode()
				dsklog.Dlogger.Infof("Current node level: %d", currentNode.GetLevel())

				// Only allow marking of actual file paths (level 2 - children of duplicate groups)
				if currentNode.GetLevel() != 2 {
					dsklog.Dlogger.Info("Can only mark individual files for deletion")
					goto Skip
				}

				filePath := currentNode.GetText()
				if _, ok := markedItems[filePath]; !ok {
					// Mark the file - store original color and mark it
					originalColors[filePath] = currentNode.GetColor()
					markedItems[filePath] = currentNode
					dsklog.Dlogger.Infof("Marked item for deletion: %s", filePath)
					currentNode.SetColor(tcell.ColorYellow)
				} else {
					// Unmark the file - restore original color and remove from marked list
					if originalColor, exists := originalColors[filePath]; exists {
						currentNode.SetColor(originalColor)
						delete(originalColors, filePath)
					} else {
						// Fallback to default color if original color not found
						currentNode.SetColor(tcell.ColorLightGreen)
					}
					delete(markedItems, filePath)
					dsklog.Dlogger.Infof("Unmarked item: %s", filePath)
				}

			case 'd':
				if len(markedItems) == 0 {
					dsklog.Dlogger.Info("No items marked for deletion")
					goto Skip
				}

				// Show confirmation dialog before deleting
				showDeleteConfirmation(markedItems, originalColors, tree)
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

	// Get file size in bytes..
	getFileSize := func(file_name string) uint64 {
		file, err := os.Stat(file_name)
		if err != nil {
			return 0
		}
		size := file.Size()
		if size < 0 {
			return 0
		}
		return uint64(size)
	}

	// Add the hash as root node and the files as children.
	// BUG(jdefr): Map seems to be getting an empty hash somewhere.
	for hash, files := range dMap.GetMap() {

		// TODO(jdefr): Fix reason for empty hash entry. This shouldn't occur.
		if hash == "" {
			continue
		}

		if len(files) > 1 {
			var fmt_str = "%s - %d Duplicates - (Using %s of storage total)"
			fSize := getFileSize(files[0])
			totalSize := uint64(fSize) * uint64(len(files))
			// Create header with relevant information
			header := fmt.Sprintf(fmt_str, hash[:8], len(files), utils.DisplaySize(totalSize))
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
func showDeleteConfirmation(markedItems map[string]*tview.TreeNode, originalColors map[string]tcell.Color, tree *tview.TreeView) {
	// Create a modal with the list of files to be deleted
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
				performDeletion(markedItems, originalColors)
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
			performDeletion(markedItems, originalColors)
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
func performDeletion(markedItems map[string]*tview.TreeNode, originalColors map[string]tcell.Color) {
	for path, node := range markedItems {
		err := os.Remove(path)
		if err != nil {
			dsklog.Dlogger.Errorf("Failed to delete file %s: %v", path, err)
			node.SetColor(tcell.ColorRed).SetText("[ERROR] " + filepath.Base(path))
		} else {
			dsklog.Dlogger.Infof("Successfully deleted file: %s", path)
			node.SetColor(tcell.ColorGray).SetText("[DELETED] " + filepath.Base(path))
		}
		// Clean up the original color tracking since the file is now processed
		delete(originalColors, path)
	}
	App.Draw()
}
