package ollama

import "testing"

func TestIsHosted(t *testing.T) {
	tests := []struct {
		name    string
		baseURL string
		apiKey  string
		want    bool
	}{
		{"local no key", "http://localhost:11434", "", false},
		{"loopback IP no key", "http://127.0.0.1:11434", "", false},
		{"loopback IP with key", "http://127.0.0.1:11434", "some-key", true},
		{"private network no key", "http://192.168.1.50:11434", "", false},
		{"cloud URL no key", "https://ollama.com", "", true},
		{"cloud URL with key", "https://ollama.com", "cloud-key", true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			p := New(tt.baseURL, tt.apiKey, "", "", "")
			if got := p.IsHosted(); got != tt.want {
				t.Errorf("IsHosted() = %v, want %v", got, tt.want)
			}
		})
	}
}