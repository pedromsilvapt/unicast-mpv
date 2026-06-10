package logger

import (
	"bytes"
	"fmt"
	"regexp"
	"strings"
	"testing"
	"time"
)

func TestNewLogger(t *testing.T) {
	var buf bytes.Buffer
	l := New(WithOutput(&buf), WithColorize(false))
	l.Info("hello")
	output := buf.String()
	if !strings.Contains(output, "INFO") {
		t.Errorf("expected INFO in output, got: %s", output)
	}
	if !strings.Contains(output, "hello") {
		t.Errorf("expected 'hello' in output, got: %s", output)
	}
}

func TestLoggerLevels(t *testing.T) {
	tests := []struct {
		level    Level
		expected string
	}{
		{DebugLevel, "DEBUG"},
		{InfoLevel, "INFO"},
		{WarnLevel, "WARN"},
		{ErrorLevel, "ERROR"},
		{FatalLevel, "FATAL"},
	}
	for _, tt := range tests {
		var buf bytes.Buffer
		l := New(WithOutput(&buf), WithColorize(false))
		switch tt.level {
		case DebugLevel:
			l.Debug("test")
		case InfoLevel:
			l.Info("test")
		case WarnLevel:
			l.Warn("test")
		case ErrorLevel:
			l.Error("test")
		case FatalLevel:
			l.Fatal("test")
		}
		if !strings.Contains(buf.String(), tt.expected) {
			t.Errorf("expected %s in output, got: %s", tt.expected, buf.String())
		}
	}
}

func TestLoggerWithService(t *testing.T) {
	var buf bytes.Buffer
	l := New(WithOutput(&buf), WithColorize(false))
	rpcLog := l.Service("rpc")
	rpcLog.Info("test message")
	output := buf.String()
	if !strings.Contains(output, "[rpc]") {
		t.Errorf("expected [rpc] in output, got: %s", output)
	}
}

func TestLoggerNestedService(t *testing.T) {
	var buf bytes.Buffer
	l := New(WithOutput(&buf), WithColorize(false))
	rpcLog := l.Service("rpc")
	cmdLog := rpcLog.Service("status")
	cmdLog.Info("nested")
	output := buf.String()
	if !strings.Contains(output, "[rpc.status]") {
		t.Errorf("expected [rpc.status] in output, got: %s", output)
	}
}

func TestLoggerServiceCaching(t *testing.T) {
	l := New()
	rpc1 := l.Service("rpc")
	rpc2 := l.Service("rpc")
	if rpc1 != rpc2 {
		t.Error("expected same service logger instance")
	}
}

func TestLoggerMinLevel(t *testing.T) {
	var buf bytes.Buffer
	l := New(WithOutput(&buf), WithMinLevel(WarnLevel), WithColorize(false))
	l.Debug("should not appear")
	l.Info("should not appear")
	l.Warn("should appear")
	if strings.Contains(buf.String(), "should not appear") {
		t.Error("debug/info messages should not be logged below min level")
	}
	if !strings.Contains(buf.String(), "should appear") {
		t.Error("warn message should be logged")
	}
}

func TestLoggerColorize(t *testing.T) {
	var buf bytes.Buffer
	l := New(WithOutput(&buf), WithColorize(true))
	l.Info("colored")
	output := buf.String()
	if !strings.Contains(output, "\033[") {
		t.Errorf("expected ANSI color codes in output, got: %s", output)
	}
}

func TestLoggerColorizeDisabled(t *testing.T) {
	var buf bytes.Buffer
	l := New(WithOutput(&buf), WithColorize(false))
	l.Info("plain")
	output := buf.String()
	if strings.Contains(output, "\033[") {
		t.Errorf("expected no ANSI color codes in output, got: %s", output)
	}
}

func TestLoggerFormattedMessage(t *testing.T) {
	var buf bytes.Buffer
	l := New(WithOutput(&buf), WithColorize(false))
	l.Infof("count: %d", 42)
	output := buf.String()
	if !strings.Contains(output, "count: 42") {
		t.Errorf("expected formatted message, got: %s", output)
	}
}

