package logger

import (
	"io"
	"os"
	"time"

	"github.com/rs/zerolog"
)

func NewJSONLogger(w io.Writer) zerolog.Logger {
	if w == nil {
		w = os.Stdout
	}
	return zerolog.New(w).With().Timestamp().Logger()
}

func NewDevLogger() zerolog.Logger {
	console := zerolog.ConsoleWriter{Out: os.Stdout, TimeFormat: time.RFC3339}
	return zerolog.New(console).With().Timestamp().Logger()
}
