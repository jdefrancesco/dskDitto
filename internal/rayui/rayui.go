package rayui

import (
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"time"

	"github.com/jdefrancesco/dskDitto/internal/dmap"
	"github.com/jdefrancesco/dskDitto/internal/dsklog"
	"github.com/jdefrancesco/dskDitto/internal/dupview"
	"github.com/jdefrancesco/dskDitto/pkg/utils"

	rl "github.com/gen2brain/raylib-go/raylib"
)

// TODO:
// 1. The scrolling with mouse isn't always working.
// 2. The hash dup batch header make stand out tiny bit more with some color.
// 3. The legend at bottom needs no entry for 1/2 sort
// 4. Banner, get rid of the Raylib mode thingy
// 5. Add a flag so no need to enter code to remove files.

const (
	initialWidth  int32 = 1000
	initialHeight int32 = 760
	minWidth            = 760
	minHeight           = 520

	defaultMargin       float32 = 18
	defaultGap          float32 = 14
	defaultHeaderHeight float32 = 66
	defaultFooterHeight float32 = 38
	defaultRowHeight    float32 = 34
	minListHeight       float32 = 150
	bottomInspectorH    float32 = 116
)

var (
	colorBackground = rl.NewColor(246, 248, 250, 255)
	colorHeader     = rl.NewColor(12, 80, 70, 255)
	colorHeaderDeep = rl.NewColor(8, 55, 49, 255)
	colorHeaderText = rl.NewColor(250, 252, 252, 255)
	colorSurface    = rl.NewColor(255, 255, 255, 255)
	colorSubtle     = rl.NewColor(240, 244, 247, 255)
	colorBorder     = rl.NewColor(210, 220, 229, 255)
	colorBorderSoft = rl.NewColor(228, 235, 241, 255)
	colorText       = rl.NewColor(17, 24, 39, 255)
	colorMuted      = rl.NewColor(96, 111, 128, 255)
	colorAccent     = rl.NewColor(15, 118, 110, 255)
	colorAccentSoft = rl.NewColor(218, 242, 238, 255)
	colorSelected   = rl.NewColor(228, 244, 240, 255)
	colorMarked     = rl.NewColor(180, 83, 9, 255)
	colorMarkedSoft = rl.NewColor(255, 247, 237, 255)
	colorSuccess    = rl.NewColor(22, 101, 52, 255)
	colorDanger     = rl.NewColor(185, 28, 28, 255)
	colorDangerSoft = rl.NewColor(254, 226, 226, 255)
	colorDisabled   = rl.NewColor(232, 238, 244, 255)
)

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

type nodeRef struct {
	typ   nodeType
	group int
	file  int
}

type button struct {
	id      string
	label   string
	rect    rl.Rectangle
	enabled bool
	danger  bool
	primary bool
}

type layout struct {
	screenWidth  float32
	screenHeight float32
	margin       float32
	gap          float32

	header  rl.Rectangle
	toolbar rl.Rectangle
	list    rl.Rectangle
	sidebar rl.Rectangle
	footer  rl.Rectangle

	showSidebar bool
	rowHeight   float32

	titleSize int32
	bodySize  int32
	rowSize   int32
	smallSize int32

	toolbarButtons []button
}

type fontSet struct {
	regular       rl.Font
	mono          rl.Font
	regularLoaded bool
	monoLoaded    bool
}

type app struct {
	results *dupview.Model
	visible []nodeRef
	layout  layout

	cursor int
	scroll int

	mode   viewMode
	action dupview.Action

	confirmCode  string
	confirmInput string
	confirmError string

	lastClickIdx int
	lastClickAt  time.Time
	lastGroupIdx int

	wheelRemainder float32
	quit           bool
}

var activeFonts fontSet

func Launch(dMap *dmap.Dmap) {
	if dMap == nil {
		if dsklog.Dlogger != nil {
			dsklog.Dlogger.Warn("nil duplicate map supplied to Raylib UI")
		}
		return
	}

	runtime.LockOSThread()
	defer runtime.UnlockOSThread()

	rl.SetConfigFlags(rl.FlagWindowResizable | rl.FlagMsaa4xHint | rl.FlagWindowHighdpi)
	rl.InitWindow(initialWidth, initialHeight, "dskDitto - Raylib Results")
	defer rl.CloseWindow()
	rl.SetWindowMinSize(minWidth, minHeight)
	rl.SetExitKey(rl.KeyNull)
	rl.SetTargetFPS(60)
	activeFonts = loadFonts()
	defer activeFonts.unload()

	a := newApp(dupview.New(dMap))
	for !a.quit && !rl.WindowShouldClose() {
		a.update()
		rl.BeginDrawing()
		a.draw()
		rl.EndDrawing()
	}
}

func newApp(results *dupview.Model) *app {
	a := &app{
		results:      results,
		lastClickIdx: -1,
		lastGroupIdx: -1,
	}
	a.refreshLayout()
	a.rebuildVisibleNodes()
	return a
}

func (a *app) update() {
	oldRows := a.visibleRows()
	a.refreshLayout()
	if rl.IsWindowResized() || oldRows != a.visibleRows() {
		a.adjustScroll()
	}

	if a.mode == modeConfirm {
		a.updateConfirm()
		return
	}

	a.updateKeyboard()
	a.updateMouse()
	a.adjustScroll()
}

