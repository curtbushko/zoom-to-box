// Package progress provides integration utilities for zoom-to-box progress reporting
package progress

import (
	"context"
	"fmt"
	"path/filepath"
	"time"

	"github.com/curtbushko/zoom-to-box/internal/download"
	"github.com/curtbushko/zoom-to-box/internal/logging"
)

// DownloadProgressTracker integrates download manager with progress reporting
type DownloadProgressTracker struct {
	reporter     ProgressReporter
	downloadMgr  download.DownloadManager
	logger       logging.Logger
	ctx          context.Context
}

// NewDownloadProgressTracker creates a new download progress tracker
func NewDownloadProgressTracker(
	reporter ProgressReporter,
	downloadMgr download.DownloadManager,
	logger logging.Logger,
) *DownloadProgressTracker {
	return &DownloadProgressTracker{
		reporter:    reporter,
		downloadMgr: downloadMgr,
		logger:      logger,
	}
}

// StartDownloadWithProgress starts a download with integrated progress tracking
func (dpt *DownloadProgressTracker) StartDownloadWithProgress(
	ctx context.Context,
	req download.DownloadRequest,
) (*download.DownloadResult, error) {
	// Add filename to metadata for progress tracking
	if req.Metadata == nil {
		req.Metadata = make(map[string]interface{})
	}
	
	filename := filepath.Base(req.Destination)
	req.Metadata["filename"] = filename

	// Create progress callback that forwards to reporter
	progressCallback := func(update download.ProgressUpdate) {
		dpt.reporter.UpdateDownload(update)
	}

	// Start the download
	result, err := dpt.downloadMgr.Download(ctx, req, progressCallback)
	
	if err != nil {
		// Add error to progress tracking
		dpt.reporter.AddError(filename, err, map[string]interface{}{
			"download_id":   req.ID,
			"destination":   req.Destination,
			"url":          req.URL,
			"expected_size": req.FileSize,
		})
	}

	return result, err
}

// ProgressConfig creates a progress configuration from CLI flags and config
type ProgressConfigBuilder struct {
	verbose       bool
	compactMode   bool
	showSpeed     bool
	showETA       bool
	enableLogging bool
	logInterval   time.Duration
}

// NewProgressConfigBuilder creates a new progress config builder
func NewProgressConfigBuilder() *ProgressConfigBuilder {
	return &ProgressConfigBuilder{
		verbose:       false,
		compactMode:   false,
		showSpeed:     true,
		showETA:       true,
		enableLogging: true,
		logInterval:   5 * time.Second,
	}
}

// WithVerbose sets verbose mode
func (pcb *ProgressConfigBuilder) WithVerbose(verbose bool) *ProgressConfigBuilder {
	pcb.verbose = verbose
	return pcb
}

// WithCompactMode sets compact display mode
func (pcb *ProgressConfigBuilder) WithCompactMode(compact bool) *ProgressConfigBuilder {
	pcb.compactMode = compact
	return pcb
}

// WithSpeedDisplay sets whether to show download speeds
func (pcb *ProgressConfigBuilder) WithSpeedDisplay(show bool) *ProgressConfigBuilder {
	pcb.showSpeed = show
	return pcb
}

// WithETADisplay sets whether to show estimated time remaining
func (pcb *ProgressConfigBuilder) WithETADisplay(show bool) *ProgressConfigBuilder {
	pcb.showETA = show
	return pcb
}

// WithFileLogging sets whether to enable file logging
func (pcb *ProgressConfigBuilder) WithFileLogging(enable bool) *ProgressConfigBuilder {
	pcb.enableLogging = enable
	return pcb
}

// WithLogInterval sets the logging interval
func (pcb *ProgressConfigBuilder) WithLogInterval(interval time.Duration) *ProgressConfigBuilder {
	pcb.logInterval = interval
	return pcb
}

// Build creates a ProgressConfig from the builder settings
func (pcb *ProgressConfigBuilder) Build() ProgressConfig {
	return ProgressConfig{
		ShowProgressBar:   true,
		UpdateInterval:    500 * time.Millisecond,
		LogInterval:       pcb.logInterval,
		Writer:            nil, // Will use default (stdout)
		EnableFileLogging: pcb.enableLogging,
		CompactMode:       pcb.compactMode,
		ShowSpeed:         pcb.showSpeed,
		ShowETA:           pcb.showETA,
	}
}

// ProgressBarConfigBuilder creates progress bar configuration
type ProgressBarConfigBuilder struct {
	width      int
	showSpeed  bool
	showETA    bool
	showElapsed bool
	units      string
	speedUnits string
}

