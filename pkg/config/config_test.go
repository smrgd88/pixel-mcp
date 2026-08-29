package config

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestResolvePathPrecedence(t *testing.T) {
	homeDir := t.TempDir()
	t.Setenv("HOME", homeDir)
	t.Setenv("USERPROFILE", homeDir)

	tests := []struct {
		name     string
		explicit string
		env      string
		want     string
	}{
		{name: "explicit path overrides environment", explicit: "/cli/config.json", env: "/env/config.json", want: "/cli/config.json"},
		{name: "environment overrides default", env: "/env/config.json", want: "/env/config.json"},
		{name: "default preserves legacy location", want: filepath.Join(homeDir, ".config", "pixel-mcp", "config.json")},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Setenv(ConfigPathEnv, tt.env)
			got, err := ResolvePath(tt.explicit)
			if err != nil {
				t.Fatalf("ResolvePath() error = %v", err)
			}
			if got != tt.want {
				t.Errorf("ResolvePath() = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestLoadUsesExplicitPathBeforeEnvironment(t *testing.T) {
	executable := createTestExecutable(t)
	explicitPath := writeConfig(t, executable, t.TempDir(), "debug")
	t.Setenv(ConfigPathEnv, filepath.Join(t.TempDir(), "missing.json"))

	cfg, err := LoadFromPath(explicitPath)
	if err != nil {
		t.Fatalf("Load() error = %v", err)
	}
	if cfg.LogLevel != "debug" {
		t.Errorf("LogLevel = %q, want debug", cfg.LogLevel)
	}
}

func TestLoadUsesEnvironmentPath(t *testing.T) {
	executable := createTestExecutable(t)
	configPath := writeConfig(t, executable, t.TempDir(), "warn")
	t.Setenv(ConfigPathEnv, configPath)

	cfg, err := Load()
	if err != nil {
		t.Fatalf("Load() error = %v", err)
	}
	if cfg.LogLevel != "warn" {
		t.Errorf("LogLevel = %q, want warn", cfg.LogLevel)
	}
}

func TestLoadUsesLegacyDefaultPath(t *testing.T) {
	homeDir := t.TempDir()
	t.Setenv("HOME", homeDir)
	t.Setenv("USERPROFILE", homeDir)
	t.Setenv(ConfigPathEnv, "")

	executable := createTestExecutable(t)
	configPath := filepath.Join(homeDir, ".config", "pixel-mcp", "config.json")
	if err := os.MkdirAll(filepath.Dir(configPath), 0o755); err != nil {
		t.Fatal(err)
	}
	writeJSON(t, configPath, map[string]any{
		"aseprite_path": executable,
		"temp_dir":      t.TempDir(),
		"timeout":       30,
		"log_level":     "info",
	})

	if _, err := Load(); err != nil {
		t.Fatalf("Load() with legacy default path error = %v", err)
	}
}

func TestConfigValidate(t *testing.T) {
	executable := createTestExecutable(t)
	tempDir := t.TempDir()

	tests := []struct {
		name    string
		config  *Config
		wantErr bool
	}{
		{name: "valid config", config: &Config{AsepritePath: executable, TempDir: tempDir, Timeout: 30 * time.Second, LogLevel: "info"}},
		{name: "missing aseprite executable", config: &Config{AsepritePath: filepath.Join(t.TempDir(), "missing"), TempDir: tempDir, Timeout: 30 * time.Second, LogLevel: "info"}, wantErr: true},
		{name: "invalid timeout", config: &Config{AsepritePath: executable, TempDir: tempDir, Timeout: -time.Second, LogLevel: "info"}, wantErr: true},
		{name: "invalid log level", config: &Config{AsepritePath: executable, TempDir: tempDir, Timeout: 30 * time.Second, LogLevel: "verbose"}, wantErr: true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := tt.config.Validate()
			if (err != nil) != tt.wantErr {
				t.Errorf("Validate() error = %v, wantErr %v", err, tt.wantErr)
			}
		})
	}
}

func TestValidateUnwritableTempDir(t *testing.T) {
	if os.Getuid() == 0 {
		t.Skip("permission checks are not reliable as root")
	}

	readOnlyDir := filepath.Join(t.TempDir(), "readonly")
	if err := os.Mkdir(readOnlyDir, 0o444); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.Chmod(readOnlyDir, 0o755) })

	cfg := &Config{
		AsepritePath: createTestExecutable(t),
		TempDir:      readOnlyDir,
		Timeout:      30 * time.Second,
		LogLevel:     "info",
	}
	err := cfg.Validate()
	if err == nil {
		t.Skip("filesystem does not enforce the test directory permissions")
	}
	if !strings.Contains(err.Error(), "not writable") {
		t.Errorf("Validate() error = %v, want not writable", err)
	}
}

