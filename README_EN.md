<div align="center">
  <img src="images/logo.png" alt="CyberStrikeAI Logo" width="200">
</div>

# CyberStrikeAI (Enhanced Fork)

[中文](README.md) | [English](README_EN.md)

> This repository is a secondary development on top of the open-source project
> **[Ed1s0nZ/CyberStrikeAI](https://github.com/Ed1s0nZ/CyberStrikeAI)**.
> Credits to [Ed1s0nZ](https://github.com/Ed1s0nZ) — the core platform (single/multi-agent
> engagement loop, knowledge base, built-in C2, HITL approval and audit) comes from upstream.
> This fork adds capabilities without altering the upstream architecture.

> [!IMPORTANT]
> Use this platform only against systems you own or are explicitly authorized to test.

## Why a local CVE corpus (vs. the upstream flow)

Upstream, every new target and every identified component re-runs the full 7-step external
intel sequence (CVE DBs → 3+ search engines → Chinese communities → GitHub PoC → asset
engines → live intel → dependency chain). The same Jenkins, the same CVE — ten projects means
ten identical web sweeps: **slow, fingerprinted, and everything found is thrown away**.

This fork turns "search" into "lookup" and "throw away" into "accumulate":

| | Upstream (full web sweep each time) | This fork (local-first + combat accumulation) |
|---|---|---|
| Component identified | run the entire external sequence | **first hit is the local corpus** (190k+ official records, sub-second, offline, zero egress) |
| A CVE you've exploited before | search the web from scratch again | **check the local POC library first** → reuse the working recipe, skip most external steps |
| Knowledge found | used once, discarded | exploit succeeds → **auto-written into the local library**, grows forever |
| Freshness | whatever the sweep returns | official layer synced daily from CNA (1-2 days ahead of NVD) |

The loop: engagement N exploits a CVE → confirmed vulnerability record → closeout **automatically**
files the POC (IPv4-redacted; `poc_auto_apply` on by default, can be switched back to manual review)
into `data/corpus/poc/` → **engagement N+1 hits it locally and never searches the web for it again**.
The system gets faster and smarter with every engagement.

**Local accelerates, never truncates**: on a local miss the full upstream external sequence runs
unchanged, step for step — a miss may be a keyword miss (component aliases / naming variants)
rather than absence of records, the external sources cross-cover each other, and their all-years
coverage also backfills history beyond the 5-year corpus window.

## What's added on top of upstream

1. **Local 5-year CVE corpus (offline-first vuln intel)** — official cvelistV5 data distilled
   into short Markdown (190k+ records, 2022–2026, no POCs), searched by the bundled
   [zvec-grep](https://github.com/zvec-ai/zvec-grep) service (hybrid semantic/fts, auto
   incremental indexing) plus a paginated browse API and UI page. Component identification
   now hits the local corpus first (`local-corpus-cve` skill); external sequences run only
   when local hits are insufficient, and then target the CVE-ID directly.
   Three layers: `cve/` official (daily sync), `poc/` combat (see #3), and
   **`pocindex/` — a public-PoC-repo index** split from
   [PocOrExp_in_Github](https://github.com/ycdxsb/PocOrExp_in_Github) (~12k CVEs, one md
   each, refresh by re-running `tools/import_pocindex.py`). When the combat layer misses,
   one local lookup returns direct PoC repo URLs — no GitHub searching needed.
2. **Daily incremental CVE sync (cvesync)** — pure-HTTP GitHub delta (commits + compare API,
   no git subprocess); an honest watermark (state advances only after every change lands),
   count gates (≥80% total, per-year floor), 3am daily + 36h catch-up, manual
   `POST /api/cve-corpus/sync?full=1`.
3. **Combat POC accumulation (the "faster over time" core, auto by default)** — after a
   confirmed vulnerability, closeout mechanically assembles the POC from the
   vulnerability record (reproduction steps verbatim, no LLM rewrite, IPv4-redacted; one
   entry per vulnerability ever) and **writes it straight into `data/corpus/poc/`** —
   repeated engagements append dated sections. `poc_auto_apply` is on by default; set it
   to false to route POCs through the human approval queue instead. **Exploits without a
   CVE id are captured too**: logic flaws, unauthorized access, weak creds and custom
   systems are keyed by「system + vuln type」with the target URL/system name in the body,
   so a semantic query finds them next time. Every auto-write leaves a visible trace on
   the drafts page (reviewer=auto, full content). The daily sync never touches the POC
   layer; `data/` is git-ignored so combat records never reach the public repo. Next
   engagement: `fts/semantic + globs:["poc/**"]` pulls the recipe directly.
- **Third-party POC collections can be imported** (wiki-style repos mostly named
  「component + vuln type」, often without CVE ids):
  `python tools/import_poc_repo.py <cloned repo> data/corpus/poc --source-name <origin>`
  — Chinese filenames preserved, CVE ids extracted from names/content into frontmatter,
  `category` distinguishes imports from auto-filed combat records, idempotent re-runs.
  After import, semantic queries by product/vuln-type hit them through the same poc/**
  index glob. Imported content stays local; check the source repo's license before
  redistributing.
4. **Custom persistence-C2 handoff (long-term retention)** — the built-in Beacon has no
   target-side persistence; this fork adds a handoff chain to **any persistence C2 you deploy
   yourself**: a human builds/hosts the agent, saves a target-reachable download URL in the
   settings page; CS delivers once via the online Beacon, verifies check-in by
   "hostname + newly-appeared", records it on the blackboard and then stops using that
   channel. **Integrating your own C2** (the controller runs wherever you deployed it; CS
   talks HTTP only and never builds agents) requires exactly three endpoints:
   `POST /login` (form username/password → session cookie), `POST /api/agents/refresh`
   (channel scan; 4xx ignorable), and `GET /api/agents` (agent-list JSON with
   hostname/username/os/id/agent_uuid/channel/last_seen/last_reply_ago). Two read-only
   MCP tools (`persistence_c2_handoff_source`, `persistence_c2_list_agents` with auto
   re-login) plus a dedicated settings section (credentials write-only), a read-only echo
   on the sessions page, and the `handoff-persistence-c2` skill (which also forbids using
   the built-in `c2_task persist` as long-term retention).
5. **Experience distillation (closeout → human review → library)** — project/batch completion
   auto-generates redacted LLM drafts from blackboard facts (independent cheap model, IPv4
   redaction, min-facts gate, 24h dedup); **humans approve them on the「经验草稿」(drafts)
   page** — into the knowledge base or as patches appended to a skill's `SKILL.md`.
   **Clean division of labor with POC accumulation**: methodology drafts (LLM + min-facts +
   human approval, `auto_apply` hard-locked false) distill cross-project method; POCs
   (mechanical, auto-filed) record per-vulnerability exploitation into the private local
   library. A successful engagement produces both. The model has no skill-writing tool
   in-session; a finished batch queue yields exactly one aggregated draft.
6. **Reliability fixes & scan-resistant login** — unauthenticated visitors get a bare
   minimal login page (no product name, icon, background or static references); the whole
   JS/CSS bundle and the favicon return 404 until authenticated, so Censys/Shodan-style
   fingerprinting finds nothing identifiable. Settings now persist to `config.yaml`
   immediately, tools survive config re-apply, corpus paths resolve against the config
   file directory, prompts/skills aligned with local-first and handoff discipline.

## Repository layout

`cmd/`+`internal/` Go server (Gin + Eino agents, MCP, C2, knowledge, HITL, audit) ·
`web/` frontend · `skills/` `agents/` `roles/` 26+ offensive skills & orchestrator roles ·
`tools/` 90+ security tool wrappers · `zvec-grep/` bundled search engine (Apache-2.0) ·
`assets/cve-corpus.tar.gz` pre-built corpus archive · `run.sh`/`upgrade.sh` deploy scripts ·
`config.example.yaml` config template.

## Deploy (Linux)

```bash
git clone https://github.com/Kxiandaoyan/CyberStrikeAI.git
cd CyberStrikeAI
./run.sh    # builds Go + zvec-grep, extracts the CVE corpus, starts the server
```

Requirements: Go 1.25+, Python 3, Node ≥ 22 (for local CVE search; optional).
Open `https://127.0.0.1:8080` (self-signed; `./run.sh --http` for plain HTTP); the bootstrap
admin password is printed on first start. Enable features via `config.yaml`:
`zvec_grep.enabled`, `cve_corpus.enabled`, `experience.enabled`, `persistence_c2.*`.

### CVE corpus data (extract it yourself)

The unpacked `data/` tree (190k+ Markdown files) is intentionally not committed; it ships as
`assets/cve-corpus.tar.gz` (36MB). `run.sh` extracts it automatically on first deploy, or:

```bash
mkdir -p data/corpus && tar xzf assets/cve-corpus.tar.gz -C data/corpus
# official layer: data/corpus/cve/{2022..2026}/CVE-*.md
# combat layer:   data/corpus/poc/ (created automatically, fed by your own engagements)
```

The first semantic index build runs in the background after boot (local CPU embedding;
fts works meanwhile); afterwards indexing is maintained incrementally.

### POC stock (auto-downloaded at deploy, never shipped in this repo)

On first deploy `./run.sh` downloads and imports two public stocks (idempotent;
`SKIP_POC_STOCK=1` opts out):

- **Public PoC index** `data/corpus/pocindex/` — a [PocOrExp_in_Github](https://github.com/ycdxsb/PocOrExp_in_Github)
  snapshot (~5MB) split into ~12k per-CVE「CVE → public PoC repo」index files; refresh by
  `git pull`-ing the source and re-running `tools/import_pocindex.py`.
- **Wiki combat stock** `data/corpus/poc/` — a [Vulnerability-Wiki-PoC](https://github.com/SourByte05/Vulnerability-Wiki-PoC)
  snapshot (~305MB, one-time) importing ~870 reproduction articles (the source repo
  carries no license — **local use only, do not redistribute**).

The stock is intentionally not committed: source-license constraints on one hand, and
derived data that any deploy box can fetch with a single curl on the other.

## Upstream

See [Ed1s0nZ/CyberStrikeAI](https://github.com/Ed1s0nZ/CyberStrikeAI) for the original
project and its documentation. Fork additions live mostly in `internal/cvesync/`,
`internal/experience/`, `internal/persistencec2sidecar/`, `internal/app/persistence_c2_tools.go`,
`internal/app/experience_apply.go` and the matching frontend pages.

## License

Apache-2.0 (as upstream).
