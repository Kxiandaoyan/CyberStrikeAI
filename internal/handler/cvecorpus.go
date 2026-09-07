// Package handler — CVE corpus search API (paginated browse + search).
// All file paths come from os.ReadDir scan (trusted), never from user input.
package handler

import (
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"sync/atomic"

	"github.com/gin-gonic/gin"
	"go.uber.org/zap"

	"cyberstrike-ai/internal/cvesync"
)

// CveCorpusHandler serves the local CVE corpus.
type CveCorpusHandler struct {
	corpusDir   string
	logger      *zap.SugaredLogger
	syncTrigger func(full bool) error // wired by app.go; nil = sync disabled
	syncing     atomic.Bool
	onSyncStart func(c *gin.Context, full bool) // optional audit hook
}

func NewCveCorpusHandler(corpusDir string, logger *zap.SugaredLogger) *CveCorpusHandler {
	return &CveCorpusHandler{corpusDir: corpusDir, logger: logger}
}

// SetSyncTrigger wires the manual sync entry point (POST /api/cve-corpus/sync).
func (h *CveCorpusHandler) SetSyncTrigger(f func(full bool) error) {
	h.syncTrigger = f
}

// SetOnSyncStart registers an audit callback fired when a sync is accepted.
func (h *CveCorpusHandler) SetOnSyncStart(f func(c *gin.Context, full bool)) {
	h.onSyncStart = f
}

type CVEEntry struct {
	ID          string `json:"id"`
	Year        string `json:"year"`
	State       string `json:"state"`
	Products    string `json:"products,omitempty"`
	Vendor      string `json:"vendor,omitempty"`
	Date        string `json:"date,omitempty"`
	Description string `json:"description,omitempty"`
}

type SearchResponse struct {
	Total      int        `json:"total"`
	Page       int        `json:"page"`
	PageSize   int        `json:"page_size"`
	TotalPages int        `json:"total_pages"`
	Entries    []CVEEntry `json:"entries"`
}

type corpusItem struct {
	entry   CVEEntry
	content string
}

var (
	corpusCache    []corpusItem
	corpusCacheDir string
	corpusMu       sync.RWMutex
)

// scanCorpus walks cve/{year}/*.md. Paths from os.ReadDir (safe).
// The package-level cache is mutex-guarded and dropped by
// InvalidateCorpusCache after every successful cvesync run.
func (h *CveCorpusHandler) scanCorpus() []corpusItem {
	corpusMu.RLock()
	if corpusCache != nil && corpusCacheDir == h.corpusDir {
		cached := corpusCache
		corpusMu.RUnlock()
		return cached
	}
	corpusMu.RUnlock()

	var result []corpusItem
	cveRoot := filepath.Join(h.corpusDir, "cve")
	years, err := os.ReadDir(cveRoot)
	if err != nil {
		return result
	}
	for _, yd := range years {
		if !yd.IsDir() {
			continue
		}
		year := yd.Name()
		yearDir := filepath.Join(cveRoot, year)
		files, err := os.ReadDir(yearDir)
		if err != nil {
			continue
		}
		for _, f := range files {
			if f.IsDir() || !strings.HasSuffix(f.Name(), ".md") {
				continue
			}
			raw, err := os.ReadFile(filepath.Join(yearDir, f.Name()))
			if err != nil {
				continue
			}
			e := parseMD(string(raw))
			e.ID = strings.TrimSuffix(f.Name(), ".md")
			e.Year = year
			result = append(result, corpusItem{entry: e, content: string(raw)})
		}
	}
	sort.Slice(result, func(i, j int) bool { return result[i].entry.ID > result[j].entry.ID })
	corpusMu.Lock()
	corpusCache = result
	corpusCacheDir = h.corpusDir
	corpusMu.Unlock()
	return result
}

// InvalidateCorpusCache drops the cached corpus scan so newly synced records
// are visible to the browse API without a restart (called by app.go via
// cvesync Config.OnSyncDone).
func InvalidateCorpusCache() {
	corpusMu.Lock()
	corpusCache = nil
	corpusCacheDir = ""
	corpusMu.Unlock()
}

func parseMD(content string) CVEEntry {
	e := CVEEntry{}
	lines := strings.Split(content, "\n")
	inFM := false
	for _, line := range lines {
		t := strings.TrimSpace(line)
		if t == "---" {
			if inFM {
				break
			}
			inFM = true
			continue
		}
		if inFM {
			if strings.HasPrefix(t, "state: ") {
				e.State = t[7:]
			}
		}
		if strings.HasPrefix(t, "- products: ") {
			e.Products = t[12:]
		}
		if strings.HasPrefix(t, "- vendor: ") {
			e.Vendor = t[10:]
		}
		if strings.HasPrefix(t, "- date: ") {
			e.Date = t[8:]
		}
	}
	dashCount, afterFM := 0, false
	for _, line := range lines {
		t := strings.TrimSpace(line)
		if t == "---" {
			dashCount++
			if dashCount == 2 {
				afterFM = true
				continue
			}
		}
		if afterFM && t != "" && !strings.HasPrefix(t, "#") && !strings.HasPrefix(t, "-") && !strings.HasPrefix(t, "refs") {
			e.Description = t
			if len(e.Description) > 200 {
				e.Description = e.Description[:200] + "..."
			}
			break
		}
	}
	return e
}

