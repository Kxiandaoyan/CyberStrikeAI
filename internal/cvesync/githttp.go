// Package cvesync — pure-HTTP incremental sync against CVEProject/cvelistV5.
//
// Why not `git pull`: the controller forbids subprocess execution, so the
// daily delta is derived from the GitHub REST API instead — commits since the
// last successful sync, then compare(base...head) per batch to obtain the
// changed CVE JSON paths. Raw file contents come from the public CDN host.
// Only fixed HTTPS public hosts are contacted; paths returned by the API are
// strictly validated before touching the filesystem.
package cvesync

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"regexp"
	"time"
)

const (
	githubAPIBase = "https://api.github.com"
	githubRawBase = "https://raw.githubusercontent.com"
	cveRepo       = "CVEProject/cvelistV5"
	cveRepoBranch = "main"

	// compareAPIFileCap is GitHub's documented per-compare file limit.
	compareAPIFileCap = 300
	// commitsPerBatch keeps each compare's file count safely below the cap
	// (~7min commit cadence → 80 commits ≈ 9h of upstream changes).
	commitsPerBatch = 80
	// maxCommitPages bounds the catch-up window (10 pages x 100 commits).
	maxCommitPages = 10
)

// cvePathPattern is the ONLY shape we accept from the API before writing to
// disk. The official repo stores records in bucket directories:
//
//	cves/{year}/{prefix}xxx/CVE-{year}-{id}.json   (e.g. cves/2026/86xxx/CVE-2026-86172.json)
//
// Anything else (delta.json, deltaLog.json, docs…) is rejected, as is any
// traversal attempt.
var cvePathPattern = regexp.MustCompile(`^cves/(20\d{2})/\d{1,5}xxx/CVE-(20\d{2})-\d{4,}\.json$`)

// fileChange is one changed raw file (path relative to the repo root).
type fileChange struct {
	Path      string // current path, e.g. cves/2024/CVE-2024-23897.json
	PrevPath  string // set on renames
	Removed   bool   // removed, or the old side of a rename
	Timestamp time.Time
}

type ghCommit struct {
	SHA    string `json:"sha"`
	Commit struct {
		Committer struct {
			Date time.Time `json:"date"`
		} `json:"committer"`
	} `json:"commit"`
	Parents []struct {
		SHA string `json:"sha"`
	} `json:"parents"`
}

type ghFile struct {
	Filename     string `json:"filename"`
	Status       string `json:"status"`
	PreviousName string `json:"previous_filename"`
}

type ghCompare struct {
	Status       string   `json:"status"`
	TotalCommits int      `json:"total_commits"`
	Files        []ghFile `json:"files"`
}

// ghHTTP returns the GitHub API token from the environment (optional).
func ghToken() string { return os.Getenv("CVE_GITHUB_TOKEN") }

func ghDoJSON(ctx context.Context, client *http.Client, rawURL string, out interface{}) error {
	req, err := http.NewRequestWithContext(ctx, "GET", rawURL, nil)
	if err != nil {
		return err
	}
	req.Header.Set("Accept", "application/vnd.github+json")
	req.Header.Set("User-Agent", "CyberStrikeAI-cvesync")
	if t := ghToken(); t != "" {
		req.Header.Set("Authorization", "Bearer "+t)
	}
	resp, err := client.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	body, _ := io.ReadAll(io.LimitReader(resp.Body, 64<<20))
	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("GET %s: HTTP %d: %.200s", redactURL(rawURL), resp.StatusCode, string(body))
	}
	if out == nil {
		return nil
	}
	return json.Unmarshal(body, out)
}

// redactURL strips query strings (may carry tokens) for logging.
func redactURL(rawURL string) string {
	if u, err := url.Parse(rawURL); err == nil {
		u.RawQuery = ""
		return u.String()
	}
	return rawURL
}

// githubHeadSHA returns the current main branch head.
func githubHeadSHA(ctx context.Context, client *http.Client) (string, error) {
	var c ghCommit
	// This endpoint redirects to the commit; the JSON shape is the same.
	if err := ghDoJSON(ctx, client, githubAPIBase+"/repos/"+cveRepo+"/commits/"+cveRepoBranch, &c); err != nil {
		return "", err
	}
	if c.SHA == "" {
		return "", fmt.Errorf("empty head sha")
	}
	return c.SHA, nil
}

