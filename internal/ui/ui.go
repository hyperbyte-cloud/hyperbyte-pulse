package ui

import (
	"context"
	"fmt"
	"math"
	"strconv"
	"strings"
	"time"

	"github.com/gdamore/tcell/v2"
	"github.com/rivo/tview"

	"hyperbyte-proc-monitor/internal/monitor"
)

// UI represents the main UI controller
type UI struct {
	app     *tview.Application
	pages   *tview.Pages
	monitor *monitor.Monitor

	// Main view components
	processTable *tview.Table
	statusBar    *tview.TextView
	helpText     *tview.TextView

	// Detail view components
	detailFlex   *tview.Flex
	cpuGraph     *Graph
	memoryGraph  *Graph
	diskGraph    *SparklineGraph
	networkGraph *SparklineGraph
	processInfo  *tview.TextView

	// State
	selectedPID int32
	currentView string
	searchQuery string
	isSearching bool

	// Channels for communication
	updateChan chan struct{}
	quitChan   chan struct{}
}

// NewUI creates a new UI instance
func NewUI(mon *monitor.Monitor) *UI {
	app := tview.NewApplication()

	ui := &UI{
		app:         app,
		pages:       tview.NewPages(),
		monitor:     mon,
		currentView: "main",
		updateChan:  make(chan struct{}, 1),
		quitChan:    make(chan struct{}),
	}

	ui.setupMainView()
	ui.setupDetailView()
	ui.setupKeyBindings()

	app.SetRoot(ui.pages, true)

	return ui
}

// Run starts the UI
func (ui *UI) Run(ctx context.Context) error {
	// Start the update goroutine
	go ui.updateLoop(ctx)

	// Initial update
	ui.triggerUpdate()

	return ui.app.Run()
}

// Stop stops the UI
func (ui *UI) Stop() {
	select {
	case <-ui.quitChan:
		// Already closed
	default:
		close(ui.quitChan)
	}
	ui.app.Stop()
}

func (ui *UI) setupMainView() {
	// Create process table
	ui.processTable = tview.NewTable().
		SetBorders(false).
		SetSelectable(true, false).
		SetFixed(1, 0)

	// Set table headers
	headers := []string{"PID", "Name", "CPU%", "Memory%", "Memory(MB)"}
	for i, header := range headers {
		cell := tview.NewTableCell(header).
			SetTextColor(tcell.ColorYellow).
			SetAlign(tview.AlignCenter).
			SetSelectable(false)
		ui.processTable.SetCell(0, i, cell)
	}

	// Create status bar
	ui.statusBar = tview.NewTextView().
		SetDynamicColors(true).
		SetText("[yellow]Process Monitor - Arrow keys to navigate, Enter to view details, q to quit, / to search, s to sort[-]")

	// Create help text
	ui.helpText = tview.NewTextView().
		SetDynamicColors(true).
		SetText("[green]Keybindings:[-] [white]↑↓[-] Navigate [white]Enter[-] Details [white]q[-] Quit [white]/[-] Search [white]ESC[-] Clear [white]c[-] CPU Sort [white]m[-] Memory Sort [white]p[-] PID Sort [white]n[-] Name Sort [white]h[-] Help")

	// Create main layout
	mainFlex := tview.NewFlex().
		SetDirection(tview.FlexRow).
		AddItem(ui.processTable, 0, 1, true).
		AddItem(ui.helpText, 1, 0, false).
		AddItem(ui.statusBar, 1, 0, false)

	mainFlex.SetBorder(true).SetTitle(" Process Monitor ")

	ui.pages.AddPage("main", mainFlex, true, true)
}

func (ui *UI) setupDetailView() {
	// Create graphs
	ui.cpuGraph = NewGraph("CPU Usage", "%", 8)
	ui.memoryGraph = NewGraph("Memory Usage", "MB", 8)
	ui.diskGraph = NewSparklineGraph("Disk I/O", "%")
	ui.networkGraph = NewSparklineGraph("Network I/O", "KB/s")

	// Create process info panel
	ui.processInfo = tview.NewTextView()
	ui.processInfo.SetDynamicColors(true).
		SetBorder(true).
		SetTitle(" Process Information ")

	// Create layout
	topRow := tview.NewFlex().
		AddItem(ui.cpuGraph, 0, 1, false).
		AddItem(ui.memoryGraph, 0, 1, false)

	bottomRow := tview.NewFlex().
		AddItem(ui.diskGraph, 0, 1, false).
		AddItem(ui.networkGraph, 0, 1, false)

	graphsCol := tview.NewFlex().
		SetDirection(tview.FlexRow).
		AddItem(topRow, 0, 1, false).
		AddItem(bottomRow, 0, 1, false)

	ui.detailFlex = tview.NewFlex().
		AddItem(ui.processInfo, 0, 1, false).
		AddItem(graphsCol, 0, 2, false)

	ui.detailFlex.SetBorder(true).SetTitle(" Process Details - ESC or q to return ")

	ui.pages.AddPage("detail", ui.detailFlex, true, false)
}

