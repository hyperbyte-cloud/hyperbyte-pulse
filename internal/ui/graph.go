package ui

import (
	"fmt"
	"math"
	"strings"

	"github.com/rivo/tview"
)

// Graph represents an ASCII graph widget
type Graph struct {
	*tview.TextView
	title       string
	data        []float64
	maxValue    float64
	height      int
	width       int
	unit        string
	colorHigh   string
	colorMedium string
	colorLow    string
}

// NewGraph creates a new graph widget
func NewGraph(title, unit string, height int) *Graph {
	textView := tview.NewTextView()
	textView.SetDynamicColors(true).
		SetBorder(true).
		SetBorderColor(tview.Styles.PrimaryTextColor).
		SetTitle(title)

	g := &Graph{
		TextView:    textView,
		title:       title,
		height:      height,
		unit:        unit,
		colorHigh:   "[red]",
		colorMedium: "[yellow]",
		colorLow:    "[green]",
	}

	return g
}

// UpdateData updates the graph with new data
func (g *Graph) UpdateData(data []float64, maxValue float64) {
	g.data = make([]float64, len(data))
	copy(g.data, data)
	g.maxValue = maxValue
	g.render()
}

// render draws the graph
func (g *Graph) render() {
	g.Clear()

	if len(g.data) == 0 {
		g.SetText("No data available")
		return
	}

	// Get the actual drawing area
	_, _, width, height := g.GetInnerRect()
	g.width = width
	if height > 0 {
		g.height = height
	}

	// Ensure we have reasonable dimensions
	if g.width < 10 || g.height < 3 {
		g.SetText("Window too small")
		return
	}

	// Build the graph
	var output strings.Builder

	// Calculate scaling
	dataLen := len(g.data)
	if dataLen > g.width {
		// Sample data to fit width
		g.data = g.sampleData(g.data, g.width)
		dataLen = len(g.data)
	}

	// Determine max value for scaling
	maxVal := g.maxValue
	if maxVal == 0 {
		for _, val := range g.data {
			if val > maxVal {
				maxVal = val
			}
		}
	}
	if maxVal == 0 {
		maxVal = 1 // Avoid division by zero
	}

	// Create the graph lines
	graphHeight := g.height - 2 // Reserve space for labels
	if graphHeight < 1 {
		graphHeight = 1
	}

	// Build graph from top to bottom
	for row := graphHeight - 1; row >= 0; row-- {
		var line strings.Builder
		threshold := float64(row+1) / float64(graphHeight) * maxVal

		for i, val := range g.data {
			if i >= g.width {
				break
			}

			var char string
			var color string

			if val >= threshold {
				// Determine color based on percentage of max
				percentage := val / maxVal
				if percentage > 0.8 {
					color = g.colorHigh
				} else if percentage > 0.5 {
					color = g.colorMedium
				} else {
					color = g.colorLow
				}

				// Choose character based on how much above threshold
				if val >= threshold+((maxVal-threshold)/4) {
					char = "█"
				} else if val >= threshold+((maxVal-threshold)/8) {
					char = "▆"
				} else {
					char = "▄"
				}
				line.WriteString(color + char + "[-]")
			} else {
				line.WriteString(" ")
			}
		}
		output.WriteString(line.String() + "\n")
	}

	// Add value labels
	if len(g.data) > 0 {
		currentVal := g.data[len(g.data)-1]
		maxLabel := fmt.Sprintf("Max: %.1f%s", maxVal, g.unit)
		currentLabel := fmt.Sprintf("Current: %.1f%s", currentVal, g.unit)

		output.WriteString(fmt.Sprintf("%-*s %s\n",
			g.width-len(currentLabel)-1, maxLabel, currentLabel))
	}

	g.SetText(output.String())
}

// sampleData reduces data points to fit the available width
func (g *Graph) sampleData(data []float64, targetWidth int) []float64 {
	if len(data) <= targetWidth {
		return data
	}

	sampled := make([]float64, targetWidth)
	step := float64(len(data)) / float64(targetWidth)

	for i := 0; i < targetWidth; i++ {
		index := int(float64(i) * step)
		if index >= len(data) {
			index = len(data) - 1
		}
		sampled[i] = data[index]
	}

	return sampled
}

// SetColors sets the color scheme for the graph
func (g *Graph) SetColors(low, medium, high string) {
	g.colorLow = low
	g.colorMedium = medium
	g.colorHigh = high
}

// SparklineGraph creates a simple sparkline-style graph
type SparklineGraph struct {
	*tview.TextView
	title string
	data  []float64
	unit  string
}

// NewSparklineGraph creates a new sparkline graph
func NewSparklineGraph(title, unit string) *SparklineGraph {
	textView := tview.NewTextView()
	textView.SetDynamicColors(true).
		SetBorder(true).
		SetTitle(title)

	g := &SparklineGraph{
		TextView: textView,
		title:    title,
		unit:     unit,
	}

	return g
}

// UpdateData updates the sparkline with new data
func (sg *SparklineGraph) UpdateData(data []float64) {
	sg.data = make([]float64, len(data))
	copy(sg.data, data)
	sg.render()
}

func (sg *SparklineGraph) render() {
	sg.Clear()

	if len(sg.data) == 0 {
		sg.SetText("No data")
		return
	}

	// Get available width
	_, _, width, _ := sg.GetInnerRect()
	if width < 5 {
		sg.SetText("Too small")
		return
	}

	// Sample data if needed
	data := sg.data
	if len(data) > width-2 {
		data = sg.sampleData(data, width-2)
	}

	// Find min and max for scaling
	minVal, maxVal := math.Inf(1), math.Inf(-1)
	for _, val := range data {
		if val < minVal {
			minVal = val
		}
		if val > maxVal {
			maxVal = val
		}
	}

	if minVal == maxVal {
		maxVal = minVal + 1 // Avoid division by zero
	}

	// Sparkline characters (8 levels)
	chars := []string{"▁", "▂", "▃", "▄", "▅", "▆", "▇", "█"}

	var output strings.Builder
	for _, val := range data {
		// Normalize to 0-7 range
		normalized := (val - minVal) / (maxVal - minVal)
		index := int(normalized * 7)
		if index < 0 {
			index = 0
		} else if index >= len(chars) {
			index = len(chars) - 1
		}

		// Color based on value
		if normalized > 0.8 {
			output.WriteString("[red]")
		} else if normalized > 0.5 {
			output.WriteString("[yellow]")
		} else {
			output.WriteString("[green]")
		}
		output.WriteString(chars[index])
		output.WriteString("[-]")
	}

	// Add current value
	if len(data) > 0 {
		current := data[len(data)-1]
		output.WriteString(fmt.Sprintf("\nCurrent: %.1f%s", current, sg.unit))
	}

	sg.SetText(output.String())
}

func (sg *SparklineGraph) sampleData(data []float64, targetWidth int) []float64 {
	if len(data) <= targetWidth {
		return data
	}

	sampled := make([]float64, targetWidth)
	step := float64(len(data)) / float64(targetWidth)

	for i := 0; i < targetWidth; i++ {
		index := int(float64(i) * step)
		if index >= len(data) {
			index = len(data) - 1
		}
		sampled[i] = data[index]
	}

	return sampled
}
