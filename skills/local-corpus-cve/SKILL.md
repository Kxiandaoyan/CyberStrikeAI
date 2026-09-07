---
name: local-corpus-cve
description: >-
  在本机近5年CVE语料仓（zvec）中检索编号、产品与CNA描述。
  Use when the user mentions CVE-IDs, a vendor/product/version,
  or asks what known issues exist before searching the public web.
metadata:
  tags: [CVE, 情报, zvec, 本地检索]
---

# 本地 CVE 语料仓

官方记录每天被抽成短 Markdown，由 zvec_grep_search 检索。这是查编号和产品的第一路，不是利用手册。

**语料仓分三层**：`cve/{年份}/` 官方层（CNA 描述，每日同步）；`poc/` **实战层** —— 历次交战利用成功后自动沉淀的实战利用记录（利用前提/复现步骤/证据，默认免审批）+ 导入的第三方 POC 复现集；`pocindex/` **公开 PoC 索引层** —— 1.2 万个 CVE 的「公开 GitHub PoC 仓库链接」索引（PocOrExp 聚合，导入/刷新见 tools/import_pocindex.py）。有 CVE 编号的实战记录按编号命名（`CVE-XXXX.md`）；**无 CVE 的实战**（逻辑漏洞/未授权/弱口令/自研系统）按「系统名+漏洞类型」命名，目标 URL/系统名在正文——都能被语义检索命中。实战层只增不减、永被同步覆盖，是「越用越快」的来源。

## 何时用

- 出现 `CVE-YYYY-NNNN`
- 已识别组件+版本，要先看近5年公开记录
- 用户说「有没有已知洞」且范围是产品名，不是「SSRF 怎么测」

## 何时不用

- 「怎么测 / 误报 / 修复步骤」→ search_knowledge_base 或其它方法类 Skill
- 要 0day、在野利用、中文分析长文 → 本地 0 命中后再走 component-vuln-intel 的外网段
- 不要把 CNA 描述当成已授权利用步骤，也不要直接 record_vulnerability

## 怎么调

root 必须是语料仓绝对路径（部署后写死，禁止填 `.`）：

- Windows 示例：`D:\Code_Space\CyberStrikeAI\CyberStrikeAI\data\corpus`
- 已知编号：`{"root":"...","fts":["CVE-2024-23897"],"limit":5}`
- 产品+现象：`{"root":"...","query":"Jenkins CLI file read","limit":8}`
- 编号+语义：`{"root":"...","query":"unauthenticated file read","fts":["jenkins"],"fuse":true,"limit":8}`

limit ≤ 8。可 `globs: ["cve/2025/**"]`。

## 查本地实战 POC（命中编号或识别出系统后必做）

已知编号且要利用时，先查实战层再考虑外网：

```
{"root":"...","fts":["CVE-2024-23897"],"globs":["poc/**"],"limit":5}
```

识别出系统/组件但无编号（自研系统、逻辑漏洞）时，按系统名+漏洞类型语义查：

```
{"root":"...","query":"某OA getfile 任意文件读取","globs":["poc/**"],"limit":8}
```

- 命中「实战利用记录」→ **优先按其步骤复用**（先核对组件版本与利用前提是否仍成立），外网序列可大幅跳过
- 实战层没有但有 CVE 编号 → **再查公开 PoC 索引层**：`{"root":"...","fts":["{CVE-ID}"],"globs":["pocindex/**"],"limit":3}`
  命中 → 拿到公开 PoC 仓库 URL，直接 curl/api 取仓库内容（跳过搜索引擎与 GitHub 搜索步骤）；可信度按 tentative 对待
- 两层都没有 → 有编号按 CVE-ID 定向外网；无编号走 component-vuln-intel 完整外网序列
- 实战记录是历史交战产物：目标是变化的，复用前必须验证前提，禁止无核对直接打

实战层还包含**导入的第三方 POC 集**（`category: imported-poc`，多为「组件名+漏洞类型」命名、
约八成无 CVE 编号）。查法改为语义查询：

```
{"root":"...","query":"Tomcat 任意文件读取 RCE","globs":["poc/**"],"limit":8}
```

有编号的导入条目同样可用 fts 编号拉取（编号在其 frontmatter）。导入来源与日期见各文件
frontmatter 的 `source` / `imported`；导入内容是公开 POC 复现，可信度按 tentative 对待。

## 落到黑板上

有编号+目标版本 → `upsert_project_fact`，key 如 `intel/cve-2024-23897`，confidence=tentative，body 只记「语料摘要 + 目标版本是否匹配」，不记 POC。
