package config

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestDSN(t *testing.T) {
	tests := []struct {
		name string
		db   DBConfig
		want string
	}{
		{
			name: "empty password",
			db:   DBConfig{Host: "localhost", Port: 5432, User: "postgres", Name: "slots", SSLMode: "disable"},
			want: "postgres://postgres@localhost:5432/slots?sslmode=disable",
		},
		{
			name: "with password",
			db:   DBConfig{Host: "db", Port: 6543, User: "app", Password: "secret", Name: "slots", SSLMode: "require"},
			want: "postgres://app:secret@db:6543/slots?sslmode=require",
		},
		{
			name: "password with special characters is escaped",
			db:   DBConfig{Host: "localhost", Port: 5432, User: "app", Password: "p@ss/w:rd", Name: "slots", SSLMode: "disable"},
			want: "postgres://app:p%40ss%2Fw%3Ard@localhost:5432/slots?sslmode=disable",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := tt.db.DSN(); got != tt.want {
				t.Errorf("DSN() = %q, want %q", got, tt.want)
			}
		})
	}
}

func writeConfig(t *testing.T, content string) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "config.yaml")
	if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
		t.Fatal(err)
	}
	return path
}

const sampleConfig = `
server:
  port: 9000
db:
  host: localhost
  port: 5432
  user: postgres
  password: ""
  name: slots
  sslmode: disable
booking:
  holdMinutes: 15
`

func TestLoad(t *testing.T) {
	t.Setenv("CONFIG_PATH", writeConfig(t, sampleConfig))
	t.Setenv("PORT", "")
	t.Setenv("DATABASE_URL", "")

	cfg, err := Load()
	if err != nil {
		t.Fatalf("Load() error: %v", err)
	}
	if cfg.Server.Port != 9000 {
		t.Errorf("Server.Port = %d, want 9000", cfg.Server.Port)
	}
	want := DBConfig{Host: "localhost", Port: 5432, User: "postgres", Password: "", Name: "slots", SSLMode: "disable"}
	if cfg.DB != want {
		t.Errorf("DB = %+v, want %+v", cfg.DB, want)
	}
	if got := cfg.DatabaseURL(); got != want.DSN() {
		t.Errorf("DatabaseURL() = %q, want %q", got, want.DSN())
	}
	if cfg.Booking.HoldMinutes != 15 || cfg.Booking.HoldDuration() != 15*time.Minute {
		t.Errorf("hold = %d min / %v, want 15 min", cfg.Booking.HoldMinutes, cfg.Booking.HoldDuration())
	}
}

func TestLoadEnvOverrides(t *testing.T) {
	t.Setenv("CONFIG_PATH", writeConfig(t, sampleConfig))
	t.Setenv("PORT", "7000")
	t.Setenv("DATABASE_URL", "postgres://other@db/x")

	cfg, err := Load()
	if err != nil {
		t.Fatalf("Load() error: %v", err)
	}
	if cfg.Server.Port != 7000 {
		t.Errorf("Server.Port = %d, want 7000 from PORT", cfg.Server.Port)
	}
	if got := cfg.DatabaseURL(); got != "postgres://other@db/x" {
		t.Errorf("DatabaseURL() = %q, want DATABASE_URL value", got)
	}
}

func TestLoadErrors(t *testing.T) {
	t.Run("missing file", func(t *testing.T) {
		t.Setenv("CONFIG_PATH", filepath.Join(t.TempDir(), "nope.yaml"))
		if _, err := Load(); err == nil {
			t.Error("expected error for missing file")
		}
	})
	t.Run("invalid yaml", func(t *testing.T) {
		t.Setenv("CONFIG_PATH", writeConfig(t, "server: [unclosed"))
		if _, err := Load(); err == nil {
			t.Error("expected error for invalid yaml")
		}
	})
	for name, hold := range map[string]string{"missing": "", "zero": "booking:\n  holdMinutes: 0\n", "negative": "booking:\n  holdMinutes: -5\n"} {
		t.Run("holdMinutes "+name, func(t *testing.T) {
			base := sampleConfig[:strings.Index(sampleConfig, "booking:")]
			t.Setenv("CONFIG_PATH", writeConfig(t, base+hold))
			t.Setenv("PORT", "")
			_, err := Load()
			if err == nil || !strings.Contains(err.Error(), "holdMinutes") {
				t.Errorf("err = %v, want holdMinutes error", err)
			}
		})
	}
	t.Run("invalid PORT", func(t *testing.T) {
		t.Setenv("CONFIG_PATH", writeConfig(t, sampleConfig))
		t.Setenv("PORT", "abc")
		if _, err := Load(); err == nil {
			t.Error("expected error for non-numeric PORT")
		}
	})
}

func TestLocalConfigFile(t *testing.T) {
	t.Setenv("CONFIG_PATH", "local.yaml")
	t.Setenv("PORT", "")
	t.Setenv("DATABASE_URL", "")

	cfg, err := Load()
	if err != nil {
		t.Fatalf("config/local.yaml does not load: %v", err)
	}
	if cfg.Booking.HoldMinutes <= 0 {
		t.Errorf("local.yaml booking.holdMinutes = %d, want > 0", cfg.Booking.HoldMinutes)
	}
	if cfg.DB.User != "postgres" || cfg.DB.Name != "slots" || cfg.DB.Password != "" {
		t.Errorf("unexpected local db config: %+v", cfg.DB)
	}
}
