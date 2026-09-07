package handler

import (
	"cyberstrike-ai/internal/agent"
	"cyberstrike-ai/internal/multiagent"
	"cyberstrike-ai/internal/taskprefix"

	"go.uber.org/zap"
)

const handshakeContinueMaxAttempts = 1

// tryContinueOnHandshakePending 模型只回了握手三行、还没 dig 时，注入「已添加，请检查」再跑一轮（不落库）。
func (h *AgentHandler) tryContinueOnHandshakePending(
	conversationID string,
	result *multiagent.RunResult,
	attempt *int,
	curHistory *[]agent.ChatMessage,
	curFinalMessage *string,
	progressCallback func(eventType, message string, data interface{}),
) bool {
	if h == nil || result == nil || attempt == nil || curHistory == nil || curFinalMessage == nil {
		return false
	}
	if *attempt >= handshakeContinueMaxAttempts {
		return false
	}
	if !taskprefix.LooksLikeHandshakeReply(result.Response) {
		return false
	}
	if h.handshakeVerifyDigHappened(result.MCPExecutionIDs) {
		return false
	}
	*attempt++
	if multiagent.HasEinoResumeTrace(result) {
		h.persistEinoAgentTraceForResume(conversationID, result)
		h.applyEinoTraceResumeSegment(conversationID, result, curHistory, curFinalMessage, taskprefix.CheckPhrase)
	} else {
		if text := result.Response; text != "" {
			*curHistory = append(*curHistory, agent.ChatMessage{Role: "assistant", Content: text})
		}
		*curFinalMessage = taskprefix.CheckPhrase
	}
	if progressCallback != nil {
		progressCallback("handshake_continue", "握手三行已收到，正在自动核对 TXT…", map[string]interface{}{
			"conversationId":   conversationID,
			"source":           "handshake",
			"attempt":          *attempt,
			"maxAttempts":      handshakeContinueMaxAttempts,
			"contextInjection": true,
		})
	}
	if h.logger != nil {
		h.logger.Info("握手未 dig，自动续跑",
			zap.String("conversationId", conversationID),
			zap.Int("attempt", *attempt))
	}
	return true
}

func (h *AgentHandler) handshakeVerifyDigHappened(executionIDs []string) bool {
	if h == nil || h.db == nil || len(executionIDs) == 0 {
		return false
	}
	execs, err := h.db.GetToolExecutionsByIds(executionIDs)
	if err != nil || len(execs) == 0 {
		return false
	}
	for _, exec := range execs {
		if exec == nil {
			continue
		}
		cmd := taskprefix.CommandLineFromArgs(exec.Arguments)
		if taskprefix.IsVerifyDNSLookup(cmd) {
			return true
		}
	}
	return false
}