// NewProgressBarConfigBuilder creates a new progress bar config builder
func NewProgressBarConfigBuilder() *ProgressBarConfigBuilder {
	return &ProgressBarConfigBuilder{
		width:       40,
		showSpeed:   true,
		showETA:     true,
		showElapsed: false,
		units:       "bytes",
		speedUnits:  "B/s",
	}
}

// WithWidth sets the progress bar width
func (pbc *ProgressBarConfigBuilder) WithWidth(width int) *ProgressBarConfigBuilder {
	pbc.width = width
	return pbc
}

// WithSpeedDisplay sets whether to show speed
func (pbc *ProgressBarConfigBuilder) WithSpeedDisplay(show bool) *ProgressBarConfigBuilder {
	pbc.showSpeed = show
	return pbc
}

// WithETADisplay sets whether to show ETA
func (pbc *ProgressBarConfigBuilder) WithETADisplay(show bool) *ProgressBarConfigBuilder {
	pbc.showETA = show
	return pbc
}

// WithElapsedDisplay sets whether to show elapsed time
func (pbc *ProgressBarConfigBuilder) WithElapsedDisplay(show bool) *ProgressBarConfigBuilder {
	pbc.showElapsed = show
	return pbc
}

// WithUnits sets the units for progress display
func (pbc *ProgressBarConfigBuilder) WithUnits(units string) *ProgressBarConfigBuilder {
	pbc.units = units
	return pbc
}

// WithSpeedUnits sets the speed units
func (pbc *ProgressBarConfigBuilder) WithSpeedUnits(speedUnits string) *ProgressBarConfigBuilder {
	pbc.speedUnits = speedUnits
	return pbc
}

// Build creates a ProgressBarConfig from the builder settings
func (pbc *ProgressBarConfigBuilder) Build() ProgressBarConfig {
	return ProgressBarConfig{
		Writer:          nil, // Will use default
		Width:           pbc.width,
		ShowPercent:     true,
		ShowSpeed:       pbc.showSpeed,
		ShowETA:         pbc.showETA,
		ShowElapsed:     pbc.showElapsed,
		RefreshInterval: 100 * time.Millisecond,
		Units:           pbc.units,
		SpeedUnits:      pbc.speedUnits,
	}
}

// LoggingProgressReporter is a decorator that adds enhanced logging to progress reporting
type LoggingProgressReporter struct {
	base   ProgressReporter
	logger logging.Logger
	ctx    context.Context
}

// NewLoggingProgressReporter creates a progress reporter with enhanced logging
func NewLoggingProgressReporter(base ProgressReporter, logger logging.Logger) *LoggingProgressReporter {
	return &LoggingProgressReporter{
		base:   base,
		logger: logger,
	}
}

// Start initializes the progress reporting session with enhanced logging
func (lpr *LoggingProgressReporter) Start(ctx context.Context, total int) error {
	lpr.ctx = ctx
	
	// Enhanced start logging
	if lpr.logger != nil {
		requestID := logging.GenerateRequestID()
		ctx = logging.WithRequestID(ctx, requestID)
		lpr.ctx = ctx
		
		lpr.logger.InfoWithContext(ctx, "Starting download session with progress tracking")
		lpr.logger.LogUserAction("session_start", "system", map[string]interface{}{
			"total_items":  total,
			"request_id":   requestID,
			"start_time":   time.Now(),
			"session_type": "download_with_progress",
		})
	}
	
	return lpr.base.Start(ctx, total)
}

// UpdateDownload updates progress with enhanced logging
func (lpr *LoggingProgressReporter) UpdateDownload(update download.ProgressUpdate) {
	// Log significant milestones
	if lpr.logger != nil && update.TotalBytes > 0 {
		percentComplete := float64(update.BytesDownloaded) / float64(update.TotalBytes) * 100
		
		// Log at 25%, 50%, 75%, and 100% completion
		milestones := []float64{25, 50, 75, 100}
		for _, milestone := range milestones {
			if percentComplete >= milestone && update.State == download.DownloadStateDownloading {
				// Check if this milestone was already logged (simple approach - could be more sophisticated)
				if update.Metadata == nil {
					update.Metadata = make(map[string]interface{})
				}
				
				milestoneKey := fmt.Sprintf("milestone_%.0f", milestone)
				if _, logged := update.Metadata[milestoneKey]; !logged {
					lpr.logger.InfoWithContext(lpr.ctx, "Download %s reached %.0f%% completion", 
						update.DownloadID, milestone)
					lpr.logger.LogPerformance(logging.PerformanceMetrics{
						Operation:      "download_milestone",
						Duration:       time.Since(update.Timestamp),
						BytesProcessed: update.BytesDownloaded,
						Success:        true,
						Metadata: map[string]interface{}{
							"download_id":      update.DownloadID,
							"milestone":        milestone,
							"bytes_downloaded": update.BytesDownloaded,
							"total_bytes":      update.TotalBytes,
							"speed":            update.Speed,
						},
					})
					
					// Mark milestone as logged
					update.Metadata[milestoneKey] = true
				}
				break
			}
		}
	}
	
	lpr.base.UpdateDownload(update)
}

