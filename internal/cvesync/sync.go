package cvesync

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"go.uber.org/zap"
)

// SyncState tracks incremental sync progress in state.json.
type SyncState struct {
	Commit         string    `json:"commit"`
	Years          []int     `json:"years"`
	PublishedCount int       `json:"published_count"`
	ChangedToday   int       `json:"changed_today"`
	IndexedAt      time.Time `json:"indexed_at"`
	// LastSyncDay is the local date (YYYY-MM-DD) of the last SUCCESSFUL sync.
	// Day-based scheduling must not use IndexedAt.Day() (breaks across months
	// and on zero values).
	LastSyncDay string `json:"last_sync_day,omitempty"`
	LastError   string `json:"last_error,omitempty"`
}

// LoadState reads data/corpus/state.json.
func LoadState(corpusDir string) (*SyncState, error) {
	path := filepath.Join(corpusDir, "state.json")
	data, err := os.ReadFile(path)
	if os.IsNotExist(err) {
		return &SyncState{}, nil
	}
	if err != nil {
		return nil, err
	}
	var st SyncState
	if err := json.Unmarshal(data, &st); err != nil {
		return nil, err
	}
	return &st, nil
}

// SaveState writes data/corpus/state.json (creating the corpus dir if the
// very first sync has not materialized it yet).
func SaveState(corpusDir string, st *SyncState) error {
	if err := os.MkdirAll(corpusDir, 0755); err != nil {
		return err
	}
	data, err := json.MarshalIndent(st, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(filepath.Join(corpusDir, "state.json"), data, 0644)
}

// Config for the sync loop.
type Config struct {
	CorpusDir     string
	RawDir        string // data/corpus/raw/cvelistV5
	Mode          string // git | daily_zip
	Years         int    // window size (default 5)
	SkipRejected  bool
	SkipReserved  bool
	DescMaxRunes  int
	SyncHourLocal int
	MinYearFiles  int // per-year floor gate for non-current years
	// OnSyncDone (optional) fires after every SUCCESSFUL sync — used by app.go
	// to drop the browse-API corpus cache so new records are visible at once.
	OnSyncDone func()
}

// SyncResult reports what happened during one sync cycle.
type SyncResult struct {
	ChangedFiles int
	NewMD        int
	DeletedMD    int
	Skipped      int
	Errors       int
}

// SyncChangedFiles processes a list of changed JSON file paths (relative to
// the raw repo root) and writes/deletes corresponding .md files.
func SyncChangedFiles(cfg Config, changed []string, logger *zap.SugaredLogger) (*SyncResult, error) {
	result := &SyncResult{}

	for _, relPath := range changed {
		result.ChangedFiles++

		jsonPath := filepath.Join(cfg.RawDir, relPath)
		cveID := filepath.Base(relPath)
		cveID = strings.TrimSuffix(cveID, ".json")

		// Check if the JSON file was deleted (git diff shows it in deleted list)
		if _, err := os.Stat(jsonPath); os.IsNotExist(err) {
			// Delete corresponding .md
			mdPath := OutputPath(cfg.CorpusDir, cveID)
			if err := os.Remove(mdPath); err == nil {
				result.DeletedMD++
			}
			continue
		}

		rec, err := ExtractFile(jsonPath, cfg.DescMaxRunes)
		if err != nil {
			logger.Warn("extract failed: ", relPath, " ", err)
			result.Errors++
			continue
		}

		if ShouldSkip(rec, cfg.SkipRejected, cfg.SkipReserved) {
			result.Skipped++
			continue
		}

		// Write .md
		mdPath := OutputPath(cfg.CorpusDir, rec.ID)
		if err := os.MkdirAll(filepath.Dir(mdPath), 0755); err != nil {
			logger.Warn("mkdir failed: ", mdPath, " ", err)
			result.Errors++
			continue
		}
		if err := os.WriteFile(mdPath, []byte(rec.ToMarkdown()), 0644); err != nil {
			logger.Warn("write failed: ", mdPath, " ", err)
			result.Errors++
			continue
		}
		result.NewMD++
	}

	return result, nil
}

// CountPublishedMD counts .md files in data/corpus/cve/ for gate validation.
func CountPublishedMD(corpusDir string) int {
	count := 0
	cveDir := filepath.Join(corpusDir, "cve")
	filepath.Walk(cveDir, func(path string, info os.FileInfo, err error) error {
		if err == nil && !info.IsDir() && strings.HasSuffix(path, ".md") {
			count++
		}
		return nil
	})
	return count
}

// CountMDByYear returns the per-year md counts under corpus/cve/.
func CountMDByYear(corpusDir string) map[string]int {
	m := make(map[string]int)
	years, err := os.ReadDir(filepath.Join(corpusDir, "cve"))
	if err != nil {
		return m
	}
	for _, yd := range years {
		if !yd.IsDir() {
			continue
		}
		files, err := os.ReadDir(filepath.Join(corpusDir, "cve", yd.Name()))
		if err != nil {
			continue
		}
		n := 0
		for _, f := range files {
			if !f.IsDir() && strings.HasSuffix(f.Name(), ".md") {
				n++
			}
		}
		if n > 0 {
			m[yd.Name()] = n
		}
	}
	return m
}

// NewestMDMtime returns the newest md modification time (corpus snapshot age
// when no state exists yet). Zero when the corpus is empty.
func NewestMDMtime(corpusDir string) time.Time {
	var newest time.Time
	filepath.Walk(filepath.Join(corpusDir, "cve"), func(path string, info os.FileInfo, err error) error {
		if err == nil && !info.IsDir() && strings.HasSuffix(path, ".md") {
			if info.ModTime().After(newest) {
				newest = info.ModTime()
			}
		}
		return nil
	})
	return newest
}

// GateCheckPlan validates a planned change set BEFORE applying it:
//   - total after applying must stay ≥ 80% of the current total
//   - any already-present year that is NOT the current UTC year must keep
//     ≥ minYearFiles records after applying (the "非新年份单年" rule; the
//     current year is still filling up and is exempt)
//
// An empty corpus skips both checks — the first full import starts from
// nothing and is gated by the import itself, not by the delta.
func GateCheckPlan(corpusDir string, addsByYear, delsByYear map[string]int, currentYear, minYearFiles int) error {
	perYear := CountMDByYear(corpusDir)
	oldCount := 0
	for _, n := range perYear {
		oldCount += n
	}
	if oldCount == 0 {
		return nil
	}
	adds, dels := 0, 0
	for _, n := range addsByYear {
		adds += n
	}
	for _, n := range delsByYear {
		dels += n
	}
	newCount := oldCount + adds - dels
	if newCount < oldCount*80/100 {
		return fmt.Errorf("planned total %d would drop below 80%% of %d — refusing to apply", newCount, oldCount)
	}
	if minYearFiles <= 0 {
		return nil
	}
	cur := fmt.Sprintf("%d", currentYear)
	for year, n := range perYear {
		if year == cur {
			continue // current year is still growing — exempt
		}
		if n < minYearFiles {
			continue // year never reached the floor (e.g. imported partial)
		}
		if n+addsByYear[year]-delsByYear[year] < minYearFiles {
			return fmt.Errorf("year %s would fall from %d below min_year_files %d — refusing to apply",
				year, n, minYearFiles)
		}
	}
	return nil
}

// PruneOldYears removes year directories that fell outside the window
// (the "每年一月删 Y-5" rule, applied idempotently on every successful sync).
func PruneOldYears(corpusDir string, windowYears int) error {
	if windowYears <= 0 {
		windowYears = 5
	}
	minYear := time.Now().UTC().Year() - windowYears + 1
	cveRoot := filepath.Join(corpusDir, "cve")
	years, err := os.ReadDir(cveRoot)
	if err != nil {
		return err
	}
	for _, yd := range years {
		if !yd.IsDir() {
			continue
		}
		var y int
		if _, err := fmt.Sscanf(yd.Name(), "%d", &y); err != nil || y >= minYear {
			continue
		}
		if err := os.RemoveAll(filepath.Join(cveRoot, yd.Name())); err != nil {
			return err
		}
	}
	return nil
}
