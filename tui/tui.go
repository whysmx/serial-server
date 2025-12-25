// Package tui provides the terminal user interface for serial-server.
package tui

import (
	"serial-server/listener"
	"fmt"
	"log"
	"os"
	"os/signal"
	"strings"
	"sync"
	"time"

	"github.com/gdamore/tcell/v2"
	"github.com/mattn/go-runewidth"
)

const (
	maxDataLines = 1000
)

// DataLine represents a single line of data in the display.
type DataLine struct {
	Timestamp time.Time
	Direction string // "TX" or "RX"
	Data      string
	RawData   []byte
}

// TUI represents the terminal user interface.
type TUI struct {
	screen     tcell.Screen
	listeners  []*listener.Listener
	focusIndex int
	dataLines  []DataLine
	mu         sync.RWMutex

	// Menu state
	showMenu bool
	menuSel  int

	// Colors
	styleHeader tcell.Style
	styleTX     tcell.Style
	styleRX     tcell.Style
	styleMenu   tcell.Style
	styleSelect tcell.Style
	styleNormal tcell.Style

	// Running state
	running bool
	stopCh  chan struct{}
}

// NewTUI creates a new TUI instance.
func NewTUI(listeners []*listener.Listener) (*TUI, error) {
	s, err := tcell.NewScreen()
	if err != nil {
		return nil, fmt.Errorf("failed to create screen: %w", err)
	}

	if err := s.Init(); err != nil {
		return nil, fmt.Errorf("failed to init screen: %w", err)
	}

	s.SetStyle(tcell.StyleDefault.
		Foreground(tcell.ColorWhite).
		Background(tcell.ColorBlack))

	s.EnablePaste()
	s.Clear()

	tui := &TUI{
		screen:      s,
		listeners:   listeners,
		focusIndex:  0,
		dataLines:   make([]DataLine, 0, maxDataLines),
		showMenu:    false,
		menuSel:     0,
		stopCh:      make(chan struct{}),
		styleHeader: tcell.StyleDefault.Foreground(tcell.ColorWhite).Background(tcell.ColorDarkBlue),
		styleTX:     tcell.StyleDefault.Foreground(tcell.ColorLightGreen).Background(tcell.ColorBlack),
		styleRX:     tcell.StyleDefault.Foreground(tcell.ColorYellow).Background(tcell.ColorBlack),
		styleMenu:   tcell.StyleDefault.Foreground(tcell.ColorWhite).Background(tcell.ColorDarkGray),
		styleSelect: tcell.StyleDefault.Foreground(tcell.ColorBlack).Background(tcell.ColorLightGray),
		styleNormal: tcell.StyleDefault.Foreground(tcell.ColorWhite).Background(tcell.ColorBlack),
	}

	return tui, nil
}

// Run starts the TUI main loop.
func (tui *TUI) Run() error {
	tui.running = true

	// Handle signals
	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, os.Interrupt)

	go tui.handleInput()

	// Main render loop
	ticker := time.NewTicker(100 * time.Millisecond)
	defer ticker.Stop()

	for tui.running {
		select {
		case <-tui.stopCh:
			return nil
		case <-sigCh:
			tui.running = false
			return nil
		case <-ticker.C:
			tui.render()
			tui.screen.Show()
		}
	}

	return nil
}

// Stop stops the TUI.
func (tui *TUI) Stop() {
	tui.running = false
	close(tui.stopCh)
}

// AddData adds a new data line to the display.
func (tui *TUI) AddData(data []byte, direction string, listenerIdx int) {
	tui.mu.Lock()
	defer tui.mu.Unlock()

	if listenerIdx < 0 || listenerIdx >= len(tui.listeners) {
		return
	}

	l := tui.listeners[listenerIdx]
	displayStr := listener.FormatForDisplay(data, l.GetDisplayFormat())

	line := DataLine{
		Timestamp: time.Now(),
		Direction: direction,
		Data:      displayStr,
		RawData:   data,
	}

	tui.dataLines = append(tui.dataLines, line)

	// Keep only recent lines
	if len(tui.dataLines) > maxDataLines {
		tui.dataLines = tui.dataLines[len(tui.dataLines)-maxDataLines:]
	}
}

// handleInput handles keyboard input.
func (tui *TUI) handleInput() {
	for tui.running {
		ev := tui.screen.PollEvent()

		switch e := ev.(type) {
		case *tcell.EventKey:
			tui.handleKey(e)
		}
	}
}

// handleKey handles keyboard events.
func (tui *TUI) handleKey(e *tcell.EventKey) {
	if tui.showMenu {
		tui.handleMenuKey(e)
		return
	}

	switch e.Key() {
	case tcell.KeyCtrlC, tcell.KeyCtrlQ:
		tui.running = false
	case tcell.KeyCtrlA, tcell.KeyRune:
		switch strings.ToUpper(string(e.Rune())) {
		case "M":
			tui.showMenu = true
			tui.menuSel = 0
		case "C":
			tui.clearData()
		case "1", "2", "3", "4", "5", "6", "7", "8", "9":
			idx := int(e.Rune() - '1')
			if idx < len(tui.listeners) {
				tui.focusIndex = idx
			}
		}
	case tcell.KeyTab:
		tui.focusIndex = (tui.focusIndex + 1) % len(tui.listeners)
	}
}

