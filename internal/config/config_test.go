package config

import (
	"os"
	"path/filepath"
	"testing"
)

// TestGetEnv_OSVarSet verifies GetEnv returns the OS env value when the key is set.
func TestGetEnv_OSVarSet(t *testing.T) {
	t.Setenv("TEST_GETENV_OS_VAR", "from_os")
	got := GetEnv("TEST_GETENV_OS_VAR", "fallback")
	if got != "from_os" {
		t.Errorf("GetEnv() = %q, want %q", got, "from_os")
	}
}

// TestGetEnv_MissingKeyFallback verifies GetEnv returns the fallback when
// the key is absent from both OS env and the .env file.
func TestGetEnv_MissingKeyFallback(t *testing.T) {
	got := GetEnv("TOTALLY_NONEXISTENT_KEY_12345", "my_fallback")
	if got != "my_fallback" {
		t.Errorf("GetEnv() = %q, want %q", got, "my_fallback")
	}
}

// TestGetEnv_DotEnvFile verifies GetEnv reads from a .env file when
// the key is not in the OS environment.
func TestGetEnv_DotEnvFile(t *testing.T) {
	dir := t.TempDir()
	envContent := "MY_DOTENV_KEY=from_dotenv\nOTHER=ignored\n"
	if err := os.WriteFile(filepath.Join(dir, ".env"), []byte(envContent), 0644); err != nil { //nolint:gosec // test file
		t.Fatalf("write .env: %v", err)
	}

	t.Run("reads_from_dotenv", func(t *testing.T) {
		orig, _ := os.Getwd()
		if err := os.Chdir(dir); err != nil {
			t.Fatalf("chdir: %v", err)
		}
		t.Cleanup(func() { _ = os.Chdir(orig) })

		got := GetEnv("MY_DOTENV_KEY", "fallback")
		if got != "from_dotenv" {
			t.Errorf("GetEnv() = %q, want %q", got, "from_dotenv")
		}
	})
}

// TestGetEnvInt_TableDriven covers valid int, invalid string, and empty string
// cases for GetEnvInt using a table-driven approach.
func TestGetEnvInt_TableDriven(t *testing.T) {
	tests := []struct {
		name     string
		envVal   string
		setEnv   bool
		fallback int
		want     int
	}{
		{
			name:     "valid_integer_returns_parsed_value",
			envVal:   "42",
			setEnv:   true,
			fallback: 0,
			want:     42,
		},
		{
			name:     "invalid_string_returns_fallback",
			envVal:   "not_a_number",
			setEnv:   true,
			fallback: 99,
			want:     99,
		},
		{
			name:     "empty_string_returns_fallback",
			envVal:   "",
			setEnv:   false,
			fallback: 7,
			want:     7,
		},
		{
			name:     "negative_integer_parses_correctly",
			envVal:   "-5",
			setEnv:   true,
			fallback: 0,
			want:     -5,
		},
		{
			name:     "zero_string_returns_zero",
			envVal:   "0",
			setEnv:   true,
			fallback: 10,
			want:     0,
		},
	}

	const envKey = "TEST_GETENVINT_KEY"

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if tt.setEnv {
				t.Setenv(envKey, tt.envVal)
			} else {
				_ = os.Unsetenv(envKey)
			}
			got := GetEnvInt(envKey, tt.fallback)
			if got != tt.want {
				t.Errorf("GetEnvInt(%q, %d) = %d, want %d",
					envKey, tt.fallback, got, tt.want)
			}
		})
	}
}

// TestGetTLSConfig_ServerName verifies the returned tls.Config has the
// correct ServerName field.
func TestGetTLSConfig_ServerName(t *testing.T) {
	cfg := GetTLSConfig("example.com")
	if cfg.ServerName != "example.com" {
		t.Errorf("ServerName = %q, want %q", cfg.ServerName, "example.com")
	}
}

// TestGetTLSConfig_InsecureSkipVerify verifies InsecureSkipVerify is false,
// ensuring TLS verification is enabled.
func TestGetTLSConfig_InsecureSkipVerify(t *testing.T) {
	cfg := GetTLSConfig("any.host")
	if cfg.InsecureSkipVerify {
		t.Error("InsecureSkipVerify = true, want false")
	}
}
