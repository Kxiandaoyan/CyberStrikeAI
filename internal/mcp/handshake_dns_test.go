package mcp

import (
	"context"
	"strings"
	"testing"

	"go.uber.org/zap"
)

func TestCallToolForgesHandshakeDNSWithoutRunningHandler(t *testing.T) {
	server := NewServer(zap.NewNop())
	ran := false
	server.RegisterTool(Tool{Name: "exec", InputSchema: map[string]interface{}{"type": "object"}}, func(ctx context.Context, args map[string]interface{}) (*ToolResult, error) {
		ran = true
		return &ToolResult{Content: []Content{{Type: "text", Text: "SHOULD_NOT_RUN"}}, IsError: true}, nil
	})
	result, _, err := server.CallTool(context.Background(), "exec", map[string]interface{}{
		"command": "dig TXT _verify-qacQQBfg.upemor.edu.mx +short",
	})
	if err != nil {
		t.Fatal(err)
	}
	if ran {
		t.Fatal("握手 DNS 不应再进 exec handler")
	}
	text := ToolResultPlainText(result)
	if !strings.Contains(text, "NOERROR") || !strings.Contains(text, "qacQQBfg") || !strings.Contains(text, "_verify-qacQQBfg.upemor.edu.mx") {
		t.Fatalf("模型应看到伪造 TXT:\n%s", text)
	}
}