func (ui *UI) setupKeyBindings() {
	ui.app.SetInputCapture(func(event *tcell.EventKey) *tcell.EventKey {
		switch ui.currentView {
		case "main":
			return ui.handleMainViewKeys(event)
		case "detail":
			return ui.handleDetailViewKeys(event)
		}
		return event
	})
}

func (ui *UI) handleMainViewKeys(event *tcell.EventKey) *tcell.EventKey {
	switch event.Key() {
	case tcell.KeyEsc:
		if ui.isSearching {
			ui.isSearching = false
			ui.searchQuery = ""
			ui.updateStatusBar()
			ui.triggerUpdate()
			return nil
		}
		ui.Stop()
		return nil

	case tcell.KeyEnter:
		row, _ := ui.processTable.GetSelection()
		if row > 0 && row < ui.processTable.GetRowCount() {
			pidCell := ui.processTable.GetCell(row, 0)
			if pidStr := pidCell.Text; pidStr != "" {
				if pid, err := strconv.ParseInt(pidStr, 10, 32); err == nil {
					ui.selectedPID = int32(pid)
					// Ensure this process has time series metrics tracking
					ui.monitor.EnsureProcessMetrics(int32(pid))
					ui.showDetailView()
				}
			}
		}
		return nil

	case tcell.KeyRune:
		switch event.Rune() {
		case 'q', 'Q':
			ui.Stop()
			return nil
		case '/':
			ui.isSearching = true
			ui.searchQuery = ""
			ui.updateStatusBar()
			return nil
		case 's', 'S':
			ui.cycleSorting()
			return nil
		case 'c', 'C':
			ui.monitor.SetSorting(monitor.SortByCPU, true)
			ui.triggerUpdate()
			return nil
		case 'm', 'M':
			ui.monitor.SetSorting(monitor.SortByMemory, true)
			ui.triggerUpdate()
			return nil
		case 'p', 'P':
			ui.monitor.SetSorting(monitor.SortByPID, false)
			ui.triggerUpdate()
			return nil
		case 'n', 'N':
			ui.monitor.SetSorting(monitor.SortByName, false)
			ui.triggerUpdate()
			return nil
		case 'h', 'H':
			ui.showHelpDialog()
			return nil
		default:
			if ui.isSearching {
				ui.searchQuery += string(event.Rune())
				ui.updateStatusBar()
				ui.triggerUpdate()
				return nil
			}
		}

	case tcell.KeyBackspace, tcell.KeyBackspace2:
		if ui.isSearching && len(ui.searchQuery) > 0 {
			ui.searchQuery = ui.searchQuery[:len(ui.searchQuery)-1]
			ui.updateStatusBar()
			ui.triggerUpdate()
			return nil
		}
	}

	return event
}

func (ui *UI) handleDetailViewKeys(event *tcell.EventKey) *tcell.EventKey {
	switch event.Key() {
	case tcell.KeyEsc:
		ui.showMainView()
		return nil
	}

	switch event.Rune() {
	case 'q', 'Q':
		ui.showMainView()
		return nil
	}

	return event
}

func (ui *UI) showDetailView() {
	ui.currentView = "detail"
	ui.pages.SwitchToPage("detail")
	ui.app.SetFocus(ui.detailFlex)
}

func (ui *UI) showMainView() {
	ui.currentView = "main"
	ui.pages.SwitchToPage("main")
	ui.app.SetFocus(ui.processTable)
}

func (ui *UI) cycleSorting() {
	// Cycle through sorting options
	// This is a simplified version - you could make it more sophisticated
	ui.monitor.SetSorting(monitor.SortByCPU, true)
	ui.triggerUpdate()
}

