// Package experience — LLM draft generation from blackboard material.
//
// Pipeline: collect facts/vulns → redact → LLM draft → human review (the
// approve endpoint applies to skills/knowledge; auto_apply is hard-locked
// false in config.Load). Drafts are deduplicated by content hash within 24h.
package experience

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"regexp"
	"sort"
	"strings"
	"time"

	"cyberstrike-ai/internal/config"
	"cyberstrike-ai/internal/database"
	"cyberstrike-ai/internal/openai"

	"go.uber.org/zap"
)

// Generator creates experience drafts from finished conversations/queues.
type Generator struct {
	db     *database.DB
	cfg    *config.Config
	logger *zap.SugaredLogger
	// autoApplier（可选）POC 免审批直接落盘的写入器，由 app.go 注入 —— 与人工
	// 批准走同一段写文件代码（experience_apply 的 poc 分支）。
	autoApplier func(d *Draft, appliedTo string) (string, error)
}

// NewGenerator builds a Generator. Callers may hold it even when disabled —
// every entry point re-checks Enabled/AutoDraft and is nil-safe.
func NewGenerator(db *database.DB, cfg *config.Config, logger *zap.SugaredLogger) *Generator {
	return &Generator{db: db, cfg: cfg, logger: logger}
}

// SetAutoApplier wires the POC auto-write backend (same file-writing code as
// the human approve path). Methodology drafts never call it.
func (g *Generator) SetAutoApplier(f func(d *Draft, appliedTo string) (string, error)) {
	g.autoApplier = f
}

// MaybeDraftConversation asynchronously drafts for a finished conversation.
// Batch children are excluded by the caller (queue completion produces ONE
// aggregated draft instead).
func (g *Generator) MaybeDraftConversation(conversationID string) {
	if g == nil || g.cfg == nil || !g.cfg.Experience.Enabled || !g.cfg.Experience.AutoDraft {
		return
	}
	if strings.TrimSpace(conversationID) == "" {
		return
	}
	go func() {
		defer func() {
			if r := recover(); r != nil {
				g.logger.Warnw("experience draft panic", "recover", r)
			}
		}()
		if err := g.draftForConversations("conversation", conversationID, []string{conversationID}); err != nil {
			g.logger.Debugw("experience draft skipped", "conversation", conversationID, "reason", err.Error())
		}
	}()
}

// MaybeDraftQueue asynchronously drafts ONE aggregated entry for a completed
// batch queue (its sub-conversations' facts are unioned).
func (g *Generator) MaybeDraftQueue(queueID string, conversationIDs []string) {
	if g == nil || g.cfg == nil || !g.cfg.Experience.Enabled || !g.cfg.Experience.AutoDraft {
		return
	}
	if len(conversationIDs) == 0 {
		return
	}
	go func() {
		defer func() {
			if r := recover(); r != nil {
				g.logger.Warnw("experience draft panic", "recover", r)
			}
		}()
		if err := g.draftForConversations("queue", queueID, conversationIDs); err != nil {
			g.logger.Debugw("experience draft skipped", "queue", queueID, "reason", err.Error())
		}
	}()
}

