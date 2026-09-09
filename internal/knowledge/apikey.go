package knowledge

import "strings"

// IsPlaceholderAPIKey reports example/dummy keys that must not be sent to an embed API.
func IsPlaceholderAPIKey(s string) bool {
	k := strings.ToLower(strings.TrimSpace(s))
	if k == "" {
		return true
	}
	switch k {
	case "sk-xxxxxxx", "sk-xxx", "sk-xxxx", "your-api-key", "changeme", "xxx", "xxxxx", "apikey":
		return true
	}
	if strings.Contains(k, "xxxx") || strings.Contains(k, "your-api-key") || strings.Contains(k, "replace-me") {
		return true
	}
	return false
}

// EffectiveEmbeddingAPIKey prefers knowledge.embedding.api_key, then the main OpenAI/AI channel key.
// Placeholder values are treated as unset.
func EffectiveEmbeddingAPIKey(knowledgeKey, fallbackKey string) string {
	if !IsPlaceholderAPIKey(knowledgeKey) {
		return strings.TrimSpace(knowledgeKey)
	}
	if !IsPlaceholderAPIKey(fallbackKey) {
		return strings.TrimSpace(fallbackKey)
	}
	return ""
}

func IsEmbedAuthError(err error) bool {
	if err == nil {
		return false
	}
	s := strings.ToLower(err.Error())
	return strings.Contains(s, "401") ||
		strings.Contains(s, "unauthorized") ||
		strings.Contains(s, "incorrect api key") ||
		strings.Contains(s, "invalid api key") ||
		strings.Contains(s, "apikey-error")
}
