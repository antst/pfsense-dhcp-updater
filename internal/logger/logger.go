// Package logger provides structured logging using stdlib log/slog.
package logger

import (
	"io"
	"log/slog"
	"net"
	"os"
	"time"
)

// AuditEntry represents an audit trail log entry for DHCP operations.
// Per SEC-008: All DNS modifications must be logged with full audit trail.
type AuditEntry struct {
	Timestamp  time.Time
	Operation  string // CREATE, UPDATE, DELETE, NOOP
	MACAddress string
	Hostname   string
	OldIP      net.IP // For updates and deletes
	NewIP      net.IP // For creates and updates
	VLAN       int
	Interface  string
	Result     string // SUCCESS, ERROR
	ErrorMsg   string // If Result=ERROR
	DryRun     bool
}

// LogAudit logs an audit trail entry in a structured format.
// This satisfies SEC-008 requirement for full audit trail of all DNS modifications.
//
// Example output (JSON format):
//
//	{
//	  "time": "2025-10-18T14:30:00Z",
//	  "level": "INFO",
//	  "msg": "AUDIT",
//	  "operation": "CREATE",
//	  "mac": "00:11:22:33:44:55",
//	  "hostname": "web-server-01",
//	  "new_ip": "10.0.10.50",
//	  "vlan": 10,
//	  "interface": "opt7",
//	  "result": "SUCCESS",
//	  "dry_run": false
//	}
func LogAudit(logger *slog.Logger, entry AuditEntry) {
	attrs := []any{
		"operation", entry.Operation,
		"mac", entry.MACAddress,
		"hostname", entry.Hostname,
		"vlan", entry.VLAN,
		"interface", entry.Interface,
		"result", entry.Result,
		"dry_run", entry.DryRun,
	}

	// Add old IP for updates and deletes
	if entry.OldIP != nil {
		attrs = append(attrs, "old_ip", entry.OldIP.String())
	}

	// Add new IP for creates and updates
	if entry.NewIP != nil {
		attrs = append(attrs, "new_ip", entry.NewIP.String())
	}

	// Add error message if present
	if entry.ErrorMsg != "" {
		attrs = append(attrs, "error", entry.ErrorMsg)
	}

	logger.Info("AUDIT", attrs...)
}

// LogLevel represents the logging level.
type LogLevel int

const (
	// LevelError logs only errors (most restrictive)
	LevelError LogLevel = iota
	// LevelWarn logs warnings and errors
	LevelWarn
	// LevelInfo logs info, warnings, and errors (default)
	LevelInfo
	// LevelDebug logs everything (most verbose)
	LevelDebug
)

// New creates a new structured logger with the specified level and format.
// level: 0=ERROR, 1=WARN, 2=INFO, 3=DEBUG
// jsonFormat: true for JSON output, false for human-readable text
// writer: output destination (typically os.Stderr for logs)
func New(level LogLevel, jsonFormat bool, writer io.Writer) *slog.Logger {
	var slogLevel slog.Level
	switch level {
	case LevelError:
		slogLevel = slog.LevelError
	case LevelWarn:
		slogLevel = slog.LevelWarn
	case LevelInfo:
		slogLevel = slog.LevelInfo
	case LevelDebug:
		slogLevel = slog.LevelDebug
	default:
		slogLevel = slog.LevelInfo
	}

	opts := &slog.HandlerOptions{
		Level: slogLevel,
	}

	var handler slog.Handler
	if jsonFormat {
		handler = slog.NewJSONHandler(writer, opts)
	} else {
		handler = slog.NewTextHandler(writer, opts)
	}

	return slog.New(handler)
}

// NewDefault creates a default logger (INFO level, text format, stderr output).
func NewDefault() *slog.Logger {
	return New(LevelInfo, false, os.Stderr)
}

// LevelFromVerbosity converts verbosity count to LogLevel.
// 0 = WARN, 1 = INFO, 2+ = DEBUG
func LevelFromVerbosity(verbosity int) LogLevel {
	switch {
	case verbosity == 0:
		return LevelWarn
	case verbosity == 1:
		return LevelInfo
	case verbosity >= 2:
		return LevelDebug
	default:
		return LevelWarn
	}
}
