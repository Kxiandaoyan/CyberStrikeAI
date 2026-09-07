// Package cvesync synchronizes CVE records from the official cvelistV5
// repository into short Markdown files for zvec indexing.
//
// Pipeline: git pull (or daily zip) → extract changed JSON to .md → zg index.
// The agent/operator only sees .md; raw JSON stays in data/corpus/raw/.
package cvesync

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"
)

// CVERecord represents the subset of CVE JSON 5.1 we extract.
type CVERecord struct {
	ID          string
	State       string
	Description string
	Products    []string
	Vendor      string
	Date        string
	Assigner    string
	References  []string
}

// rawCVE mirrors the official CVE JSON 5.1 structure.
type rawCVE struct {
	DataType    string `json:"dataType"`
	CveMetadata struct {
		CveID     string `json:"cveId"`
		State     string `json:"state"`
		DatePublished string `json:"datePublished"`
		AssignerShortName string `json:"assignerShortName"`
	} `json:"cveMetadata"`
	Containers struct {
		CNA struct {
			Descriptions []struct {
				Lang  string `json:"lang"`
				Value string `json:"value"`
			} `json:"descriptions"`
			Affected []struct {
				Vendor  string `json:"vendor"`
				Product string `json:"product"`
			} `json:"affected"`
			References []struct {
				URL string `json:"url"`
			} `json:"references"`
		} `json:"cna"`
	} `json:"containers"`
}

// cveIDPattern validates CVE-YYYY-NNNN format.
var cveIDPattern = regexp.MustCompile(`^CVE-\d{4}-\d{4,}$`)

// ExtractFile reads a single CVE JSON 5.1 file and converts it to a
// CVERecord. Returns nil for REJECTED/RESERVED (caller skips based on config).
func ExtractFile(path string, descMaxRunes int) (*CVERecord, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("read %s: %w", path, err)
	}

	var raw rawCVE
	if err := json.Unmarshal(data, &raw); err != nil {
		return nil, fmt.Errorf("parse %s: %w", path, err)
	}

	if raw.CveMetadata.CveID == "" || !cveIDPattern.MatchString(raw.CveMetadata.CveID) {
		return nil, nil // skip invalid
	}

	rec := &CVERecord{
		ID:       raw.CveMetadata.CveID,
		State:    raw.CveMetadata.State,
		Date:     raw.CveMetadata.DatePublished,
		Assigner: raw.CveMetadata.AssignerShortName,
	}

	// Extract description (prefer lang=en)
	for _, d := range raw.Containers.CNA.Descriptions {
		if d.Lang == "en" {
			rec.Description = d.Value
			break
		}
	}
	if rec.Description == "" && len(raw.Containers.CNA.Descriptions) > 0 {
		rec.Description = raw.Containers.CNA.Descriptions[0].Value
	}
	if descMaxRunes > 0 && len([]rune(rec.Description)) > descMaxRunes {
		rec.Description = string([]rune(rec.Description)[:descMaxRunes])
	}

	// Extract unique products
	seen := make(map[string]bool)
	for _, a := range raw.Containers.CNA.Affected {
		if a.Product != "" && !seen[a.Product] {
			seen[a.Product] = true
			rec.Products = append(rec.Products, a.Product)
		}
		if a.Vendor != "" && rec.Vendor == "" {
			rec.Vendor = a.Vendor
		}
	}

	// Extract references (max 5)
	for i, r := range raw.Containers.CNA.References {
		if i >= 5 {
			break
		}
		rec.References = append(rec.References, r.URL)
	}

	return rec, nil
}

// ToMarkdown converts a CVERecord to the short .md format for zvec.
func (r *CVERecord) ToMarkdown() string {
	var b strings.Builder
	b.WriteString("---\n")
	fmt.Fprintf(&b, "id: %s\n", r.ID)
	fmt.Fprintf(&b, "state: %s\n", r.State)
	fmt.Fprintf(&b, "year: %s\n", r.ID[4:8])
	b.WriteString("---\n\n")
	fmt.Fprintf(&b, "# %s\n\n", r.ID)
	if len(r.Products) > 0 {
		fmt.Fprintf(&b, "- products: %s\n", strings.Join(r.Products, ", "))
	}
	if r.Vendor != "" {
		fmt.Fprintf(&b, "- vendor: %s\n", r.Vendor)
	}
	if r.Date != "" {
		fmt.Fprintf(&b, "- date: %s\n", r.Date)
	}
	if r.Assigner != "" {
		fmt.Fprintf(&b, "- assigner: %s\n", r.Assigner)
	}
	b.WriteString("\n")
	b.WriteString(r.Description)
	b.WriteString("\n")
	if len(r.References) > 0 {
		b.WriteString("\nrefs:\n")
		for _, ref := range r.References {
			fmt.Fprintf(&b, "- %s\n", ref)
		}
	}
	return b.String()
}

// OutputPath returns the .md output path for a CVE ID within the corpus dir.
func OutputPath(corpusDir, cveID string) string {
	year := cveID[4:8] // CVE-YYYY-NNNN
	return filepath.Join(corpusDir, "cve", year, cveID+".md")
}

// ShouldSkip returns true if the record should be excluded based on config.
func ShouldSkip(r *CVERecord, skipRejected, skipReserved bool) bool {
	if r == nil {
		return true
	}
	if skipRejected && r.State == "REJECTED" {
		return true
	}
	if skipReserved && (r.State == "RESERVED" || r.Description == "") {
		return true
	}
	return false
}
