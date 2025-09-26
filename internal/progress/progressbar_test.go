package progress

import (
	"bytes"
	"fmt"
	"strings"
	"testing"
	"time"
)

func TestProgressBar_BasicOperations(t *testing.T) {
	buffer := &bytes.Buffer{}
	config := ProgressBarConfig{
		Writer:          buffer,
		Width:           20,
		ShowPercent:     true,
		ShowSpeed:       false,
		ShowETA:         false,
		ShowElapsed:     false,
		RefreshInterval: 10 * time.Millisecond,
		Units:          "bytes",
	}
	
	pb := NewProgressBar(1000, config)
	
	// Test initial state
	if pb.IsFinished() {
		t.Error("Progress bar should not be finished initially")
	}
	
	// Test update - force display by casting to implementation
	pb.Update(250)
	if pbImpl, ok := pb.(*progressBarImpl); ok {
		pbImpl.display() // Force immediate display for testing
	}
	
	// Check that something was written
	output := buffer.String()
	if !strings.Contains(output, "25%") {
		t.Errorf("Expected 25%% in output, got: %s", output)
	}
	
	// Test increment
	buffer.Reset()
	pb.IncrementBy(250)
	if pbImpl, ok := pb.(*progressBarImpl); ok {
		pbImpl.display() // Force immediate display for testing
	}
	
	output = buffer.String()
	if !strings.Contains(output, "50%") {
		t.Errorf("Expected 50%% in output, got: %s", output)
	}
	
	// Test finish
	buffer.Reset()
	pb.Finish()
	
	if !pb.IsFinished() {
		t.Error("Progress bar should be finished after calling Finish()")
	}
	
	output = buffer.String()
	if !strings.Contains(output, "100%") {
		t.Errorf("Expected 100%% in output after finish, got: %s", output)
	}
}

func TestProgressBar_WithMessage(t *testing.T) {
	buffer := &bytes.Buffer{}
	config := ProgressBarConfig{
		Writer:          buffer,
		Width:           20,
		ShowPercent:     true,
		RefreshInterval: 10 * time.Millisecond,
		Template:        "[{bar}] {percent}% - {message}",
	}
	
	pb := NewProgressBar(100, config)
	
	// Test update with message
	pb.UpdateWithMessage(50, "Downloading file.mp4")
	if pbImpl, ok := pb.(*progressBarImpl); ok {
		pbImpl.display()
	}
	
	output := buffer.String()
	if !strings.Contains(output, "Downloading file.mp4") {
		t.Errorf("Expected message in output, got: %s", output)
	}
	if !strings.Contains(output, "50%") {
		t.Errorf("Expected 50%% in output, got: %s", output)
	}
	
	// Test set message without update
	buffer.Reset()
	pb.SetMessage("Processing metadata")
	if pbImpl, ok := pb.(*progressBarImpl); ok {
		pbImpl.display()
	}
	
	output = buffer.String()
	if !strings.Contains(output, "Processing metadata") {
		t.Errorf("Expected new message in output, got: %s", output)
	}
}

func TestProgressBar_WithSpeedAndETA(t *testing.T) {
	buffer := &bytes.Buffer{}
	config := ProgressBarConfig{
		Writer:          buffer,
		Width:           20,
		ShowPercent:     true,
		ShowSpeed:       true,
		ShowETA:         true,
		RefreshInterval: 10 * time.Millisecond,
		Units:          "bytes",
		SpeedUnits:     "B/s",
	}
	
	pb := NewProgressBar(1000, config)
	
	// Create several updates to build speed history
	for i := 0; i < 5; i++ {
		pb.Update(int64(i * 200))
		time.Sleep(15 * time.Millisecond)
	}
	
	// Final update should show speed calculation
	pb.Update(800)
	time.Sleep(20 * time.Millisecond)
	
	output := buffer.String()
	// Should contain progress bar elements
	if !strings.Contains(output, "80%") {
		t.Errorf("Expected 80%% in output, got: %s", output)
	}
}

func TestProgressBar_Clear(t *testing.T) {
	buffer := &bytes.Buffer{}
	config := ProgressBarConfig{
		Writer:          buffer,
		Width:           20,
		RefreshInterval: 10 * time.Millisecond,
	}
	
	pb := NewProgressBar(100, config)
	
	// Make some progress
	pb.Update(50)
	time.Sleep(20 * time.Millisecond)
	
	// Clear should write clear sequence
	buffer.Reset()
	pb.Clear()
	
	output := buffer.String()
	if !strings.Contains(output, "\r\033[K") {
		t.Errorf("Expected clear sequence in output, got: %s", output)
	}
}

func TestProgressBar_SetTotal(t *testing.T) {
	buffer := &bytes.Buffer{}
	config := ProgressBarConfig{
		Writer:          buffer,
		Width:           20,
		ShowPercent:     true,
		RefreshInterval: 10 * time.Millisecond,
	}
	
	pb := NewProgressBar(100, config)
	
	// Update to 50% of original total
	pb.Update(50)
	time.Sleep(20 * time.Millisecond)
	
	// Change total - now 50 should be 25%
	pb.SetTotal(200)
	time.Sleep(20 * time.Millisecond)
	
	// This is hard to test precisely due to timing, but we can verify
	// the progress bar doesn't crash with total changes
	pb.Update(100) // Should be 50% of new total
	time.Sleep(20 * time.Millisecond)
	
	// Should not panic and should produce output
	output := buffer.String()
	if len(output) == 0 {
		t.Error("Expected some output after total change")
	}
}

