package logger

import (
	"fmt"
	"io"
	"os"
	"regexp"
	"sync"
	"time"
)

type Level int

const (
	DebugLevel Level = iota
	InfoLevel
	WarnLevel
	ErrorLevel
	FatalLevel
)

func (l Level) String() string {
	switch l {
	case DebugLevel:
		return "DEBUG"
	case InfoLevel:
		return "INFO"
	case WarnLevel:
		return "WARN"
	case ErrorLevel:
		return "ERROR"
	case FatalLevel:
		return "FATAL"
	default:
		return "UNKNOWN"
	}
}

type color int

const (
	colorReset   color = 0
	colorRed     color = 31
	colorGreen   color = 32
	colorYellow  color = 33
	colorBlue    color = 34
	colorMagenta color = 35
	colorCyan    color = 36
	colorGray    color = 90
)

func (c color) format() string {
	if c == colorReset {
		return "\033[0m"
	}
	return fmt.Sprintf("\033[%dm", c)
}

func levelColor(l Level) color {
	switch l {
	case DebugLevel:
		return colorGray
	case InfoLevel:
		return colorGreen
	case WarnLevel:
		return colorYellow
	case ErrorLevel:
		return colorRed
	case FatalLevel:
		return colorMagenta
	default:
		return colorReset
	}
}

type HFPattern struct {
	Pattern    *regexp.Regexp
	SuppressMs int64
}

type Logger struct {
	mu         sync.Mutex
	out        io.Writer
	service    string
	colorize   bool
	minLevel   Level
	hfPatterns []HFPattern
	hfLastTime map[string]int64
	hfCount    map[string]int64
	services   map[string]*Logger
}

func New(opts ...Option) *Logger {
	l := &Logger{
		out:        os.Stdout,
		colorize:   true,
		minLevel:   DebugLevel,
		hfPatterns: nil,
		hfLastTime: make(map[string]int64),
		hfCount:    make(map[string]int64),
		services:   make(map[string]*Logger),
	}
	for _, opt := range opts {
		opt(l)
	}
	return l
}

type Option func(*Logger)

func WithOutput(w io.Writer) Option {
	return func(l *Logger) { l.out = w }
}

func WithService(name string) Option {
	return func(l *Logger) { l.service = name }
}

func WithColorize(enabled bool) Option {
	return func(l *Logger) { l.colorize = enabled }
}

func WithMinLevel(level Level) Option {
	return func(l *Logger) { l.minLevel = level }
}

func WithHFPatterns(patterns ...HFPattern) Option {
	return func(l *Logger) { l.hfPatterns = patterns }
}

func (l *Logger) Service(name string) *Logger {
	l.mu.Lock()
	defer l.mu.Unlock()
	if child, ok := l.services[name]; ok {
		return child
	}
	child := &Logger{
		out:        l.out,
		service:    joinService(l.service, name),
		colorize:   l.colorize,
		minLevel:   l.minLevel,
		hfPatterns: l.hfPatterns,
		hfLastTime: l.hfLastTime,
		hfCount:    l.hfCount,
		services:   make(map[string]*Logger),
	}
	l.services[name] = child
	return child
}

func joinService(parent, child string) string {
	if parent == "" {
		return child
	}
	return parent + "." + child
}

func (l *Logger) Debug(msg string)  { l.log(DebugLevel, msg) }
func (l *Logger) Info(msg string)   { l.log(InfoLevel, msg) }
func (l *Logger) Warn(msg string)   { l.log(WarnLevel, msg) }
func (l *Logger) Error(msg string)  { l.log(ErrorLevel, msg) }
func (l *Logger) Fatal(msg string)  { l.log(FatalLevel, msg) }

func (l *Logger) Debugf(format string, args ...interface{}) { l.log(DebugLevel, fmt.Sprintf(format, args...)) }
func (l *Logger) Infof(format string, args ...interface{})  { l.log(InfoLevel, fmt.Sprintf(format, args...)) }
func (l *Logger) Warnf(format string, args ...interface{})  { l.log(WarnLevel, fmt.Sprintf(format, args...)) }
func (l *Logger) Errorf(format string, args ...interface{}) { l.log(ErrorLevel, fmt.Sprintf(format, args...)) }
func (l *Logger) Fatalf(format string, args ...interface{}) { l.log(FatalLevel, fmt.Sprintf(format, args...)) }