// handleMenuKey handles keyboard events in menu mode.
func (tui *TUI) handleMenuKey(e *tcell.EventKey) {
	switch e.Key() {
	case tcell.KeyEscape, tcell.KeyRune:
		switch strings.ToUpper(string(e.Rune())) {
		case "Q":
			tui.showMenu = false
		}
	case tcell.KeyUp, tcell.KeyCtrlP:
		if tui.menuSel > 0 {
			tui.menuSel--
		}
	case tcell.KeyDown, tcell.KeyCtrlN:
		if tui.menuSel < len(tui.listeners) {
			tui.menuSel++
		}
	case tcell.KeyEnter:
		if tui.menuSel < len(tui.listeners) {
			tui.focusIndex = tui.menuSel
			tui.showMenu = false
		}
	}
}

// clearData clears all data lines.
func (tui *TUI) clearData() {
	tui.mu.Lock()
	defer tui.mu.Unlock()
	tui.dataLines = make([]DataLine, 0, maxDataLines)
}

// render redraws the entire screen.
func (tui *TUI) render() {
	tui.screen.Clear()

	if tui.showMenu {
		tui.renderMenu()
		return
	}

	// Render header
	tui.renderHeader()

	// Render data area
	tui.renderData()

	// Render status bar
	tui.renderStatusBar()
}

// renderHeader renders the top navigation bar.
func (tui *TUI) renderHeader() {
	width, _ := tui.screen.Size()

	// Draw header line
	for x := 0; x < width; x++ {
		tui.screen.SetContent(x, 0, ' ', nil, tui.styleHeader)
	}

	var parts []string
	for i, l := range tui.listeners {
		stats := l.GetStats()
		port := l.GetListenPort()
		serialPort := l.GetSerialPort()
		// Shorten port name
		shortPort := serialPort
		if strings.HasPrefix(serialPort, "/dev/") {
			pp := strings.Split(serialPort, "/")
			if len(pp) > 0 {
				shortPort = pp[len(pp)-1]
			}
		} else if strings.HasPrefix(serialPort, "COM") {
			shortPort = serialPort
		}
		info := fmt.Sprintf("[%d] :%d %s %d %s ↑%d↓%d",
			i+1, port, shortPort, l.GetBaudRate(), l.GetDisplayFormat(), stats.TxPackets, stats.RxPackets)

		parts = append(parts, info)
	}

	headerStr := strings.Join(parts, "   [M]菜单")
	headerStr += "   "

	// Fill header with focus indicator
	x := 0
	for _, part := range parts {
		prefix := fmt.Sprintf("[%d]", x/4+1)

		style := tui.styleHeader
		if x == tui.focusIndex*4 {
			style = style.Background(tcell.ColorLightBlue)
		}

		for _, c := range prefix {
			if x < width {
				tui.screen.SetContent(x, 0, c, nil, style)
				x++
			}
		}

		for _, c := range part[len(prefix):] {
			if x < width {
				tui.screen.SetContent(x, 0, c, nil, style)
				x++
			}
		}

		// Add separator
		if x < width {
			tui.screen.SetContent(x, 0, ' ', nil, style)
			x++
		}
	}

	// Add menu button
	menuStyle := tui.styleHeader
	if tui.focusIndex >= len(tui.listeners)*4 {
		menuStyle = menuStyle.Background(tcell.ColorLightBlue)
	}
	for _, c := range "[M]菜单" {
		if x < width {
			tui.screen.SetContent(x, 0, c, nil, menuStyle)
			x++
		}
	}
}

// renderData renders the data display area.
func (tui *TUI) renderData() {
	_, height := tui.screen.Size()
	dataAreaHeight := height - 2 // Header + Status bar

	tui.mu.RLock()
	defer tui.mu.RUnlock()

	startIdx := 0
	if len(tui.dataLines) > dataAreaHeight {
		startIdx = len(tui.dataLines) - dataAreaHeight
	}

	lineNum := 1
	for i := startIdx; i < len(tui.dataLines) && lineNum < dataAreaHeight; i++ {
		line := tui.dataLines[i]

		// Format: [HH:MM:SS] TX: data
		timeStr := line.Timestamp.Format("15:04:05")
		prefix := fmt.Sprintf("[%s] %s: ", timeStr, line.Direction)

		var style tcell.Style
		if line.Direction == "TX" {
			style = tui.styleTX
		} else {
			style = tui.styleRX
		}

		// Draw prefix
		x := 0
		for _, c := range prefix {
			if x < 80 { // Limit line width
				tui.screen.SetContent(x, lineNum, c, nil, style)
				x++
			}
		}

		// Draw data
		for _, c := range line.Data {
			if x < 80 {
				tui.screen.SetContent(x, lineNum, c, nil, style)
				x++
			}
		}

		lineNum++
	}
}

