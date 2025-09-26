// Package progress provides advanced progress bar functionality for zoom-to-box
package progress

import (
	"fmt"
	"io"
	"os"
	"strings"
	"sync"
	"time"
)

// ProgressBar represents an interactive progress bar with real-time updates
type ProgressBar interface {
	// Update sets the current progress value
	Update(current int64)
	
	// UpdateWithMessage updates progress with a custom message
	UpdateWithMessage(current int64, message string)
	
	// IncrementBy increases current progress by the specified amount
	IncrementBy(amount int64)
	
	// SetTotal updates the total expected value
	SetTotal(total int64)
	
	// SetMessage sets a custom message without updating progress
	SetMessage(message string)
	
	// Finish completes the progress bar and shows final state
	Finish()
	
	// Clear clears the progress bar from the display
	Clear()
	
	// IsFinished returns whether the progress bar is finished
	IsFinished() bool
}

// ProgressBarConfig holds configuration for progress bars
type ProgressBarConfig struct {
	Writer          io.Writer     // Where to write output (default: os.Stdout)
	Width           int           // Width of the progress bar in characters
	ShowPercent     bool          // Show percentage
	ShowSpeed       bool          // Show speed (requires speed tracking)
	ShowETA         bool          // Show estimated time remaining
	ShowElapsed     bool          // Show elapsed time
	RefreshInterval time.Duration // How often to refresh the display
	Template        string        // Custom template for progress display
	Units           string        // Units for the progress (e.g., "bytes", "files")
	SpeedUnits      string        // Units for speed display (e.g., "B/s", "files/s")
}

// progressBarImpl implements the ProgressBar interface
type progressBarImpl struct {
	config      ProgressBarConfig
	current     int64
	total       int64
	message     string
	startTime   time.Time
	lastUpdate  time.Time
	finished    bool
	mutex       sync.RWMutex
	lastOutput  string
	speedBuffer []speedSample
}

// speedSample represents a point in time for speed calculation
type speedSample struct {
	timestamp time.Time
	value     int64
}

// NewProgressBar creates a new progress bar with the given configuration
func NewProgressBar(total int64, config ProgressBarConfig) ProgressBar {
	// Set default values
	if config.Writer == nil {
		config.Writer = os.Stdout
	}
	if config.Width <= 0 {
		config.Width = 40
	}
	if config.RefreshInterval <= 0 {
		config.RefreshInterval = 100 * time.Millisecond
	}
	if config.Template == "" {
		config.Template = "[{bar}] {percent}% | {current}/{total} {units}"
		if config.ShowSpeed {
			config.Template += " | {speed} {speed_units}"
		}
		if config.ShowETA {
			config.Template += " | ETA: {eta}"
		}
		if config.ShowElapsed {
			config.Template += " | {elapsed}"
		}
	}
	if config.Units == "" {
		config.Units = "items"
	}
	if config.SpeedUnits == "" {
		config.SpeedUnits = "items/s"
	}

	return &progressBarImpl{
		config:      config,
		current:     0,
		total:       total,
		startTime:   time.Now(),
		lastUpdate:  time.Now(),
		speedBuffer: make([]speedSample, 0, 10),
	}
}

// Update sets the current progress value
func (pb *progressBarImpl) Update(current int64) {
	pb.UpdateWithMessage(current, pb.message)
}

// UpdateWithMessage updates progress with a custom message
func (pb *progressBarImpl) UpdateWithMessage(current int64, message string) {
	pb.mutex.Lock()
	defer pb.mutex.Unlock()

	if pb.finished {
		return
	}

	pb.current = current
	pb.message = message
	now := time.Now()

	// Add speed sample if enough time has passed
	if now.Sub(pb.lastUpdate) >= pb.config.RefreshInterval {
		pb.addSpeedSample(now, current)
		pb.display()
		pb.lastUpdate = now
	}
}

// IncrementBy increases current progress by the specified amount
func (pb *progressBarImpl) IncrementBy(amount int64) {
	pb.mutex.RLock()
	newCurrent := pb.current + amount
	pb.mutex.RUnlock()
	
	pb.Update(newCurrent)
}

