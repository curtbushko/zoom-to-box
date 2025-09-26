// Package progress provides real-time progress reporting and logging integration for zoom-to-box
package progress

import (
	"context"
	"fmt"
	"io"
	"os"
	"strings"
	"sync"
	"time"

	"github.com/curtbushko/zoom-to-box/internal/download"
	"github.com/curtbushko/zoom-to-box/internal/logging"
)

// ProgressReporter defines the interface for progress reporting operations
type ProgressReporter interface {
	// Start initializes the progress reporting session
	Start(ctx context.Context, total int) error
	
	// UpdateDownload updates progress for a specific download
	UpdateDownload(update download.ProgressUpdate)
	
	// AddSkipped adds a skipped item to the progress tracking
	AddSkipped(reason SkipReason, item string, details map[string]interface{})
	
	// AddError adds an error to the progress tracking
	AddError(item string, err error, details map[string]interface{})
	
	// Finish completes the progress reporting session and shows summary
	Finish() *Summary
	
	// GetSummary returns current progress summary
	GetSummary() *Summary
}

// SkipReason represents why an item was skipped
type SkipReason int

const (
	SkipReasonAlreadyExists SkipReason = iota
	SkipReasonInactiveUser
	SkipReasonUnsupportedFile
	SkipReasonPermissionDenied
	SkipReasonMetaOnlyMode
)

func (r SkipReason) String() string {
	switch r {
	case SkipReasonAlreadyExists:
		return "already_exists"
	case SkipReasonInactiveUser:
		return "inactive_user"
	case SkipReasonUnsupportedFile:
		return "unsupported_file"
	case SkipReasonPermissionDenied:
		return "permission_denied"
	case SkipReasonMetaOnlyMode:
		return "meta_only_mode"
	default:
		return "unknown"
	}
}

// DownloadProgress tracks individual download progress
type DownloadProgress struct {
	ID              string                 `json:"id"`
	Filename        string                 `json:"filename"`
	BytesDownloaded int64                  `json:"bytes_downloaded"`
	TotalBytes      int64                  `json:"total_bytes"`
	Speed           float64                `json:"speed"`
	ETA             time.Duration          `json:"-"`
	State           download.DownloadState `json:"state"`
	StartTime       time.Time              `json:"start_time"`
	LastUpdate      time.Time              `json:"last_update"`
	Error           error                  `json:"-"`
	Metadata        map[string]interface{} `json:"metadata,omitempty"`
}

// SkippedItem represents an item that was skipped
type SkippedItem struct {
	Item      string                 `json:"item"`
	Reason    SkipReason             `json:"reason"`
	Details   map[string]interface{} `json:"details,omitempty"`
	Timestamp time.Time              `json:"timestamp"`
}

// ErrorItem represents an item that encountered an error
type ErrorItem struct {
	Item      string                 `json:"item"`
	Error     error                  `json:"-"`
	ErrorMsg  string                 `json:"error"`
	Details   map[string]interface{} `json:"details,omitempty"`
	Timestamp time.Time              `json:"timestamp"`
}

// Summary represents the final progress summary
type Summary struct {
	TotalItems          int                     `json:"total_items"`
	CompletedDownloads  int                     `json:"completed_downloads"`
	FailedDownloads     int                     `json:"failed_downloads"`
	SkippedItems        []SkippedItem           `json:"skipped_items"`
	ErrorItems          []ErrorItem             `json:"error_items"`
	TotalBytesDownloaded int64                  `json:"total_bytes_downloaded"`
	AverageSpeed        float64                 `json:"average_speed"`
	TotalDuration       time.Duration           `json:"-"`
	StartTime           time.Time               `json:"start_time"`
	EndTime             time.Time               `json:"end_time"`
	ActiveDownloads     map[string]*DownloadProgress `json:"active_downloads"`
}

// GetSkippedByReason returns skipped items grouped by reason
func (s *Summary) GetSkippedByReason() map[SkipReason][]SkippedItem {
	result := make(map[SkipReason][]SkippedItem)
	for _, item := range s.SkippedItems {
		result[item.Reason] = append(result[item.Reason], item)
	}
	return result
}

