---
name: component-vuln-intel
description: >-
  识别组件后先查本机近5年CVE语料（zvec），无命中或需在野/分析长文再联网。
  Use when a framework/component/version is identified; local corpus first,
  then public search. Search hits are tentative, not confirmed vulns.
metadata:
  tags: [渗透测试, penetration-testing, 红队, CVE, 情报]
---

## 组件漏洞情报（本地语料优先，不足再外网）

一识别出 {C}+{V}：**先 local-corpus-cve / zvec_grep_search**，再决定是否外网。
不搜就利用仍是盲打；但「搜」的第一跳是**本地语料**，不是 circl。

```
1.本地 CVE（必做第一跳）：按 local-corpus-cve 的调用方式使用 zvec_grep_search。
  有命中 → tentative Fact `intel/{cve-id}`，核对版本后再谈验证。然后分两路：
    a. 只需判断有无洞/写报告 → 到此为止，Fact 落库即可。
    b. 要利用/复现 → 先按编号查本地实战层 `{"root":"...","fts":["{CVE-ID}"],"globs":["poc/**"]}`：
       命中实战利用记录 → 核对前提后优先复用，外网序列仅补缺口（元数据/在野）；
       实战层没有 → 进下列外网序列，且搜索关键词**优先用 {CVE-ID} 定向**（如 `{CVE-ID}+poc`、
       GitHub 搜 `{CVE-ID}`）而非只用 {C}+{V} 盲搜。
  0 命中 → **不要断言无洞**：语料窗口只有近 5 年；版本早于 2022 时先做 2a 历史补查
  （Advisories/CVEDetails/circl 全年份覆盖），确属近年组件则从第 3 步起。

2.外网序列（本地不足时按序执行）,用{C}=组件名 {V}=版本替换:

2a.CVE漏洞库:
  terminal: searchsploit {C} {V}
  terminal: curl -s "https://cve.circl.lu/api/search/{C}/{V}" | python3 -c "import sys,json;[print(x['id'],x.get('summary','')[:80]) for x in json.load(sys.stdin)[:10]]"
  browser_navigate: https://github.com/advisories?query={C}+{V}
  browser_navigate: https://www.cvedetails.com/google-search-results.php?q={C}+{V}&sa=Search

2.搜索引擎(至少执行3个,找漏洞分析+PoC):
  browser_navigate: https://www.google.com/search?q={C}+{V}+exploit+PoC+RCE+site:github.com
  browser_navigate: https://www.google.com/search?q={C}+{V}+漏洞+利用+复现
  browser_navigate: https://www.baidu.com/s?wd={C}+{V}+漏洞+利用+poc+getshell
  browser_navigate: https://www.bing.com/search?q={C}+{V}+CVE+exploit+poc
  browser_navigate: https://duckduckgo.com/?q={C}+{V}+vulnerability+exploit

3.中文安全社区(必做,中文首发多且深度分析好):
  browser_navigate: https://xz.aliyun.com/search?keyword={C}+漏洞
  browser_navigate: https://www.seebug.org/search/?keywords={C}
  browser_navigate: https://paper.seebug.org/search/?keyword={C}
  browser_navigate: https://www.freebuf.com/search?search={C}+{V}
  browser_navigate: https://ti.qianxin.com/vulnerability?keyword={C}
  browser_navigate: https://www.anquanke.com/search?s={C}

4.GitHub搜PoC/exploit代码(必做,最直接拿利用代码):
  terminal: curl -s "https://api.github.com/search/repositories?q={C}+{V}+exploit+OR+poc+OR+CVE&sort=updated&per_page=10" | python3 -c "import sys,json;d=json.load(sys.stdin);[print(x['full_name'],x['html_url'],x.get('description','')[:60]) for x in d.get('items',[])]"
  terminal: curl -s "https://api.github.com/search/code?q={C}+RCE+OR+shell+OR+exploit+language:python&per_page=5" | python3 -c "import sys,json;d=json.load(sys.stdin);[print(x['html_url']) for x in d.get('items',[])]"
  terminal: curl -s "https://api.github.com/search/repositories?q={C}+CVE&sort=stars&per_page=5" | python3 -c "import sys,json;d=json.load(sys.stdin);[print(x['full_name'],x['stargazers_count'],'★',x.get('description','')[:50]) for x in d.get('items',[])]"
  找到仓库后: curl -s "https://api.github.com/repos/{owner}/{repo}/readme" | python3 -c "import sys,json,base64;print(base64.b64decode(json.load(sys.stdin)['content']).decode())"

5.资产引擎(找同类目标/暴露面):
  browser_navigate: https://fofa.info/result?qbase64=$(echo -n 'app="{C}"' | base64)
  browser_navigate: https://www.shodan.io/search?query={C}+{V}
  browser_navigate: https://www.zoomeye.org/searchResult?q={C}
  browser_navigate: https://search.censys.io/search?resource=hosts&q=services.software.product:{C}

6.即时情报(最新0day/在野利用):
  browser_navigate: https://x.com/search?q={C}+CVE+OR+0day+OR+exploit&f=live
  browser_navigate: https://www.reddit.com/r/netsec/search/?q={C}&sort=new&t=month
  browser_navigate: https://www.exploit-db.com/search?q={C}

7.扩展链(必做): 搜完{C}后,提取其依赖清单(package.json/pom.xml/requirements.txt/go.mod)→对每个依赖重复1-6

🔴搜索受阻处理序列(碰到403/验证码/空结果/超时→按序执行不放弃):
  ①换UA: curl -H "User-Agent: Mozilla/5.0 (compatible; Googlebot/2.1; +http://www.google.com/bot.html)" "{URL}"
  ②Jina读取器: browser_navigate: https://r.jina.ai/{原始URL}
  ③Google缓存: browser_navigate: https://webcache.googleusercontent.com/search?q=cache:{域名}+{关键词}
  ④Archive: browser_navigate: https://web.archive.org/web/{URL}
  ⑤GitHub API替代(GitHub页面拦但API不拦): 用上面第4步的curl命令
  ⑥换引擎: Google拦→执行Bing/DuckDuckGo/百度; 百度拦→执行Google/Bing
  ⑦走代理: 按 `proxy-tool-bootstrap` 序列获取SOCKS5代理后重试
  全部受阻仍无结果→写负Fact"已搜{C} {V}全渠道无公开漏洞"→转 `zero-day-discovery`
```

