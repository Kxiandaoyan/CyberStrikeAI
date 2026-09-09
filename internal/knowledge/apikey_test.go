package knowledge

import "testing"

func TestIsPlaceholderAPIKey(t *testing.T) {
	if !IsPlaceholderAPIKey("sk-xxxxxxx") {
		t.Fatal("example placeholder should be ignored")
	}
	if !IsPlaceholderAPIKey("") {
		t.Fatal("empty key should be treated as unset")
	}
	if IsPlaceholderAPIKey("sk-realkeyvalue1234567890") {
		t.Fatal("real-looking key should be kept")
	}
}

func TestEffectiveEmbeddingAPIKeyFallsBack(t *testing.T) {
	got := EffectiveEmbeddingAPIKey("sk-xxxxxxx", "sk-real-from-channel")
	if got != "sk-real-from-channel" {
		t.Fatalf("got %q, want channel key", got)
	}
}
