package cvesync

import (
	"context"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

// TestPlanChangesLiveSmoke exercises the real GitHub API chain used by the
// daily sync: head → commits-since → batched compare → path filter.
// The raw-CDN fetch + JSON→Markdown round trip runs as the last step and is
// skipped independently when raw.githubusercontent.com is unreachable from
// the current network.
//
// Run with: CS_CVESYNC_SMOKE=1 go test ./internal/cvesync/ -run LiveSmoke -v
func TestPlanChangesLiveSmoke(t *testing.T) {
	if os.Getenv("CS_CVESYNC_SMOKE") != "1" {
		t.Skip("set CS_CVESYNC_SMOKE=1 to run the live GitHub smoke test")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 120*time.Second)
	defer cancel()
	client := &http.Client{Timeout: 30 * time.Second}

	head, err := githubHeadSHA(ctx, client)
	if err != nil {
		t.Fatalf("githubHeadSHA: %v", err)
	}
	if len(head) != 40 {
		t.Fatalf("head sha length = %d, want 40", len(head))
	}
	t.Logf("head = %.12s…", head)

	changes, head2, err := planChanges(ctx, client, time.Now().Add(-24*time.Hour))
	if err != nil {
		t.Fatalf("planChanges (last 24h): %v", err)
	}
	if head2 != head {
		t.Fatalf("plan head %s != head %s", head2, head)
	}
	// Diagnostics when the plan is suspiciously empty.
	if len(changes) == 0 {
		commits, cerr := githubCommitsSince(ctx, client, time.Now().Add(-24*time.Hour))
		if cerr != nil {
			t.Fatalf("empty plan; commits diagnostic failed: %v", cerr)
		}
		t.Logf("empty plan; commits API returned %d commits in window", len(commits))
		for i := 0; i < len(commits) && i < 3; i++ {
			base := commits[i].SHA
			if len(commits[i].Parents) > 0 && commits[i].Parents[0].SHA != "" {
				base = commits[i].Parents[0].SHA
			}
			files, ferr := githubCompareFiles(ctx, client, base, commits[i].SHA)
			if ferr != nil {
				t.Logf("commit[%d] %.10s compare error: %v", i, commits[i].SHA, ferr)
				continue
			}
			names := make([]string, 0, 3)
			for j, f := range files {
				if j >= 3 {
					break
				}
				names = append(names, f.Filename)
			}
			t.Logf("commit[%d] %.10s → %d files: %v", i, commits[i].SHA, len(files), names)
		}
	}
	t.Logf("changed CVE files in the last 24h: %d", len(changes))
	removed, added := 0, 0
	for _, ch := range changes {
		if !isCVEPath(ch.Path) {
			t.Fatalf("non-CVE path leaked into the plan: %q", ch.Path)
		}
		if ch.Removed {
			removed++
		} else {
			added++
		}
	}
	t.Logf("plan breakdown: +%d add/modify, -%d removed", added, removed)

	// Raw fetch + extract round trip on the first live change.
	for _, ch := range changes {
		if ch.Removed {
			continue
		}
		rawCtx, rawCancel := context.WithTimeout(ctx, 30*time.Second)
		data, rerr := githubRawFile(rawCtx, client, ch.Path)
		rawCancel()
		if rerr != nil {
			t.Skipf("raw CDN unreachable from this network (deploy targets are not): %v", rerr)
		}
		tmp := filepath.Join(t.TempDir(), "cve.json")
		if werr := os.WriteFile(tmp, data, 0644); werr != nil {
			t.Fatalf("write temp json: %v", werr)
		}
		rec, eerr := ExtractFile(tmp, 2000)
		if eerr != nil {
			t.Fatalf("ExtractFile: %v", eerr)
		}
		if rec == nil || rec.ID == "" || !strings.HasPrefix(rec.ID, "CVE-") {
			t.Fatalf("extract produced no CVE id for %s", ch.Path)
		}
		md := rec.ToMarkdown()
		if !strings.Contains(md, rec.ID) || !strings.HasPrefix(md, "---\n") {
			t.Fatalf("markdown round trip broken for %s", rec.ID)
		}
		t.Logf("raw %s fetched (%d bytes) → %s [%s], md %d bytes",
			ch.Path, len(data), rec.ID, rec.State, len(md))
		return
	}
	t.Log("no non-removed change in the window; raw round trip not exercised")
}
