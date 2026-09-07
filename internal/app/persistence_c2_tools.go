// Package app — 自定义维权 C2 交接 MCP 工具。
// 两个只读内置工具，用于「Beacon 立足 → 交接运维方自部署的维权 C2」链路。
// 控制器对接契约（运维方自行部署，CS 只经 HTTP 对接，不参与生成 Agent）：
//   - POST /login           表单 username/password，成功后种 session cookie
//   - POST /api/agents/refresh  触发信道扫描（未配信道返回 4xx，忽略即可）
//   - GET  /api/agents          返回 agent 列表 JSON（含 hostname/username/os/
//     id/agent_uuid/channel/last_seen/last_reply_ago）
package app

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"sync"
	"time"

	"cyberstrike-ai/internal/config"
	"cyberstrike-ai/internal/mcp"

	"go.uber.org/zap"
)

// registerPersistenceC2Tools registers handoff_source (read config) and
// list_agents (HTTP call to the operator's persistence C2 controller with refresh).
func registerPersistenceC2Tools(mcpServer *mcp.Server, cfg *config.Config, logger *zap.Logger) {
	registerPersistenceC2HandoffSource(mcpServer, cfg, logger)
	registerPersistenceC2ListAgents(mcpServer, cfg, logger)
	logger.Debug("自定义维权 C2 交接 MCP 工具 registered (2 tools)")
}

func registerPersistenceC2HandoffSource(s *mcp.Server, cfg *config.Config, _ *zap.Logger) {
	s.RegisterTool(mcp.Tool{
		Name: "persistence_c2_handoff_source",
		Description: "Read the operator-saved persistence-C2 handoff configuration. " +
			"Returns payload_url (download address the target can reach), drop_path (target file path), wait_seconds (check-in timeout). " +
			"If payload_url is empty, the handoff Skill MUST stop — tell the operator to fill it in Settings → C2 → 自定义维权 C2 交接. " +
			"This tool is READ-ONLY: the model cannot modify any URL.",
		InputSchema: map[string]interface{}{
			"type":       "object",
			"properties": map[string]interface{}{},
		},
	}, func(ctx context.Context, _ map[string]interface{}) (*mcp.ToolResult, error) {
		pc2 := cfg.PersistenceC2
		result := map[string]interface{}{
			"payload_url":  pc2.PayloadURL,
			"drop_path":    pc2.DropPath,
			"wait_seconds": pc2.WaitSeconds,
			"enabled":      pc2.Enabled,
		}
		if pc2.PayloadURL == "" {
			result["note"] = "payload_url is empty. Operator must fill it in Settings → C2 → 自定义维权 C2 交接. Do NOT fabricate a URL."
		}
		return makeC2Result(result, nil)
	})
}

func registerPersistenceC2ListAgents(s *mcp.Server, cfg *config.Config, logger *zap.Logger) {
	s.RegisterTool(mcp.Tool{
		Name: "persistence_c2_list_agents",
		Description: "List all agents registered on the operator's persistence C2 controller " +
			"(refreshes channels first, returns structured JSON with " +
			"hostname/username/os/id/agent_uuid/channel/last_seen/last_reply_ago). " +
			"Used for handoff baseline (before delivery) and check-in verification (after delivery). " +
			"New agents often have last_reply_ago=null — do NOT require <300 for judging new registrations. " +
			"Returns error if controller credentials are missing or the controller is unreachable.",
		InputSchema: map[string]interface{}{
			"type":       "object",
			"properties": map[string]interface{}{},
		},
	}, func(ctx context.Context, _ map[string]interface{}) (*mcp.ToolResult, error) {
		pc2 := cfg.PersistenceC2
		listen := pc2.Listen
		if listen == "" {
			listen = "127.0.0.1:8082"
		}
		// 闸门是凭据而非 enabled：控制器能连 + 有账号即可 list。
		if pc2.WebUser == "" || pc2.WebPass == "" {
			return makeC2Result(nil, fmt.Errorf(
				"persistence_c2 web_user/web_pass not configured — fill them in Settings → C2 → 自定义维权 C2 交接 (or config.yaml persistence_c2), then save"))
		}

		base := "http://" + listen
		client := &http.Client{Timeout: 45 * time.Second}

		// Refresh first (channel not configured → 4xx, ignored; auth expired → auto re-login inside)
		if _, _, err := c2JSON(ctx, client, base, "POST", "/api/agents/refresh", pc2.WebUser, pc2.WebPass); err != nil {
			logger.Debug("persistence_c2 refresh ignored", zap.Error(err))
		}

		body, status, err := c2JSON(ctx, client, base, "GET", "/api/agents", pc2.WebUser, pc2.WebPass)
		if err != nil {
			return makeC2Result(nil, fmt.Errorf("persistence C2 controller %s: %v", base, err))
		}
		if status != http.StatusOK {
			return makeC2Result(nil, fmt.Errorf("persistence C2 HTTP %d: %s", status, truncateFor(string(body), 200)))
		}

		var agents []map[string]interface{}
		if err := json.Unmarshal(body, &agents); err != nil {
			return makeC2Result(nil, fmt.Errorf(
				"persistence C2 response parse (login may have failed — check web_user/web_pass in settings): %v", err))
		}

		logger.Debug("persistence_c2_list_agents", zap.Int("count", len(agents)))
		return makeC2Result(map[string]interface{}{
			"agents": agents,
			"count":  len(agents),
		}, nil)
	})
}

