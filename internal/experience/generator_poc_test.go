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
	content := buildPocDraftContent(v, "CVE-2024-99999")
	if strings.Contains(content, "10.1.2.3") || strings.Contains(content, "192.168.1.10") || strings.Contains(content, "10.0.0.5") {
		t.Fatalf("IPv4 not redacted:\n%s", content)
	}
	if !strings.Contains(content, "CVE-2024-99999") || !strings.Contains(content, "## 复现步骤") {
		t.Fatalf("template missing fields:\n%s", content)
	}
	if !strings.Contains(content, "x.x.x.x") {
		t.Fatalf("redaction placeholder missing:\n%s", content)
	}
}