func (a *app) updateKeyboard() {
	switch {
	case rl.IsKeyPressed(rl.KeyQ):
		a.quit = true
	case rl.IsKeyPressed(rl.KeyEscape):
		a.quit = true
	case keyPressed(rl.KeyUp) || keyPressed(rl.KeyK):
		a.moveCursor(-1)
	case keyPressed(rl.KeyDown) || keyPressed(rl.KeyJ):
		a.moveCursor(1)
	case keyPressed(rl.KeyPageUp):
		a.pageMove(-1)
	case keyPressed(rl.KeyPageDown):
		a.pageMove(1)
	case rl.IsKeyPressed(rl.KeyHome):
		a.cursor = 0
	case rl.IsKeyPressed(rl.KeyEnd):
		a.cursor = max(len(a.visible)-1, 0)
	case rl.IsKeyPressed(rl.KeyLeft) || rl.IsKeyPressed(rl.KeyH):
		a.collapseCurrentGroup()
	case rl.IsKeyPressed(rl.KeyRight):
		a.expandCurrentGroup()
	case rl.IsKeyPressed(rl.KeyL):
		if shiftDown() {
			a.startConfirmation(dupview.ActionLink)
		} else {
			a.expandCurrentGroup()
		}
	case rl.IsKeyPressed(rl.KeyEnter):
		a.toggleCurrentGroup()
	case rl.IsKeyPressed(rl.KeySpace) || rl.IsKeyPressed(rl.KeyM):
		a.toggleCurrentFileMark()
	case rl.IsKeyPressed(rl.KeyA):
		dupview.MarkAll(a.results.Groups)
		a.results.Result = ""
	case rl.IsKeyPressed(rl.KeyU):
		dupview.UnmarkAll(a.results.Groups)
		a.results.Result = ""
	case rl.IsKeyPressed(rl.KeyD):
		a.startConfirmation(dupview.ActionDelete)
	case rl.IsKeyPressed(rl.KeyOne):
		a.setSortMode(dupview.SortByTotalSize)
	case rl.IsKeyPressed(rl.KeyTwo):
		a.setSortMode(dupview.SortByCount)
	case rl.IsKeyPressed(rl.KeyS):
		a.results.CycleSortMode()
		a.rebuildVisibleNodes()
	}
}

func (a *app) updateMouse() {
	mouse := rl.GetMousePosition()
	for _, b := range a.layout.toolbarButtons {
		if !b.enabled || !rl.IsMouseButtonPressed(rl.MouseButtonLeft) {
			continue
		}
		if rl.CheckCollisionPointRec(mouse, b.rect) {
			a.handleButton(b.id)
			return
		}
	}

	listContent := insetRect(a.layout.list, 1)
	if !rl.CheckCollisionPointRec(mouse, listContent) {
		return
	}

	if wheel := rl.GetMouseWheelMove(); wheel != 0 {
		a.applyWheelScroll(wheel)
	}

	if !rl.IsMouseButtonPressed(rl.MouseButtonLeft) {
		return
	}

	row := int((mouse.Y-listContent.Y)/a.layout.rowHeight) + a.scroll
	if row < 0 || row >= len(a.visible) {
		return
	}

	now := time.Now()
	doubleClick := row == a.lastClickIdx && now.Sub(a.lastClickAt) <= 350*time.Millisecond
	a.cursor = row
	a.adjustScroll()

	ref := a.visible[row]
	if ref.typ == nodeGroup && doubleClick {
		a.toggleCurrentGroup()
		a.lastClickIdx = -1
		a.lastClickAt = time.Time{}
		return
	}
	if ref.typ == nodeFile && mouse.X <= listContent.X+68 {
		a.toggleCurrentFileMark()
	}

	a.lastClickIdx = row
	a.lastClickAt = now
}

func (a *app) updateConfirm() {
	if rl.IsKeyPressed(rl.KeyEscape) {
		a.cancelConfirmation()
		return
	}
	if rl.IsKeyPressed(rl.KeyEnter) {
		if a.confirmInput == a.confirmCode {
			a.mode = modeTree
			a.confirmError = ""
			a.confirmInput = ""
			switch a.action {
			case dupview.ActionLink:
				a.results.Result = dupview.LinkMarked(a.results.Groups)
			default:
				a.results.Result = dupview.DeleteMarked(a.results.Groups)
			}
		} else {
			a.confirmError = "Incorrect code. Try again."
			a.confirmInput = ""
		}
		return
	}
	if rl.IsKeyPressed(rl.KeyBackspace) && len(a.confirmInput) > 0 {
		a.confirmInput = a.confirmInput[:len(a.confirmInput)-1]
		return
	}

	for ch := rl.GetCharPressed(); ch != 0; ch = rl.GetCharPressed() {
		if len(a.confirmInput) >= len(a.confirmCode) {
			continue
		}
		r := rune(ch)
		if dupview.IsAlphaNumeric(r) {
			a.confirmInput += string(r)
		}
	}
}

