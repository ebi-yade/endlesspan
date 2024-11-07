package endlesspan

import (
	"log/slog"
	"os"
)

func init() {
	if os.Getenv("ENDLESSPAN_DEBUG") == "true" {
		slog.SetLogLoggerLevel(slog.LevelDebug)
	}
}