// draftForConversations unions the facts of the given conversations' projects,
// runs the gates, asks the LLM and stores one draft.
func (g *Generator) draftForConversations(sourceType, source string, conversationIDs []string) error {
	minFacts := g.cfg.Experience.MinFacts
	if minFacts <= 0 {
		minFacts = 2
	}

	seen := make(map[string]bool)
	var facts []*database.ProjectFact
	projectIDs := make(map[string]bool)
	for _, cid := range conversationIDs {
		conv, err := g.db.GetConversation(cid)
		if err != nil || conv == nil || strings.TrimSpace(conv.ProjectID) == "" {
			continue
		}
		projectIDs[conv.ProjectID] = true
	}
	if len(projectIDs) == 0 {
		return fmt.Errorf("no project-bound conversation")
	}

	// POC 沉淀（独立于 min_facts 门槛）：一条 confirmed 且能对上 CVE 编号的
	// 漏洞就值得出草稿。机械组装自漏洞记录（复现步骤/前提/证据），
	// 不过 LLM —— 保真、零幻觉；IPv4 机械脱敏，其余目标特征靠人审把关。
	g.collectPocDrafts(projectIDs)

	for pid := range projectIDs {
		pf, err := g.db.ListProjectFactsForIndex(pid, false)
		if err != nil {
			continue
		}
		for _, f := range pf {
			if f != nil && !seen[f.ID] {
				seen[f.ID] = true
				facts = append(facts, f)
			}
		}
	}
	if len(facts) < minFacts {
		return fmt.Errorf("facts below min_facts (%d < %d)", len(facts), minFacts)
	}
	sort.Slice(facts, func(i, j int) bool { return facts[i].CreatedAt.Before(facts[j].CreatedAt) })

	var vulns []*database.Vulnerability
	for pid := range projectIDs {
		vs, err := g.db.ListVulnerabilities(30, 0, database.VulnerabilityListFilter{ProjectID: pid})
		if err == nil {
			vulns = append(vulns, vs...)
		}
	}

	factKeys := make([]string, 0, len(facts))
	for _, f := range facts {
		factKeys = append(factKeys, f.FactKey)
	}
	sort.Strings(factKeys)
	factKeysJSON, _ := json.Marshal(factKeys)

	exists, err := RecentFactKeysExists(g.db.DB, string(factKeysJSON))
	if err == nil && exists {
		return fmt.Errorf("duplicate draft within 24h")
	}

	title, content, err := g.llmDraft(facts, vulns)
	if err != nil {
		return fmt.Errorf("llm draft: %w", err)
	}
	maxRunes := g.cfg.Experience.ContentMaxRunes
	if maxRunes <= 0 {
		maxRunes = 4000
	}
	if r := []rune(content); len(r) > maxRunes {
		content = string(r[:maxRunes])
	}

	d := &Draft{
		ProjectID:  firstKey(projectIDs),
		Source:     source,
		SourceType: sourceType,
		Title:      title,
		Content:    redact(content),
		Category:   "methodology",
		FactKeys:   string(factKeysJSON),
	}
	if _, err := CreateDraft(g.db.DB, d); err != nil {
		return err
	}
	g.logger.Infow("experience draft created", "source", source, "type", sourceType, "facts", len(facts))
	return nil
}

// llmDraft asks the configured (independent) LLM to distill a reusable lesson.
func (g *Generator) llmDraft(facts []*database.ProjectFact, vulns []*database.Vulnerability) (string, string, error) {
	llm := g.cfg.Experience.LLM
	baseURL := strings.TrimSpace(llm.BaseURL)
	apiKey := strings.TrimSpace(llm.APIKey)
	model := strings.TrimSpace(llm.Model)
	if baseURL == "" {
		baseURL = g.cfg.OpenAI.BaseURL
	}
	if apiKey == "" {
		apiKey = g.cfg.OpenAI.APIKey
	}
	if model == "" {
		model = g.cfg.OpenAI.Model
	}
	if apiKey == "" || model == "" {
		return "", "", fmt.Errorf("experience llm not configured (api_key/model empty)")
	}

	var mat strings.Builder
	mat.WriteString("黑板事实（key / 置信度 / 摘要）:\n")
	for i, f := range facts {
		if i >= 40 {
			mat.WriteString("…(其余省略)\n")
			break
		}
		summary := f.Summary
		if r := []rune(summary); len(r) > 200 {
			summary = string(r[:200])
		}
		fmt.Fprintf(&mat, "- %s [%s] %s\n", f.FactKey, f.Confidence, summary)
	}
	if len(vulns) > 0 {
		mat.WriteString("\n漏洞记录（标题 / 严重度 / 状态）:\n")
		for i, v := range vulns {
			if i >= 20 {
				break
			}
			fmt.Fprintf(&mat, "- %s [%s/%s]\n", v.Title, v.Severity, v.Status)
		}
	}

	payload := map[string]interface{}{
		"model": model,
		"messages": []map[string]string{
			{"role": "system", "content": "你是安全评估经验总结助手。基于项目黑板事实与漏洞记录，蒸馏一条跨项目可复用的方法论经验。" +
				"要求：1) 只写可复用的检测顺序/误报辨别/换路思路，不写具体目标 IP、域名、内网路径、Cookie、凭据；" +
				"2) 用中文，条目式，简洁；3) 只输出 JSON：{\"title\": \"短标题\", \"content\": \"正文\"}，不要输出其他文字。"},
			{"role": "user", "content": mat.String()},
		},
		"max_completion_tokens": 1200,
	}

	client := openai.NewClient(&config.OpenAIConfig{
		Provider: g.cfg.OpenAI.Provider,
		BaseURL:  baseURL,
		APIKey:   apiKey,
		Model:    model,
	}, nil, g.logger.Desugar())

	ctx, cancel := context.WithTimeout(context.Background(), 90*time.Second)
	defer cancel()

	var resp struct {
		Choices []struct {
			Message struct {
				Content string `json:"content"`
			} `json:"message"`
		} `json:"choices"`
	}
	if err := client.ChatCompletion(ctx, payload, &resp); err != nil {
		return "", "", err
	}
	if len(resp.Choices) == 0 {
		return "", "", fmt.Errorf("empty completion")
	}
	return parseDraftJSON(resp.Choices[0].Message.Content)
}

