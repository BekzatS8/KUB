package app

// BuildCommit and BuildTime are populated via -ldflags at build time.
var (
	BuildCommit = "dev"
	BuildTime   = "unknown"
)