// renderStatusBar renders the bottom status bar.
func (tui *TUI) renderStatusBar() {
	_, height := tui.screen.Size()
	width, _ := tui.screen.Size()

	// Draw status bar at bottom
	style := tui.styleMenu
	statusStr := " [1-9/Tab] 切换焦点  [C] 清屏  [M] 菜单  [Ctrl+C] 退出"

	for x := 0; x < width; x++ {
		tui.screen.SetContent(x, height-1, ' ', nil, style)
	}

	x := 0
	for _, c := range statusStr {
		if x < width {
			tui.screen.SetContent(x, height-1, c, nil, style)
			x++
		}
	}
}

// renderMenu renders the menu overlay.
func (tui *TUI) renderMenu() {
	width, height := tui.screen.Size()

	// Calculate menu position
	menuWidth := 50
	menuHeight := len(tui.listeners) + 5
	menuX := (width - menuWidth) / 2
	menuY := (height - menuHeight) / 2

	// Draw menu background
	for y := menuY; y < menuY+menuHeight; y++ {
		for x := menuX; x < menuX+menuWidth; x++ {
			tui.screen.SetContent(x, y, ' ', nil, tui.styleMenu)
		}
	}

	// Draw border
	for x := menuX; x < menuX+menuWidth; x++ {
		tui.screen.SetContent(x, menuY, '─', nil, tui.styleMenu)
		tui.screen.SetContent(x, menuY+menuHeight-1, '─', nil, tui.styleMenu)
	}
	for y := menuY; y < menuY+menuHeight; y++ {
		tui.screen.SetContent(menuX, y, '│', nil, tui.styleMenu)
		tui.screen.SetContent(menuX+menuWidth-1, y, '│', nil, tui.styleMenu)
	}
	// Corners
	tui.screen.SetContent(menuX, menuY, '┌', nil, tui.styleMenu)
	tui.screen.SetContent(menuX+menuWidth-1, menuY, '┐', nil, tui.styleMenu)
	tui.screen.SetContent(menuX, menuY+menuHeight-1, '└', nil, tui.styleMenu)
	tui.screen.SetContent(menuX+menuWidth-1, menuY+menuHeight-1, '┘', nil, tui.styleMenu)

	// Title
	title := " Serial-Server 控制菜单 "
	x := menuX + (menuWidth-len(title))/2
	for i, c := range title {
		tui.screen.SetContent(x+i, menuY, c, nil, tui.styleMenu)
	}

	// Listeners
	for i, l := range tui.listeners {
		y := menuY + 2 + i

		serialPort := l.GetSerialPort()
		shortPort := serialPort
		if strings.HasPrefix(serialPort, "/dev/") {
			pp := strings.Split(serialPort, "/")
			shortPort = pp[len(pp)-1]
		}
		line := fmt.Sprintf("  %d. %s  :%d  %d  %s",
			i+1, shortPort, l.GetListenPort(), l.GetBaudRate(), l.GetDisplayFormat())

		style := tui.styleMenu
		if i == tui.menuSel {
			style = tui.styleSelect
		}

		for x := menuX + 2; x < menuX+menuWidth-2; x++ {
			tui.screen.SetContent(x, y, ' ', nil, style)
		}

		for j, c := range line {
			tui.screen.SetContent(menuX+2+j, y, c, nil, style)
		}
	}

	// Separator line
	sepY := menuY + len(tui.listeners) + 2
	for x := menuX + 1; x < menuX+menuWidth-1; x++ {
		tui.screen.SetContent(x, sepY, '─', nil, tui.styleMenu)
	}

	// Help text
	helpY := sepY + 1
	help := " [1-9] 切换焦点  [Enter] 确定  [Q] 返回  [C] 清屏  [R] 重载"
	for i, c := range help {
		tui.screen.SetContent(menuX+2+i, helpY, c, nil, tui.styleMenu)
	}
}

// SetFocusIndex sets the current focus index.
func (tui *TUI) SetFocusIndex(idx int) {
	if idx >= 0 && idx < len(tui.listeners) {
		tui.focusIndex = idx
	}
}

// GetFocusIndex returns the current focus index.
func (tui *TUI) GetFocusIndex() int {
	return tui.focusIndex
}

// Close closes the TUI.
func (tui *TUI) Close() {
	tui.screen.Fini()
}

// InitLogger initializes the logger to write to TUI.
func InitLogger(tui *TUI) {
	log.SetOutput(os.Stdout)
	log.SetFlags(log.LstdFlags | log.Lmicroseconds)
}

// RuneWidth returns the width of a rune.
func RuneWidth(r rune) int {
	return runewidth.RuneWidth(r)
}
