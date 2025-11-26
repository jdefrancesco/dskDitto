package config

import "ditto/internal/dfs"

type Config struct {
	// Skip over empty files.
	SkipEmpty bool
	// Ignore Symbolic Links.
	SkipSymLinks bool
	// Ignore Hard Links.
	SkipHardLinks bool
	// SkipHidden controls whether hidden dotfiles and directories are skipped.
	SkipHidden bool
	// File size limits.
	MinFileSize uint
	MaxFileSize uint
	// HashAlgorithm selects which digest is used when hashing file contents.
	HashAlgorithm dfs.HashAlgorithm
}
