package config

import (
	"os"
	"path/filepath"
	"strings"

	"github.com/BurntSushi/toml"
)

type Config struct {
	Repo      string   `toml:"repo"`
	Branch    string   `toml:"branch"`
	SkillsDir string   `toml:"skills_dir"`
	Interval  int      `toml:"interval_minutes"`
	Targets   []string `toml:"targets"`
	Ignore    []string `toml:"ignore"`
	MaxFileMB int      `toml:"max_file_mb"`

	dir string `toml:"-"`
}

func DefaultPath() string {
	base, err := os.UserConfigDir()
	if err != nil {
		base = filepath.Join(os.Getenv("HOME"), ".config")
	}
	return filepath.Join(base, "skillsync", "config.toml")
}

func Expand(p string) string {
	if p == "~" || strings.HasPrefix(p, "~/") {
		home, _ := os.UserHomeDir()
		return filepath.Join(home, strings.TrimPrefix(p, "~"))
	}
	return p
}

func Load(path string) (*Config, error) {
	if path == "" {
		path = DefaultPath()
	}
	c := &Config{Branch: "main", Interval: 60, MaxFileMB: 10, dir: filepath.Dir(path)}
	if _, err := toml.DecodeFile(path, c); err != nil {
		return nil, err
	}
	c.SkillsDir = Expand(c.SkillsDir)
	for i := range c.Targets {
		c.Targets[i] = Expand(c.Targets[i])
	}
	if len(c.Ignore) == 0 {
		c.Ignore = []string{"node_modules", "__pycache__", ".DS_Store", ".ruff_cache"}
	}
	return c, nil
}

func (c *Config) Save(path string) error {
	if path == "" {
		path = DefaultPath()
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	f, err := os.Create(path)
	if err != nil {
		return err
	}
	defer f.Close()
	return toml.NewEncoder(f).Encode(c)
}

func StateDir() string {
	base, err := os.UserConfigDir()
	if err != nil {
		base = filepath.Join(os.Getenv("HOME"), ".config")
	}
	d := filepath.Join(base, "skillsync")
	_ = os.MkdirAll(d, 0o755)
	return d
}