func (a *app) draw() {
	rl.ClearBackground(colorBackground)
	a.drawHeader()
	a.drawToolbar()
	a.drawList()
	a.drawSelectionPanel()
	a.drawFooter()
	if a.mode == modeConfirm {
		a.drawConfirmModal()
	}
}

func (a *app) drawHeader() {
	l := a.layout
	rl.DrawRectangleRec(l.header, colorHeaderDeep)
	rl.DrawRectangleRec(rl.NewRectangle(0, 0, l.screenWidth, l.header.Height-5), colorHeader)
	rl.DrawRectangleRec(rl.NewRectangle(0, l.header.Height-5, l.screenWidth, 5), colorAccent)
	drawText("dskDitto", l.margin, 13, 28, colorHeaderText)
	drawText("Duplicate review", l.margin+126, 23, 16, rl.NewColor(204, 251, 241, 255))

	pill := "Raylib mode"
	pillW := measureText(pill, 14) + 24
	pillRect := rl.NewRectangle(l.screenWidth-pillW-l.margin, 18, pillW, 28)
	rl.DrawRectangleRounded(pillRect, 0.45, 12, rl.NewColor(9, 65, 57, 255))
	drawText(pill, pillRect.X+12, pillRect.Y+7, 14, rl.NewColor(217, 249, 240, 255))
}

func (a *app) drawToolbar() {
	l := a.layout
	rl.DrawRectangleRec(l.toolbar, colorBackground)
	rl.DrawLine(0, int32(l.toolbar.Y+l.toolbar.Height), int32(l.screenWidth), int32(l.toolbar.Y+l.toolbar.Height), colorBorderSoft)
	for _, b := range l.toolbarButtons {
		drawButton(b)
	}
}

func (a *app) drawList() {
	list := a.layout.list
	drawPanel(list)

	if len(a.visible) == 0 {
		title := "No duplicates found"
		subtitle := "This scan has no duplicate groups to review."
		titleW := measureText(title, 22)
		subtitleW := measureText(subtitle, 15)
		centerY := list.Y + list.Height/2 - 22
		rl.DrawCircle(int32(list.X+list.Width/2), int32(centerY-32), 22, colorAccentSoft)
		drawText(title, list.X+(list.Width-titleW)/2, centerY, 22, colorText)
		drawText(subtitle, list.X+(list.Width-subtitleW)/2, centerY+32, 15, colorMuted)
		return
	}

	content := insetRect(list, 1)
	rows := a.visibleRows()
	start := clamp(a.scroll, 0, max(len(a.visible)-1, 0))
	end := min(start+rows, len(a.visible))
	y := content.Y
	rl.BeginScissorMode(int32(content.X), int32(content.Y), int32(content.Width), int32(content.Height))
	for i := start; i < end; i++ {
		ref := a.visible[i]
		row := rl.NewRectangle(content.X, y, content.Width, a.layout.rowHeight)
		a.drawRow(ref, row, i == a.cursor)
		y += a.layout.rowHeight
	}
	rl.EndScissorMode()
}

func (a *app) drawRow(ref nodeRef, rect rl.Rectangle, selected bool) {
	if selected {
		rl.DrawRectangleRec(rect, colorSelected)
	}
	rl.DrawLine(int32(rect.X), int32(rect.Y+rect.Height), int32(rect.X+rect.Width), int32(rect.Y+rect.Height), colorBorder)

	switch ref.typ {
	case nodeGroup:
		group := a.results.Groups[ref.group]
		if selected {
			rl.DrawRectangleRec(rl.NewRectangle(rect.X, rect.Y, 3, rect.Height), colorAccent)
		}
		drawChevron(rect.X+18, rect.Y+rect.Height/2, group.Expanded, colorAccent)
		title := truncateText(formatCompactGroupTitle(group), a.layout.rowSize, rect.Width-58)
		drawText(title, rect.X+40, textY(rect, a.layout.rowSize), a.layout.rowSize, colorText)
	case nodeFile:
		entry := a.results.Groups[ref.group].Files[ref.file]
		box := rl.NewRectangle(rect.X+32, rect.Y+(rect.Height-16)/2, 16, 16)
		drawCheckbox(box, entry.Marked)

		path := entry.Path
		if dupview.IsSymlink(entry.Path) {
			path += " [symlink]"
		}
		status := fileStatusLabel(entry)
		statusWidth := measureText(status, a.layout.smallSize)
		pathMax := rect.Width - 82 - statusWidth
		if pathMax < 80 {
			pathMax = rect.Width - 82
			status = ""
			statusWidth = 0
		}
		drawText(truncateText(path, a.layout.rowSize, pathMax), rect.X+68, textY(rect, a.layout.rowSize), a.layout.rowSize, colorText)
		if status != "" {
			drawText(status, rect.X+rect.Width-statusWidth-14, textY(rect, a.layout.smallSize), a.layout.smallSize, fileStatusColor(entry))
		}
	}
}

