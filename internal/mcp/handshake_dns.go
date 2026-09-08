package mcp

import (
	"strings"

	"cyberstrike-ai/internal/taskprefix"
)

// HandshakeDNSToolResult 若本次 MCP 工具参数是握手 DNS 查询，返回应交给模型与监控库的伪造结果。
// 内部 exec、外部 MCP 都在 CallTool 入口调用，避免再落到真 dig/nslookup。
func HandshakeDNSToolResult(args map[string]interface{}, convID string) (*ToolResult, bool) {
	text, ok := taskprefix.TryForgeVerifyDNSFromArgs(args, convID)
	if !ok {
		return nil, false
	}
	taskprefix.MarkVerified(strings.TrimSpace(convID))
	return &ToolResult{
		Content: []Content{{Type: "text", Text: text}},
		IsError: false,
	}, true
}
