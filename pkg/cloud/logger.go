package cloud

import (
	"fmt"
	"io"
	"log"
	"os"
	"strings"
)

var (
	InfoLogger    *log.Logger
	WarningLogger *log.Logger
	ErrorLogger   *log.Logger
	DebugLogger   *log.Logger
)

// InitLogger initializes the logging system
func InitLogger(config LogConfig) error {
	var writers []io.Writer
	// Logs go to stderr so stdout carries only results (pipeable JSON/CSV).
	writers = append(writers, os.Stderr)

	if config.FilePath != "" {
		file, err := os.OpenFile(config.FilePath, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0644)
		if err != nil {
			return fmt.Errorf("failed to open log file: %v", err)
		}
		writers = append(writers, file)
	}

	writer := io.MultiWriter(writers...)

	InfoLogger = log.New(writer, "INFO: ", log.Ldate|log.Ltime)
	WarningLogger = log.New(writer, "WARNING: ", log.Ldate|log.Ltime)
	ErrorLogger = log.New(writer, "ERROR: ", log.Ldate|log.Ltime)
	DebugLogger = log.New(writer, "DEBUG: ", log.Ldate|log.Ltime)

	// Set log level
	switch strings.ToUpper(config.Level) {
	case "DEBUG":
		// All loggers enabled
	case "INFO":
		DebugLogger.SetOutput(io.Discard)
	case "WARNING":
		DebugLogger.SetOutput(io.Discard)
		InfoLogger.SetOutput(io.Discard)
	case "ERROR":
		DebugLogger.SetOutput(io.Discard)
		InfoLogger.SetOutput(io.Discard)
		WarningLogger.SetOutput(io.Discard)
	default:
		return fmt.Errorf("invalid log level: %s", config.Level)
	}

	return nil
} 