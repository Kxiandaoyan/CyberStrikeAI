// Package app — experience draft persistence on human approval (spec §7).
//
// Targets:
//   - "skill:<name>"  append an「经验补丁」section to skills/<name>/SKILL.md,
//     creating a minimal skill when it does not exist yet
//   - "knowledge"     knowledge.CreateItem("经验总结", title, content)
//
// Only ever invoked from the human approve endpoint — auto_apply is locked
// false in config.Load.
package app

import (
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"time"

	"cyberstrike-ai/internal/experience"
	"cyberstrike-ai/internal/knowledge"
)

// skillNamePattern mirrors the Agent Skills name rule enforced by
// skillpackage (lowercase letters, digits, single hyphens) and doubles as
// traversal protection for the on-disk skill path.
var skillNamePattern = regexp.MustCompile(`^[a-z0-9]+(-[a-z0-9]+)*$`)

// applyExperienceDraft persists an approved draft and returns the actual
// destination (path or knowledge item id) recorded on the draft row.
func applyExperienceDraft(skillsDir string, km *knowledge.Manager, d *experience.Draft, appliedTo string) (string, error) {
	target := strings.TrimSpace(appliedTo)
	if target == "" {
		target = "knowledge"
	}

	if strings.HasPrefix(target, "skill:") {
		name := strings.TrimSpace(strings.TrimPrefix(target, "skill:"))
		if !skillNamePattern.MatchString(name) {
			return "", fmt.Errorf("invalid skill name %q — lowercase letters, digits and single hyphens only", name)
		}
		dir := filepath.Join(skillsDir, name)
		skillMD := filepath.Join(dir, "SKILL.md")
		section := fmt.Sprintf("\n\n## 经验补丁（%s）\n\n%s\n", time.Now().Format("2006-01-02"), d.Content)

		if _, err := os.Stat(skillMD); err == nil {
			f, err := os.OpenFile(skillMD, os.O_APPEND|os.O_WRONLY, 0644)
			if err != nil {
				return "", err
			}
			defer f.Close()
			if _, err := f.WriteString(section); err != nil {
				return "", err
			}
			return skillMD, nil
		}
		if err := os.MkdirAll(dir, 0755); err != nil {
			return "", err
		}
		title := truncateRunes(d.Title, 80)
		content := fmt.Sprintf("---\nname: %s\ndescription: >-\n  %s\n---\n\n# %s\n%s",
			name, title, title, section)
		if err := os.WriteFile(skillMD, []byte(content), 0644); err != nil {
			return "", err
		}
		return skillMD, nil
	}

	if target == "knowledge" {
		if km == nil {
			return "", fmt.Errorf("知识库未启用 — 前往设置开启知识库，或改用 applied_to=skill:<name>")
		}
		item, err := km.CreateItem("经验总结", d.Title, d.Content)
		if err != nil {
			return "", err
		}
		return "knowledge:" + item.ID, nil
	}

	return "", fmt.Errorf("unsupported applied_to %q — use skill:<name> or knowledge", appliedTo)
}