// parseDraftJSON extracts {title, content}, tolerating code fences.
func parseDraftJSON(s string) (string, string, error) {
	t := strings.TrimSpace(s)
	if i := strings.Index(t, "{"); i >= 0 {
		if j := strings.LastIndex(t, "}"); j > i {
			t = t[i : j+1]
		}
	}
	var out struct {
		Title   string `json:"title"`
		Content string `json:"content"`
	}
	if err := json.Unmarshal([]byte(t), &out); err != nil {
		return "", "", fmt.Errorf("model did not return JSON: %.120s", s)
	}
	out.Title = strings.TrimSpace(out.Title)
	out.Content = strings.TrimSpace(out.Content)
	if out.Title == "" || out.Content == "" {
		return "", "", fmt.Errorf("draft title/content empty")
	}
	return out.Title, out.Content, nil
}

// ipv4Pattern / redact strip one-off infrastructure details.
var ipv4Pattern = regexp.MustCompile(`\b\d{1,3}\.\d{1,3}\.\d{1,3}\.\d{1,3}\b`)

func redact(s string) string {
	return ipv4Pattern.ReplaceAllString(s, "x.x.x.x")
}

// cveIDPattern extracts a CVE id from free text (vuln title/description/steps).
var cveIDPattern = regexp.MustCompile(`\bCVE-\d{4}-\d{4,}\b`)

// pocUnsafeKeyChars: path separators / traversal / FS-illegal characters.
var pocUnsafeKeyChars = regexp.MustCompile(`[\\/:*?"<>|\r\n\t]`)

// SanitizePocKey turns a free-text key (a no-CVE vulnerability title such as
// 「某OA系统 getfile 任意文件读取」) into a safe single poc/ filename stem.
// Chinese and common punctuation survive; separators and traversal are
// stripped so the result can never escape the poc directory.
func SanitizePocKey(s string) string {
	s = pocUnsafeKeyChars.ReplaceAllString(strings.TrimSpace(s), " ")
	s = strings.Join(strings.Fields(s), "-")
	s = strings.Trim(s, "-.")
	if s == "" {
		return ""
	}
	r := []rune(s)
	if len(r) > 60 {
		s = string(r[:60])
	}
	return s
}

// collectPocDrafts emits one POC entry per CONFIRMED vulnerability — CVE or
// not. CVE-tagged entries key on the id (poc/CVE-YYYY-NNNN.md); no-CVE ones
// (logic flaws, unauthorized access, weak creds, custom systems) key on the
// sanitized vulnerability title「系统名+漏洞类型」, so a custom system taken
// down without any CVE still lands in the library and repeats accumulate as
// dated sections. Default mode (poc_auto_apply, on unless disabled): entries
// are written straight into the local POC library — the draft row is still
// created and immediately marked approved (reviewer="auto") so the operator
// can see exactly what landed, and one-shot dedup still applies. If
// auto-write fails (or poc_auto_apply=false) the row stays a normal draft
// for human approval.
func (g *Generator) collectPocDrafts(projectIDs map[string]bool) {
	auto := g.cfg.Experience.PocAutoApplyEffective() && g.autoApplier != nil
	for pid := range projectIDs {
		vulns, err := g.db.ListVulnerabilities(100, 0,
			database.VulnerabilityListFilter{ProjectID: pid, Status: "confirmed"})
		if err != nil {
			continue
		}
		for _, v := range vulns {
			if v == nil || strings.TrimSpace(v.Title) == "" {
				continue
			}
			// 检索键：有 CVE 用编号；没有则用「系统名+漏洞类型」（标题清洗）。
			cve := firstCVEID(v.Title, v.Description, v.ReproSteps)
			key := cve
			if key == "" {
				key = SanitizePocKey(v.Title)
				if key == "" {
					continue
				}
			}
			exists, err := DraftExistsBySource(g.db.DB, "poc", v.ID)
			if err != nil || exists {
				continue // one entry per vulnerability ever (rejected = stay rejected)
			}
			// fact_keys 存检索键（CVE 编号或标题键）：人工批准时弹窗据此预填
			// applied_to=poc:<key>，同时也是草稿的可检索标签。
			fk, _ := json.Marshal([]string{key})
			d := &Draft{
				ProjectID:  pid,
				Source:     v.ID,
				SourceType: "poc",
				Title:      "POC " + key + " — " + firstRunes(v.Title, 60),
				Content:    buildPocDraftContent(v, cve, key, !auto),
				Category:   "poc",
				FactKeys:   string(fk),
			}
			id, err := CreateDraft(g.db.DB, d)
			if err != nil {
				g.logger.Warnw("poc draft create failed", "vuln", v.ID, "error", err)
				continue
			}
			if auto {
				if dest, aerr := g.autoApplier(d, "poc:"+key); aerr == nil {
					if uerr := ApproveDraft(g.db.DB, id, "auto", dest); uerr == nil {
						g.logger.Infow("poc auto-applied", "key", key, "vuln", v.ID, "dest", dest)
						continue
					}
				} else {
					g.logger.Warnw("poc auto-apply failed — left for human approval",
						"key", key, "vuln", v.ID, "error", aerr)
				}
			}
			g.logger.Infow("poc draft created (pending review)", "key", key, "vuln", v.ID)
		}
	}
}

