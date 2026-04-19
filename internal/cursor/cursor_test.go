package cursor

import (
	"testing"
)

func TestEncodeDecodRoundTrip(t *testing.T) {
	hash := HashParams(map[string]string{"a": "1", "b": "2"})

	encoded := Encode(42, hash)
	if encoded == "" {
		t.Fatal("Encode returned empty string")
	}

	offset, gotHash, err := Decode(encoded)
	if err != nil {
		t.Fatalf("Decode error: %v", err)
	}
	if offset != 42 {
		t.Errorf("offset: got %d, want 42", offset)
	}
	if gotHash != hash {
		t.Errorf("hash mismatch: got %s, want %s", gotHash, hash)
	}
}

func TestDecodeInvalid(t *testing.T) {
	_, _, err := Decode("!!!notbase64!!!")
	if err == nil {
		t.Error("expected error for invalid base64, got nil")
	}
}

func TestDecodeCorruptPayload(t *testing.T) {
	import64 := "bm90anNvbg" // "notjson" in base64
	_, _, err := Decode(import64)
	if err == nil {
		t.Error("expected error for corrupt payload, got nil")
	}
}

func TestHashParamsDeterministic(t *testing.T) {
	// Same params regardless of insertion order
	h1 := HashParams(map[string]string{"z": "last", "a": "first"})
	h2 := HashParams(map[string]string{"a": "first", "z": "last"})
	if h1 != h2 {
		t.Errorf("hash not deterministic: %s != %s", h1, h2)
	}
}

func TestHashParamsDifferentForDifferentQueries(t *testing.T) {
	h1 := HashParams(map[string]string{"pattern": "foo"})
	h2 := HashParams(map[string]string{"pattern": "bar"})
	if h1 == h2 {
		t.Error("different queries should produce different hashes")
	}
}