func (a *app) drawSelectionPanel() {
	panel := a.layout.sidebar
	if panel.Width <= 0 || panel.Height <= 0 {
		return
	}
	drawPanel(panel)

	marked := dupview.CountMarked(a.results.Groups)
	markedLabel := fmt.Sprintf("%d marked files", marked)
	chipW := measureText(markedLabel, a.layout.bodySize) + 22
	chipY := panel.Y + 54
	chipX := panel.X + 18
	if !a.layout.showSidebar {
		chipY = panel.Y + 14
		chipX = panel.X + 112
	}
	drawText("Selection", panel.X+18, panel.Y+18, a.layout.titleSize, colorText)
	chipMaxW := max(panel.Width-(chipX-panel.X)-18, 0)
	chip := rl.NewRectangle(chipX, chipY, min(chipW, chipMaxW), 28)
	rl.DrawRectangleRounded(chip, 0.45, 12, colorMarkedSoft)
	drawText(truncateText(markedLabel, a.layout.bodySize, chip.Width-22), chip.X+11, chip.Y+7, a.layout.bodySize, colorMarked)

	group := a.selectedGroup()
	if group == nil {
		y := panel.Y + 104
		if !a.layout.showSidebar {
			y = panel.Y + 58
		}
		drawText("Select a group to inspect it.", panel.X+18, y, a.layout.bodySize, colorMuted)
		return
	}

	if a.layout.showSidebar {
		a.drawSidebarDetails(panel, group)
		return
	}
	a.drawBottomInspector(panel, group)
}

func (a *app) drawSidebarDetails(panel rl.Rectangle, group *dupview.Group) {
	y := panel.Y + 108
	drawText("Current group", panel.X+18, y, a.layout.bodySize, colorMuted)
	y += 30
	drawText(fmt.Sprintf("%d files", len(group.Files)), panel.X+18, y, 18, colorText)
	y += 28
	drawText(utils.DisplaySize(group.TotalSz), panel.X+18, y, 18, colorText)
	y += 34

	hash := hashPrefix(group)
	drawText("Hash prefix", panel.X+18, y, a.layout.bodySize, colorMuted)
	y += 26
	drawText(truncateText(hash, 16, panel.Width-36), panel.X+18, y, 16, colorText)
}

func (a *app) drawBottomInspector(panel rl.Rectangle, group *dupview.Group) {
	y := panel.Y + 58
	x := panel.X + 18
	metricGap := float32(120)
	drawText("Current group", x, y, a.layout.bodySize, colorMuted)
	drawText(fmt.Sprintf("%d files", len(group.Files)), x, y+24, 17, colorText)
	drawText(utils.DisplaySize(group.TotalSz), x+metricGap, y+24, 17, colorText)

	hash := hashPrefix(group)
	hashX := x + metricGap*2.25
	available := panel.X + panel.Width - hashX - 18
	if available > 80 {
		drawText("Hash prefix", hashX, y, a.layout.bodySize, colorMuted)
		drawText(truncateText(hash, 15, available), hashX, y+26, 15, colorText)
	}
}

func (a *app) drawFooter() {
	footer := a.layout.footer
	rl.DrawRectangleRec(footer, colorSurface)
	rl.DrawLine(0, int32(footer.Y), int32(a.layout.screenWidth), int32(footer.Y), colorBorder)

	help := footerHelp(a.layout.screenWidth)
	helpMax := a.layout.screenWidth - a.layout.margin*2
	if a.results.Result != "" {
		resultWidth := measureText(a.results.Result, a.layout.smallSize)
		helpMax = max(120, a.layout.screenWidth-resultWidth-a.layout.margin*3)
		drawText(a.results.Result, a.layout.screenWidth-resultWidth-a.layout.margin, footer.Y+12, a.layout.smallSize, colorSuccess)
	}
	drawText(truncateText(help, a.layout.smallSize, helpMax), a.layout.margin, footer.Y+12, a.layout.smallSize, colorMuted)
}

func (a *app) drawConfirmModal() {
	width := float32(rl.GetScreenWidth())
	height := float32(rl.GetScreenHeight())
	rl.DrawRectangleRec(rl.NewRectangle(0, 0, width, height), rl.NewColor(15, 23, 42, 135))

	panel := rl.NewRectangle(width/2-235, height/2-126, 470, 252)
	rl.DrawRectangleRounded(panel, 0.04, 8, colorSurface)
	rl.DrawRectangleRoundedLinesEx(panel, 0.04, 8, 1, colorBorder)

	title := "Confirm Deletion"
	verb := "delete"
	if a.action == dupview.ActionLink {
		title = "Confirm Symlink Conversion"
		verb = "convert to symlinks"
	}

	x := panel.X + 24
	y := panel.Y + 22
	drawText(title, x, y, 22, colorText)
	y += 42
	drawText(fmt.Sprintf("You are about to %s %d file(s).", verb, dupview.CountMarked(a.results.Groups)), x, y, 16, colorMuted)
	y += 36
	drawText("Confirmation code", x, y, 15, colorMuted)
	y += 24
	drawText(a.confirmCode, x, y, 24, colorMarked)
	y += 42
	drawText("Your input", x, y, 15, colorMuted)
	drawMonoText(a.confirmInput, x+92, y-2, 18, colorText)
	y += 32
	if a.confirmError != "" {
		drawText(a.confirmError, x, y, 15, colorDanger)
	} else {
		drawText("Enter confirms. Esc cancels.", x, y, 15, colorMuted)
	}
}