// SetTotal updates the total expected value
func (pb *progressBarImpl) SetTotal(total int64) {
	pb.mutex.Lock()
	defer pb.mutex.Unlock()
	
	pb.total = total
}

// SetMessage sets a custom message without updating progress
func (pb *progressBarImpl) SetMessage(message string) {
	pb.mutex.Lock()
	defer pb.mutex.Unlock()
	
	pb.message = message
	pb.display()
}

// Finish completes the progress bar and shows final state
func (pb *progressBarImpl) Finish() {
	pb.mutex.Lock()
	defer pb.mutex.Unlock()

	if pb.finished {
		return
	}

	pb.finished = true
	pb.current = pb.total
	pb.display()
	
	// Move to next line
	fmt.Fprint(pb.config.Writer, "\n")
}

// Clear clears the progress bar from the display
func (pb *progressBarImpl) Clear() {
	pb.mutex.Lock()
	defer pb.mutex.Unlock()
	
	// Clear current line
	fmt.Fprint(pb.config.Writer, "\r\033[K")
}

// IsFinished returns whether the progress bar is finished
func (pb *progressBarImpl) IsFinished() bool {
	pb.mutex.RLock()
	defer pb.mutex.RUnlock()
	
	return pb.finished
}

// addSpeedSample adds a new sample for speed calculation
func (pb *progressBarImpl) addSpeedSample(timestamp time.Time, value int64) {
	sample := speedSample{timestamp: timestamp, value: value}
	
	// Add sample
	pb.speedBuffer = append(pb.speedBuffer, sample)
	
	// Keep only last 10 samples (for ~1 second of history)
	if len(pb.speedBuffer) > 10 {
		pb.speedBuffer = pb.speedBuffer[1:]
	}
}

// calculateSpeed calculates current speed based on recent samples
func (pb *progressBarImpl) calculateSpeed() float64 {
	if len(pb.speedBuffer) < 2 {
		return 0
	}
	
	// Use first and last sample for calculation
	first := pb.speedBuffer[0]
	last := pb.speedBuffer[len(pb.speedBuffer)-1]
	
	timeDiff := last.timestamp.Sub(first.timestamp).Seconds()
	if timeDiff <= 0 {
		return 0
	}
	
	valueDiff := last.value - first.value
	return float64(valueDiff) / timeDiff
}

// calculateETA calculates estimated time to completion
func (pb *progressBarImpl) calculateETA() time.Duration {
	if pb.total <= 0 || pb.current >= pb.total {
		return 0
	}
	
	speed := pb.calculateSpeed()
	if speed <= 0 {
		return 0
	}
	
	remaining := pb.total - pb.current
	seconds := float64(remaining) / speed
	return time.Duration(seconds * float64(time.Second))
}

// display renders the progress bar to the writer
func (pb *progressBarImpl) display() {
	// Calculate values
	var percent float64
	if pb.total > 0 {
		percent = float64(pb.current) / float64(pb.total) * 100
		if percent > 100 {
			percent = 100
		}
	}

	// Create progress bar
	bar := pb.createBar(percent)
	
	// Calculate speed and ETA
	speed := pb.calculateSpeed()
	eta := pb.calculateETA()
	elapsed := time.Since(pb.startTime)

	// Format values
	values := map[string]string{
		"{bar}":        bar,
		"{percent}":    fmt.Sprintf("%.0f", percent),
		"{current}":    formatValue(pb.current),
		"{total}":      formatValue(pb.total),
		"{units}":      pb.config.Units,
		"{speed}":      formatSpeed(speed),
		"{speed_units}": pb.config.SpeedUnits,
		"{eta}":        formatDuration(eta),
		"{elapsed}":    formatDuration(elapsed),
		"{message}":    pb.message,
	}

	// Apply template
	output := pb.config.Template
	for placeholder, value := range values {
		output = strings.ReplaceAll(output, placeholder, value)
	}

	// Add message if provided and not in template
	if pb.message != "" && !strings.Contains(pb.config.Template, "{message}") {
		output = fmt.Sprintf("%s - %s", output, pb.message)
	}

	// Clear line and write output
	if output != pb.lastOutput {
		fmt.Fprintf(pb.config.Writer, "\r\033[K%s", output)
		pb.lastOutput = output
	}
}

