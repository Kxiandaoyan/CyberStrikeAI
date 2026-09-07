#!/usr/bin/env python3
# -*- coding: utf-8 -*-
"""import_poc_repo.py — 把第三方 POC 仓库导入本地实战库（data/corpus/poc/）。

为什么需要：外部 POC 集（如 Vulnerability-Wiki-PoC 类仓库）大多以「组件名+漏洞类型」
命名、约八成无 CVE 编号，进不了按编号组织的官方层。导入到实战层后：
  - zvec 索引已含 poc/**（run.sh 首建索引自带 glob），产品名/漏洞类型的语义检索直接命中
  - 文件名或正文中的 CVE 编号会被提取进 frontmatter，fts 可按编号拉
  - frontmatter category=imported-poc 与交战自动沉淀（category=auto）区分来源

只做本地文件处理：不联网、不起子进程。先把源仓库克隆/解压到本地，再运行本脚本。

用法：
  git clone --depth 1 https://github.com/<org>/<poc-repo> /tmp/pocrepo
  python tools/import_poc_repo.py /tmp/pocrepo data/corpus/poc \
      --source-name "github.com/<org>/<poc-repo>" [--force]

重跑安全：目标已存在的文件默认跳过（--force 覆盖）。附件（图片/脚本）不导入，仅 md。
导入内容仅存本机（data/ 不入 git）；对外分发需自行确认源仓库许可。
"""
import argparse
import datetime
import os
import re
import sys

CVE_RE = re.compile(r"CVE-\d{4}-\d{4,}", re.IGNORECASE)
UNSAFE_CHARS = re.compile(r'[\\/:*?"<>|\r\n\t]')


def sanitize(name: str) -> str:
    """文件名清洗：去掉路径分隔与 Windows/Linux 非法字符，保留中文与常用符号。"""
    name = UNSAFE_CHARS.sub(" ", name)
    name = re.sub(r"\s+", " ", name).strip().strip(".")
    name = name[:120].strip()
    return name or "poc"


def first_cve(*texts: str) -> str:
    for t in texts:
        m = CVE_RE.search(t or "")
        if m:
            return m.group(0).upper()
    return ""


def main() -> int:
    ap = argparse.ArgumentParser(description="导入第三方 POC 仓库到本地实战库")
    ap.add_argument("source", help="已克隆/解压到本地的 POC 仓库目录")
    ap.add_argument("poc_dir", help="目标实战库目录（如 data/corpus/poc）")
    ap.add_argument("--source-name", default="", help="来源标注（仓库名或 URL，写进 frontmatter）")
    ap.add_argument("--force", action="store_true", help="覆盖已存在的目标文件（默认跳过）")
    args = ap.parse_args()

    src = os.path.abspath(args.source)
    dst = os.path.abspath(args.poc_dir)
    if not os.path.isdir(src):
        print(f"[error] source dir not found: {src}")
        return 1
    os.makedirs(dst, exist_ok=True)

    today = datetime.date.today().isoformat()
    source_name = args.source_name or os.path.basename(src.rstrip("/\\")) or "imported"
    imported = skipped = failed = cve_tagged = 0
    seen_targets = {}

    md_files = []
    for root, _dirs, files in os.walk(src):
        if ".git" in root.split(os.sep):
            continue
        for f in files:
            if f.lower().endswith(".md") and f.lower() != "readme.md":
                md_files.append(os.path.join(root, f))

    for path in sorted(md_files):
        try:
            with open(path, "r", encoding="utf-8", errors="replace") as fh:
                content = fh.read()
        except OSError as e:
            print(f"[warn] read failed: {path}: {e}")
            failed += 1
            continue

        rel = os.path.relpath(path, src)
        parts = rel.split(os.sep)
        # 首段为 4 位年份目录时并入目标名，避免同名冲突（如 2024/xxx 与 2025/xxx）
        year = parts[0] if len(parts) > 1 and re.fullmatch(r"(19|20)\d{2}", parts[0]) else ""
        stem = os.path.splitext(os.path.basename(path))[0]
        base = sanitize(f"{year}-{stem}" if year else stem)

        target = os.path.join(dst, base + ".md")
        if target in seen_targets:
            seen_targets[target] += 1
            target = os.path.join(dst, f"{base}-{seen_targets[target]}.md")
        else:
            seen_targets[target] = 1
        if os.path.exists(target) and not args.force:
            skipped += 1
            continue

        cve = first_cve(stem, content[:4000])
        fm = [
            "---",
            f"title: {stem}",
            f"category: imported-poc",
            f"source: {source_name}",
            f"imported: {today}",
        ]
        if cve:
            fm.append(f"cve: {cve}")
        fm.append("---\n")

        try:
            with open(target, "w", encoding="utf-8") as fh:
                fh.write("\n".join(fm))
                fh.write("\n# " + stem + "\n\n")
                fh.write(content)
        except OSError as e:
            print(f"[warn] write failed: {target}: {e}")
            failed += 1
            continue
        imported += 1
        if cve:
            cve_tagged += 1

    print(f"[done] imported={imported} (cve-tagged={cve_tagged}) "
          f"skipped(exists)={skipped} failed={failed} -> {dst}")
    print("note: 附件未导入；重跑默认跳过已存在文件；如索引未含 poc/** 请按 run.sh 提示重建一次")
    return 0


if __name__ == "__main__":
    sys.exit(main())