func (a *app) refreshLayout() {
	width := float32(rl.GetScreenWidth())
	height := float32(rl.GetScreenHeight())
	margin := defaultMargin
	gap := defaultGap
	if width < 920 {
		margin = 12
		gap = 10
	}

	rowHeight := defaultRowHeight
	rowSize := int32(14)
	bodySize := int32(15)
	smallSize := int32(13)
	if height < 620 {
		rowHeight = 30
		rowSize = 13
		bodySize = 14
		smallSize = 12
	}

	buttons, toolbarHeight := a.buildToolbarButtons(width, margin, gap)
	footerHeight := defaultFooterHeight
	headerHeight := defaultHeaderHeight
	contentTop := headerHeight + toolbarHeight + gap
	contentBottom := height - footerHeight - margin
	if contentBottom < contentTop+minListHeight {
		contentBottom = contentTop + minListHeight
	}

	showSidebar := width >= 1040
	var list rl.Rectangle
	var sidebar rl.Rectangle
	if showSidebar {
		sidebarW := clamp(width*0.18, 240, 280)
		listW := width - sidebarW - gap - margin*2
		list = rl.NewRectangle(margin, contentTop, max(listW, 280), contentBottom-contentTop)
		sidebar = rl.NewRectangle(list.X+list.Width+gap, contentTop, sidebarW, list.Height)
	} else {
		inspectorH := bottomInspectorH
		if height < 650 {
			inspectorH = 96
		}
		available := contentBottom - contentTop
		listH := max(available-inspectorH-gap, minListHeight)
		sidebar = rl.NewRectangle(margin, contentTop+listH+gap, width-margin*2, min(inspectorH, max(available-listH-gap, 0)))
		list = rl.NewRectangle(margin, contentTop, width-margin*2, listH)
	}

	a.layout = layout{
		screenWidth:    width,
		screenHeight:   height,
		margin:         margin,
		gap:            gap,
		header:         rl.NewRectangle(0, 0, width, headerHeight),
		toolbar:        rl.NewRectangle(0, headerHeight, width, toolbarHeight),
		list:           list,
		sidebar:        sidebar,
		footer:         rl.NewRectangle(0, height-footerHeight, width, footerHeight),
		showSidebar:    showSidebar,
		rowHeight:      rowHeight,
		titleSize:      20,
		bodySize:       bodySize,
		rowSize:        rowSize,
		smallSize:      smallSize,
		toolbarButtons: buttons,
	}
}

func (a *app) buildToolbarButtons(width, margin, gap float32) ([]button, float32) {
	x := margin
	y := defaultHeaderHeight + 14
	marked := dupview.CountMarked(a.results.Groups)
	sortLabel := "Sort: size"
	if a.results.SortMode == dupview.SortByCount {
		sortLabel = "Sort: count"
	}

	specs := []button{
		{id: "mark", label: "Mark all", enabled: len(a.results.Groups) > 0},
		{id: "clear", label: "Clear", enabled: marked > 0},
		{id: "delete", label: "Delete", enabled: marked > 0, danger: true},
		{id: "link", label: "Link", enabled: marked > 0, primary: true},
		{id: "sort", label: sortLabel, enabled: len(a.results.Groups) > 0},
	}

	buttons := make([]button, 0, len(specs))
	buttonHeight := float32(32)
	rowGap := float32(8)
	maxX := width - margin
	for _, spec := range specs {
		width := measureText(spec.label, 14) + 30
		if x > margin && x+width > maxX {
			x = margin
			y += buttonHeight + rowGap
		}
		spec.rect = rl.NewRectangle(x, y, width, 32)
		buttons = append(buttons, spec)
		x += width + gap
	}
	toolbarHeight := (y - defaultHeaderHeight) + buttonHeight + 14
	return buttons, toolbarHeight
}

func (a *app) handleButton(id string) {
	switch id {
	case "mark":
		dupview.MarkAll(a.results.Groups)
		a.results.Result = ""
	case "clear":
		dupview.UnmarkAll(a.results.Groups)
		a.results.Result = ""
	case "delete":
		a.startConfirmation(dupview.ActionDelete)
	case "link":
		a.startConfirmation(dupview.ActionLink)
	case "sort":
		a.results.CycleSortMode()
		a.rebuildVisibleNodes()
	}
}

func (a *app) visibleRows() int {
	if a.layout.rowHeight <= 0 {
		return 1
	}
	return max(int(insetRect(a.layout.list, 1).Height/a.layout.rowHeight), 1)
}

func (a *app) rebuildVisibleNodes() {
	a.visible = a.visible[:0]
	for gi, group := range a.results.Groups {
		a.visible = append(a.visible, nodeRef{typ: nodeGroup, group: gi})
		if group.Expanded {
			for fi := range group.Files {
				a.visible = append(a.visible, nodeRef{typ: nodeFile, group: gi, file: fi})
			}
		}
	}
	a.adjustScroll()
}

func (a *app) moveCursor(delta int) {
	if len(a.visible) == 0 {
		a.cursor = 0
		return
	}
	a.cursor = clamp(a.cursor+delta, 0, len(a.visible)-1)
	a.adjustScroll()
}

