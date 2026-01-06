package worker

import "testing"

func TestNormalizeURL(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		expected string
	}{
		{
			name:     "empty string",
			input:    "",
			expected: "",
		},
		{
			name:     "hostname only",
			input:    "wonder.strrl.cloud",
			expected: "https://wonder.strrl.cloud",
		},
		{
			name:     "hostname with port",
			input:    "localhost:9080",
			expected: "https://localhost:9080",
		},
		{
			name:     "https URL",
			input:    "https://wonder.strrl.cloud",
			expected: "https://wonder.strrl.cloud",
		},
		{
			name:     "http URL",
			input:    "http://localhost:9080",
			expected: "http://localhost:9080",
		},
		{
			name:     "single trailing slash",
			input:    "https://wonder.strrl.cloud/",
			expected: "https://wonder.strrl.cloud",
		},
		{
			name:     "multiple trailing slashes",
			input:    "https://wonder.strrl.cloud///",
			expected: "https://wonder.strrl.cloud",
		},
		{
			name:     "URL with path",
			input:    "https://wonder.strrl.cloud/coordinator",
			expected: "https://wonder.strrl.cloud",
		},
		{
			name:     "URL with path and trailing slash",
			input:    "wonder.strrl.cloud/coordinator/",
			expected: "https://wonder.strrl.cloud",
		},
		{
			name:     "URL with query parameters",
			input:    "https://wonder.strrl.cloud?foo=bar",
			expected: "https://wonder.strrl.cloud",
		},
		{
			name:     "URL with fragment",
			input:    "https://wonder.strrl.cloud#section",
			expected: "https://wonder.strrl.cloud",
		},
		{
			name:     "URL with path query and fragment",
			input:    "https://wonder.strrl.cloud/path?query=1#frag",
			expected: "https://wonder.strrl.cloud",
		},
		{
			name:     "hostname without scheme with path",
			input:    "wonder.strrl.cloud/some/path",
			expected: "https://wonder.strrl.cloud",
		},
		{
			name:     "IP address",
			input:    "192.168.1.1",
			expected: "https://192.168.1.1",
		},
		{
			name:     "IP address with port",
			input:    "192.168.1.1:9080",
			expected: "https://192.168.1.1:9080",
		},
		{
			name:     "localhost",
			input:    "localhost",
			expected: "https://localhost",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := normalizeURL(tt.input)
			if result != tt.expected {
				t.Errorf("normalizeURL(%q) = %q, want %q", tt.input, result, tt.expected)
			}
		})
	}
}