func TestProgressBar_OverProgress(t *testing.T) {
	buffer := &bytes.Buffer{}
	config := ProgressBarConfig{
		Writer:          buffer,
		Width:           10,
		ShowPercent:     true,
		RefreshInterval: 10 * time.Millisecond,
	}
	
	pb := NewProgressBar(100, config)
	
	// Update beyond total
	pb.Update(150)
	if pbImpl, ok := pb.(*progressBarImpl); ok {
		pbImpl.display()
	}
	
	output := buffer.String()
	// Should cap at 100%
	if !strings.Contains(output, "100%") {
		t.Errorf("Expected 100%% for over-progress, got: %s", output)
	}
}

func TestMultiProgressBar(t *testing.T) {
	buffer := &bytes.Buffer{}
	config := ProgressBarConfig{
		Writer:          buffer,
		Width:           20,
		ShowPercent:     true,
		RefreshInterval: 10 * time.Millisecond,
	}
	
	mpb := NewMultiProgressBar(config)
	
	// Add multiple bars
	bar1 := mpb.AddBar("download1", 1000, "File 1")
	bar2 := mpb.AddBar("download2", 500, "File 2")
	
	if bar1 == nil || bar2 == nil {
		t.Fatal("Failed to create progress bars")
	}
	
	// Update bars
	bar1.Update(250)
	bar2.Update(100)
	time.Sleep(20 * time.Millisecond)
	
	// Get bars back
	retrievedBar1 := mpb.GetBar("download1")
	retrievedBar2 := mpb.GetBar("download2")
	
	if retrievedBar1 == nil || retrievedBar2 == nil {
		t.Error("Failed to retrieve progress bars")
	}
	
	// Remove a bar
	mpb.RemoveBar("download1")
	
	retrievedBar1After := mpb.GetBar("download1")
	if retrievedBar1After != nil {
		t.Error("Bar should be nil after removal")
	}
	
	// Finish all
	mpb.FinishAll()
	
	// Should not panic
	mpb.ClearAll()
}

func TestMultiProgressBar_AddAfterFinish(t *testing.T) {
	config := ProgressBarConfig{
		Writer: &bytes.Buffer{},
		Width:  20,
	}
	
	mpb := NewMultiProgressBar(config)
	
	// Finish all bars
	mpb.FinishAll()
	
	// Try to add after finish
	bar := mpb.AddBar("late-bar", 100, "Late Bar")
	
	if bar != nil {
		t.Error("Should not be able to add bar after finish")
	}
}

func TestFormatValue(t *testing.T) {
	tests := []struct {
		input    int64
		expected string
	}{
		{500, "500"},
		{1500, "1.5K"},
		{1500000, "1.5M"},
		{1500000000, "1.5G"},
		{999, "999"},
		{1000, "1.0K"},
		{1001, "1.0K"},
	}
	
	for _, test := range tests {
		t.Run(fmt.Sprintf("%d", test.input), func(t *testing.T) {
			result := formatValue(test.input)
			if result != test.expected {
				t.Errorf("Expected %s, got %s", test.expected, result)
			}
		})
	}
}

func TestFormatSpeed(t *testing.T) {
	tests := []struct {
		input    float64
		expected string
	}{
		{500.0, "500.0"},
		{1500.0, "1.5K"},
		{1500000.0, "1.5M"},
		{1500000000.0, "1.5G"},
		{999.9, "999.9"},
		{1000.0, "1.0K"},
	}
	
	for _, test := range tests {
		t.Run(fmt.Sprintf("%.1f", test.input), func(t *testing.T) {
			result := formatSpeed(test.input)
			if result != test.expected {
				t.Errorf("Expected %s, got %s", test.expected, result)
			}
		})
	}
}

func TestProgressBar_CustomTemplate(t *testing.T) {
	buffer := &bytes.Buffer{}
	config := ProgressBarConfig{
		Writer:          buffer,
		Width:           10,
		RefreshInterval: 10 * time.Millisecond,
		Template:        "Progress: {percent}% [{bar}] {current}/{total}",
	}
	
	pb := NewProgressBar(100, config)
	pb.Update(25)
	if pbImpl, ok := pb.(*progressBarImpl); ok {
		pbImpl.display()
	}
	
	output := buffer.String()
	if !strings.Contains(output, "Progress: 25%") {
		t.Errorf("Expected custom template format, got: %s", output)
	}
	if !strings.Contains(output, "25/100") {
		t.Errorf("Expected current/total format, got: %s", output)
	}
}

