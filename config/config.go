package config

import (
	"fmt"
	"net/url"
	"os"
	"strconv"
	"time"

	"gopkg.in/yaml.v3"
)

const defaultPath = "config/local.yaml"

type Config struct {
	Server  ServerConfig  `yaml:"server"`
	DB      DBConfig      `yaml:"db"`
	Booking BookingConfig `yaml:"booking"`
}

type BookingConfig struct {
	// HoldMinutes is how long a slot stays on hold before it reads as free again.
	HoldMinutes int `yaml:"holdMinutes"`
}

func (b BookingConfig) HoldDuration() time.Duration {
	return time.Duration(b.HoldMinutes) * time.Minute
}

type ServerConfig struct {
	Port int `yaml:"port"`
}

type DBConfig struct {
	Host     string `yaml:"host"`
	Port     int    `yaml:"port"`
	User     string `yaml:"user"`
	Password string `yaml:"password"`
	Name     string `yaml:"name"`
	SSLMode  string `yaml:"sslmode"`
}

// DSN returns a postgres:// URL. A URL is used rather than key=value form so an
// empty password doesn't swallow the next parameter.
func (d DBConfig) DSN() string {
	u := url.URL{
		Scheme:   "postgres",
		Host:     fmt.Sprintf("%s:%d", d.Host, d.Port),
		Path:     "/" + d.Name,
		RawQuery: url.Values{"sslmode": {d.SSLMode}}.Encode(),
	}
	if d.Password == "" {
		u.User = url.User(d.User)
	} else {
		u.User = url.UserPassword(d.User, d.Password)
	}
	return u.String()
}

// Load reads the YAML file at CONFIG_PATH (default config/local.yaml).
// PORT overrides server.port, and DATABASE_URL, if set, is used instead of the db section.
func Load() (Config, error) {
	path := os.Getenv("CONFIG_PATH")
	if path == "" {
		path = defaultPath
	}
	data, err := os.ReadFile(path)
	if err != nil {
		return Config{}, fmt.Errorf("read config %s: %w", path, err)
	}
	var cfg Config
	if err := yaml.Unmarshal(data, &cfg); err != nil {
		return Config{}, fmt.Errorf("parse config %s: %w", path, err)
	}
	if cfg.Booking.HoldMinutes <= 0 {
		return Config{}, fmt.Errorf("config %s: booking.holdMinutes must be a positive number of minutes", path)
	}
	if p := os.Getenv("PORT"); p != "" {
		port, err := strconv.Atoi(p)
		if err != nil {
			return Config{}, fmt.Errorf("invalid PORT %q", p)
		}
		cfg.Server.Port = port
	}
	return cfg, nil
}

func (c Config) DatabaseURL() string {
	if dsn := os.Getenv("DATABASE_URL"); dsn != "" {
		return dsn
	}
	return c.DB.DSN()
}
