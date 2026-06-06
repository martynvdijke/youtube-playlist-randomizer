package logging

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"strings"
	"sync"
	"time"

	"github.com/martynvdijke/youtube-playlist-randomizer/internal/store"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/trace"
)

// Severity levels in ascending order.
type Severity int

const (
	DEBUG Severity = iota
	INFO
	WARN
	ERROR
)

func (s Severity) String() string {
	switch s {
	case DEBUG:
		return "DEBUG"
	case INFO:
		return "INFO"
	case WARN:
		return "WARN"
	case ERROR:
		return "ERROR"
	default:
		return "INFO"
	}
}

func ParseSeverity(s string) Severity {
	switch strings.ToUpper(s) {
	case "DEBUG":
		return DEBUG
	case "INFO":
		return INFO
	case "WARN", "WARNING":
		return WARN
	case "ERROR":
		return ERROR
	default:
		return INFO
	}
}

// Logger provides structured logging with severity filtering, DB storage, and
// OpenTelemetry span event export.
type Logger struct {
	mu        sync.RWMutex
	store     *store.Store
	minLevel  Severity
	tracer    trace.Tracer
	service   string
}

type LogOptions struct {
	Store    *store.Store
	MinLevel Severity
	Service  string
}

func New(opts LogOptions) *Logger {
	tracer := otel.Tracer("logs")

	return &Logger{
		store:    opts.Store,
		minLevel: opts.MinLevel,
		tracer:   tracer,
		service:  opts.Service,
	}
}

// SetMinLevel updates the minimum severity level at runtime.
func (l *Logger) SetMinLevel(level Severity) {
	l.mu.Lock()
	defer l.mu.Unlock()
	l.minLevel = level
}

// MinLevel returns the current minimum severity level.
func (l *Logger) MinLevel() Severity {
	l.mu.RLock()
	defer l.mu.RUnlock()
	return l.minLevel
}

func (l *Logger) Debug(msg string, attrs ...string) {
	l.log(context.Background(), DEBUG, msg, attrs...)
}

func (l *Logger) Info(msg string, attrs ...string) {
	l.log(context.Background(), INFO, msg, attrs...)
}

func (l *Logger) Warn(msg string, attrs ...string) {
	l.log(context.Background(), WARN, msg, attrs...)
}

func (l *Logger) Error(msg string, attrs ...string) {
	l.log(context.Background(), ERROR, msg, attrs...)
}

// Debugc logs with a context (for OTEL span correlation).
func (l *Logger) Debugc(ctx context.Context, msg string, attrs ...string) {
	l.log(ctx, DEBUG, msg, attrs...)
}

// Infoc logs with a context.
func (l *Logger) Infoc(ctx context.Context, msg string, attrs ...string) {
	l.log(ctx, INFO, msg, attrs...)
}

// Warnc logs with a context.
func (l *Logger) Warnc(ctx context.Context, msg string, attrs ...string) {
	l.log(ctx, WARN, msg, attrs...)
}

// Errorc logs with a context.
func (l *Logger) Errorc(ctx context.Context, msg string, attrs ...string) {
	l.log(ctx, ERROR, msg, attrs...)
}

func (l *Logger) log(ctx context.Context, sev Severity, msg string, attrs ...string) {
	l.mu.RLock()
	minLevel := l.minLevel
	l.mu.RUnlock()

	if sev < minLevel {
		return // filtered out
	}

	now := time.Now().UTC()
	ts := now.Format(time.RFC3339Nano)
	source := l.service

	// Build attributes map
	attrMap := make(map[string]string, len(attrs)/2)
	for i := 0; i < len(attrs)-1; i += 2 {
		attrMap[attrs[i]] = attrs[i+1]
	}
	// If odd number of attrs, last one becomes a value with empty key
	if len(attrs)%2 == 1 {
		attrMap["_value"] = attrs[len(attrs)-1]
	}

	attrJSON, _ := json.Marshal(attrMap)

	// Write to DB
	if l.store != nil {
		entry := store.LogEntry{
			Timestamp:  ts,
			Severity:   sev.String(),
			Source:     source,
			Message:    msg,
			Attributes: string(attrJSON),
		}
		if err := l.store.InsertLog(entry); err != nil {
			log.Printf("logging: failed to insert log entry: %v", err)
		}
	}

	// Export to OTEL as span event
	if l.tracer != nil {
		otelAttrs := make([]attribute.KeyValue, 0, len(attrMap)+2)
		otelAttrs = append(otelAttrs,
			attribute.String("log.severity", sev.String()),
			attribute.String("log.message", msg),
			attribute.String("log.source", source),
		)
		for k, v := range attrMap {
			otelAttrs = append(otelAttrs, attribute.String("log."+k, v))
		}

		// Start a new span for the log event, or add as event to existing span
		_, span := l.tracer.Start(ctx, fmt.Sprintf("log.%s", sev.String()),
			trace.WithAttributes(otelAttrs...),
		)
		span.AddEvent("log", trace.WithAttributes(otelAttrs...))
		span.End()
	}
}
