// Package config loads TMDB credentials from the environment, falling back
// to ~/.config/renamer/config.toml.
package config

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/BurntSushi/toml"
)

// Config holds runtime settings.
type Config struct {
	Token  string `toml:"tmdb_token"`   // v4 read access token
	APIKey string `toml:"tmdb_api_key"` // v3 API key
}

// Path returns the config file location.
func Path() string {
	d, err := os.UserConfigDir()
	if err != nil {
		d = filepath.Join(os.Getenv("HOME"), ".config")
	}
	return filepath.Join(d, "renamer", "config.toml")
}

// Load reads env vars first (TMDB_TOKEN, TMDB_API_KEY), then the config file.
func Load() (Config, error) {
	c := Config{
		Token:  strings.TrimSpace(os.Getenv("TMDB_TOKEN")),
		APIKey: strings.TrimSpace(os.Getenv("TMDB_API_KEY")),
	}
	if c.Token != "" || c.APIKey != "" {
		return c, nil
	}
	p := Path()
	if _, err := toml.DecodeFile(p, &c); err != nil && !errors.Is(err, os.ErrNotExist) {
		return c, fmt.Errorf("reading %s: %w", p, err)
	}
	if c.Token == "" && c.APIKey == "" {
		return c, fmt.Errorf("no TMDB credentials: set TMDB_TOKEN (v4 read access token) or TMDB_API_KEY, or put tmdb_token = \"...\" in %s", p)
	}
	return c, nil
}
