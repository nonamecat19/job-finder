package dbutil

import (
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgtype"
)

func TestUUIDString(t *testing.T) {
	tests := []struct {
		name     string
		input    pgtype.UUID
		expected string
	}{
		{
			name:     "valid UUID",
			input:    pgtype.UUID{Bytes: [16]byte{0x55, 0x0e, 0x84, 0x00, 0xe2, 0x9b, 0x41, 0xd4, 0xa7, 0x16, 0x44, 0x66, 0x55, 0x44, 0x00, 0x00}, Valid: true},
			expected: "550e8400-e29b-41d4-a716-446655440000",
		},
		{
			name:     "invalid UUID",
			input:    pgtype.UUID{Valid: false},
			expected: "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := UUIDString(tt.input)
			if result != tt.expected {
				t.Errorf("expected %q, got %q", tt.expected, result)
			}
		})
	}
}

func TestUUIDStringPtr(t *testing.T) {
	tests := []struct {
		name    string
		input   pgtype.UUID
		wantNil bool
	}{
		{
			name:    "valid UUID",
			input:   pgtype.UUID{Bytes: [16]byte{1, 2, 3, 4, 5, 6, 7, 8, 9, 10, 11, 12, 13, 14, 15, 16}, Valid: true},
			wantNil: false,
		},
		{
			name:    "invalid UUID",
			input:   pgtype.UUID{Valid: false},
			wantNil: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := UUIDStringPtr(tt.input)
			if tt.wantNil {
				if result != nil {
					t.Errorf("expected nil, got %q", *result)
				}
			} else {
				if result == nil {
					t.Error("expected non-nil result")
				}
			}
		})
	}
}

func TestParseUUID(t *testing.T) {
	tests := []struct {
		name    string
		input   string
		wantErr bool
	}{
		{
			name:    "valid UUID",
			input:   "550e8400-e29b-41d4-a716-446655440000",
			wantErr: false,
		},
		{
			name:    "empty string",
			input:   "",
			wantErr: true,
		},
		{
			name:    "invalid UUID",
			input:   "not-a-uuid",
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result, err := ParseUUID(tt.input)
			if (err != nil) != tt.wantErr {
				t.Errorf("ParseUUID() error = %v, wantErr %v", err, tt.wantErr)
			}
			if !tt.wantErr && !result.Valid {
				t.Error("expected valid UUID")
			}
		})
	}
}

func TestTimestamp(t *testing.T) {
	now := time.Now().UTC()
	ts := pgtype.Timestamp{Time: now, Valid: true}

	result := Timestamp(ts)

	if result == "" {
		t.Error("expected non-empty timestamp")
	}

	parsed, err := time.Parse(time.RFC3339, result)
	if err != nil {
		t.Fatalf("failed to parse timestamp: %v", err)
	}

	if !parsed.Equal(now.Truncate(time.Second)) {
		t.Errorf("expected %v, got %v", now, parsed)
	}
}

func TestTimestampInvalid(t *testing.T) {
	ts := pgtype.Timestamp{Valid: false}

	result := Timestamp(ts)

	if result != "" {
		t.Errorf("expected empty string for invalid timestamp, got %q", result)
	}
}

func TestTimestampPtr(t *testing.T) {
	now := time.Now().UTC()
	ts := pgtype.Timestamp{Time: now, Valid: true}

	result := TimestampPtr(ts)

	if result == nil {
		t.Fatal("expected non-nil pointer")
	}

	parsed, err := time.Parse(time.RFC3339, *result)
	if err != nil {
		t.Fatalf("failed to parse timestamp: %v", err)
	}

	if !parsed.Equal(now.Truncate(time.Second)) {
		t.Errorf("expected %v, got %v", now, parsed)
	}
}

func TestTimestampPtrInvalid(t *testing.T) {
	ts := pgtype.Timestamp{Valid: false}

	result := TimestampPtr(ts)

	if result != nil {
		t.Errorf("expected nil for invalid timestamp, got %q", *result)
	}
}

func TestNowTimestamp(t *testing.T) {
	before := time.Now().UTC()
	ts := NowTimestamp()
	after := time.Now().UTC()

	if !ts.Valid {
		t.Error("expected valid timestamp")
	}

	if ts.Time.Before(before.Add(-time.Second)) || ts.Time.After(after.Add(time.Second)) {
		t.Errorf("timestamp %v not between %v and %v", ts.Time, before, after)
	}
}

func TestTimestampFromPtr(t *testing.T) {
	now := time.Now().UTC().Format(time.RFC3339)
	ts := TimestampFromPtr(&now)

	if !ts.Valid {
		t.Error("expected valid timestamp")
	}
}

func TestTimestampFromPtrNil(t *testing.T) {
	ts := TimestampFromPtr(nil)

	if ts.Valid {
		t.Error("expected invalid timestamp for nil pointer")
	}
}

func TestTimestampFromPtrEmpty(t *testing.T) {
	empty := ""
	ts := TimestampFromPtr(&empty)

	if ts.Valid {
		t.Error("expected invalid timestamp for empty string")
	}
}

func TestUnmarshalJSONB(t *testing.T) {
	tests := []struct {
		name    string
		input   []byte
		wantErr bool
	}{
		{
			name:    "valid JSON",
			input:   []byte(`{"key": "value"}`),
			wantErr: false,
		},
		{
			name:    "empty JSON",
			input:   []byte(`{}`),
			wantErr: false,
		},
		{
			name:    "invalid JSON",
			input:   []byte(`not json`),
			wantErr: true,
		},
		{
			name:    "null JSON",
			input:   []byte(`null`),
			wantErr: false,
		},
		{
			name:    "empty bytes",
			input:   []byte{},
			wantErr: false,
		},
		{
			name:    "nil bytes",
			input:   nil,
			wantErr: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var result map[string]interface{}
			err := UnmarshalJSONB(tt.input, &result)
			if (err != nil) != tt.wantErr {
				t.Errorf("UnmarshalJSONB() error = %v, wantErr %v", err, tt.wantErr)
			}
		})
	}
}

func TestMarshalJSONB(t *testing.T) {
	tests := []struct {
		name    string
		input   interface{}
		wantErr bool
	}{
		{
			name:    "valid struct",
			input:   map[string]string{"key": "value"},
			wantErr: false,
		},
		{
			name:    "nil value",
			input:   nil,
			wantErr: false,
		},
		{
			name:    "empty struct",
			input:   struct{}{},
			wantErr: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := MarshalJSONB(tt.input)
			if (err != nil) != tt.wantErr {
				t.Errorf("MarshalJSONB() error = %v, wantErr %v", err, tt.wantErr)
			}
		})
	}
}

func TestParseUUIDRoundTrip(t *testing.T) {
	original := "550e8400-e29b-41d4-a716-446655440000"
	parsed, err := ParseUUID(original)
	if err != nil {
		t.Fatalf("failed to parse UUID: %v", err)
	}

	result := UUIDString(parsed)
	if result != original {
		t.Errorf("round trip failed: expected %q, got %q", original, result)
	}
}
