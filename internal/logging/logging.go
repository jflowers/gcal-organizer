// Package logging provides a shared charmbracelet/log logger for the application.
package logging

import (
	"os"

	"github.com/charmbracelet/log"
)

// Logger is the shared application logger.
var Logger *log.Logger

func init() {
	Logger = log.NewWithOptions(os.Stderr, log.Options{
		ReportTimestamp: true,
	})
	Logger.SetLevel(log.InfoLevel)
}

// SetVerbose switches the logger to debug level.
// NOTE: Not goroutine-safe. Must only be called during initialization
// (e.g., from cobra.OnInitialize), before any concurrent logging begins.
func SetVerbose(verbose bool) {
	if verbose {
		Logger.SetLevel(log.DebugLevel)
	} else {
		Logger.SetLevel(log.InfoLevel)
	}
}