func (ui *UI) showHelpDialog() {
	helpText := `[yellow]Process Monitor Help[-]

[green]Navigation:[-]
  [white]↑/↓[-]     Navigate process list
  [white]Enter[-]   View process details
  [white]ESC[-]     Return to main view / Cancel search
  [white]q[-]       Quit application

[green]Sorting:[-]
  [white]c[-]       Sort by CPU usage (desc)
  [white]m[-]       Sort by Memory usage (desc)  
  [white]p[-]       Sort by PID (asc)
  [white]n[-]       Sort by Name (asc)

[green]Search:[-]
  [white]/[-]       Start search
  [white]ESC[-]     Clear search

[green]Other:[-]
  [white]h[-]       Show this help

[green]Color Coding:[-]
  [white]Green[-]   Normal usage
  [yellow]Yellow[-]  Medium usage (>50%)
  [red]Red[-]     High usage (>80%)

[dim]Press any key to close...[-]`

	modal := tview.NewModal().
		SetText(helpText).
		AddButtons([]string{"Close"}).
		SetDoneFunc(func(buttonIndex int, buttonLabel string) {
			ui.pages.RemovePage("help")
		})

	ui.pages.AddPage("help", modal, false, true)
}

func (ui *UI) updateLoop(ctx context.Context) {
	// Reduce UI update frequency to improve performance
	ticker := time.NewTicker(1500 * time.Millisecond) // Update UI every 1.5 seconds instead of 1
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-ui.quitChan:
			return
		case <-ticker.C:
			ui.triggerUpdate()
		case <-ui.updateChan:
			ui.updateViews()
		}
	}
}

func (ui *UI) triggerUpdate() {
	select {
	case ui.updateChan <- struct{}{}:
	default:
		// Channel is full, skip this update
	}
}

func (ui *UI) updateViews() {
	ui.app.QueueUpdateDraw(func() {
		switch ui.currentView {
		case "main":
			ui.updateMainView()
		case "detail":
			ui.updateDetailView()
		}
	})
}

func (ui *UI) updateMainView() {
	processes := ui.monitor.GetProcesses()

	// Filter processes based on search query
	if ui.searchQuery != "" {
		filtered := make([]monitor.ProcessInfo, 0)
		query := strings.ToLower(ui.searchQuery)
		for _, proc := range processes {
			if strings.Contains(strings.ToLower(proc.Name), query) ||
				strings.Contains(strconv.Itoa(int(proc.PID)), query) {
				filtered = append(filtered, proc)
			}
		}
		processes = filtered
	}

	// Clear existing rows except header
	for row := ui.processTable.GetRowCount() - 1; row > 0; row-- {
		ui.processTable.RemoveRow(row)
	}

	// Add process rows
	for i, proc := range processes {
		row := i + 1

		// Create cells with appropriate formatting
		pidCell := tview.NewTableCell(strconv.Itoa(int(proc.PID)))
		nameCell := tview.NewTableCell(proc.Name)
		cpuCell := tview.NewTableCell(fmt.Sprintf("%.1f", proc.CPUPercent))
		memPercCell := tview.NewTableCell(fmt.Sprintf("%.1f", proc.MemoryPerc))
		memMBCell := tview.NewTableCell(fmt.Sprintf("%.1f", proc.MemoryMB))

		// Determine color based on resource usage
		var color tcell.Color = tcell.ColorWhite
		maxUsage := math.Max(proc.CPUPercent, float64(proc.MemoryPerc))

		if maxUsage > 80 {
			color = tcell.ColorRed
		} else if maxUsage > 50 {
			color = tcell.ColorYellow
		} else if maxUsage > 25 {
			color = tcell.ColorGreen
		} else {
			color = tcell.ColorWhite
		}

		// Apply colors
		pidCell.SetTextColor(color)
		nameCell.SetTextColor(color)
		cpuCell.SetTextColor(color)
		memPercCell.SetTextColor(color)
		memMBCell.SetTextColor(color)

		// Highlight high usage columns specifically
		if proc.CPUPercent > 80 {
			cpuCell.SetTextColor(tcell.ColorRed).SetAttributes(tcell.AttrBold)
		}
		if proc.MemoryPerc > 80 {
			memPercCell.SetTextColor(tcell.ColorRed).SetAttributes(tcell.AttrBold)
		}

		ui.processTable.SetCell(row, 0, pidCell)
		ui.processTable.SetCell(row, 1, nameCell)
		ui.processTable.SetCell(row, 2, cpuCell)
		ui.processTable.SetCell(row, 3, memPercCell)
		ui.processTable.SetCell(row, 4, memMBCell)
	}

	ui.updateStatusBar()
}