// Search: GET /api/cve-corpus/search?q=...&year=...&page=1&size=20
func (h *CveCorpusHandler) Search(c *gin.Context) {
	query := strings.ToLower(strings.TrimSpace(c.Query("q")))
	year := strings.TrimSpace(c.Query("year"))
	page, size := 1, 20
	fmt.Sscanf(c.Query("page"), "%d", &page)
	fmt.Sscanf(c.Query("size"), "%d", &size)
	if page < 1 {
		page = 1
	}
	if size < 1 || size > 100 {
		size = 20
	}

	all := h.scanCorpus()
	var filtered []CVEEntry
	for _, ci := range all {
		e := ci.entry
		if year != "" && e.Year != year {
			continue
		}
		if query != "" {
			if !(strings.Contains(strings.ToLower(e.ID), query) ||
				strings.Contains(strings.ToLower(e.Products), query) ||
				strings.Contains(strings.ToLower(e.Vendor), query) ||
				strings.Contains(strings.ToLower(e.Description), query)) {
				continue
			}
		}
		filtered = append(filtered, e)
	}

	total := len(filtered)
	tp := (total + size - 1) / size
	if tp == 0 {
		tp = 1
	}
	start := (page - 1) * size
	if start > total {
		start = total
	}
	end := start + size
	if end > total {
		end = total
	}

	c.JSON(http.StatusOK, SearchResponse{
		Total: total, Page: page, PageSize: size, TotalPages: tp,
		Entries: filtered[start:end],
	})
}

// GetDetail: GET /api/cve-corpus/:id — lookup via scan cache (no path from URL).
func (h *CveCorpusHandler) GetDetail(c *gin.Context) {
	cveID := strings.ToUpper(strings.TrimSpace(c.Param("id")))
	if len(cveID) < 9 || cveID[:4] != "CVE-" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid CVE ID"})
		return
	}
	for _, ci := range h.scanCorpus() {
		if ci.entry.ID == cveID {
			c.JSON(http.StatusOK, gin.H{
				"id": cveID, "year": ci.entry.Year, "content": ci.content,
			})
			return
		}
	}
	c.JSON(http.StatusNotFound, gin.H{"error": "CVE not found"})
}

// State: GET /api/cve-corpus/state
// Corpus scan stats merged with the cvesync watermark (state.json).
func (h *CveCorpusHandler) State(c *gin.Context) {
	items := h.scanCorpus()
	years := make(map[string]int)
	for _, ci := range items {
		years[ci.entry.Year]++
	}
	resp := gin.H{
		"total": len(items), "years": years, "corpus_dir": h.corpusDir,
	}
	if st, err := cvesync.LoadState(h.corpusDir); err == nil && st != nil {
		resp["sync"] = gin.H{
			"commit":          st.Commit,
			"published_count": st.PublishedCount,
			"changed_today":   st.ChangedToday,
			"indexed_at":      st.IndexedAt,
			"last_sync_day":   st.LastSyncDay,
			"last_error":      st.LastError,
			"years_window":    st.Years,
		}
	}
	c.JSON(http.StatusOK, resp)
}

// Sync: POST /api/cve-corpus/sync?full=1 — manual trigger (admin + audit).
// Runs asynchronously; poll GET /state for progress. Not exposed to agents.
func (h *CveCorpusHandler) Sync(c *gin.Context) {
	if h.syncTrigger == nil {
		c.JSON(http.StatusServiceUnavailable, gin.H{
			"error": "cve_corpus sync disabled (cve_corpus.enabled=false)",
		})
		return
	}
	full := c.Query("full") == "1" || c.Query("full") == "true"
	if !h.syncing.CompareAndSwap(false, true) {
		c.JSON(http.StatusConflict, gin.H{"error": "sync already running"})
		return
	}
	go func() {
		defer h.syncing.Store(false)
		if err := h.syncTrigger(full); err != nil {
			h.logger.Warnw("cve-corpus manual sync failed", "error", err, "full", full)
		}
	}()
	if h.onSyncStart != nil {
		h.onSyncStart(c, full)
	}
	c.JSON(http.StatusAccepted, gin.H{
		"started": true,
		"full":    full,
		"hint":    "poll GET /api/cve-corpus/state (sync.indexed_at / sync.last_error)",
	})
}
