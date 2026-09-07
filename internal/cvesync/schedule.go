// Package cvesync — scheduling and one-shot execution of the daily sync.
//
// Honesty contract (审计 §3.2): state.json only claims what succeeded.
// indexed_at / last_sync_day / commit advance exclusively after every planned
// change landed; failures write last_error and leave the watermark untouched
// so the next cycle retries the remainder.
package cvesync

import (
	"context"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"time"

	"go.uber.org/zap"
)

// Loop checks every 15 minutes. It triggers a sync when:
//   - It's past sync_hour_local and today's successful sync hasn't happened
//   - On startup, if the last successful sync was > 36 hours ago (catch-up)
//
// Day tracking uses state.last_sync_day (YYYY-MM-DD), never IndexedAt.Day().
func Loop(ctx context.Context, cfg Config, logger *zap.SugaredLogger) {
	ticker := time.NewTicker(15 * time.Minute)
	defer ticker.Stop()

	// Startup catch-up: if last successful sync > 36h ago (or never), run now.
	go func() {
		time.Sleep(2 * time.Minute)
		st, err := LoadState(cfg.CorpusDir)
		if err != nil {
			logger.Warn("cvesync: state load failed: ", err)
			return
		}
		if st.IndexedAt.IsZero() || time.Since(st.IndexedAt) > 36*time.Hour {
			logger.Info("cvesync: last successful sync > 36h ago (or never), running catch-up")
			if err := RunOnce(ctx, cfg, logger, false); err != nil {
				logger.Warn("cvesync: catch-up failed: ", err)
			}
		}
	}()

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			st, err := LoadState(cfg.CorpusDir)
			if err != nil {
				continue
			}
			now := time.Now()
			today := now.Format("2006-01-02")
			if now.Hour() >= syncHourEffective(cfg) && st.LastSyncDay != today {
				if err := RunOnce(ctx, cfg, logger, false); err != nil {
					logger.Warn("cvesync: scheduled sync failed: ", err)
				}
			}
		}
	}
}

func syncHourEffective(cfg Config) int {
	if cfg.SyncHourLocal >= 0 && cfg.SyncHourLocal <= 23 {
		return cfg.SyncHourLocal
	}
	return 3
}

func yearsEffective(cfg Config) int {
	if cfg.Years >= 1 && cfg.Years <= 20 {
		return cfg.Years
	}
	return 5
}

// runOnceMu serializes sync cycles inside this process (Loop vs API trigger).
var runOnceMu sync.Mutex

// RunOnce performs one sync cycle (incremental, or full cold rebuild).
// The corpus lock file guards against a second CS process doing the same.
func RunOnce(ctx context.Context, cfg Config, logger *zap.SugaredLogger, full bool) error {
	runOnceMu.Lock()
	defer runOnceMu.Unlock()

	release, err := acquireLock(cfg.CorpusDir)
	if err != nil {
		return err
	}
	defer release()

	if full {
		return fullRefresh(ctx, cfg, logger)
	}
	// Mode=daily_zip：官方 midnight/delta zip 尚未接入，当前 zip 通道只有
	// 全量仓库包（且同样经 github.com）。诚实降级为全量刷新并说明流量代价，
	// 不假装走了增量。
	if strings.EqualFold(strings.TrimSpace(cfg.Mode), "daily_zip") {
		logger.Warn("cvesync: mode=daily_zip — 官方 delta zip 未接，按全量仓库 zip 刷新（较重，github.com 仍需可达）")
		return fullRefresh(ctx, cfg, logger)
	}
	return incrementalSync(ctx, cfg, logger)
}

// acquireLock creates data/corpus/lock (O_EXCL). A lock older than 2h is
// considered stale (crashed holder) and taken over.
func acquireLock(corpusDir string) (func(), error) {
	if err := os.MkdirAll(corpusDir, 0755); err != nil {
		return nil, err
	}
	lockPath := filepath.Join(corpusDir, "lock")
	f, err := os.OpenFile(lockPath, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0644)
	if err == nil {
		fmt.Fprintf(f, "%d", time.Now().Unix())
		f.Close()
		return func() { os.Remove(lockPath) }, nil
	}
	// Exists — check staleness.
	info, statErr := os.Stat(lockPath)
	if statErr != nil || time.Since(info.ModTime()) > 2*time.Hour {
		os.Remove(lockPath)
		f2, err2 := os.OpenFile(lockPath, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0644)
		if err2 != nil {
			return nil, fmt.Errorf("sync already running (lock busy)")
		}
		fmt.Fprintf(f2, "%d", time.Now().Unix())
		f2.Close()
		return func() { os.Remove(lockPath) }, nil
	}
	return nil, fmt.Errorf("sync already running (lock held since %s)", info.ModTime().Format(time.RFC3339))
}

