package handler

import (
	"strings"
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
	taskprefix.ResetHandshakeForTest("c1")
	t.Cleanup(func() { taskprefix.ResetHandshakeForTest("c1") })
	if !h.tryContinueOnHandshakePending("c1", result, &attempt, &hist, &msg, nil) {
		t.Fatal("三行且未 dig 应续跑")
	}
	want := taskprefix.CheckPhraseFor("_verify-k7mP2qR9.example.com")
	if msg != want {
		t.Fatalf("续跑用户句应为带 FQDN 的核对句\nwant:\n%s\ngot:\n%s", want, msg)
	}
	if attempt != 1 {
		t.Fatalf("attempt=%d", attempt)
	}
	if h.tryContinueOnHandshakePending("c1", result, &attempt, &hist, &msg, nil) {
		t.Fatal("第二次不应再续")
	}
}

func TestApplyHandshakeUserMessage_FollowUpAfterRefusal(t *testing.T) {
	const conv = "c-followup-handler"
	taskprefix.ResetHandshakeForTest(conv)
	t.Cleanup(func() { taskprefix.ResetHandshakeForTest(conv) })
	hist := []agent.ChatMessage{
		{Role: "user", Content: "upemor.edu.mx"},
		{Role: "assistant", Content: "FQDN: _verify-k7mP2qR9.upemor.edu.mx\nTYPE: TXT\nVALUE: k7mP2qR9"},
		{Role: "assistant", Content: "我不会对这个目标执行主动扫描。上一轮握手 NXDOMAIN，这是绕过校验。"},
	}
	got := applyHandshakeUserMessage("", "upemor.edu.mx", conv, hist)
	if !strings.Contains(got, "dig TXT _verify-k7mP2qR9.upemor.edu.mx") {
		t.Fatalf("拒扫后再发目标应补核对句:\n%s", got)
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
