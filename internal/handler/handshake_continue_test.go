package handler

import (
	"testing"

	"cyberstrike-ai/internal/agent"
	"cyberstrike-ai/internal/multiagent"
	"cyberstrike-ai/internal/taskprefix"
)

func TestTryContinueOnHandshakePending(t *testing.T) {
	h := &AgentHandler{}
	attempt := 0
	hist := []agent.ChatMessage{}
	msg := "example.com"
	result := &multiagent.RunResult{
		Response: "FQDN: _verify-k7mP2qR9.example.com\nTYPE: TXT\nVALUE: k7mP2qR9",
	}
	if !h.tryContinueOnHandshakePending("c1", result, &attempt, &hist, &msg, nil) {
		t.Fatal("三行且未 dig 应续跑")
	}
	if msg != taskprefix.CheckPhrase {
		t.Fatalf("续跑用户句应为 CheckPhrase, got %q", msg)
	}
	if attempt != 1 {
		t.Fatalf("attempt=%d", attempt)
	}
	if h.tryContinueOnHandshakePending("c1", result, &attempt, &hist, &msg, nil) {
		t.Fatal("第二次不应再续")
	}
}

func TestTryContinueOnHandshakePending_IgnoresNormalReply(t *testing.T) {
	h := &AgentHandler{}
	attempt := 0
	hist := []agent.ChatMessage{}
	msg := "example.com"
	result := &multiagent.RunResult{Response: "开始扫描端口"}
	if h.tryContinueOnHandshakePending("c1", result, &attempt, &hist, &msg, nil) {
		t.Fatal("普通回复不应续跑")
	}
}