// failState records last_error WITHOUT advancing the success watermark.
func failState(corpusDir string, st *SyncState, err error, logger *zap.SugaredLogger) {
	msg := err.Error()
	if len(msg) > 500 {
		msg = msg[:500]
	}
	st.LastError = msg
	if saveErr := SaveState(corpusDir, st); saveErr != nil {
		logger.Warn("cvesync: state save failed: ", saveErr)
	}
}

// fullRefresh rebuilds the corpus from the official repo zip (cold start or
// explicit ?full=1), then bootstraps state at the repo head.
func fullRefresh(ctx context.Context, cfg Config, logger *zap.SugaredLogger) error {
	logger.Info("cvesync: full refresh starting (downloads the official repo zip)...")
	st, err := LoadState(cfg.CorpusDir)
	if err != nil {
		st = &SyncState{}
	}
	if err := InitialDownload(DownloadConfig{
		RawDir:       cfg.RawDir,
		CorpusDir:    cfg.CorpusDir,
		Years:        yearsEffective(cfg),
		DescMaxRunes: cfg.DescMaxRunes,
		SkipRejected: cfg.SkipRejected,
		SkipReserved: cfg.SkipReserved,
	}, logger); err != nil {
		failState(cfg.CorpusDir, st, err, logger)
		return err
	}

	// Bootstrap the watermark at the current repo head.
	head := ""
	if h, err := githubHeadSHA(ctx, &http.Client{Timeout: 30 * time.Second}); err == nil {
		head = h
	} else {
		logger.Warn("cvesync: head lookup after full refresh failed (watermark left empty): ", err)
	}
	now := time.Now()
	st.Commit = head
	st.IndexedAt = now
	st.LastSyncDay = now.Format("2006-01-02")
	st.ChangedToday = 0
	st.LastError = ""
	st.PublishedCount = CountPublishedMD(cfg.CorpusDir)
	st.Years = yearWindow(yearsEffective(cfg))
	if err := SaveState(cfg.CorpusDir, st); err != nil {
		return err
	}
	notifySyncDone(cfg)
	logger.Infof("cvesync: full refresh complete — %d records", st.PublishedCount)
	return nil
}

// incrementalSync: plan (commits+compare over HTTP) → gate → apply → advance.
func incrementalSync(ctx context.Context, cfg Config, logger *zap.SugaredLogger) error {
	st, err := LoadState(cfg.CorpusDir)
	if err != nil {
		return err
	}
	currentYear := time.Now().UTC().Year()
	minYear := currentYear - yearsEffective(cfg) + 1

	if CountPublishedMD(cfg.CorpusDir) == 0 {
		err := fmt.Errorf("corpus is empty — run a full sync first (POST /api/cve-corpus/sync?full=1) or extract assets/cve-corpus.tar.gz via run.sh")
		failState(cfg.CorpusDir, st, err, logger)
		return err
	}

	// Diff window: last success, or (bootstrap) the corpus snapshot time
	// minus a 24h safety margin — the data is as-of no later than mtime.
	since := st.IndexedAt
	if since.IsZero() {
		snap := NewestMDMtime(cfg.CorpusDir)
		if snap.IsZero() {
			err := fmt.Errorf("cannot determine sync window (no state, no corpus mtimes)")
			failState(cfg.CorpusDir, st, err, logger)
			return err
		}
		since = snap.Add(-24 * time.Hour)
		logger.Infof("cvesync: bootstrapping sync window from corpus mtime %s", snap.Format(time.RFC3339))
	}

	client := &http.Client{Timeout: 60 * time.Second}
	changes, head, err := planChanges(ctx, client, since)
	if err != nil {
		failState(cfg.CorpusDir, st, err, logger)
		return err
	}
	if len(changes) == 0 {
		// Nothing new upstream — still a successful cycle.
		now := time.Now()
		st.Commit = head
		st.IndexedAt = now
		st.LastSyncDay = now.Format("2006-01-02")
		st.ChangedToday = 0
		st.LastError = ""
		st.PublishedCount = CountPublishedMD(cfg.CorpusDir)
		st.Years = yearWindow(yearsEffective(cfg))
		if err := SaveState(cfg.CorpusDir, st); err != nil {
			return err
		}
		notifySyncDone(cfg)
		logger.Info("cvesync: no upstream changes")
		return nil
	}

	// Window filter + per-year gate maps.
	addsByYear := map[string]int{}
	delsByYear := map[string]int{}
	var inWindow []fileChange
	for _, ch := range changes {
		year := pathYear(ch.Path)
		if year < minYear || year > currentYear {
			continue
		}
		key := strconv.Itoa(year)
		if ch.Removed {
			delsByYear[key]++
		} else {
			addsByYear[key]++
		}
		inWindow = append(inWindow, ch)
	}

	if err := GateCheckPlan(cfg.CorpusDir, addsByYear, delsByYear, currentYear, cfg.MinYearFiles); err != nil {
		failState(cfg.CorpusDir, st, err, logger)
		return err
	}

	res := applyChanges(ctx, client, cfg, inWindow, logger)
	if res.Errors > 0 {
		err := fmt.Errorf("%d/%d changed files failed — watermark not advanced, next cycle retries", res.Errors, len(inWindow))
		failState(cfg.CorpusDir, st, err, logger)
		return err
	}

	// Success: prune expired years, advance the watermark.
	if err := PruneOldYears(cfg.CorpusDir, yearsEffective(cfg)); err != nil {
		logger.Warn("cvesync: year pruning failed (continuing): ", err)
	}
	now := time.Now()
	st.Commit = head
	st.IndexedAt = now
	st.LastSyncDay = now.Format("2006-01-02")
	st.ChangedToday = len(inWindow)
	st.LastError = ""
	st.PublishedCount = CountPublishedMD(cfg.CorpusDir)
	st.Years = yearWindow(yearsEffective(cfg))
	if err := SaveState(cfg.CorpusDir, st); err != nil {
		return err
	}
	notifySyncDone(cfg)
	logger.Infof("cvesync: %d files applied (+%d md / -%d md / %d skipped), corpus now %d records (head %.8s)",
		len(inWindow), res.NewMD, res.DeletedMD, res.Skipped, st.PublishedCount, head)
	return nil
}