func (l *Logger) log(level Level, msg string) {
	if level < l.minLevel {
		return
	}

	if l.isSuppressed(level, msg) {
		return
	}

	timestamp := time.Now().Format("15:04:05")
	var line string
	if l.service != "" {
		line = fmt.Sprintf("%s %s [%s] %s", timestamp, level.String(), l.service, msg)
	} else {
		line = fmt.Sprintf("%s %s %s", timestamp, level.String(), msg)
	}

	if l.colorize {
		line = colorizeLine(line, level)
	}

	l.mu.Lock()
	defer l.mu.Unlock()
	fmt.Fprintln(l.out, line)
}

func (l *Logger) isSuppressed(level Level, msg string) bool {
	if len(l.hfPatterns) == 0 {
		return false
	}

	serviceName := l.service
	for _, pattern := range l.hfPatterns {
		if pattern.Pattern.MatchString(serviceName) {
			now := time.Now().UnixMilli()
			key := pattern.Pattern.String()

			l.mu.Lock()
			lastTime, exists := l.hfLastTime[key]
			if !exists || (now-lastTime) >= pattern.SuppressMs {
				l.hfLastTime[key] = now
				count := l.hfCount[key]
				l.hfCount[key] = 0
				l.mu.Unlock()
				if count > 0 {
					suppressed := NewLoggerDirect(l, level)
					suppressed.log(level, fmt.Sprintf("(%d suppressed)", count))
				}
				return false
			}
			l.hfCount[key]++
			l.mu.Unlock()
			return true
		}
	}
	return false
}

func NewLoggerDirect(source *Logger, overrideLevel Level) *Logger {
	return &Logger{
		out:        source.out,
		service:    source.service,
		colorize:   source.colorize,
		minLevel:   overrideLevel,
		hfPatterns: nil,
		hfLastTime: source.hfLastTime,
		hfCount:    source.hfCount,
		services:   make(map[string]*Logger),
	}
}

func colorizeLine(line string, level Level) string {
	c := levelColor(level)
	return fmt.Sprintf("%s%s%s", c.format(), line, colorReset.format())
}

type ActivityLogger struct {
	logger      *Logger
	hfPatterns  []HFPattern
	ignored    map[string]bool
	maxTimeMs  int64
}

func NewActivityLogger(logger *Logger) *ActivityLogger {
	return &ActivityLogger{
		logger:     logger,
		hfPatterns: nil,
		ignored:    make(map[string]bool),
		maxTimeMs:  300,
	}
}

func (a *ActivityLogger) RegisterHFPattern(pattern *regexp.Regexp, suppressMs int64) {
	a.hfPatterns = append(a.hfPatterns, HFPattern{
		Pattern:    pattern,
		SuppressMs: suppressMs,
	})
}

func (a *ActivityLogger) IgnoreCommand(cmd string) {
	a.ignored[cmd] = true
}

func (a *ActivityLogger) Before(command string, args []interface{}) {
	if a.ignored[command] {
		return
	}
	svcLog := a.logger.Service(command)
	argsStr := formatArgs(args)
	svcLog.Info(fmt.Sprintf("%s running...", argsStr))
}

func (a *ActivityLogger) After(command string, args []interface{}, err error, result interface{}, elapsed time.Duration) {
	forceLog := err != nil || elapsed.Milliseconds() > a.maxTimeMs
	if a.ignored[command] && !forceLog {
		return
	}

	svcLog := a.logger.Service(command)
	argsStr := formatArgs(args)

	if err != nil {
		svcLog.Errorf("%s %s FAILED: %v", argsStr, elapsed.Truncate(time.Millisecond), err)
	} else {
		svcLog.Infof("%s %s", argsStr, elapsed.Truncate(time.Millisecond))
	}
}

func formatArgs(args []interface{}) string {
	parts := make([]string, len(args))
	for i, arg := range args {
		parts[i] = fmt.Sprintf("%v", arg)
	}
	return fmt.Sprintf("[%s]", joinParts(parts, " "))
}

func joinParts(parts []string, sep string) string {
	result := ""
	for i, p := range parts {
		if i > 0 {
			result += sep
		}
		result += p
	}
	return result
}