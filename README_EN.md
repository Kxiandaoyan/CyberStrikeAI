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

## What's added on top of upstream

1. **Local 5-year CVE corpus (offline-first vuln intel)** — official cvelistV5 data distilled
   into short Markdown (190k+ records, 2022–2026, no POCs), searched by the bundled
   [zvec-grep](https://github.com/zvec-ai/zvec-grep) service (hybrid semantic/fts, auto
   incremental indexing) plus a paginated browse API and UI page. Component identification
   now hits the local corpus first (`local-corpus-cve` skill); public web sequences run only
   when local hits are insufficient.
2. **Daily incremental CVE sync (cvesync)** — pure-HTTP GitHub delta (commits + compare API,
   no git subprocess); an honest watermark (state advances only after every change lands),
   count gates (≥80% total, per-year floor), 3am daily + 36h catch-up, manual
   `POST /api/cve-corpus/sync?full=1`.
3. **GitHub-C2 handoff (long-term persistence)** — the built-in Beacon has no target-side
   persistence; this fork adds a handoff chain to an independently deployed GitHub-C2
   controller: a human builds the agent and saves a target-reachable download URL in the
   settings page; CS delivers once via the online Beacon, verifies check-in by
   "hostname + newly-appeared", records it on the blackboard and then stops using that
   channel. Two read-only MCP tools (`github_c2_handoff_source`, `github_c2_list_agents`
   with auto re-login) plus a dedicated settings section (credentials write-only) and the
   `handoff-github-c2` skill.
4. **Experience distillation (closeout → human review → library)** — project/batch completion
   auto-generates redacted LLM drafts from blackboard facts (independent cheap model, IPv4
   redaction, min-facts gate, 24h dedup); humans approve via `/api/experience/drafts` into
   the knowledge base or as patches appended to a skill's `SKILL.md`. `auto_apply` is
   hard-locked false; the model has no skill-writing tool in-session. A finished batch queue
   yields exactly one aggregated draft.
5. **Reliability fixes** — settings now persist to `config.yaml` immediately, tools survive
   config re-apply, corpus paths resolve against the config file directory, prompts/skills
   aligned with local-first and handoff discipline.

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
`zvec_grep.enabled`, `cve_corpus.enabled`, `experience.enabled`, `github_c2.*`.

### CVE corpus data (extract it yourself)

The unpacked `data/` tree (190k+ Markdown files) is intentionally not committed; it ships as
`assets/cve-corpus.tar.gz` (36MB). `run.sh` extracts it automatically on first deploy, or:

```bash
mkdir -p data/corpus && tar xzf assets/cve-corpus.tar.gz -C data/corpus
```

The first semantic index build runs in the background after boot (local CPU embedding;
fts works meanwhile); afterwards indexing is maintained incrementally.

## Upstream

See [Ed1s0nZ/CyberStrikeAI](https://github.com/Ed1s0nZ/CyberStrikeAI) for the original
project and its documentation. Fork additions live mostly in `internal/cvesync/`,
`internal/experience/`, `internal/githubc2sidecar/`, `internal/app/github_c2_tools.go`,
`internal/app/experience_apply.go` and the matching frontend pages.

## License

Apache-2.0 (as upstream).