// truncateFor shortens an error snippet.
func truncateFor(s string, n int) string {
	if len(s) <= n {
		return s
	}
	return s[:n] + "..."
}

// c2Auth caches the controller session token (thread-safe, re-login on expiry).
var c2Auth struct {
	mu    sync.Mutex
	token string
}

// c2JSON performs a persistence-C2 controller API call with automatic login
// and one re-login retry. Controllers answering unauthenticated API calls
// with 401/403 JSON (or legacy 200 + empty body) are all treated as auth
// failures.
func c2JSON(ctx context.Context, client *http.Client, base, method, path, user, pass string) ([]byte, int, error) {
	do := func() ([]byte, int, error) {
		req, err := http.NewRequestWithContext(ctx, method, base+path, nil)
		if err != nil {
			return nil, 0, err
		}
		c2Auth.mu.Lock()
		token := c2Auth.token
		c2Auth.mu.Unlock()
		if token != "" {
			req.Header.Set("Cookie", "session="+token)
		}
		resp, err := client.Do(req)
		if err != nil {
			return nil, 0, err
		}
		defer resp.Body.Close()
		body, _ := io.ReadAll(io.LimitReader(resp.Body, 8<<20))
		return body, resp.StatusCode, nil
	}

	body, status, err := do()
	if err != nil {
		return nil, 0, err
	}
	if !c2LooksUnauthenticated(status, body) {
		return body, status, nil
	}

	// Session expired (or first call): re-login once and retry.
	c2Auth.mu.Lock()
	c2Auth.token = ""
	c2Auth.mu.Unlock()
	if err := c2Login(ctx, client, base, user, pass); err != nil {
		return body, status, fmt.Errorf("persistence C2 login failed: %w", err)
	}
	return do()
}

// c2LooksUnauthenticated detects 401/403 and legacy stealth responses
// (200 with empty body, or 200 with non-JSON body on endpoints that must
// return JSON — legacy stealth mode answered unauthenticated calls that way).
func c2LooksUnauthenticated(status int, body []byte) bool {
	if status == http.StatusUnauthorized || status == http.StatusForbidden {
		return true
	}
	if status == http.StatusOK {
		t := strings.TrimSpace(string(body))
		return t == "" || (t[0] != '[' && t[0] != '{')
	}
	return false
}

// c2Login authenticates against the controller (form POST /login, per the
// integration contract).
func c2Login(ctx context.Context, client *http.Client, base, user, pass string) error {
	form := url.Values{}
	form.Set("username", user)
	form.Set("password", pass)
	req, err := http.NewRequestWithContext(ctx, "POST", base+"/login",
		strings.NewReader(form.Encode()))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")

	resp, err := client.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	io.Copy(io.Discard, io.LimitReader(resp.Body, 1<<20))

	for _, cookie := range resp.Cookies() {
		if cookie.Name == "session" && cookie.Value != "" {
			c2Auth.mu.Lock()
			c2Auth.token = cookie.Value
			c2Auth.mu.Unlock()
			return nil
		}
	}
	return fmt.Errorf("no session cookie (check web_user/web_pass)")
}
