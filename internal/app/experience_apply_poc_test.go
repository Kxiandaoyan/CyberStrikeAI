package app

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"cyberstrike-ai/internal/experience"
)

func TestApplyPocDraftCreateAndAppend(t *testing.T) {
	corpusDir := filepath.Join(t.TempDir(), "corpus")
	d1 := &experience.Draft{Title: "POC CVE-2024-23897 — Jenkins file read", Content: "第一次交战: read /etc/passwd"}

	p1, err := applyExperienceDraft("", corpusDir, nil, d1, "poc:cve-2024-23897")
	if err != nil {
		t.Fatalf("create: %v", err)
	}
	want := filepath.Join(corpusDir, "poc", "CVE-2024-23897.md")
	if p1 != want {
		t.Fatalf("path = %q, want %q", p1, want)
	}
	data, err := os.ReadFile(want)
	if err != nil || !strings.Contains(string(data), "第一次交战") || !strings.Contains(string(data), "cve: CVE-2024-23897") {
		t.Fatalf("created file wrong:\n%s (%v)", string(data), err)
	}

	// Second approval appends a dated section, keeps the first.
	d2 := &experience.Draft{Title: "POC CVE-2024-23897 — again", Content: "第二次交战: 变体打法"}
	if _, err := applyExperienceDraft("", corpusDir, nil, d2, "poc:CVE-2024-23897"); err != nil {
		t.Fatalf("append: %v", err)
	}
	data2, _ := os.ReadFile(want)
	if !strings.Contains(string(data2), "第一次交战") || !strings.Contains(string(data2), "第二次交战") {
		t.Fatalf("append lost history:\n%s", string(data2))
	}
	if got := strings.Count(string(data2), "## 实战补记"); got != 2 {
		t.Fatalf("expected 2 sections, got %d:\n%s", got, string(data2))
	}
}

func TestApplyPocDraftNoCVEKey(t *testing.T) {
	corpusDir := filepath.Join(t.TempDir(), "corpus")
	d := &experience.Draft{Title: "某OA getfile 任意文件读取", Content: "第一次: POST /getfile 读 /etc/passwd"}

	p, err := applyExperienceDraft("", corpusDir, nil, d, "poc:某OA系统 getfile 任意文件读取")
	if err != nil {
		t.Fatalf("create: %v", err)
	}
	want := filepath.Join(corpusDir, "poc", "某OA系统-getfile-任意文件读取.md")
	if p != want {
		t.Fatalf("path = %q, want %q", p, want)
	}
	data, _ := os.ReadFile(want)
	if !strings.Contains(string(data), "第一次") || !strings.Contains(string(data), "实战利用记录") {
		t.Fatalf("created file wrong:\n%s", string(data))
	}
	if strings.Contains(string(data), "cve:") {
		t.Fatalf("no-CVE entry must not carry cve frontmatter:\n%s", string(data))
	}

	// Same custom system taken down again → appends a dated section.
	if _, err := applyExperienceDraft("", corpusDir, nil,
		&experience.Draft{Title: "again", Content: "第二次: 变体"}, "poc:某OA系统 getfile 任意文件读取"); err != nil {
		t.Fatalf("append: %v", err)
	}
	data2, _ := os.ReadFile(want)
	if strings.Count(string(data2), "## 实战补记") != 2 || !strings.Contains(string(data2), "第二次") {
		t.Fatalf("append lost history:\n%s", string(data2))
	}
}

func TestApplyPocDraftRejectsBadIDs(t *testing.T) {
	corpusDir := filepath.Join(t.TempDir(), "corpus")
	d := &experience.Draft{Title: "x", Content: "y"}
	for _, bad := range []string{"poc:../../etc/passwd", "poc:a/b/c", "poc:", "poc:..."} {
		if _, err := applyExperienceDraft("", corpusDir, nil, d, bad); err == nil {
			t.Fatalf("expected error for %q", bad)
		}
	}
	// Nothing may have been created outside the poc dir.
	if _, err := os.Stat(corpusDir); !os.IsNotExist(err) {
		entries, _ := os.ReadDir(corpusDir)
		t.Fatalf("corpus dir should stay empty/absent, has %d entries", len(entries))
	}
}
