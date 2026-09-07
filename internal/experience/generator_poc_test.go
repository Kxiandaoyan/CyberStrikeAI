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

func TestBuildPocDraftContentRedacts(t *testing.T) {
	v := &database.Vulnerability{
		Title:         "RCE via deserialization (CVE-2024-99999)",
		Type:          "rce",
		Severity:      "critical",
		Target:        "http://10.1.2.3:8080",
		Preconditions: "auth bypass needed on 192.168.1.10",
		ReproSteps:    "1. send payload to 10.0.0.5:9001\n2. get shell",
	}
	content := buildPocDraftContent(v, "CVE-2024-99999", true)
	autoContent := buildPocDraftContent(v, "CVE-2024-99999", false)
	for name, c := range map[string]string{"manual": content, "auto": autoContent} {
		if strings.Contains(c, "10.1.2.3") || strings.Contains(c, "192.168.1.10") || strings.Contains(c, "10.0.0.5") {
			t.Fatalf("IPv4 not redacted (%s):\n%s", name, c)
		}
		if !strings.Contains(c, "CVE-2024-99999") || !strings.Contains(c, "## 复现步骤") {
			t.Fatalf("template missing fields (%s):\n%s", name, c)
		}
		if !strings.Contains(c, "x.x.x.x") {
			t.Fatalf("redaction placeholder missing (%s):\n%s", name, c)
		}
	}
	if !strings.Contains(content, "批准时") || !strings.Contains(autoContent, "已自动写入") {
		t.Fatal("mode-specific guidance missing")
	}
}