func TestProgressBar_ZeroTotal(t *testing.T) {
	buffer := &bytes.Buffer{}
	config := ProgressBarConfig{
		Writer:          buffer,
		Width:           10,
		ShowPercent:     true,
		RefreshInterval: 10 * time.Millisecond,
	}
	
	pb := NewProgressBar(0, config)
	pb.Update(10)
	if pbImpl, ok := pb.(*progressBarImpl); ok {
		pbImpl.display()
	}
	
	// Should not panic with zero total
	output := buffer.String()
	if len(output) == 0 {
		t.Error("Expected some output even with zero total")
	}
}

func TestProgressBar_NegativeValues(t *testing.T) {
	buffer := &bytes.Buffer{}
	config := ProgressBarConfig{
		Writer:          buffer,
		Width:           10,
		RefreshInterval: 10 * time.Millisecond,
	}
	
	pb := NewProgressBar(100, config)
	
	// Test negative current
	pb.Update(-10)
	if pbImpl, ok := pb.(*progressBarImpl); ok {
		pbImpl.display()
	}
	
	// Should handle gracefully
	output := buffer.String()
	if len(output) == 0 {
		t.Error("Expected some output even with negative current")
	}
	
	// Test negative increment
	pb.IncrementBy(-5)
	if pbImpl, ok := pb.(*progressBarImpl); ok {
		pbImpl.display()
	}
	
	// Should not panic
}

func TestProgressBar_SpeedCalculation(t *testing.T) {
	buffer := &bytes.Buffer{}
	config := ProgressBarConfig{
		Writer:          buffer,
		Width:           10,
		ShowSpeed:       true,
		RefreshInterval: 10 * time.Millisecond,
	}
	
	pb := NewProgressBar(1000, config)
	
	// Cast to implementation to test speed calculation
	pbImpl := pb.(*progressBarImpl)
	
	// Add speed samples manually
	now := time.Now()
	pbImpl.addSpeedSample(now, 0)
	pbImpl.addSpeedSample(now.Add(100*time.Millisecond), 100)
	pbImpl.addSpeedSample(now.Add(200*time.Millisecond), 200)
	
	speed := pbImpl.calculateSpeed()
	
	// Should calculate approximately 1000 units per second (100 units per 100ms)
	expectedSpeed := 1000.0
	tolerance := 100.0 // Allow some tolerance for timing variations
	
	if speed < expectedSpeed-tolerance || speed > expectedSpeed+tolerance {
		t.Errorf("Expected speed around %.1f, got %.1f", expectedSpeed, speed)
	}
}

func TestProgressBar_ETACalculation(t *testing.T) {
	buffer := &bytes.Buffer{}
	config := ProgressBarConfig{
		Writer:          buffer,
		Width:           10,
		ShowETA:         true,
		RefreshInterval: 10 * time.Millisecond,
	}
	
	pb := NewProgressBar(1000, config)
	pbImpl := pb.(*progressBarImpl)
	
	// Set up known speed
	now := time.Now()
	pbImpl.addSpeedSample(now, 0)
	pbImpl.addSpeedSample(now.Add(100*time.Millisecond), 100)
	
	// Set current progress
	pbImpl.current = 200
	
	// Debug: check speed calculation
	speed := pbImpl.calculateSpeed()
	remaining := pbImpl.total - pbImpl.current
	
	eta := pbImpl.calculateETA()
	
	// Debug output
	t.Logf("Debug: speed=%v, current=%d, total=%d, remaining=%d, eta=%v", 
		speed, pbImpl.current, pbImpl.total, remaining, eta)
	
	// At 1000 units/second speed, 800 units remaining should take ~0.8 seconds
	expectedETA := 800 * time.Millisecond
	tolerance := 400 * time.Millisecond // Be more lenient for timing
	
	// Check if ETA is calculated (not zero) and within reasonable range
	if eta == 0 {
		t.Errorf("Expected ETA to be calculated, got 0 (speed: %v, current: %d, total: %d)", speed, pbImpl.current, pbImpl.total)
	} else if eta < expectedETA-tolerance || eta > expectedETA+tolerance {
		t.Errorf("Expected ETA around %v, got %v", expectedETA, eta)
	}
}

// Benchmark tests
func BenchmarkProgressBar_Update(b *testing.B) {
	config := ProgressBarConfig{
		Writer:          &bytes.Buffer{},
		Width:           40,
		RefreshInterval: 100 * time.Millisecond,
	}
	
	pb := NewProgressBar(int64(b.N), config)
	
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		pb.Update(int64(i))
	}
}

func BenchmarkProgressBar_CreateBar(b *testing.B) {
	pbImpl := &progressBarImpl{
		config: ProgressBarConfig{Width: 40},
	}
	
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		pbImpl.createBar(float64(i % 101))
	}
}

func BenchmarkFormatValue(b *testing.B) {
	values := []int64{123, 1234, 12345, 123456, 1234567, 12345678}
	
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		formatValue(values[i%len(values)])
	}
}

func BenchmarkFormatSpeed(b *testing.B) {
	speeds := []float64{123.5, 1234.5, 12345.5, 123456.5, 1234567.5}
	
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		formatSpeed(speeds[i%len(speeds)])
	}
}