// ProgressConfig holds configuration for progress reporting
type ProgressConfig struct {
	ShowProgressBar    bool          // Whether to show visual progress bar
	UpdateInterval     time.Duration // How often to update progress display
	LogInterval        time.Duration // How often to log progress to file
	Writer             io.Writer     // Where to write progress output (default: os.Stdout)
	EnableFileLogging  bool          // Whether to log progress to file
	CompactMode        bool          // Use compact progress display
	ShowSpeed          bool          // Show download speeds
	ShowETA            bool          // Show estimated time remaining
}

// progressReporterImpl implements the ProgressReporter interface
type progressReporterImpl struct {
	config          ProgressConfig
	logger          logging.Logger
	total           int
	downloads       map[string]*DownloadProgress
	skipped         []SkippedItem
	errors          []ErrorItem
	startTime       time.Time
	lastLogTime     time.Time
	lastDisplayTime time.Time
	mutex           sync.RWMutex
	writer          io.Writer
	ctx             context.Context
	cancel          context.CancelFunc
}

// NewProgressReporter creates a new progress reporter with the given configuration
func NewProgressReporter(config ProgressConfig, logger logging.Logger) ProgressReporter {
	// Set default values
	if config.UpdateInterval <= 0 {
		config.UpdateInterval = 500 * time.Millisecond
	}
	if config.LogInterval <= 0 {
		config.LogInterval = 5 * time.Second
	}
	if config.Writer == nil {
		config.Writer = os.Stdout
	}

	return &progressReporterImpl{
		config:    config,
		logger:    logger,
		downloads: make(map[string]*DownloadProgress),
		skipped:   []SkippedItem{},
		errors:    []ErrorItem{},
		writer:    config.Writer,
	}
}

// Start initializes the progress reporting session
func (pr *progressReporterImpl) Start(ctx context.Context, total int) error {
	pr.mutex.Lock()
	defer pr.mutex.Unlock()

	pr.total = total
	pr.startTime = time.Now()
	pr.lastLogTime = pr.startTime
	pr.lastDisplayTime = pr.startTime
	
	// Create context for background updates
	pr.ctx, pr.cancel = context.WithCancel(ctx)

	// Log session start
	if pr.logger != nil {
		pr.logger.InfoWithContext(ctx, "Progress tracking started: %d items to process", total)
		pr.logger.LogUserAction("progress_start", "system", map[string]interface{}{
			"total_items": total,
			"start_time":  pr.startTime,
		})
	}

	// Start background display updates if enabled
	if pr.config.ShowProgressBar {
		go pr.displayLoop()
	}

	// Start background logging if enabled
	if pr.config.EnableFileLogging && pr.logger != nil {
		go pr.loggingLoop()
	}

	return nil
}

// UpdateDownload updates progress for a specific download
func (pr *progressReporterImpl) UpdateDownload(update download.ProgressUpdate) {
	pr.mutex.Lock()
	defer pr.mutex.Unlock()

	// Find or create download progress
	progress, exists := pr.downloads[update.DownloadID]
	if !exists {
		// Extract filename from metadata or use download ID
		filename := update.DownloadID
		if update.Metadata != nil {
			if name, ok := update.Metadata["filename"].(string); ok {
				filename = name
			}
		}

		progress = &DownloadProgress{
			ID:        update.DownloadID,
			Filename:  filename,
			StartTime: time.Now(),
			Metadata:  update.Metadata,
		}
		pr.downloads[update.DownloadID] = progress
	}

	// Update progress fields
	progress.BytesDownloaded = update.BytesDownloaded
	progress.TotalBytes = update.TotalBytes
	progress.Speed = update.Speed
	progress.ETA = update.ETA
	progress.State = update.State
	progress.LastUpdate = update.Timestamp
	progress.Error = update.Error

	// Log significant state changes
	if pr.logger != nil {
		switch update.State {
		case download.DownloadStateDownloading:
			if !exists { // Only log when starting
				pr.logger.InfoWithContext(pr.ctx, "Starting download: %s (%s)", 
					progress.Filename, formatBytes(update.TotalBytes))
				pr.logger.LogUserAction("download_start", "system", map[string]interface{}{
					"download_id": update.DownloadID,
					"filename":    progress.Filename,
					"total_bytes": update.TotalBytes,
				})
			}
		case download.DownloadStateCompleted:
			duration := time.Since(progress.StartTime)
			avgSpeed := float64(update.BytesDownloaded) / duration.Seconds()
			pr.logger.InfoWithContext(pr.ctx, "Download completed: %s (%s in %v, avg speed: %s/s)", 
				progress.Filename, formatBytes(update.BytesDownloaded), duration, formatBytes(int64(avgSpeed)))
			pr.logger.LogPerformance(logging.PerformanceMetrics{
				Operation:      "download_file",
				Duration:       duration,
				BytesProcessed: update.BytesDownloaded,
				Success:        true,
				Metadata: map[string]interface{}{
					"download_id": update.DownloadID,
					"filename":    progress.Filename,
					"avg_speed":   avgSpeed,
				},
			})
		case download.DownloadStateFailed:
			pr.logger.ErrorWithContext(pr.ctx, "Download failed: %s - %v", progress.Filename, update.Error)
			pr.logger.LogPerformance(logging.PerformanceMetrics{
				Operation:      "download_file",
				Duration:       time.Since(progress.StartTime),
				BytesProcessed: update.BytesDownloaded,
				Success:        false,
				Error:          update.Error.Error(),
				Metadata: map[string]interface{}{
					"download_id": update.DownloadID,
					"filename":    progress.Filename,
				},
			})
		}
	}
}

