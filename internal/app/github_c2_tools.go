// Package app — GitHub-C2 handoff MCP tools.
// Two read-only built-in tools for the handoff chain (CyberStrikeAI §6.5-6.6).
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

// registerGithubC2Tools registers handoff_source (read config) and
// list_agents (HTTP call to G-C2 controller with refresh).
func registerGithubC2Tools(mcpServer *mcp.Server, cfg *config.Config, logger *zap.Logger) {
	registerGithubC2HandoffSource(mcpServer, cfg, logger)
	registerGithubC2ListAgents(mcpServer, cfg, logger)
	logger.Debug("GitHub-C2 handoff MCP tools registered (2 tools)")
}

func registerGithubC2HandoffSource(s *mcp.Server, cfg *config.Config, _ *zap.Logger) {
	s.RegisterTool(mcp.Tool{
		Name: "github_c2_handoff_source",
		Description: "Read the operator-saved GitHub-C2 handoff configuration. " +
			"Returns payload_url (download address), drop_path (target file path), wait_seconds (check-in timeout). " +
			"If payload_url is empty, the handoff Skill MUST stop — tell the operator to fill it in Settings → C2 → GitHub-C2 交接. " +
			"This tool is READ-ONLY: the model cannot modify any URL.",
		InputSchema: map[string]interface{}{
			"type":       "object",
			"properties": map[string]interface{}{},
		},
	}, func(ctx context.Context, _ map[string]interface{}) (*mcp.ToolResult, error) {
		gc2 := cfg.GitHubC2
		result := map[string]interface{}{
			"payload_url":  gc2.PayloadURL,
			"drop_path":    gc2.DropPath,
			"wait_seconds": gc2.WaitSeconds,
			"enabled":      gc2.Enabled,
		}
		if gc2.PayloadURL == "" {
			result["note"] = "payload_url is empty. Operator must fill it in Settings → C2 → GitHub-C2 交接. Do NOT fabricate a URL."
		}
		return makeC2Result(result, nil)
	})
}

func registerGithubC2ListAgents(s *mcp.Server, cfg *config.Config, logger *zap.Logger) {
	s.RegisterTool(mcp.Tool{
		Name: "github_c2_list_agents",
		Description: "List all GitHub-C2 agents (refreshes channels first, returns structured JSON with " +
			"hostname/username/os/id/agent_uuid/channel/last_seen/last_reply_ago). " +
			"Used for handoff baseline (before delivery) and check-in verification (after delivery per §6.5). " +
			"New agents often have last_reply_ago=null — do NOT require <300 for judging new registrations. " +
			"Returns error if G-C2 controller credentials are missing or the controller is unreachable.",
		InputSchema: map[string]interface{}{
			"type":       "object",
			"properties": map[string]interface{}{},
		},
	}, func(ctx context.Context, _ map[string]interface{}) (*mcp.ToolResult, error) {
		gc2 := cfg.GitHubC2
		listen := gc2.Listen
		if listen == "" {
			listen = "127.0.0.1:8082"
		}
		// 闸门是凭据而非 enabled：enabled 留给将来的 Flask sidecar（审计 §2.5）。
		// 控制器能连 + 有账号即可 list。
		if gc2.WebUser == "" || gc2.WebPass == "" {
			return makeC2Result(nil, fmt.Errorf(
				"github_c2 web_user/web_pass not configured — fill them in Settings → C2 → GitHub-C2 交接 (or config.yaml github_c2), then save"))
		}

		base := "http://" + listen
		client := &http.Client{Timeout: 45 * time.Second}

		// Refresh first (channel not configured → 400, ignored; auth expired → auto re-login inside)
		if _, _, err := gc2JSON(ctx, client, base, "POST", "/api/agents/refresh", gc2.WebUser, gc2.WebPass); err != nil {
			logger.Debug("github_c2 refresh ignored", zap.Error(err))
		}

		body, status, err := gc2JSON(ctx, client, base, "GET", "/api/agents", gc2.WebUser, gc2.WebPass)
		if err != nil {
			return makeC2Result(nil, fmt.Errorf("G-C2 controller %s: %v", base, err))
		}
		if status != http.StatusOK {
			return makeC2Result(nil, fmt.Errorf("G-C2 HTTP %d: %s", status, truncateFor(string(body), 200)))
		}

		var agents []map[string]interface{}
		if err := json.Unmarshal(body, &agents); err != nil {
			return makeC2Result(nil, fmt.Errorf(
				"G-C2 response parse (login may have failed — check web_user/web_pass in settings): %v", err))
		}

		logger.Debug("github_c2_list_agents", zap.Int("count", len(agents)))
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

// gc2Auth caches the controller session token (thread-safe, re-login on expiry).
var gc2Auth struct {
	mu    sync.Mutex
	token string
}

// gc2JSON performs a G-C2 API call with automatic login and one re-login retry.
// A stealth-mode controller answers unauthenticated API calls with 401 JSON
// (or legacy 200 + empty body) — both are treated as auth failures.
func gc2JSON(ctx context.Context, client *http.Client, base, method, path, user, pass string) ([]byte, int, error) {
	do := func() ([]byte, int, error) {
		req, err := http.NewRequestWithContext(ctx, method, base+path, nil)
		if err != nil {
			return nil, 0, err
		}
		gc2Auth.mu.Lock()
		token := gc2Auth.token
		gc2Auth.mu.Unlock()
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
	if !gc2LooksUnauthenticated(status, body) {
		return body, status, nil
	}

	// Session expired (or first call): re-login once and retry.
	gc2Auth.mu.Lock()
	gc2Auth.token = ""
	gc2Auth.mu.Unlock()
	if err := gc2Login(ctx, client, base, user, pass); err != nil {
		return body, status, fmt.Errorf("G-C2 login failed: %w", err)
	}
	return do()
}

// gc2LooksUnauthenticated detects 401/403 and legacy stealth responses
// (200 with empty body, or 200 with non-JSON body on endpoints that must
// return JSON — legacy stealth mode answered unauthenticated calls that way).
func gc2LooksUnauthenticated(status int, body []byte) bool {
	if status == http.StatusUnauthorized || status == http.StatusForbidden {
		return true
	}
	if status == http.StatusOK {
		t := strings.TrimSpace(string(body))
		return t == "" || (t[0] != '[' && t[0] != '{')
	}
	return false
}

// gc2Login authenticates against the G-C2 controller (form POST /login).
func gc2Login(ctx context.Context, client *http.Client, base, user, pass string) error {
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
			gc2Auth.mu.Lock()
			gc2Auth.token = cookie.Value
			gc2Auth.mu.Unlock()
			return nil
		}
	}
	return fmt.Errorf("no session cookie (check web_user/web_pass)")
}