func (a *app) pageMove(direction int) {
	amount := a.visibleRows()
	if direction < 0 {
		a.moveCursor(-amount)
	} else {
		a.moveCursor(amount)
	}
}

func (a *app) currentNode() *nodeRef {
	if len(a.visible) == 0 || a.cursor < 0 || a.cursor >= len(a.visible) {
		return nil
	}
	return &a.visible[a.cursor]
}

func (a *app) collapseCurrentGroup() {
	node := a.currentNode()
	if node == nil || node.typ != nodeGroup {
		return
	}
	group := a.results.Groups[node.group]
	if group.Expanded {
		group.Expanded = false
		a.rebuildVisibleNodes()
	}
}

func (a *app) expandCurrentGroup() {
	node := a.currentNode()
	if node == nil || node.typ != nodeGroup {
		return
	}
	group := a.results.Groups[node.group]
	if !group.Expanded {
		group.Expanded = true
		a.rebuildVisibleNodes()
	}
}

func (a *app) toggleCurrentGroup() {
	node := a.currentNode()
	if node == nil || node.typ != nodeGroup {
		return
	}
	group := a.results.Groups[node.group]
	group.Expanded = !group.Expanded
	a.rebuildVisibleNodes()
}

func (a *app) toggleCurrentFileMark() {
	node := a.currentNode()
	if node == nil || node.typ != nodeFile {
		return
	}
	entry := a.results.Groups[node.group].Files[node.file]
	if entry.Status == dupview.FileStatusDeleted {
		return
	}
	entry.Marked = !entry.Marked
	a.results.Result = ""
}

func (a *app) startConfirmation(action dupview.Action) {
	if dupview.CountMarked(a.results.Groups) == 0 {
		return
	}
	a.action = action
	a.confirmCode = dupview.GenConfirmationCode()
	a.confirmInput = ""
	a.confirmError = ""
	a.results.Result = ""
	a.mode = modeConfirm
}

func (a *app) cancelConfirmation() {
	a.mode = modeTree
	a.confirmInput = ""
	a.confirmError = ""
}

func (a *app) setSortMode(mode dupview.SortMode) {
	a.results.SetSortMode(mode)
	a.rebuildVisibleNodes()
}

func (a *app) adjustScroll() {
	if len(a.visible) == 0 {
		a.cursor = 0
		a.scroll = 0
		a.lastGroupIdx = -1
		return
	}
	a.cursor = clamp(a.cursor, 0, len(a.visible)-1)
	a.clampScroll()
	if a.cursor < a.scroll {
		a.scroll = a.cursor
	}
	rows := a.visibleRows()
	if a.cursor >= a.scroll+rows {
		a.scroll = a.cursor - rows + 1
	}
	a.clampScroll()
	a.recordGroupFocus()
}

func (a *app) applyWheelScroll(wheel float32) {
	if len(a.visible) == 0 {
		a.wheelRemainder = 0
		return
	}

	a.wheelRemainder += wheel * 3
	delta := int(a.wheelRemainder)
	if delta == 0 {
		return
	}

	a.scroll -= delta
	a.wheelRemainder -= float32(delta)
	a.clampScroll()
}

func (a *app) clampScroll() {
	maxScroll := max(len(a.visible)-a.visibleRows(), 0)
	a.scroll = clamp(a.scroll, 0, maxScroll)
}

func (a *app) recordGroupFocus() {
	node := a.currentNode()
	if node == nil || node.typ != nodeGroup {
		return
	}
	a.lastGroupIdx = node.group
}

func (a *app) selectedGroup() *dupview.Group {
	node := a.currentNode()
	if node != nil && node.group >= 0 && node.group < len(a.results.Groups) {
		return a.results.Groups[node.group]
	}
	if a.lastGroupIdx >= 0 && a.lastGroupIdx < len(a.results.Groups) {
		return a.results.Groups[a.lastGroupIdx]
	}
	return nil
}

func drawButton(b button) {
	fill := colorSurface
	text := colorText
	border := colorBorder
	if !b.enabled {
		fill = colorDisabled
		text = colorMuted
		border = colorBorderSoft
	} else if b.danger {
		fill = colorDanger
		text = rl.White
		border = colorDanger
	} else if b.primary {
		fill = colorAccent
		text = rl.White
		border = colorAccent
	}
	rl.DrawRectangleRounded(b.rect, 0.20, 10, fill)
	rl.DrawRectangleRoundedLinesEx(b.rect, 0.20, 10, 1, border)

	textWidth := measureText(b.label, 14)
	drawText(b.label, b.rect.X+(b.rect.Width-textWidth)/2, b.rect.Y+9, 14, text)
}

func drawPanel(rect rl.Rectangle) {
	shadow := rl.NewRectangle(rect.X+1, rect.Y+2, rect.Width, rect.Height)
	rl.DrawRectangleRounded(shadow, 0.025, 10, rl.NewColor(15, 23, 42, 18))
	rl.DrawRectangleRounded(rect, 0.025, 10, colorSurface)
	rl.DrawRectangleRoundedLinesEx(rect, 0.025, 10, 1, colorBorder)
}