// AddSkipped adds a skipped item to the progress tracking
func (pr *progressReporterImpl) AddSkipped(reason SkipReason, item string, details map[string]interface{}) {
	pr.mutex.Lock()
	defer pr.mutex.Unlock()

	skipped := SkippedItem{
		Item:      item,
		Reason:    reason,
		Details:   details,
		Timestamp: time.Now(),
	}

	pr.skipped = append(pr.skipped, skipped)

	// Log the skip
	if pr.logger != nil {
		pr.logger.WarnWithContext(pr.ctx, "Skipping item: %s (reason: %s)", item, reason.String())
		pr.logger.LogUserAction("item_skipped", "system", map[string]interface{}{
			"item":   item,
			"reason": reason.String(),
			"details": details,
		})
	}
}

// AddError adds an error to the progress tracking
func (pr *progressReporterImpl) AddError(item string, err error, details map[string]interface{}) {
	pr.mutex.Lock()
	defer pr.mutex.Unlock()

	errorItem := ErrorItem{
		Item:      item,
		Error:     err,
		ErrorMsg:  err.Error(),
		Details:   details,
		Timestamp: time.Now(),
	}

	pr.errors = append(pr.errors, errorItem)

	// Log the error
	if pr.logger != nil {
		pr.logger.ErrorWithContext(pr.ctx, "Error processing item: %s - %v", item, err)
		pr.logger.LogUserAction("item_error", "system", map[string]interface{}{
			"item":    item,
			"error":   err.Error(),
			"details": details,
		})
	}
}

// Finish completes the progress reporting session and shows summary
func (pr *progressReporterImpl) Finish() *Summary {
	pr.mutex.Lock()
	defer pr.mutex.Unlock()

	// Stop background routines
	if pr.cancel != nil {
		pr.cancel()
	}

	endTime := time.Now()
	duration := endTime.Sub(pr.startTime)

	// Calculate statistics
	var completed, failed int
	var totalBytes int64
	var totalSpeed float64
	activeDownloads := make(map[string]*DownloadProgress)

	for _, progress := range pr.downloads {
		switch progress.State {
		case download.DownloadStateCompleted:
			completed++
			totalBytes += progress.BytesDownloaded
			if progress.LastUpdate.After(progress.StartTime) {
				downloadDuration := progress.LastUpdate.Sub(progress.StartTime)
				if downloadDuration > 0 {
					totalSpeed += float64(progress.BytesDownloaded) / downloadDuration.Seconds()
				}
			}
		case download.DownloadStateFailed, download.DownloadStateCancelled:
			failed++
		default:
			// Still active
			activeDownloads[progress.ID] = progress
		}
	}

	var avgSpeed float64
	if completed > 0 {
		avgSpeed = totalSpeed / float64(completed)
	}

	summary := &Summary{
		TotalItems:          pr.total,
		CompletedDownloads:  completed,
		FailedDownloads:     failed,
		SkippedItems:        pr.skipped,
		ErrorItems:          pr.errors,
		TotalBytesDownloaded: totalBytes,
		AverageSpeed:        avgSpeed,
		TotalDuration:       duration,
		StartTime:           pr.startTime,
		EndTime:             endTime,
		ActiveDownloads:     activeDownloads,
	}

	// Show final summary
	pr.displaySummary(summary)

	// Log session completion
	if pr.logger != nil {
		pr.logger.InfoWithContext(pr.ctx, "Progress tracking completed: %d total, %d completed, %d failed, %d skipped", 
			pr.total, completed, failed, len(pr.skipped))
		pr.logger.LogPerformance(logging.PerformanceMetrics{
			Operation:      "progress_session",
			Duration:       duration,
			BytesProcessed: totalBytes,
			Success:        failed == 0,
			Metadata: map[string]interface{}{
				"total_items":      pr.total,
				"completed":        completed,
				"failed":           failed,
				"skipped":          len(pr.skipped),
				"average_speed":    avgSpeed,
			},
		})
	}

	return summary
}

