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

## 落到黑板上

有编号+目标版本 → `upsert_project_fact`，key 如 `intel/cve-2024-23897`，confidence=tentative，body 只记「语料摘要 + 目标版本是否匹配」，不记 POC。