func loadFonts() fontSet {
	atlasSize := fontAtlasSize()
	glyphs := uiGlyphs()
	regular, regularLoaded := loadFontFromCandidates(fontCandidates(false), atlasSize, glyphs)
	mono, monoLoaded := loadFontFromCandidates(fontCandidates(true), atlasSize, glyphs)
	if !monoLoaded && regularLoaded {
		mono = regular
	}
	return fontSet{
		regular:       regular,
		mono:          mono,
		regularLoaded: regularLoaded,
		monoLoaded:    monoLoaded,
	}
}

func loadFontFromCandidates(paths []string, atlasSize int32, glyphs []rune) (rl.Font, bool) {
	glyphCount := boundedGlyphCount(glyphs)
	if glyphCount == 0 {
		return rl.Font{}, false
	}
	for _, path := range paths {
		if path == "" || !fileExists(path) {
			continue
		}
		font := rl.LoadFontEx(path, atlasSize, glyphs, glyphCount)
		if !rl.IsFontValid(font) {
			continue
		}
		rl.GenTextureMipmaps(&font.Texture)
		rl.SetTextureFilter(font.Texture, rl.FilterTrilinear)
		return font, true
	}
	return rl.Font{}, false
}

func boundedGlyphCount(glyphs []rune) int32 {
	if len(glyphs) > 1<<31-1 {
		return 0
	}
	return int32(len(glyphs)) // #nosec G115 -- guarded above for Raylib's int32 API.
}

func fontAtlasSize() int32 {
	dpi := rl.GetWindowScaleDPI()
	scale := max(dpi.X, dpi.Y)
	if scale < 1 {
		scale = 1
	}
	return int32(clamp(96*scale, 96, 192))
}

func uiGlyphs() []rune {
	glyphs := make([]rune, 0, 224)
	for r := rune(32); r <= 255; r++ {
		glyphs = append(glyphs, r)
	}
	return glyphs
}

func fontCandidates(mono bool) []string {
	if mono {
		candidates := []string{os.Getenv("DSKDITTO_RAYLIB_MONO_FONT")}
		switch runtime.GOOS {
		case "darwin":
			candidates = append(candidates,
				"/System/Library/Fonts/Menlo.ttc",
				"/System/Library/Fonts/Supplemental/Courier New.ttf",
				"/System/Library/Fonts/SFNSMono.ttf",
			)
		case "windows":
			winDir := os.Getenv("WINDIR")
			candidates = append(candidates,
				filepath.Join(winDir, "Fonts", "consola.ttf"),
				filepath.Join(winDir, "Fonts", "CascadiaMono.ttf"),
			)
		default:
			candidates = append(candidates,
				"/usr/share/fonts/truetype/dejavu/DejaVuSansMono.ttf",
				"/usr/share/fonts/truetype/liberation2/LiberationMono-Regular.ttf",
				"/usr/share/fonts/truetype/noto/NotoSansMono-Regular.ttf",
			)
		}
		return candidates
	}

	candidates := []string{os.Getenv("DSKDITTO_RAYLIB_FONT")}
	switch runtime.GOOS {
	case "darwin":
		candidates = append(candidates,
			"/System/Library/Fonts/Supplemental/Arial.ttf",
			"/System/Library/Fonts/Helvetica.ttc",
			"/System/Library/Fonts/SFNS.ttf",
		)
	case "windows":
		winDir := os.Getenv("WINDIR")
		candidates = append(candidates,
			filepath.Join(winDir, "Fonts", "segoeui.ttf"),
			filepath.Join(winDir, "Fonts", "arial.ttf"),
		)
	default:
		candidates = append(candidates,
			"/usr/share/fonts/truetype/dejavu/DejaVuSans.ttf",
			"/usr/share/fonts/truetype/liberation2/LiberationSans-Regular.ttf",
			"/usr/share/fonts/truetype/noto/NotoSans-Regular.ttf",
		)
	}
	return candidates
}

func fileExists(path string) bool {
	info, err := os.Stat(path)
	return err == nil && !info.IsDir()
}

func (f fontSet) regularFont() rl.Font {
	if rl.IsFontValid(f.regular) {
		return f.regular
	}
	return rl.GetFontDefault()
}

func (f fontSet) monoFont() rl.Font {
	if rl.IsFontValid(f.mono) {
		return f.mono
	}
	return f.regularFont()
}

func (f fontSet) unload() {
	if f.monoLoaded && f.mono.Texture.ID != f.regular.Texture.ID {
		rl.UnloadFont(f.mono)
	}
	if f.regularLoaded {
		rl.UnloadFont(f.regular)
	}
}

func drawText(text string, x, y float32, size int32, color rl.Color) {
	rl.DrawTextEx(activeFonts.regularFont(), text, rl.NewVector2(x, y), float32(size), 0, color)
}

func drawMonoText(text string, x, y float32, size int32, color rl.Color) {
	rl.DrawTextEx(activeFonts.monoFont(), text, rl.NewVector2(x, y), float32(size), 0, color)
}

func measureText(text string, size int32) float32 {
	if text == "" {
		return 0
	}
	return rl.MeasureTextEx(activeFonts.regularFont(), text, float32(size), 0).X
}

