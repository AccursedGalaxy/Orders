package logger

import (
	"os"
	"path/filepath"
	"sync"

	"go.uber.org/zap"
	"go.uber.org/zap/zapcore"
	"gopkg.in/natefinch/lumberjack.v2"
)

var (
	log *zap.Logger
	once sync.Once
)

type Config struct {
	LogLevel    string
	ConsoleLog  bool
	FileLog     bool
	FileLogPath string
	MaxSize     int  // megabytes
	MaxBackups  int  // number of backups
	MaxAge      int  // days
	Compress    bool // compress old files
}

func InitLogger(cfg Config) *zap.Logger {
	once.Do(func() {
		// Create encoder config
		encoderConfig := zapcore.EncoderConfig{
			TimeKey:        "ts",
			LevelKey:      "level",
			NameKey:       "logger",
			CallerKey:     "caller",
			MessageKey:    "msg",
			StacktraceKey: "stacktrace",
			LineEnding:    zapcore.DefaultLineEnding,
			EncodeLevel:   zapcore.CapitalColorLevelEncoder,
			EncodeTime:    zapcore.ISO8601TimeEncoder,
			EncodeDuration: zapcore.SecondsDurationEncoder,
			EncodeCaller:   zapcore.ShortCallerEncoder,
		}

		// Define log level
		level := getLogLevel(cfg.LogLevel)

		// Create cores
		var cores []zapcore.Core

		// Console logging
		if cfg.ConsoleLog {
			consoleEncoder := zapcore.NewConsoleEncoder(encoderConfig)
			consoleCore := zapcore.NewCore(
				consoleEncoder,
				zapcore.AddSync(os.Stdout),
				level,
			)
			cores = append(cores, consoleCore)
		}

		// File logging
		if cfg.FileLog {
			// Ensure directory exists
			if err := os.MkdirAll(filepath.Dir(cfg.FileLogPath), 0755); err != nil {
				panic(err)
			}

			// Setup log rotation
			fileWriter := &lumberjack.Logger{
				Filename:   cfg.FileLogPath,
				MaxSize:    cfg.MaxSize,    // megabytes
				MaxBackups: cfg.MaxBackups, // number of backups
				MaxAge:     cfg.MaxAge,     // days
				Compress:   cfg.Compress,   // compress old files
			}

			fileEncoder := zapcore.NewJSONEncoder(encoderConfig)
			fileCore := zapcore.NewCore(
				fileEncoder,
				zapcore.AddSync(fileWriter),
				level,
			)
			cores = append(cores, fileCore)
		}

		// Create logger
		log = zap.New(
			zapcore.NewTee(cores...),
			zap.AddCaller(),
			zap.AddStacktrace(zapcore.ErrorLevel),
		)
	})

	return log
}

func getLogLevel(level string) zapcore.Level {
	switch level {
	case "debug":
		return zapcore.DebugLevel
	case "info":
		return zapcore.InfoLevel
	case "warn":
		return zapcore.WarnLevel
	case "error":
		return zapcore.ErrorLevel
	default:
		return zapcore.InfoLevel
	}
}

// Get returns the initialized logger
func Get() *zap.Logger {
	if log == nil {
		panic("Logger not initialized")
	}
	return log
} 