// GetSummary returns current progress summary
func (pr *progressReporterImpl) GetSummary() *Summary {
	pr.mutex.RLock()
	defer pr.mutex.RUnlock()

	// Calculate current statistics
	var completed, failed int
	var totalBytes int64
	activeDownloads := make(map[string]*DownloadProgress)

	for _, progress := range pr.downloads {
		switch progress.State {
		case download.DownloadStateCompleted:
			completed++
			totalBytes += progress.BytesDownloaded
		case download.DownloadStateFailed, download.DownloadStateCancelled:
			failed++
		default:
			activeDownloads[progress.ID] = progress
		}
	}

	return &Summary{
		TotalItems:          pr.total,
		CompletedDownloads:  completed,
		FailedDownloads:     failed,
		SkippedItems:        pr.skipped,
		ErrorItems:          pr.errors,
		TotalBytesDownloaded: totalBytes,
		StartTime:           pr.startTime,
		EndTime:             time.Now(),
		ActiveDownloads:     activeDownloads,
	}
}

// displayLoop runs in background to update progress display
func (pr *progressReporterImpl) displayLoop() {
	ticker := time.NewTicker(pr.config.UpdateInterval)
	defer ticker.Stop()

	for {
		select {
		case <-pr.ctx.Done():
			return
		case <-ticker.C:
			pr.displayProgress()
		}
	}
}

// loggingLoop runs in background to log progress periodically
func (pr *progressReporterImpl) loggingLoop() {
	ticker := time.NewTicker(pr.config.LogInterval)
	defer ticker.Stop()

	for {
		select {
		case <-pr.ctx.Done():
			return
		case <-ticker.C:
			pr.logProgress()
		}
	}
}

// displayProgress shows current progress on console
func (pr *progressReporterImpl) displayProgress() {
	if !pr.config.ShowProgressBar {
		return
	}

	pr.mutex.RLock()
	defer pr.mutex.RUnlock()

	summary := pr.GetSummary()
	
	// Clear current line and move to beginning
	fmt.Fprint(pr.writer, "\r\033[K")
	
	// Calculate progress percentage
	processed := summary.CompletedDownloads + summary.FailedDownloads + len(summary.SkippedItems)
	var progressPercent float64
	if pr.total > 0 {
		progressPercent = float64(processed) / float64(pr.total) * 100
	}

	if pr.config.CompactMode {
		// Compact display: [████████████████████████████████████████] 100% | 15/15 recordings
		fmt.Fprintf(pr.writer, "[%s] %.0f%% | %d/%d recordings", 
			createProgressBar(progressPercent, 40), progressPercent, processed, pr.total)
	} else {
		// Full display with current download
		fmt.Fprintf(pr.writer, "Downloading recordings...\n[%s] %.0f%% | %d/%d recordings\n", 
			createProgressBar(progressPercent, 40), progressPercent, processed, pr.total)
		
		// Show current active downloads
		for _, progress := range summary.ActiveDownloads {
			if progress.State == download.DownloadStateDownloading {
				var progressBar string
				var percent float64
				if progress.TotalBytes > 0 {
					percent = float64(progress.BytesDownloaded) / float64(progress.TotalBytes) * 100
					progressBar = createProgressBar(percent, 20)
				} else {
					progressBar = "downloading..."
				}

				fmt.Fprintf(pr.writer, "└─ %s: %s [%.0f%%]", 
					progress.Filename, progressBar, percent)
				
				if pr.config.ShowSpeed && progress.Speed > 0 {
					fmt.Fprintf(pr.writer, " %s/s", formatBytes(int64(progress.Speed)))
				}
				
				if pr.config.ShowETA && progress.ETA > 0 {
					fmt.Fprintf(pr.writer, " ETA: %v", formatDuration(progress.ETA))
				}
				
				fmt.Fprint(pr.writer, "\n")
			}
		}
	}
}

