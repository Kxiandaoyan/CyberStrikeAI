// Package cvesync — initial download of CVE data.
// Uses the official daily zip (CVEProject/cvelistV5) instead of git clone
// to avoid subprocess execution. Downloads over HTTPS only.
package cvesync

import (
	"archive/zip"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"

	"go.uber.org/zap"
)

// DownloadConfig controls the initial CVE data download.
type DownloadConfig struct {
	RawDir       string // data/corpus/raw/cvelistV5
	CorpusDir    string // data/corpus
	Years        int    // how many years to keep (default 5)
	DescMaxRunes int
	SkipRejected bool
	SkipReserved bool
}

// downloadURL returns the URL for the latest midnight zip.
// Official source: https://github.com/CVEProject/cvelistV5
// We use the raw GitHub release zip for the full dataset.
func midnightZipURL() string {
	// The official daily zip is at:
	// https://github.com/CVEProject/cvelistV5/releases/download/{tag}/cve.zip
	// But tags change daily. Use the cve.org download endpoint instead:
	// https://cve.org/downloads/ links to the same data.
	// For reliability, use the static GitHub zip of the master branch:
	return "https://github.com/CVEProject/cvelistV5/archive/refs/heads/main.zip"
}

// NeedsDownload returns true if the raw directory has no meaningful CVE data.
// Records live in bucket directories (cves/{year}/{prefix}xxx/*.json), so a
// bounded recursive count is used instead of a flat dir listing.
func NeedsDownload(rawDir string) bool {
	for i := 0; i < 5; i++ {
		year := fmt.Sprintf("%d", time.Now().UTC().Year()-i)
		if countJSONBounded(filepath.Join(rawDir, "cves", year), 500) > 500 {
			return false // this year has real data
		}
	}
	return true
}

// countJSONBounded counts *.json files under root, stopping once the bound
// is exceeded (keeps the check cheap on 40K-file years).
func countJSONBounded(root string, bound int) int {
	count := 0
	filepath.WalkDir(root, func(path string, d os.DirEntry, err error) error {
		if err != nil || count > bound {
			return filepath.SkipAll
		}
		if !d.IsDir() && strings.HasSuffix(path, ".json") {
			count++
		}
		return nil
	})
	return count
}

// InitialDownload downloads the CVE dataset zip and extracts to raw/.
// This is a cold-start operation (first run only, ~200MB zip).
// Uses HTTPS only; validates content is a zip before extracting.
func InitialDownload(cfg DownloadConfig, logger *zap.SugaredLogger) error {
	if !NeedsDownload(cfg.RawDir) {
		logger.Info("cvesync: raw data already present, skipping download")
		return nil
	}

	logger.Info("cvesync: starting initial download (this may take several minutes)...")
	url := midnightZipURL()

	// Download zip to temp file
	tmpZip := filepath.Join(os.TempDir(), "cvelistV5.zip")
	logger.Info("cvesync: downloading ", url)
	if err := downloadFile(url, tmpZip, logger); err != nil {
		return fmt.Errorf("download failed: %w", err)
	}
	defer os.Remove(tmpZip)

	// Verify it's a zip file
	if !isZipFile(tmpZip) {
		return fmt.Errorf("downloaded file is not a valid zip")
	}

	// Extract to raw directory
	logger.Info("cvesync: extracting to ", cfg.RawDir)
	if err := extractZip(tmpZip, cfg.RawDir); err != nil {
		return fmt.Errorf("extract failed: %w", err)
	}

	// Convert JSON to Markdown
	logger.Info("cvesync: converting JSON to Markdown...")
	result, err := ConvertAll(cfg, logger)
	if err != nil {
		return fmt.Errorf("convert failed: %w", err)
	}

	logger.Info("cvesync: initial download complete: ",
		result.NewMD, " CVEs extracted, ", result.Skipped, " skipped, ", result.Errors, " errors")
	return nil
}

// downloadFile downloads a URL to a local path using HTTPS.
func downloadFile(url, dest string, logger *zap.SugaredLogger) error {
	if !strings.HasPrefix(url, "https://") {
		return fmt.Errorf("only HTTPS downloads are allowed")
	}

	resp, err := http.Get(url)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("HTTP %d", resp.StatusCode)
	}

	f, err := os.Create(dest)
	if err != nil {
		return err
	}
	defer f.Close()

	written, err := io.Copy(f, resp.Body)
	if err != nil {
		return err
	}
	logger.Info("cvesync: downloaded ", written/1024/1024, " MB")
	return nil
}

