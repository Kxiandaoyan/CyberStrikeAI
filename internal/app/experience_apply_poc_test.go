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

func TestApplyPocDraftRejectsBadIDs(t *testing.T) {
	corpusDir := filepath.Join(t.TempDir(), "corpus")
	d := &experience.Draft{Title: "x", Content: "y"}
	for _, bad := range []string{"poc:../../etc/passwd", "poc:CVE-abcd", "poc:", "poc:cve-2024"} {
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
