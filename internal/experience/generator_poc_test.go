package experience

import (
	"strings"
	"testing"

	"cyberstrike-ai/internal/database"
)

func TestFirstCVEID(t *testing.T) {
	cases := []struct {
		fields []string
		want   string
	}{
		{[]string{"Jenkins 任意文件读取 (CVE-2024-23897)"}, "CVE-2024-23897"},
		{[]string{"no id here", "desc mentions cve-2023-12345 lowercase"}, "CVE-2023-12345"},
		{[]string{"nothing"}, ""},
		{[]string{"steps: exploit CVE-2021-44228 worked"}, "CVE-2021-44228"},
	}
	for _, c := range cases {
		if got := firstCVEID(c.fields...); got != c.want {
			t.Errorf("firstCVEID(%v) = %q, want %q", c.fields, got, c.want)
		}
	}
}

func TestSanitizePocKey(t *testing.T) {
	cases := []struct{ in, want string }{
		{"某OA系统 getfile 任意文件读取", "某OA系统-getfile-任意文件读取"},
		{"  多个   空格\t换行 ", "多个-空格-换行"},
		{"a/b\\c:d*e?f\"g<h>i|j", "a-b-c-d-e-f-g-h-i-j"},
		{"../../etc/passwd", "etc-passwd"}, // separators die, leading dots trimmed — traversal impossible
		{"", ""},
		{"   ", ""},
		{"。", "。"},
	}
	for _, c := range cases {
		if got := SanitizePocKey(c.in); got != c.want {
			t.Errorf("SanitizePocKey(%q) = %q, want %q", c.in, got, c.want)
		}
	}
	if got := SanitizePocKey(strings.Repeat("长", 100)); len([]rune(got)) != 60 {
		t.Errorf("length cap failed: %d runes", len([]rune(got)))
	}
}

func TestBuildPocDraftContentRedacts(t *testing.T) {
	v := &database.Vulnerability{
		Title:         "RCE via deserialization (CVE-2024-99999)",
		Type:          "rce",
		Severity:      "critical",
		Target:        "http://10.1.2.3:8080",
		Preconditions: "auth bypass needed on 192.168.1.10",
		ReproSteps:    "1. send payload to 10.0.0.5:9001\n2. get shell",
	}
	content := buildPocDraftContent(v, "CVE-2024-99999", "CVE-2024-99999", true)
	autoContent := buildPocDraftContent(v, "CVE-2024-99999", "CVE-2024-99999", false)
	noCveContent := buildPocDraftContent(v, "", "某OA系统-getfile-任意文件读取", true)
	for name, c := range map[string]string{"manual": content, "auto": autoContent, "nocve": noCveContent} {
		if strings.Contains(c, "10.1.2.3") || strings.Contains(c, "192.168.1.10") || strings.Contains(c, "10.0.0.5") {
			t.Fatalf("IPv4 not redacted (%s):\n%s", name, c)
		}
		if !strings.Contains(c, "## 复现步骤") {
			t.Fatalf("template missing fields (%s):\n%s", name, c)
		}
		if !strings.Contains(c, "x.x.x.x") {
			t.Fatalf("redaction placeholder missing (%s):\n%s", name, c)
		}
	}
	if !strings.Contains(content, "- CVE: CVE-2024-99999") {
		t.Fatal("CVE line missing for CVE-tagged entry")
	}
	if strings.Contains(noCveContent, "- CVE:") {
		t.Fatal("CVE line must be absent for no-CVE entry")
	}
	if !strings.Contains(noCveContent, "poc:某OA系统-getfile-任意文件读取") {
		t.Fatal("no-CVE manual guidance missing key")
	}
	if !strings.Contains(content, "批准时") || !strings.Contains(autoContent, "已自动写入") {
		t.Fatal("mode-specific guidance missing")
	}
}