// notifySyncDone fires the optional post-sync hook (cache invalidation).
func notifySyncDone(cfg Config) {
	if cfg.OnSyncDone != nil {
		cfg.OnSyncDone()
	}
}

// applyChanges fetches each changed JSON (3 retries), mirrors it into raw/ and
// rewrites the corresponding md. REJECTED/RESERVED records delete stale md.
func applyChanges(ctx context.Context, client *http.Client, cfg Config, changes []fileChange, logger *zap.SugaredLogger) *SyncResult {
	result := &SyncResult{}
	for _, ch := range changes {
		cveID := strings.TrimSuffix(filepath.Base(ch.Path), ".json")
		mdPath := OutputPath(cfg.CorpusDir, cveID)
		rawPath := filepath.Join(cfg.RawDir, ch.Path)

		if ch.Removed {
			if err := os.Remove(mdPath); err == nil {
				result.DeletedMD++
			}
			os.Remove(rawPath)
			continue
		}

		data, err := fetchRawRetry(ctx, client, ch.Path)
		if err != nil {
			logger.Warn("cvesync: fetch failed: ", ch.Path, " ", err)
			result.Errors++
			continue
		}
		if err := os.MkdirAll(filepath.Dir(rawPath), 0755); err != nil {
			result.Errors++
			continue
		}
		if err := os.WriteFile(rawPath, data, 0644); err != nil {
			result.Errors++
			continue
		}

		rec, err := ExtractFile(rawPath, cfg.DescMaxRunes)
		if err != nil {
			logger.Warn("cvesync: extract failed: ", ch.Path, " ", err)
			result.Errors++
			continue
		}
		if ShouldSkip(rec, cfg.SkipRejected, cfg.SkipReserved) {
			os.Remove(mdPath) // record turned REJECTED/RESERVED — drop stale md
			result.Skipped++
			continue
		}
		if err := os.MkdirAll(filepath.Dir(mdPath), 0755); err != nil {
			result.Errors++
			continue
		}
		if err := os.WriteFile(mdPath, []byte(rec.ToMarkdown()), 0644); err != nil {
			result.Errors++
			continue
		}
		result.NewMD++
	}
	return result
}

// fetchRawRetry fetches one raw file with bounded retries.
func fetchRawRetry(ctx context.Context, client *http.Client, path string) ([]byte, error) {
	var lastErr error
	for attempt := 0; attempt < 3; attempt++ {
		if attempt > 0 {
			select {
			case <-ctx.Done():
				return nil, ctx.Err()
			case <-time.After(time.Duration(attempt*2) * time.Second):
			}
		}
		data, err := githubRawFile(ctx, client, path)
		if err == nil {
			return data, nil
		}
		lastErr = err
	}
	return nil, lastErr
}

// pathYear extracts the year from a validated cves/{year}/... path.
func pathYear(repoRel string) int {
	year, _ := strconv.Atoi(strings.Split(repoRel, "/")[1])
	return year
}

// yearWindow returns [currentYear-years+1 .. currentYear].
func yearWindow(years int) []int {
	cur := time.Now().UTC().Year()
	out := make([]int, 0, years)
	for y := cur - years + 1; y <= cur; y++ {
		out = append(out, y)
	}
	return out
}
