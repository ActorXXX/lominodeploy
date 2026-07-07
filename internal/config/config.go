package config

import (
	"os"
	"path/filepath"
	"time"

	"github.com/BurntSushi/toml"
)

const defaultConfigPath = "/etc/lominodeploy/config.toml"

type Config struct {
	Server   ServerConfig    `toml:"server"`
	Products []InstalledProduct `toml:"products"`
}

type ServerConfig struct {
	Port      int    `toml:"port"`
	SetupDone bool   `toml:"setup_done"`
}

type InstalledProduct struct {
	ID          string    `toml:"id"`
	Name        string    `toml:"name"`
	Version     string    `toml:"version"`
	BasePath    string    `toml:"base_path"`
	Domain      string    `toml:"domain"`
	Port        int       `toml:"port"`
	SSLMode     string    `toml:"ssl_mode"`   // "none", "letsencrypt", "custom"
	SSLEnabled  bool      `toml:"ssl_enabled"`
	LicenseKey  string    `toml:"license_key"`
	Image       string    `toml:"image"`
	InstalledAt time.Time `toml:"installed_at"`
}

var configPath string
var current *Config

func Load() (*Config, error) {
	configPath = envOr("LOMINODEPLOY_CONFIG", defaultConfigPath)

	cfg := &Config{
		Server: ServerConfig{Port: 8888},
	}

	if _, err := os.Stat(configPath); os.IsNotExist(err) {
		current = cfg
		return cfg, nil
	}

	if _, err := toml.DecodeFile(configPath, cfg); err != nil {
		return nil, err
	}

	current = cfg
	return cfg, nil
}

func Save(cfg *Config) error {
	dir := filepath.Dir(configPath)
	if err := os.MkdirAll(dir, 0755); err != nil {
		return err
	}

	f, err := os.Create(configPath)
	if err != nil {
		return err
	}
	defer f.Close()

	current = cfg
	return toml.NewEncoder(f).Encode(cfg)
}

func (c *Config) FindProduct(id string) *InstalledProduct {
	for i := range c.Products {
		if c.Products[i].ID == id {
			return &c.Products[i]
		}
	}
	return nil
}

func (c *Config) AddProduct(p InstalledProduct) {
	for i := range c.Products {
		if c.Products[i].ID == p.ID {
			c.Products[i] = p
			return
		}
	}
	c.Products = append(c.Products, p)
}

func (c *Config) RemoveProduct(id string) {
	filtered := c.Products[:0]
	for _, p := range c.Products {
		if p.ID != id {
			filtered = append(filtered, p)
		}
	}
	c.Products = filtered
}

func envOr(key, def string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return def
}
