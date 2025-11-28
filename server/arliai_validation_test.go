package server

import (
	"testing"
)

func TestValidateArliaiConfig(t *testing.T) {
	tests := []struct {
		name    string
		baseURL string
		apiKey  string
		wantErr bool
	}{
		{
			name:    "valid https URL",
			baseURL: "https://api.arliai.com/v1",
			apiKey:  "test-key",
			wantErr: false,
		},
		{
			name:    "valid http URL",
			baseURL: "http://localhost:8080",
			apiKey:  "test-key",
			wantErr: false,
		},
		{
			name:    "empty baseURL",
			baseURL: "",
			apiKey:  "test-key",
			wantErr: true,
		},
		{
			name:    "invalid URL",
			baseURL: "not-a-url",
			apiKey:  "test-key",
			wantErr: true,
		},
		{
			name:    "invalid scheme",
			baseURL: "ftp://example.com",
			apiKey:  "test-key",
			wantErr: true,
		},
		{
			name:    "empty host",
			baseURL: "https://",
			apiKey:  "test-key",
			wantErr: true,
		},
		{
			name:    "empty apiKey is allowed",
			baseURL: "https://api.arliai.com/v1",
			apiKey:  "",
			wantErr: false,
		},
		{
			name:    "whitespace-only apiKey",
			baseURL: "https://api.arliai.com/v1",
			apiKey:  "   ",
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := ValidateArliaiConfig(tt.baseURL, tt.apiKey)
			if (err != nil) != tt.wantErr {
				t.Errorf("ValidateArliaiConfig() error = %v, wantErr %v", err, tt.wantErr)
			}
		})
	}
}

func TestValidateModelName(t *testing.T) {
	tests := []struct {
		name    string
		model   string
		wantErr bool
	}{
		{
			name:    "valid model name",
			model:   "GLM-4.5-Air",
			wantErr: false,
		},
		{
			name:    "empty model name",
			model:   "",
			wantErr: true,
		},
		{
			name:    "model name too long",
			model:   string(make([]byte, 101)),
			wantErr: true,
		},
		{
			name:    "model name with newline",
			model:   "model\nname",
			wantErr: true,
		},
		{
			name:    "model name with carriage return",
			model:   "model\rname",
			wantErr: true,
		},
		{
			name:    "model name with tab",
			model:   "model\tname",
			wantErr: true,
		},
		{
			name:    "model name with spaces",
			model:   "model name",
			wantErr: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := ValidateModelName(tt.model)
			if (err != nil) != tt.wantErr {
				t.Errorf("ValidateModelName() error = %v, wantErr %v", err, tt.wantErr)
			}
		})
	}
}

func TestSanitizeModelName(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		expected string
	}{
		{
			name:     "remove newlines",
			input:    "model\nname",
			expected: "modelname",
		},
		{
			name:     "remove carriage returns",
			input:    "model\rname",
			expected: "modelname",
		},
		{
			name:     "remove tabs",
			input:    "model\tname",
			expected: "modelname",
		},
		{
			name:     "trim whitespace",
			input:    "  model name  ",
			expected: "model name",
		},
		{
			name:     "truncate long name",
			input:    string(make([]byte, 150)),
			expected: string(make([]byte, 100)),
		},
		{
			name:     "valid name unchanged",
			input:    "GLM-4.5-Air",
			expected: "GLM-4.5-Air",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := SanitizeModelName(tt.input)
			if result != tt.expected {
				t.Errorf("SanitizeModelName() = %q, want %q", result, tt.expected)
			}
		})
	}
}