func measureMonoText(text string, size int32) float32 {
	if text == "" {
		return 0
	}
	return rl.MeasureTextEx(activeFonts.monoFont(), text, float32(size), 0).X
}

func truncateText(text string, size int32, maxWidth float32) string {
	if maxWidth <= 0 {
		return ""
	}
	return truncateMeasuredText(text, size, maxWidth, measureText)
}

func truncateMonoText(text string, size int32, maxWidth float32) string {
	if maxWidth <= 0 {
		return ""
	}
	return truncateMeasuredText(text, size, maxWidth, measureMonoText)
}

func truncateMeasuredText(text string, size int32, maxWidth float32, measure func(string, int32) float32) string {
	if measure(text, size) <= maxWidth {
		return text
	}
	const suffix = "..."
	if measure(suffix, size) > maxWidth {
		return ""
	}

	runes := []rune(text)
	lo, hi := 0, len(runes)
	for lo < hi {
		mid := (lo + hi + 1) / 2
		candidate := string(runes[:mid]) + suffix
		if measure(candidate, size) <= maxWidth {
			lo = mid
		} else {
			hi = mid - 1
		}
	}
	return strings.TrimSpace(string(runes[:lo])) + suffix
}

func drawCheckbox(rect rl.Rectangle, checked bool) {
	fill := colorSurface
	border := colorBorder
	if checked {
		fill = colorMarkedSoft
		border = colorMarked
	}
	rl.DrawRectangleRounded(rect, 0.18, 6, fill)
	rl.DrawRectangleRoundedLinesEx(rect, 0.18, 6, 1, border)
	if !checked {
		return
	}
	x := rect.X
	y := rect.Y
	rl.DrawLineEx(rl.NewVector2(x+4, y+8), rl.NewVector2(x+7, y+11), 2, colorMarked)
	rl.DrawLineEx(rl.NewVector2(x+7, y+11), rl.NewVector2(x+12, y+5), 2, colorMarked)
}

func drawChevron(x, y float32, expanded bool, color rl.Color) {
	if expanded {
		rl.DrawTriangle(
			rl.NewVector2(x-5, y-3),
			rl.NewVector2(x+5, y-3),
			rl.NewVector2(x, y+4),
			color,
		)
		return
	}
	rl.DrawTriangle(
		rl.NewVector2(x-3, y-5),
		rl.NewVector2(x-3, y+5),
		rl.NewVector2(x+4, y),
		color,
	)
}

func formatCompactGroupTitle(group *dupview.Group) string {
	if group == nil {
		return ""
	}
	hash := hashPrefix(group)
	return fmt.Sprintf("%s - %d files - approx. %s", hash, len(group.Files), utils.DisplaySize(group.TotalSz))
}

func hashPrefix(group *dupview.Group) string {
	if group == nil {
		return ""
	}
	hash := fmt.Sprintf("%x", group.Hash)
	if len(hash) > 16 {
		return hash[:16]
	}
	return hash
}

func footerHelp(width float32) string {
	switch {
	case width < 850:
		return "Arrows navigate | Space marks | d delete | Shift+L link | q exits"
	case width < 1180:
		return "Arrows/jk navigate | Enter folds | Space marks | a mark all | u clear | d delete | q exits"
	default:
		return "Arrows/jk navigate | Enter folds | Space/m marks | a mark all | u clear | d delete | Shift+L link | 1/2 sort | q exits"
	}
}

func insetRect(rect rl.Rectangle, amount float32) rl.Rectangle {
	return rl.NewRectangle(rect.X+amount, rect.Y+amount, max(rect.Width-amount*2, 0), max(rect.Height-amount*2, 0))
}

func textY(rect rl.Rectangle, size int32) float32 {
	return rect.Y + (rect.Height-float32(size))/2 - 1
}

func fileStatusLabel(entry *dupview.FileEntry) string {
	switch entry.Status {
	case dupview.FileStatusDeleted:
		return "DELETED"
	case dupview.FileStatusLinked:
		return "LINKED"
	case dupview.FileStatusError:
		if entry.Message != "" {
			return "ERROR: " + entry.Message
		}
		return "ERROR"
	default:
		return ""
	}
}

func fileStatusColor(entry *dupview.FileEntry) rl.Color {
	switch entry.Status {
	case dupview.FileStatusDeleted, dupview.FileStatusLinked:
		return colorSuccess
	case dupview.FileStatusError:
		return colorDanger
	default:
		return colorMuted
	}
}

func keyPressed(key int32) bool {
	return rl.IsKeyPressed(key) || rl.IsKeyPressedRepeat(key)
}

func shiftDown() bool {
	return rl.IsKeyDown(rl.KeyLeftShift) || rl.IsKeyDown(rl.KeyRightShift)
}

func clamp[T int | float32](v, minValue, maxValue T) T {
	if v < minValue {
		return minValue
	}
	if v > maxValue {
		return maxValue
	}
	return v
}

func min[T int | float32](a, b T) T {
	if a < b {
		return a
	}
	return b
}

func max[T int | float32](a, b T) T {
	if a > b {
		return a
	}
	return b
}