// AddSkipped adds a skipped item with enhanced logging
func (lpr *LoggingProgressReporter) AddSkipped(reason SkipReason, item string, details map[string]interface{}) {
	if lpr.logger != nil {
		// Add contextual information
		enhancedDetails := make(map[string]interface{})
		for k, v := range details {
			enhancedDetails[k] = v
		}
		enhancedDetails["skip_timestamp"] = time.Now()
		enhancedDetails["session_context"] = "download_with_progress"
		
		lpr.logger.WarnWithContext(lpr.ctx, "Item skipped during download session: %s (reason: %s)", 
			item, reason.String())
	}
	
	lpr.base.AddSkipped(reason, item, details)
}

// AddError adds an error with enhanced logging
func (lpr *LoggingProgressReporter) AddError(item string, err error, details map[string]interface{}) {
	if lpr.logger != nil {
		// Add contextual information
		enhancedDetails := make(map[string]interface{})
		for k, v := range details {
			enhancedDetails[k] = v
		}
		enhancedDetails["error_timestamp"] = time.Now()
		enhancedDetails["session_context"] = "download_with_progress"
		enhancedDetails["error_type"] = fmt.Sprintf("%T", err)
		
		lpr.logger.ErrorWithContext(lpr.ctx, "Error during download session: %s - %v", item, err)
	}
	
	lpr.base.AddError(item, err, details)
}

// Finish completes the session with enhanced logging
func (lpr *LoggingProgressReporter) Finish() *Summary {
	summary := lpr.base.Finish()
	
	if lpr.logger != nil && summary != nil {
		// Log comprehensive session summary
		lpr.logger.InfoWithContext(lpr.ctx, "Download session completed with progress tracking")
		lpr.logger.LogPerformance(logging.PerformanceMetrics{
			Operation:      "download_session_complete",
			Duration:       summary.TotalDuration,
			BytesProcessed: summary.TotalBytesDownloaded,
			Success:        summary.FailedDownloads == 0 && len(summary.ErrorItems) == 0,
			Metadata: map[string]interface{}{
				"total_items":       summary.TotalItems,
				"completed":         summary.CompletedDownloads,
				"failed":            summary.FailedDownloads,
				"skipped":           len(summary.SkippedItems),
				"errors":            len(summary.ErrorItems),
				"total_bytes":       summary.TotalBytesDownloaded,
				"average_speed":     summary.AverageSpeed,
				"session_duration":  summary.TotalDuration.String(),
			},
		})
		
		// Log detailed breakdown of skipped items
		skippedByReason := summary.GetSkippedByReason()
		for reason, items := range skippedByReason {
			if len(items) > 0 {
				lpr.logger.InfoWithContext(lpr.ctx, "Session summary - skipped items (%s): %d", 
					reason.String(), len(items))
			}
		}
	}
	
	return summary
}

// GetSummary returns current summary
func (lpr *LoggingProgressReporter) GetSummary() *Summary {
	return lpr.base.GetSummary()
}

// ProgressMetrics provides metrics collection for progress reporting
type ProgressMetrics struct {
	SessionStart        time.Time
	TotalDownloads      int
	CompletedDownloads  int
	FailedDownloads     int
	SkippedDownloads    int
	TotalBytesProcessed int64
	AverageSpeed        float64
	PeakSpeed           float64
	ActiveConnections   int
}

// GetMetrics extracts metrics from a progress summary
func GetMetrics(summary *Summary) ProgressMetrics {
	var peakSpeed float64
	for _, download := range summary.ActiveDownloads {
		if download.Speed > peakSpeed {
			peakSpeed = download.Speed
		}
	}
	
	return ProgressMetrics{
		SessionStart:        summary.StartTime,
		TotalDownloads:      summary.TotalItems,
		CompletedDownloads:  summary.CompletedDownloads,
		FailedDownloads:     summary.FailedDownloads,
		SkippedDownloads:    len(summary.SkippedItems),
		TotalBytesProcessed: summary.TotalBytesDownloaded,
		AverageSpeed:        summary.AverageSpeed,
		PeakSpeed:           peakSpeed,
		ActiveConnections:   len(summary.ActiveDownloads),
	}
}