func TestValidateZeroTimeout(t *testing.T) {
	cfg := &Config{
		AsepritePath: createTestExecutable(t),
		TempDir:      t.TempDir(),
		Timeout:      0,
		LogLevel:     "info",
	}

	err := cfg.Validate()
	if err == nil || !strings.Contains(err.Error(), "timeout must be positive") {
		t.Errorf("Validate() error = %v, want timeout must be positive", err)
	}
}

func TestConfigSetDefaults(t *testing.T) {
	executable := createTestExecutable(t)
	t.Setenv("TMPDIR", t.TempDir())

	t.Run("sets defaults for empty optional fields", func(t *testing.T) {
		cfg := &Config{AsepritePath: executable}
		if err := cfg.setDefaults(); err != nil {
			t.Fatalf("setDefaults() error = %v", err)
		}
		if cfg.TempDir == "" {
			t.Error("TempDir was not set")
		}
		if cfg.Timeout != DefaultTimeout {
			t.Errorf("Timeout = %v, want %v", cfg.Timeout, DefaultTimeout)
		}
		if cfg.LogLevel != DefaultLogLevel {
			t.Errorf("LogLevel = %q, want %q", cfg.LogLevel, DefaultLogLevel)
		}
	})

	t.Run("requires aseprite path", func(t *testing.T) {
		if err := (&Config{}).setDefaults(); err == nil {
			t.Error("setDefaults() expected error for missing aseprite_path")
		}
	})
}

func TestConfigLoadFromFile(t *testing.T) {
	configPath := writeConfig(t, `D:\SRC\aseprite\aseprite.exe`, t.TempDir(), "debug")
	cfg := &Config{}
	if err := cfg.loadFromFile(configPath); err != nil {
		t.Fatalf("loadFromFile() error = %v", err)
	}
	if cfg.AsepritePath != `D:\SRC\aseprite\aseprite.exe` {
		t.Errorf("AsepritePath = %q", cfg.AsepritePath)
	}
	if cfg.LogLevel != "debug" {
		t.Errorf("LogLevel = %q, want debug", cfg.LogLevel)
	}
}

func TestLoadErrors(t *testing.T) {
	executable := createTestExecutable(t)
	tests := []struct {
		name        string
		write       func(t *testing.T) string
		wantInError string
	}{
		{name: "missing file", write: func(t *testing.T) string { return filepath.Join(t.TempDir(), "missing.json") }, wantInError: "config file not found"},
		{name: "malformed JSON", write: func(t *testing.T) string {
			path := filepath.Join(t.TempDir(), "config.json")
			if err := os.WriteFile(path, []byte("{invalid json"), 0o644); err != nil {
				t.Fatal(err)
			}
			return path
		}, wantInError: "failed to load config file"},
		{name: "missing aseprite path", write: func(t *testing.T) string { return writeConfig(t, "", t.TempDir(), "info") }, wantInError: "aseprite_path must be explicitly configured"},
		{name: "invalid aseprite path", write: func(t *testing.T) string {
			return writeConfig(t, filepath.Join(t.TempDir(), "missing"), t.TempDir(), "info")
		}, wantInError: "invalid configuration"},
		{name: "zero timeout receives default", write: func(t *testing.T) string {
			path := filepath.Join(t.TempDir(), "config.json")
			writeJSON(t, path, map[string]any{"aseprite_path": executable, "temp_dir": t.TempDir(), "timeout": 0, "log_level": "info"})
			return path
		}},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := LoadFromPath(tt.write(t))
			if tt.wantInError == "" {
				if err != nil {
					t.Fatalf("Load() error = %v", err)
				}
				return
			}
			if err == nil || !strings.Contains(err.Error(), tt.wantInError) {
				t.Errorf("Load() error = %v, want substring %q", err, tt.wantInError)
			}
		})
	}
}

func createTestExecutable(t *testing.T) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "aseprite")
	if err := os.WriteFile(path, []byte("test executable"), 0o755); err != nil {
		t.Fatal(err)
	}
	return path
}

func writeConfig(t *testing.T, executable, tempDir, logLevel string) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "config.json")
	writeJSON(t, path, map[string]any{"aseprite_path": executable, "temp_dir": tempDir, "timeout": 30, "log_level": logLevel})
	return path
}

func writeJSON(t *testing.T, path string, value any) {
	t.Helper()
	data, err := json.Marshal(value)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, data, 0o644); err != nil {
		t.Fatal(err)
	}
}
