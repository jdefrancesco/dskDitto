package config

type Config struct {
	// Skip over empty files.
	SkipEmpty bool
	// Ignore Symbolic Links.
	SkipSymLinks bool
	// Ignore Hard Links.
	SkipHardLinks bool
	// File size limits.
	MinFileSize uint
	MaxFileSize uint
}
