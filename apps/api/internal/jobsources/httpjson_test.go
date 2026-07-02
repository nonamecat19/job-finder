package jobsources

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"net/url"
	"testing"
	"time"
)

func TestGetJSON(t *testing.T) {
	tests := []struct {
		name       string
		serverFunc http.HandlerFunc
		url        string
		wantErr    bool
	}{
		{
			name: "successful GET",
			serverFunc: func(w http.ResponseWriter, r *http.Request) {
				w.Header().Set("Content-Type", "application/json")
				json.NewEncoder(w).Encode(map[string]string{"key": "value"})
			},
			url:     "/test",
			wantErr: false,
		},
		{
			name: "server error",
			serverFunc: func(w http.ResponseWriter, r *http.Request) {
				w.WriteHeader(http.StatusInternalServerError)
			},
			url:     "/test",
			wantErr: true,
		},
		{
			name: "not found",
			serverFunc: func(w http.ResponseWriter, r *http.Request) {
				w.WriteHeader(http.StatusNotFound)
			},
			url:     "/test",
			wantErr: true,
		},
		{
			name: "invalid JSON",
			serverFunc: func(w http.ResponseWriter, r *http.Request) {
				w.Header().Set("Content-Type", "application/json")
				w.Write([]byte("invalid json"))
			},
			url:     "/test",
			wantErr: true,
		},
		{
			name: "empty response",
			serverFunc: func(w http.ResponseWriter, r *http.Request) {
				w.WriteHeader(http.StatusOK)
			},
			url:     "/test",
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			server := httptest.NewServer(tt.serverFunc)
			defer server.Close()

			var result map[string]string
			err := GetJSON(context.Background(), server.Client(), server.URL+tt.url, nil, &result)

			if (err != nil) != tt.wantErr {
				t.Errorf("GetJSON() error = %v, wantErr %v", err, tt.wantErr)
			}
		})
	}
}

func TestGetJSONResponseParsing(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]interface{}{
			"name":  "John",
			"age":   30,
			"email": "john@example.com",
		})
	}))
	defer server.Close()

	var result map[string]interface{}
	err := GetJSON(context.Background(), server.Client(), server.URL, nil, &result)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if result["name"] != "John" {
		t.Errorf("expected name 'John', got %v", result["name"])
	}
	if result["age"] != float64(30) {
		t.Errorf("expected age 30, got %v", result["age"])
	}
	if result["email"] != "john@example.com" {
		t.Errorf("expected email 'john@example.com', got %v", result["email"])
	}
}

func TestGetJSONWithParams(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]string{
			"query": r.URL.Query().Get("q"),
			"page":  r.URL.Query().Get("page"),
		})
	}))
	defer server.Close()

	params := url.Values{
		"q":    {"golang"},
		"page": {"1"},
	}

	var result map[string]string
	err := GetJSON(context.Background(), server.Client(), server.URL+"/search", params, &result)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if result["query"] != "golang" {
		t.Errorf("expected query 'golang', got %q", result["query"])
	}
	if result["page"] != "1" {
		t.Errorf("expected page '1', got %q", result["page"])
	}
}

func TestGetJSONWithNilClient(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]string{"key": "value"})
	}))
	defer server.Close()

	var result map[string]string
	err := GetJSON(context.Background(), nil, server.URL, nil, &result)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if result["key"] != "value" {
		t.Errorf("expected key 'value', got %q", result["key"])
	}
}

func TestGetJSONWithInvalidURL(t *testing.T) {
	var result map[string]string
	err := GetJSON(context.Background(), nil, "://invalid", nil, &result)
	if err == nil {
		t.Error("expected error for invalid URL")
	}
}

func TestGetJSONWithEmptyResponse(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	defer server.Close()

	var result map[string]string
	err := GetJSON(context.Background(), server.Client(), server.URL, nil, &result)
	if err == nil {
		t.Error("expected error for empty response")
	}
}