// displaySummary shows the final summary
func (pr *progressReporterImpl) displaySummary(summary *Summary) {
	fmt.Fprintf(pr.writer, "\n\nSummary:\n")
	fmt.Fprintf(pr.writer, "- Total recordings: %d\n", summary.TotalItems)
	fmt.Fprintf(pr.writer, "- Downloaded: %d\n", summary.CompletedDownloads)
	
	if summary.FailedDownloads > 0 {
		fmt.Fprintf(pr.writer, "- Failed: %d\n", summary.FailedDownloads)
	}
	
	// Show skipped items by reason
	skippedByReason := summary.GetSkippedByReason()
	if len(skippedByReason[SkipReasonAlreadyExists]) > 0 {
		fmt.Fprintf(pr.writer, "- Skipped (already exists): %d\n", len(skippedByReason[SkipReasonAlreadyExists]))
	}
	if len(skippedByReason[SkipReasonInactiveUser]) > 0 {
		fmt.Fprintf(pr.writer, "- Skipped (inactive users): %d\n", len(skippedByReason[SkipReasonInactiveUser]))
	}
	if len(skippedByReason[SkipReasonMetaOnlyMode]) > 0 {
		fmt.Fprintf(pr.writer, "- Skipped (metadata only): %d\n", len(skippedByReason[SkipReasonMetaOnlyMode]))
	}
	
	if summary.TotalBytesDownloaded > 0 {
		fmt.Fprintf(pr.writer, "- Total size: %s\n", formatBytes(summary.TotalBytesDownloaded))
	}
	
	fmt.Fprintf(pr.writer, "- Time elapsed: %v\n", formatDuration(summary.TotalDuration))
	
	if pr.logger != nil && pr.config.EnableFileLogging {
		fmt.Fprintf(pr.writer, "\nAll operations logged to: %s\n", "zoom-downloader.log")
	}
}

// logProgress logs current progress to file
func (pr *progressReporterImpl) logProgress() {
	if pr.logger == nil {
		return
	}

	pr.mutex.RLock()
	defer pr.mutex.RUnlock()

	now := time.Now()
	if now.Sub(pr.lastLogTime) < pr.config.LogInterval {
		return
	}

	summary := pr.GetSummary()
	processed := summary.CompletedDownloads + summary.FailedDownloads + len(summary.SkippedItems)

	pr.logger.InfoWithContext(pr.ctx, "Progress update: %d/%d processed (%d completed, %d failed, %d skipped)", 
		processed, pr.total, summary.CompletedDownloads, summary.FailedDownloads, len(summary.SkippedItems))

	pr.lastLogTime = now
}

// Helper functions

// createProgressBar creates a visual progress bar string
func createProgressBar(percent float64, width int) string {
	filled := int(percent / 100 * float64(width))
	if filled > width {
		filled = width
	}
	
	bar := strings.Repeat("█", filled) + strings.Repeat("░", width-filled)
	return bar
}

// formatBytes formats byte count as human readable string
func formatBytes(bytes int64) string {
	const unit = 1024
	if bytes < unit {
		return fmt.Sprintf("%d B", bytes)
	}
	
	div, exp := int64(unit), 0
	for n := bytes / unit; n >= unit; n /= unit {
		div *= unit
		exp++
	}
	
	units := []string{"KB", "MB", "GB", "TB"}
	return fmt.Sprintf("%.1f %s", float64(bytes)/float64(div), units[exp])
}

// formatDuration formats duration as human readable string
func formatDuration(d time.Duration) string {
	if d < time.Minute {
		return fmt.Sprintf("%.0fs", d.Seconds())
	}
	if d < time.Hour {
		minutes := int(d.Minutes())
		seconds := int(d.Seconds()) - minutes*60
		return fmt.Sprintf("%dm %ds", minutes, seconds)
	}
	hours := int(d.Hours())
	minutes := int(d.Minutes()) - hours*60
	return fmt.Sprintf("%dh %dm", hours, minutes)
}