// createBar creates the visual progress bar
func (pb *progressBarImpl) createBar(percent float64) string {
	filled := int(percent / 100 * float64(pb.config.Width))
	if filled > pb.config.Width {
		filled = pb.config.Width
	}
	if filled < 0 {
		filled = 0
	}
	
	empty := pb.config.Width - filled
	if empty < 0 {
		empty = 0
	}
	
	// Use different characters for filled and empty portions
	filledChar := "█"
	emptyChar := "░"
	
	return strings.Repeat(filledChar, filled) + strings.Repeat(emptyChar, empty)
}

// MultiProgressBar manages multiple progress bars
type MultiProgressBar struct {
	bars     map[string]ProgressBar
	config   ProgressBarConfig
	mutex    sync.RWMutex
	writer   io.Writer
	finished bool
}

// NewMultiProgressBar creates a new multi-progress bar manager
func NewMultiProgressBar(config ProgressBarConfig) *MultiProgressBar {
	if config.Writer == nil {
		config.Writer = os.Stdout
	}
	
	return &MultiProgressBar{
		bars:   make(map[string]ProgressBar),
		config: config,
		writer: config.Writer,
	}
}

// AddBar adds a new progress bar with the given ID and total
func (mpb *MultiProgressBar) AddBar(id string, total int64, description string) ProgressBar {
	mpb.mutex.Lock()
	defer mpb.mutex.Unlock()
	
	if mpb.finished {
		return nil
	}
	
	// Create new bar with custom template that includes description
	config := mpb.config
	config.Template = fmt.Sprintf("%s: [{bar}] {percent}%% | {current}/{total} {units}", description)
	if config.ShowSpeed {
		config.Template += " | {speed} {speed_units}"
	}
	if config.ShowETA {
		config.Template += " | ETA: {eta}"
	}
	
	bar := NewProgressBar(total, config)
	mpb.bars[id] = bar
	
	return bar
}

// RemoveBar removes a progress bar
func (mpb *MultiProgressBar) RemoveBar(id string) {
	mpb.mutex.Lock()
	defer mpb.mutex.Unlock()
	
	if bar, exists := mpb.bars[id]; exists {
		bar.Clear()
		delete(mpb.bars, id)
	}
}

// GetBar returns the progress bar with the given ID
func (mpb *MultiProgressBar) GetBar(id string) ProgressBar {
	mpb.mutex.RLock()
	defer mpb.mutex.RUnlock()
	
	return mpb.bars[id]
}

// FinishAll finishes all progress bars
func (mpb *MultiProgressBar) FinishAll() {
	mpb.mutex.Lock()
	defer mpb.mutex.Unlock()
	
	mpb.finished = true
	
	for _, bar := range mpb.bars {
		bar.Finish()
	}
}

// ClearAll clears all progress bars
func (mpb *MultiProgressBar) ClearAll() {
	mpb.mutex.Lock()
	defer mpb.mutex.Unlock()
	
	for _, bar := range mpb.bars {
		bar.Clear()
	}
}

// Helper functions

// formatValue formats numeric values with appropriate units
func formatValue(value int64) string {
	if value < 1000 {
		return fmt.Sprintf("%d", value)
	}
	if value < 1000000 {
		return fmt.Sprintf("%.1fK", float64(value)/1000)
	}
	if value < 1000000000 {
		return fmt.Sprintf("%.1fM", float64(value)/1000000)
	}
	return fmt.Sprintf("%.1fG", float64(value)/1000000000)
}

// formatSpeed formats speed values
func formatSpeed(speed float64) string {
	if speed < 1000 {
		return fmt.Sprintf("%.1f", speed)
	}
	if speed < 1000000 {
		return fmt.Sprintf("%.1fK", speed/1000)
	}
	if speed < 1000000000 {
		return fmt.Sprintf("%.1fM", speed/1000000)
	}
	return fmt.Sprintf("%.1fG", speed/1000000000)
}