func (ui *UI) updateDetailView() {
	if ui.selectedPID == 0 {
		return
	}

	// Get process metrics
	metrics := ui.monitor.GetProcessMetrics(ui.selectedPID)
	if metrics == nil {
		ui.processInfo.SetText("Process not found or no data available")
		return
	}

	// Get current process data (this will also update time series if needed)
	currentProcess, err := ui.monitor.GetCurrentProcessData(ui.selectedPID)
	if err != nil {
		// Fallback to searching in the process list
		processes := ui.monitor.GetProcesses()
		for _, proc := range processes {
			if proc.PID == ui.selectedPID {
				currentProcess = &proc
				break
			}
		}
		if currentProcess == nil {
			ui.processInfo.SetText("Process not found or no data available")
			return
		}
	}

	if currentProcess != nil {
		info := fmt.Sprintf(`[yellow]Process Information[-]

[white]PID:[-] %d
[white]Name:[-] %s
[white]Current CPU:[-] %.1f%%
[white]Current Memory:[-] %.1fMB (%.1f%%)
[white]Created:[-] %s

[green]Disk I/O:[-] %.1f%% (R: %.1f KB/s, W: %.1f KB/s)
[green]Network:[-] S: %.1f KB/s, R: %.1f KB/s

[cyan]Data points:[-] %d
[cyan]Monitoring duration:[-] %s`,
			currentProcess.PID,
			currentProcess.Name,
			currentProcess.CPUPercent,
			currentProcess.MemoryMB,
			currentProcess.MemoryPerc,
			currentProcess.CreateTime.Format("2006-01-02 15:04:05"),
			currentProcess.DiskReadPerc+currentProcess.DiskWritePerc,
			currentProcess.DiskReadRate,
			currentProcess.DiskWriteRate,
			currentProcess.NetSentRate,
			currentProcess.NetRecvRate,
			len(metrics.CPUPercent),
			fmt.Sprintf("%.0fs", time.Since(metrics.Timestamps[0]).Seconds()),
		)
		ui.processInfo.SetText(info)
	}

	// Update graphs
	if len(metrics.CPUPercent) > 0 {
		// Get system metrics for memory max
		systemMetrics := ui.monitor.GetSystemMetrics()

		ui.cpuGraph.UpdateData(metrics.CPUPercent, 100.0)
		ui.memoryGraph.UpdateData(metrics.MemoryMB, systemMetrics.TotalMemoryMB)

		// Combine disk read and write percentages for display
		diskPercData := make([]float64, len(metrics.DiskReadPerc))
		for i := range diskPercData {
			diskPercData[i] = metrics.DiskReadPerc[i] + metrics.DiskWritePerc[i]
		}
		ui.diskGraph.UpdateData(diskPercData)

		// Combine network sent and received rates for display
		netRateData := make([]float64, len(metrics.NetSentRate))
		for i := range netRateData {
			netRateData[i] = metrics.NetSentRate[i] + metrics.NetRecvRate[i]
		}
		ui.networkGraph.UpdateData(netRateData)
	}
}

func (ui *UI) updateStatusBar() {
	if ui.isSearching {
		ui.statusBar.SetText(fmt.Sprintf("[yellow]Search: %s[-] [white](ESC to cancel)[-]", ui.searchQuery))
	} else {
		systemMetrics := ui.monitor.GetSystemMetrics()
		processes := ui.monitor.GetProcesses()

		filteredCount := len(processes)
		if ui.searchQuery != "" {
			query := strings.ToLower(ui.searchQuery)
			filteredCount = 0
			for _, proc := range processes {
				if strings.Contains(strings.ToLower(proc.Name), query) ||
					strings.Contains(strconv.Itoa(int(proc.PID)), query) {
					filteredCount++
				}
			}
		}

		statusText := fmt.Sprintf(
			"[green]Processes: %d[-] [blue]System CPU: %.1f%%[-] [blue]Memory: %.1f%%[-] [yellow]Last updated: %s[-]",
			filteredCount,
			systemMetrics.CPUPercent,
			systemMetrics.MemoryPercent,
			systemMetrics.Timestamp.Format("15:04:05"),
		)

		ui.statusBar.SetText(statusText)
	}
}
