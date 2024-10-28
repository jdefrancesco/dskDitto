package ui

import (
	"fmt"
	"log"
	"os"

	"ditto/internal/dmap"
	"ditto/internal/dsklog"

	"ditto/pkg/utils"

	"github.com/gdamore/tcell/v2"
	"github.com/rivo/tview"
)

// LaunchTUI launches the TUI.
func LaunchTUI(dMap *dmap.Dmap) {

	app := tview.NewApplication()
	tree := tview.NewTreeView().
		SetRoot(tview.NewTreeNode("Duplicates").SetColor(tcell.ColorGreen)).
		SetTopLevel(0)

	tree.SetBorder(true).
		SetBorderPadding(0, 0, 1, 1).
		SetTitleColor(tcell.ColorDeepSkyBlue).
		SetTitle("dskDitto: Interactive Duplicate Management").SetBorderColor(tcell.ColorGreen)

	// Add the nodes to the tree.
	addTreeData(tree, dMap)

	// Map to keep track of marked items
	markedItems := make(map[string]*tview.TreeNode)

	// Key binding to quit.
	tree.SetInputCapture(func(event *tcell.EventKey) *tcell.EventKey {
		switch event.Key() {
		case tcell.KeyEsc:
			app.Stop()
		case tcell.KeyRune:
			switch event.Rune() {
			case 'q':
				app.Stop()

			// Handle marking items for deletion
			case 'm':
				log.Println("Marking item")
				currentNode := tree.GetCurrentNode()
				log.Printf("Current node: %d", currentNode.GetLevel())
				// Skip selection of root node for now.
				if currentNode.GetLevel() == 1 {
					goto Skip
				}

				if node, ok := markedItems[currentNode.GetText()]; !ok {
					markedItems[currentNode.GetText()] = currentNode
					log.Printf("Marked item: %v", markedItems)
					currentNode.SetColor(tcell.ColorYellow)
				} else {
					delete(markedItems, node.GetText())
					node.SetColor(tcell.ColorWhite)
					log.Printf("Unmarking item: %v", markedItems)
				}

			case 'd':
				// for path, node := range markedItems {
				// 	err := os.Remove(path)
				// 	if err != nil {
				// 		log.Printf("Failed to delete file: %s", err)
				// 	} else {
				// 		node.SetColor(tcell.ColorGray).SetText("[Deleted] " + filepath.Base(path))
				// 	}
				// }
				// markedItems = make(map[string]*tview.TreeNode) // Clear marked items
				app.Draw()
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
	if err := app.SetRoot(tree, true).
		EnableMouse(true).
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
		return uint64(file.Size())
	}

	// Add the hash as root node and the files as children.
	for hash, files := range dMap.GetMap() {

		// XXX: Right now something isn't hashing correctly (symlinks). Fix for real later.
		if hash == "" {
			continue
		}

		dsklog.Dlogger.Printf("Hash: %s\n", hash)
		if len(files) > 1 {
			var fmt_str = "%s - %d Duplicates - (Using %s of storage total)"
			fSize := getFileSize(files[0])
			totalSize := uint64(fSize) * uint64(len(files))
			header := fmt.Sprintf(fmt_str, hash[:8], len(files), utils.DisplaySize(totalSize))
			dupSet := tview.NewTreeNode(header).SetSelectable(true)
			for _, file := range files {
				dupSet.AddChild(tview.NewTreeNode(file)).
					SetColor(tcell.ColorLightGreen)
			}
			tree.GetRoot().AddChild(dupSet)
		}
	}

}
