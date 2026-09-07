#!/usr/bin/env python3
"""Convert local cvelistV5 JSON to short Markdown for zvec indexing.

Usage: python tools/convert_cve.py <raw_dir> <corpus_dir> [years]
  raw_dir:    path to cvelistV5-main/cves (contains year subdirs)
  corpus_dir: output path for .md files (data/corpus)
  years:      comma-separated (default: last 5 years from current UTC year)

Skips REJECTED and RESERVED (empty description) records.
"""
import json
import os
import re
import sys
from concurrent.futures import ProcessPoolExecutor
from pathlib import Path

CVE_PATTERN = re.compile(r"^CVE-\d{4}-\d{4,}$")


def extract_one(args):
    """Extract a single CVE JSON file to Markdown. Returns cve_id or None."""
    json_path, desc_max, corpus_dir = args
    try:
        with open(json_path, "r", encoding="utf-8") as f:
            data = json.load(f)
    except (json.JSONDecodeError, OSError):
        return None

    meta = data.get("cveMetadata", {})
    cve_id = meta.get("cveId", "")
    state = meta.get("state", "")

    if not CVE_PATTERN.match(cve_id):
        return None
    if state == "REJECTED":
        return None

    # Extract description (prefer lang=en)
    containers = data.get("containers", {})
    cna = containers.get("cna", {})
    desc = ""
    for d in cna.get("descriptions", []):
        if d.get("lang") == "en":
            desc = d.get("value", "")
            break
    if not desc and cna.get("descriptions"):
        desc = cna["descriptions"][0].get("value", "")

    # Skip RESERVED (empty description)
    if not desc.strip():
        return None

    # Truncate
    if len(desc) > desc_max:
        desc = desc[:desc_max]

    # Extract products and vendor
    products = []
    vendor = ""
    seen = set()
    for a in cna.get("affected", []):
        prod = a.get("product", "")
        if prod and prod not in seen:
            seen.add(prod)
            products.append(prod)
        if not vendor and a.get("vendor"):
            vendor = a["vendor"]

    # Extract references (max 5)
    refs = [r.get("url", "") for r in cna.get("references", [])[:5] if r.get("url")]

    # Build Markdown
    year = cve_id[4:8]
    lines = [
        "---",
        f"id: {cve_id}",
        f"state: {state}",
        f"year: {year}",
        "---",
        "",
        f"# {cve_id}",
        "",
    ]
    if products:
        lines.append(f"- products: {', '.join(products)}")
    if vendor:
        lines.append(f"- vendor: {vendor}")
    date = meta.get("datePublished", "")
    if date:
        lines.append(f"- date: {date[:10]}")
    assigner = meta.get("assignerShortName", "")
    if assigner:
        lines.append(f"- assigner: {assigner}")
    lines.append("")
    lines.append(desc)
    lines.append("")
    if refs:
        lines.append("refs:")
        for r in refs:
            lines.append(f"- {r}")

    out_path = Path(args[2]) / "cve" / year / f"{cve_id}.md"
    out_path.parent.mkdir(parents=True, exist_ok=True)
    out_path.write_text("\n".join(lines), encoding="utf-8")
    return cve_id


def main():
    if len(sys.argv) < 3:
        print(__doc__)
        sys.exit(1)

    raw_dir = Path(sys.argv[1])
    corpus_dir = Path(sys.argv[2])
    desc_max = 2000

    # Determine year range
    from datetime import datetime, timezone
    now_year = datetime.now(timezone.utc).year
    if len(sys.argv) > 3:
        years = [int(y) for y in sys.argv[3].split(",")]
    else:
        years = list(range(now_year - 4, now_year + 1))

    # Collect all JSON files
    jobs = []
    for year in years:
        year_dir = raw_dir / "cves" / str(year)
        if not year_dir.exists():
            print(f"skip {year}: dir not found")
            continue
        for f in year_dir.rglob("*.json"):
            jobs.append((str(f), desc_max, str(corpus_dir)))

    print(f"converting {len(jobs)} JSON files ({years[0]}-{years[-1]})...")

    # Process with thread pool (I/O bound)
    count = 0
    with ProcessPoolExecutor(max_workers=8) as pool:
        for result in pool.map(extract_one, jobs, chunksize=100):
            if result:
                count += 1
                if count % 10000 == 0:
                    print(f"  {count} done...", flush=True)

    print(f"done: {count} CVE Markdown files written to {corpus_dir}/cve/")


if __name__ == "__main__":
    main()