// githubCommitsSince lists commits newer than since (newest first), bounded
// by maxCommitPages pages. Returns oldest-first order. Hitting the page cap
// is an error, not a silent gap: a corpus that old needs a full refresh.
func githubCommitsSince(ctx context.Context, client *http.Client, since time.Time) ([]ghCommit, error) {
	var all []ghCommit
	for page := 1; page <= maxCommitPages; page++ {
		q := url.Values{}
		q.Set("since", since.UTC().Format(time.RFC3339))
		q.Set("per_page", "100")
		q.Set("page", fmt.Sprintf("%d", page))
		var batch []ghCommit
		err := ghDoJSON(ctx, client,
			githubAPIBase+"/repos/"+cveRepo+"/commits?"+q.Encode(), &batch)
		if err != nil {
			return nil, err
		}
		all = append(all, batch...)
		if len(batch) < 100 {
			return reverseOldestFirst(all), nil
		}
		if page == maxCommitPages {
			return nil, fmt.Errorf("catch-up window exceeded %d commits since %s — run a full sync (POST /api/cve-corpus/sync?full=1)",
				maxCommitPages*100, since.UTC().Format(time.RFC3339))
		}
	}
	return reverseOldestFirst(all), nil
}

func reverseOldestFirst(all []ghCommit) []ghCommit {
	for i, j := 0, len(all)-1; i < j; i, j = i+1, j-1 {
		all[i], all[j] = all[j], all[i]
	}
	return all
}

// githubCompareFiles returns the changed files between two commits.
func githubCompareFiles(ctx context.Context, client *http.Client, base, head string) ([]ghFile, error) {
	var cmp ghCompare
	err := ghDoJSON(ctx, client,
		githubAPIBase+"/repos/"+cveRepo+"/compare/"+base+"..."+head, &cmp)
	if err != nil {
		return nil, err
	}
	if cmp.Status == "diverged" {
		return nil, fmt.Errorf("history diverged (base %s not an ancestor of head)", base[:min(len(base), 8)])
	}
	if len(cmp.Files) >= compareAPIFileCap {
		return nil, fmt.Errorf("compare %s...%s hit the %d-file API cap",
			base[:min(len(base), 8)], head[:min(len(head), 8)], compareAPIFileCap)
	}
	return cmp.Files, nil
}

// githubRawFile downloads one file's content from the public CDN.
func githubRawFile(ctx context.Context, client *http.Client, path string) ([]byte, error) {
	rawURL := githubRawBase + "/" + cveRepo + "/" + cveRepoBranch + "/" + path
	req, err := http.NewRequestWithContext(ctx, "GET", rawURL, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("User-Agent", "CyberStrikeAI-cvesync")
	resp, err := client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("raw GET %s: HTTP %d", path, resp.StatusCode)
	}
	return io.ReadAll(io.LimitReader(resp.Body, 4<<20))
}

// planChanges derives the full change set since the given time using batched
// compares. Returned head is the repo head at planning time.
func planChanges(ctx context.Context, client *http.Client, since time.Time) ([]fileChange, string, error) {
	head, err := githubHeadSHA(ctx, client)
	if err != nil {
		return nil, "", fmt.Errorf("head: %w", err)
	}
	commits, err := githubCommitsSince(ctx, client, since)
	if err != nil {
		return nil, "", fmt.Errorf("commits since: %w", err)
	}
	if len(commits) == 0 {
		return nil, head, nil
	}

	byPath := make(map[string]fileChange) // later batches overwrite earlier
	for start := 0; start < len(commits); start += commitsPerBatch {
		end := start + commitsPerBatch
		if end > len(commits) {
			end = len(commits)
		}
		batch := commits[start:end]
		base := batch[0].SHA
		if len(batch[0].Parents) > 0 && batch[0].Parents[0].SHA != "" {
			base = batch[0].Parents[0].SHA
		}
		tip := batch[len(batch)-1].SHA
		files, err := githubCompareFiles(ctx, client, base, tip)
		if err != nil {
			return nil, "", fmt.Errorf("batch compare: %w", err)
		}
		ts := batch[len(batch)-1].Commit.Committer.Date
		for _, f := range files {
			if !isCVEPath(f.Filename) {
				continue // docs, schemas, workflows — not CVE records
			}
			switch f.Status {
			case "removed":
				byPath[f.Filename] = fileChange{Path: f.Filename, Removed: true, Timestamp: ts}
			case "renamed":
				if isCVEPath(f.PreviousName) {
					byPath[f.PreviousName] = fileChange{Path: f.PreviousName, Removed: true, Timestamp: ts}
				}
				byPath[f.Filename] = fileChange{Path: f.Filename, PrevPath: f.PreviousName, Timestamp: ts}
			default: // added / modified / changed
				byPath[f.Filename] = fileChange{Path: f.Filename, Timestamp: ts}
			}
		}
	}

	out := make([]fileChange, 0, len(byPath))
	for _, c := range byPath {
		out = append(out, c)
	}
	return out, head, nil
}

// isCVEPath validates the strict cves/{year}/CVE-...json shape.
func isCVEPath(p string) bool {
	m := cvePathPattern.FindStringSubmatch(p)
	return m != nil && m[1] == m[2]
}