// firstCVEID returns the first CVE id found, title first.
func firstCVEID(fields ...string) string {
	for _, f := range fields {
		if m := cveIDPattern.FindString(strings.ToUpper(f)); m != "" {
			return m
		}
	}
	return ""
}

// buildPocDraftContent assembles the POC record from the vulnerability
// fields verbatim (IPv4-redacted). No LLM rewrite: commands and payloads
// must survive byte-for-byte. cve may be empty (no-CVE exploits key on the
// title). manual=true (human-review mode) prefixes the approve guidance;
// auto mode notes it has already been written.
func buildPocDraftContent(v *database.Vulnerability, cve, key string, manual bool) string {
	var b strings.Builder
	if manual {
		b.WriteString("> POC 沉淀草稿：批准时 applied_to 填 `poc:" + key + "`，将写入本地实战库 data/corpus/poc/。\n")
		b.WriteString("> 内容机械摘自漏洞记录并已做 IPv4 脱敏；目标域名/路径/凭据是否保留请人工过目后再批准。\n\n")
	} else {
		b.WriteString("> 已自动写入本地实战库 data/corpus/poc/" + key + ".md（poc_auto_apply 默认开启）。\n")
		b.WriteString("> 内容机械摘自漏洞记录并已做 IPv4 脱敏。此记录仅存本机，不出公开仓库、不被每日同步覆盖。\n\n")
	}
	if cve != "" {
		fmt.Fprintf(&b, "- CVE: %s\n", cve)
	}
	fmt.Fprintf(&b, "- 标题: %s\n", v.Title)
	fmt.Fprintf(&b, "- 类型 / 严重度: %s / %s\n", v.Type, v.Severity)
	if v.Target != "" {
		fmt.Fprintf(&b, "- 目标: %s\n", redact(v.Target))
	}
	if v.Preconditions != "" {
		fmt.Fprintf(&b, "\n## 利用前提\n\n%s\n", redact(v.Preconditions))
	}
	if v.ReproSteps != "" {
		fmt.Fprintf(&b, "\n## 复现步骤\n\n%s\n", redact(v.ReproSteps))
	}
	if v.Evidence != "" {
		fmt.Fprintf(&b, "\n## 证据\n\n%s\n", redact(v.Evidence))
	}
	if v.Impact != "" {
		fmt.Fprintf(&b, "\n## 影响\n\n%s\n", redact(v.Impact))
	}
	return b.String()
}

func firstRunes(s string, n int) string {
	r := []rune(s)
	if len(r) <= n {
		return s
	}
	return string(r[:n])
}

func firstKey(m map[string]bool) string {
	for k := range m {
		return k
	}
	return ""
}

// RecentFactKeysExists reports a non-rejected draft with the same fact_keys
// set created within 24h (the dedup gate; fact_keys is the sorted material
// fingerprint, stored by CreateDraft).
func RecentFactKeysExists(db *sql.DB, factKeysJSON string) (bool, error) {
	if strings.TrimSpace(factKeysJSON) == "" || factKeysJSON == "[]" {
		return false, nil
	}
	var n int
	err := db.QueryRow(`SELECT COUNT(*) FROM experience_drafts
		WHERE fact_keys = ? AND status != 'rejected'
		AND created_at > datetime('now', '-1 day')`, factKeysJSON).Scan(&n)
	if err != nil {
		return false, err
	}
	return n > 0, nil
}
