// Package env provides utilities for loading environment variables from .env files.
// It uses the functional options pattern for flexible configuration.
//
// Usage:
//
//	env.Load()                                    // Load .env from current directory
//	env.Load(env.WithFile(".env.local"))          // Load specific file
//	env.Load(env.WithOverride())                  // Override existing env vars
//	env.Load(env.WithDir("/path/to/dir"))         // Load from directory
//	env.Load(env.WithFile(".env"), env.WithOverride())  // Combine options
package env

import (
	"bufio"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

// Options holds configuration for loading environment variables.
type Options struct {
	filename string
	dir      string
	override bool
	required bool
}

// Option is a functional option for configuring the loader.
type Option func(*Options)

// WithFile specifies the filename to load (default: ".env").
func WithFile(filename string) Option {
	return func(o *Options) {
		o.filename = filename
	}
}

// WithDir specifies the directory to load .env from.
func WithDir(dir string) Option {
	return func(o *Options) {
		o.dir = dir
	}
}

// WithOverride enables overriding existing environment variables.
func WithOverride() Option {
	return func(o *Options) {
		o.override = true
	}
}

// WithRequired makes it an error if the file doesn't exist.
func WithRequired() Option {
	return func(o *Options) {
		o.required = true
	}
}

// Load loads environment variables from a .env file.
// By default, it loads ".env" from the current directory and does not override existing variables.
func Load(opts ...Option) error {
	options := &Options{
		filename: ".env",
		override: false,
		required: false,
	}

	for _, opt := range opts {
		opt(options)
	}

	// Determine the full path
	path := options.filename
	if options.dir != "" {
		path = filepath.Join(options.dir, options.filename)
	}

	return loadFile(path, options.override, options.required)
}

// MustLoad is like Load but panics on error.
func MustLoad(opts ...Option) {
	if err := Load(opts...); err != nil {
		panic(fmt.Sprintf("env: failed to load: %v", err))
	}
}

// Get returns the value of an environment variable, or empty string if not set.
func Get(key string) string {
	return os.Getenv(key)
}

// GetDefault returns the value of an environment variable, or the default value if not set.
func GetDefault(key, defaultValue string) string {
	if value := os.Getenv(key); value != "" {
		return value
	}
	return defaultValue
}

// GetRequired returns the value of an environment variable, or an error if not set.
func GetRequired(key string) (string, error) {
	value := os.Getenv(key)
	if value == "" {
		return "", fmt.Errorf("env: required variable %q is not set", key)
	}
	return value, nil
}

// MustGet returns the value of an environment variable, or panics if not set.
func MustGet(key string) string {
	value, err := GetRequired(key)
	if err != nil {
		panic(err)
	}
	return value
}

// Set sets an environment variable.
func Set(key, value string) error {
	return os.Setenv(key, value)
}

// loadFile reads and parses the env file
func loadFile(filename string, override, required bool) error {
	file, err := os.Open(filename)
	if err != nil {
		if os.IsNotExist(err) {
			if required {
				return fmt.Errorf("env: file %q not found", filename)
			}
			return nil
		}
		return fmt.Errorf("env: failed to open %q: %w", filename, err)
	}
	defer file.Close()

	scanner := bufio.NewScanner(file)
	lineNum := 0

	for scanner.Scan() {
		lineNum++
		line := strings.TrimSpace(scanner.Text())

		// Skip empty lines and comments
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}

		// Parse KEY=VALUE
		key, value, err := parseLine(line)
		if err != nil {
			return fmt.Errorf("env: line %d: %w", lineNum, err)
		}

		// Set environment variable
		if override || os.Getenv(key) == "" {
			if err := os.Setenv(key, value); err != nil {
				return fmt.Errorf("env: failed to set %q: %w", key, err)
			}
		}
	}

	return scanner.Err()
}

// parseLine parses a single line from the env file
func parseLine(line string) (key, value string, err error) {
	// Handle export prefix
	line = strings.TrimPrefix(line, "export ")
	line = strings.TrimSpace(line)

	// Find the first = sign
	idx := strings.Index(line, "=")
	if idx == -1 {
		return "", "", fmt.Errorf("invalid format, expected KEY=VALUE")
	}

	key = strings.TrimSpace(line[:idx])
	value = strings.TrimSpace(line[idx+1:])

	// Validate key
	if key == "" {
		return "", "", fmt.Errorf("empty key")
	}

	// Remove surrounding quotes from value
	value = unquote(value)

	return key, value, nil
}

// unquote removes surrounding quotes and handles escape sequences
func unquote(s string) string {
	if len(s) < 2 {
		return s
	}

	// Check for single or double quotes
	if (s[0] == '"' && s[len(s)-1] == '"') || (s[0] == '\'' && s[len(s)-1] == '\'') {
		s = s[1 : len(s)-1]
	}

	// Handle common escape sequences in double-quoted strings
	s = strings.ReplaceAll(s, `\n`, "\n")
	s = strings.ReplaceAll(s, `\t`, "\t")
	s = strings.ReplaceAll(s, `\\`, "\\")
	s = strings.ReplaceAll(s, `\"`, "\"")

	return s
}
