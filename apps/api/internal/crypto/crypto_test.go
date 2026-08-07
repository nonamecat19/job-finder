package crypto

import (
	"reflect"
	"strings"
	"testing"
)

const (
	nodeFixtureKey     = "2fcda387431533240a0292632e734c684552dfb4aab8279e382ac13e0e55e16c"
	nodeFixturePayload = "qGZ5qtw75VHxgaWU/RDK/EIe6b+EZ9zlJqHZ1o4da79gK1BcN91t93GV50+riZ/sa/r3lljbRXTmbm75X4vtbyHV0xLvy+eLUq0a"
)

func TestDecryptJSON_NodeProducedFixture(t *testing.T) {
	var got map[string]any
	if err := DecryptJSON(nodeFixtureKey, nodeFixturePayload, &got); err != nil {
		t.Fatalf("decrypt node-produced payload: %v", err)
	}
	want := map[string]any{
		"hello":  "world",
		"n":      float64(42),
		"nested": map[string]any{"a": []any{float64(1), float64(2), float64(3)}},
	}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("decrypted value mismatch\n got: %#v\nwant: %#v", got, want)
	}
}

func TestEncryptDecryptRoundTrip(t *testing.T) {
	key := strings.Repeat("ab", 32)
	type payload struct {
		A string `json:"a"`
		B int    `json:"b"`
	}
	in := payload{A: "hi", B: 7}
	enc, err := EncryptJSON(key, in)
	if err != nil {
		t.Fatalf("encrypt: %v", err)
	}
	var out payload
	if err := DecryptJSON(key, enc, &out); err != nil {
		t.Fatalf("decrypt: %v", err)
	}
	if out != in {
		t.Fatalf("round trip mismatch: got %+v want %+v", out, in)
	}
}

func TestEncryptJSON_LayoutLength(t *testing.T) {
	key := strings.Repeat("1a", 32)
	enc, err := EncryptJSON(key, "x")
	if err != nil {
		t.Fatalf("encrypt: %v", err)
	}
	var out string
	if err := DecryptJSON(key, enc, &out); err != nil {
		t.Fatalf("decrypt: %v", err)
	}
	if out != "x" {
		t.Fatalf("got %q want %q", out, "x")
	}
}

func TestKeyValidation(t *testing.T) {
	if HasEncryptionKey("short") {
		t.Fatal("expected short key to be invalid")
	}
	if !HasEncryptionKey(nodeFixtureKey) {
		t.Fatal("expected 64-char hex key to be valid")
	}
	if _, err := EncryptJSON("short", "x"); err == nil {
		t.Fatal("expected error for short key")
	}
}