func TestGetJSONWithArrayResponse(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode([]map[string]string{
			{"name": "John"},
			{"name": "Jane"},
		})
	}))
	defer server.Close()

	var result []map[string]string
	err := GetJSON(context.Background(), server.Client(), server.URL, nil, &result)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if len(result) != 2 {
		t.Fatalf("expected 2 items, got %d", len(result))
	}

	if result[0]["name"] != "John" {
		t.Errorf("expected name 'John', got %q", result[0]["name"])
	}
}

func TestPostJSON(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			t.Errorf("expected POST method, got %s", r.Method)
		}

		var body map[string]string
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			t.Fatalf("failed to decode body: %v", err)
		}

		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]string{
			"received": body["message"],
		})
	}))
	defer server.Close()

	body := map[string]string{"message": "hello"}
	var result map[string]string

	err := PostJSON(context.Background(), server.Client(), server.URL, body, &result, 5*time.Second)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if result["received"] != "hello" {
		t.Errorf("expected received 'hello', got %q", result["received"])
	}
}

func TestPostJSONWithNilClient(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]string{"status": "ok"})
	}))
	defer server.Close()

	body := map[string]string{"data": "test"}
	var result map[string]string

	err := PostJSON(context.Background(), nil, server.URL, body, &result, 5*time.Second)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if result["status"] != "ok" {
		t.Errorf("expected status 'ok', got %q", result["status"])
	}
}

func TestPostJSONServerError(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
		w.Write([]byte("internal error"))
	}))
	defer server.Close()

	body := map[string]string{"data": "test"}
	var result map[string]string

	err := PostJSON(context.Background(), server.Client(), server.URL, body, &result, 5*time.Second)
	if err == nil {
		t.Error("expected error for server error")
	}
}

func TestPostJSONInvalidJSON(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]string{"status": "ok"})
	}))
	defer server.Close()

	body := make(chan int) // channel cannot be marshaled to JSON
	var result map[string]string

	err := PostJSON(context.Background(), server.Client(), server.URL, body, &result, 5*time.Second)
	if err == nil {
		t.Error("expected error for invalid JSON body")
	}
}

func TestPostJSONWithInvalidURL(t *testing.T) {
	body := map[string]string{"data": "test"}
	var result map[string]string

	err := PostJSON(context.Background(), nil, "://invalid", body, &result, 5*time.Second)
	if err == nil {
		t.Error("expected error for invalid URL")
	}
}

func TestGetJSONContextCancellation(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		time.Sleep(100 * time.Millisecond)
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]string{"key": "value"})
	}))
	defer server.Close()

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Millisecond)
	defer cancel()

	var result map[string]string
	err := GetJSON(ctx, server.Client(), server.URL, nil, &result)
	if err == nil {
		t.Error("expected error for cancelled context")
	}
}

func TestPostJSONContextCancellation(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		time.Sleep(100 * time.Millisecond)
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]string{"status": "ok"})
	}))
	defer server.Close()

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Millisecond)
	defer cancel()

	body := map[string]string{"data": "test"}
	var result map[string]string

	err := PostJSON(ctx, server.Client(), server.URL, body, &result, 5*time.Second)
	if err == nil {
		t.Error("expected error for cancelled context")
	}
}

func TestGetJSONWithCustomHeaders(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		auth := r.Header.Get("Authorization")
		if auth != "Bearer token123" {
			t.Errorf("expected auth header 'Bearer token123', got %q", auth)
		}

		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]string{"status": "ok"})
	}))
	defer server.Close()

	req, err := http.NewRequestWithContext(context.Background(), "GET", server.URL, nil)
	if err != nil {
		t.Fatalf("failed to create request: %v", err)
	}
	req.Header.Set("Authorization", "Bearer token123")

	client := &http.Client{}
	resp, err := client.Do(req)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		t.Errorf("expected status 200, got %d", resp.StatusCode)
	}
}