func TestLoggerTimestamp(t *testing.T) {
	var buf bytes.Buffer
	l := New(WithOutput(&buf), WithColorize(false))
	l.Info("test")
	output := buf.String()
	if len(output) < 8 {
		t.Errorf("output too short, expected timestamp prefix, got: %s", output)
	}
}

func TestHFPatternSuppression(t *testing.T) {
	var buf bytes.Buffer
	l := New(
		WithOutput(&buf),
		WithColorize(false),
		WithHFPatterns(HFPattern{
			Pattern:    regexp.MustCompile(`status`),
			SuppressMs: 5000,
		}),
	)

	statusLog := l.Service("status")
	statusLog.Info("should appear")
	output1 := buf.String()
	if !strings.Contains(output1, "should appear") {
		t.Errorf("first message should appear, got: %s", output1)
	}

	buf.Reset()
	statusLog.Info("should be suppressed")
	output2 := buf.String()
	if output2 != "" {
		t.Errorf("second rapid message should be suppressed, got: %s", output2)
	}
}

func TestHFPatternNoSuppressionForOtherServices(t *testing.T) {
	var buf bytes.Buffer
	l := New(
		WithOutput(&buf),
		WithColorize(false),
		WithHFPatterns(HFPattern{
			Pattern:    regexp.MustCompile(`status`),
			SuppressMs: 5000,
		}),
	)

	playLog := l.Service("play")
	playLog.Info("should appear")
	if !strings.Contains(buf.String(), "should appear") {
		t.Error("non-matching service should not be suppressed")
	}
}

func TestActivityLogger(t *testing.T) {
	var buf bytes.Buffer
	l := New(WithOutput(&buf), WithColorize(false))
	al := NewActivityLogger(l)

	al.Before("play", []interface{}{"file.mp4"})
	if !strings.Contains(buf.String(), "running...") {
		t.Errorf("expected 'running...' in output, got: %s", buf.String())
	}
}

func TestActivityLoggerIgnoreCommand(t *testing.T) {
	var buf bytes.Buffer
	l := New(WithOutput(&buf), WithColorize(false))
	al := NewActivityLogger(l)
	al.IgnoreCommand("status")

	al.Before("status", []interface{}{})
	if strings.Contains(buf.String(), "running") {
		t.Errorf("ignored command should not be logged, got: %s", buf.String())
	}
}

func TestActivityLoggerAfterWithError(t *testing.T) {
	var buf bytes.Buffer
	l := New(WithOutput(&buf), WithColorize(false))
	al := NewActivityLogger(l)

	al.After("play", []interface{}{"file.mp4"}, fmt.Errorf("test error"), nil, 100*time.Millisecond)
	if !strings.Contains(buf.String(), "FAILED") {
		t.Errorf("expected FAILED in output, got: %s", buf.String())
	}
}

func TestActivityLoggerHFPattern(t *testing.T) {
	var buf bytes.Buffer
	l := New(WithOutput(&buf), WithColorize(false))
	al := NewActivityLogger(l)
	al.RegisterHFPattern(regexp.MustCompile(`status`), 5000)

	_ = al
}

func TestColorValues(t *testing.T) {
	tests := []struct {
		level Level
		color color
	}{
		{DebugLevel, colorGray},
		{InfoLevel, colorGreen},
		{WarnLevel, colorYellow},
		{ErrorLevel, colorRed},
		{FatalLevel, colorMagenta},
	}
	for _, tt := range tests {
		got := levelColor(tt.level)
		if got != tt.color {
			t.Errorf("expected color %d for level %v, got %d", tt.color, tt.level, got)
		}
	}
}

func TestJoinService(t *testing.T) {
	tests := []struct {
		parent, child, expected string
	}{
		{"", "rpc", "rpc"},
		{"rpc", "status", "rpc.status"},
	}
	for _, tt := range tests {
		got := joinService(tt.parent, tt.child)
		if got != tt.expected {
			t.Errorf("joinService(%q, %q) = %q, want %q", tt.parent, tt.child, got, tt.expected)
		}
	}
}

func TestFormatArgs(t *testing.T) {
	result := formatArgs([]interface{}{"hello", 42})
	if !strings.Contains(result, "hello") || !strings.Contains(result, "42") {
		t.Errorf("expected formatted args, got: %s", result)
	}
}