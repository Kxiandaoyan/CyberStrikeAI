#!/usr/bin/env python3
# -*- coding: utf-8 -*-
"""import_pocindex.py — 把 ycdxsb/PocOrExp_in_Github 年度索引拆分导入本地 pocindex 层。

来源仓库结构：{year}/README.md，每个条目形如
    ## CVE-2024-58290
     <一行描述>
    - [repo](repo) : ![starts](badges...)
每日由上游自动聚合「CVE → 公开 PoC 仓库」链接。本脚本按 `## CVE-` 标题切分成
data/corpus/pocindex/CVE-XXXX.md（frontmatter 带编号/来源/日期，正文保留描述与仓库链接，
shields.io 徽章清除），zvec 通过 pocindex/** glob 检索。

与实战层（poc/）分工：pocindex = 「哪里有公开 PoC」（链接索引，上游保鲜，重跑覆盖更新）；
poc = 「我们自己怎么打的」（交战沉淀 + 导入的复现文章，只增不减）。

只做本地文件处理：先自行克隆/下载源仓库（git clone --depth 1 即可，工作树仅 ~12MB），
再运行本脚本。重跑语义 = 刷新（上游是唯一事实源，已存在文件覆盖更新）。

用法：
  git clone --depth 1 https://github.com/ycdxsb/PocOrExp_in_Github /tmp/pocindex-src
  python tools/import_pocindex.py /tmp/pocindex-src data/corpus/pocindex \
      --source-name "github.com/ycdxsb/PocOrExp_in_Github"
"""
import argparse
import datetime
import os
import re
import sys

CVE_HEAD = re.compile(r"^## (CVE-\d{4}-\d{4,})\s*$", re.M)
BADGE = re.compile(r"\s*:?\s*!\[[^\]]*\]\(https://img\.shields\.io/[^)]*\)")
UNSAFE_CHARS = re.compile(r'[\\/:*?"<>|\r\n\t]')


def sanitize_cve(cve: str) -> str:
    cve = UNSAFE_CHARS.sub("", cve).strip().upper()
    return cve if re.fullmatch(r"CVE-\d{4}-\d{4,}", cve) else ""


def main() -> int:
    ap = argparse.ArgumentParser(description="拆分导入 PocOrExp 年度索引到 pocindex 层")
    ap.add_argument("source", help="已克隆到本地的 PocOrExp_in_Github 仓库目录")
    ap.add_argument("out_dir", help="目标目录（如 data/corpus/pocindex）")
    ap.add_argument("--source-name", default="github.com/ycdxsb/PocOrExp_in_Github")
    args = ap.parse_args()

    src = os.path.abspath(args.source)
    dst = os.path.abspath(args.out_dir)
    if not os.path.isdir(src):
        print(f"[error] source dir not found: {src}")
        return 1
    os.makedirs(dst, exist_ok=True)

    today = datetime.date.today().isoformat()
    written = 0
    entries_total = 0

    # 每个年份 README 独立拆分；同年份文件里的同编号去重（保留后出现的=更新）
    for entry in sorted(os.listdir(src)):
        yeardir = os.path.join(src, entry)
        if not (os.path.isdir(yeardir) and re.fullmatch(r"(19|20)\d{2}", entry)):
            continue
        readme = os.path.join(yeardir, "README.md")
        if not os.path.isfile(readme):
            continue
        try:
            with open(readme, "r", encoding="utf-8", errors="replace") as fh:
                text = fh.read()
        except OSError as e:
            print(f"[warn] read failed: {readme}: {e}")
            continue

        parts = CVE_HEAD.split(text)
        # split 结果: [前言, id1, 正文1, id2, 正文2, ...]
        for i in range(1, len(parts) - 1, 2):
            cve = sanitize_cve(parts[i])
            body = parts[i + 1]
            if not cve:
                continue
            body = BADGE.sub("", body).strip()
            if not body:
                continue
            entries_total += 1
            content = (
                "---\n"
                f"cve: {cve}\n"
                "category: pocindex\n"
                f"source: {args.source_name}\n"
                f"imported: {today}\n"
                f"year: {entry}\n"
                "---\n\n"
                f"# {cve} 公开 PoC 仓库索引\n\n"
                f"{body}\n"
            )
            out = os.path.join(dst, cve + ".md")
            try:
                with open(out, "w", encoding="utf-8") as fh:
                    fh.write(content)
                written += 1
            except OSError as e:
                print(f"[warn] write failed: {out}: {e}")

    print(f"[done] entries={entries_total} written={written} -> {dst}")
    print("note: 重跑=刷新（上游为唯一事实源，覆盖更新）；更新方式: cd <源仓库> && git pull && 重跑本脚本")
    return 0


if __name__ == "__main__":
    sys.exit(main())
