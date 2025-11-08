package main

import (
	"os"
	"testing"
)

func TestGetEnv(t *testing.T) {
	tests := []struct {
		name         string
		key          string
		defaultValue string
		envValue     string
		want         string
	}{
		{
			name:         "returns environment variable when set",
			key:          "TEST_VAR",
			defaultValue: "default",
			envValue:     "custom",
			want:         "custom",
		},
		{
			name:         "returns default when environment variable not set",
			key:          "UNSET_VAR",
			defaultValue: "default",
			envValue:     "",
			want:         "default",
		},
		{
			name:         "handles empty string environment variable",
			key:          "EMPTY_VAR",
			defaultValue: "default",
			envValue:     "",
			want:         "default",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Set or unset environment variable
			if tt.envValue != "" {
				os.Setenv(tt.key, tt.envValue)
				defer os.Unsetenv(tt.key)
			} else {
				os.Unsetenv(tt.key)
			}

			got := getEnv(tt.key, tt.defaultValue)
			if got != tt.want {
				t.Errorf("getEnv() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestGetEnvInt(t *testing.T) {
	tests := []struct {
		name         string
		key          string
		defaultValue int
		envValue     string
		want         int
	}{
		{
			name:         "returns parsed integer when valid",
			key:          "TEST_INT",
			defaultValue: 10,
			envValue:     "42",
			want:         42,
		},
		{
			name:         "returns default when environment variable not set",
			key:          "UNSET_INT",
			defaultValue: 10,
			envValue:     "",
			want:         10,
		},
		{
			name:         "returns default when environment variable is invalid",
			key:          "INVALID_INT",
			defaultValue: 10,
			envValue:     "not_a_number",
			want:         10,
		},
		{
			name:         "handles zero value",
			key:          "ZERO_INT",
			defaultValue: 10,
			envValue:     "0",
			want:         0,
		},
		{
			name:         "handles negative value",
			key:          "NEG_INT",
			defaultValue: 10,
			envValue:     "-5",
			want:         -5,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Set or unset environment variable
			if tt.envValue != "" {
				os.Setenv(tt.key, tt.envValue)
				defer os.Unsetenv(tt.key)
			} else {
				os.Unsetenv(tt.key)
			}

			got := getEnvInt(tt.key, tt.defaultValue)
			if got != tt.want {
				t.Errorf("getEnvInt() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestDefaultEnvironmentVariables(t *testing.T) {
	// Clear all relevant environment variables
	os.Unsetenv("WORKER_ID")
	os.Unsetenv("GRPC_PORT")
	os.Unsetenv("MAX_SESSIONS")
	os.Unsetenv("BASE_WORKSPACE")

	tests := []struct {
		name     string
		key      string
		defValue interface{}
		want     interface{}
	}{
		{
			name:     "WORKER_ID defaults to worker-1",
			key:      "WORKER_ID",
			defValue: "worker-1",
			want:     "worker-1",
		},
		{
			name:     "GRPC_PORT defaults to 50051",
			key:      "GRPC_PORT",
			defValue: "50051",
			want:     "50051",
		},
		{
			name:     "MAX_SESSIONS defaults to 5",
			key:      "MAX_SESSIONS",
			defValue: 5,
			want:     5,
		},
		{
			name:     "BASE_WORKSPACE defaults to /tmp/codegen-workspaces",
			key:      "BASE_WORKSPACE",
			defValue: "/tmp/codegen-workspaces",
			want:     "/tmp/codegen-workspaces",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var got interface{}
			switch v := tt.defValue.(type) {
			case string:
				got = getEnv(tt.key, v)
			case int:
				got = getEnvInt(tt.key, v)
			}

			if got != tt.want {
				t.Errorf("Default for %s = %v, want %v", tt.key, got, tt.want)
			}
		})
	}
}