// isZipFile checks the magic bytes of a file.
func isZipFile(path string) bool {
	f, err := os.Open(path)
	if err != nil {
		return false
	}
	defer f.Close()
	magic := make([]byte, 4)
	if _, err := f.Read(magic); err != nil {
		return false
	}
	// ZIP magic: PK\x03\x04
	return magic[0] == 'P' && magic[1] == 'K' && magic[2] == 3 && magic[3] == 4
}

// extractZip extracts a zip file to a destination directory.
func extractZip(zipPath, destDir string) error {
	r, err := zip.OpenReader(zipPath)
	if err != nil {
		return err
	}
	defer r.Close()

	if err := os.MkdirAll(destDir, 0755); err != nil {
		return err
	}

	for _, f := range r.File {
		// Only extract cves/ directory (skip .git, docs, etc.)
		if !strings.Contains(f.Name, "/cves/") {
			continue
		}

		// Skip directories
		if f.FileInfo().IsDir() {
			continue
		}

		// Only .json files
		if !strings.HasSuffix(f.Name, ".json") {
			continue
		}

		// Calculate extraction path (strip the top-level directory name)
		// zip entries look like: cvelistV5-main/cves/2024/CVE-2024-XXXXX.json
		// we want: cves/2024/CVE-2024-XXXXX.json
		parts := strings.SplitN(f.Name, "/", 2)
		if len(parts) < 2 {
			continue
		}
		relPath := parts[1]

		extractPath := filepath.Join(destDir, relPath)
		if err := os.MkdirAll(filepath.Dir(extractPath), 0755); err != nil {
			continue
		}

		// Open source
		src, err := f.Open()
		if err != nil {
			continue
		}

		// Create dest
		dst, err := os.Create(extractPath)
		if err != nil {
			src.Close()
			continue
		}

		io.Copy(dst, src)
		src.Close()
		dst.Close()
	}
	return nil
}

// ConvertAll converts all JSON files in raw/ to Markdown in corpus/.
// Records live in bucket directories (cves/{year}/{prefix}xxx/), so each
// year is walked recursively.
func ConvertAll(cfg DownloadConfig, logger *zap.SugaredLogger) (*SyncResult, error) {
	result := &SyncResult{}

	// Walk raw/cves/{year}/**.json (bucket dirs included)
	cvesDir := filepath.Join(cfg.RawDir, "cves")

	// Determine year range
	currentYear := time.Now().UTC().Year()
	minYear := currentYear - cfg.Years + 1
	if cfg.Years <= 0 {
		minYear = currentYear - 4 // default 5 years
	}

	perYear := make(map[string]int)
	walkErr := filepath.WalkDir(cvesDir, func(path string, d os.DirEntry, werr error) error {
		if werr != nil || d.IsDir() {
			return nil
		}
		if !strings.HasSuffix(path, ".json") {
			return nil
		}
		rel, rerr := filepath.Rel(cvesDir, path)
		if rerr != nil {
			return nil
		}
		sep := strings.IndexByte(rel, filepath.Separator)
		if sep <= 0 {
			return nil // file at cves/ root (delta.json / deltaLog.json) — not a record
		}
		year := rel[:sep]
		var yearInt int
		if _, serr := fmt.Sscanf(year, "%d", &yearInt); serr != nil || yearInt < minYear || yearInt > currentYear {
			return nil // outside window or non-year directory
		}

		if perYear[year] == 0 {
			logger.Info("cvesync: processing year ", year, " …")
		}
		perYear[year]++

		rec, eerr := ExtractFile(path, cfg.DescMaxRunes)
		if eerr != nil {
			result.Errors++
			return nil
		}
		if ShouldSkip(rec, cfg.SkipRejected, cfg.SkipReserved) {
			result.Skipped++
			return nil
		}
		mdPath := OutputPath(cfg.CorpusDir, rec.ID)
		if merr := os.MkdirAll(filepath.Dir(mdPath), 0755); merr != nil {
			result.Errors++
			return nil
		}
		if werr := os.WriteFile(mdPath, []byte(rec.ToMarkdown()), 0644); werr != nil {
			result.Errors++
			return nil
		}
		result.NewMD++
		return nil
	})
	if walkErr != nil {
		return result, walkErr
	}
	for y, n := range perYear {
		logger.Info("cvesync: year ", y, " scanned ", n, " JSON files")
	}

	return result, nil
}
