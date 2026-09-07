<div align="center">
  <img src="images/logo.png" alt="CyberStrikeAI Logo" width="200">
</div>

# CyberStrikeAI（二次开发版）

[中文](README.md) | [English](README_EN.md)

> 本仓库基于开源项目 **[Ed1s0nZ/CyberStrikeAI](https://github.com/Ed1s0nZ/CyberStrikeAI)** 二次开发。
> 感谢原作者 [Ed1s0nZ](https://github.com/Ed1s0nZ) 的高质量工作 —— 单/多智能体交战循环、知识库、
> 内置 C2、HITL 审批与审计等核心能力均来自上游，本仓库在其基础上做增强，未修改上游核心架构。

> [!IMPORTANT]
> 仅可对自有系统或已获得明确授权的目标使用本平台。使用前请确认你的授权范围。

## 相比上游新增了什么

### 1. 本地近 5 年 CVE 语料仓（离线优先的漏洞情报）
- 官方 cvelistV5 数据被抽取为**短 Markdown**（19 万+ 条，2022–2026，不含 POC/验证步骤），
  作为语料仓随仓库分发（见下方「CVE 语料数据」）。
- 内嵌 [zvec-grep](https://github.com/zvec-ai/zvec-grep) 本地检索服务（`./run.sh` 一键构建并启动，
  agent 工具集）：`zvec_grep_search` 支持语义 / fts 混合检索，检索时自动增量索引。
- 另有分页浏览 / 搜索 API（`GET /api/cve-corpus/search`）与前端 CVE 检索页。
- 配套 Skill：`local-corpus-cve`（何时用 / 何时不用 / 怎么调），组件识别后的第一跳是**本地语料**，
  本地不足再走外网序列（`component-vuln-intel` 已改为本地优先）。

### 2. CVE 每日增量同步（cvesync）
- 纯 HTTP 的 GitHub 增量：commits + compare API 分批拿变更文件，只重抽变更项为 md；
  不依赖 git 子进程。
- 诚实水位线：全部变更成功落盘才推进 `state.json` 的 `indexed_at` / `commit`，失败只写
  `last_error` 并在下轮重试；数量闸门（总量 ≥80% 且非新年份单年 ≥ `min_year_files`）防灾难性覆盖。
- 每天本地 3 点定时 + 启动 36h 补跑；`POST /api/cve-corpus/sync?full=1` 可手动全量重建。

### 3. GitHub-C2 交接（长期维权 handoff）
- 内置 C2 Beacon 没有目标侧自启动/长期维权；本版本新增与独立部署的
  [GitHub-C2](https://github.com/) 控制器的**交接**链路：人在 G-C2 控制台生成 Agent →
  在 CS 设置页保存「目标可达的下载地址」→ CS 经在线 Beacon 投递一次 →
  按「主机名 + 新出现」判定上线 → 记黑板后**不再使用该信道**。
- 内置两个只读 MCP 工具：`github_c2_handoff_source`（读已保存的投递配置）、
  `github_c2_list_agents`（refresh + 结构化列表，含自动登录/会话过期重登）。
- 设置页「GitHub-C2 交接」小节独立保存（不校验 OpenAI 必填），控制器凭据**只写不回显**；
  C2 会话页有只读回显。配套 Skill：`handoff-github-c2`（含判定纪律与 `persist/handoff-url-override` 约定）。

### 4. 经验总结（项目收尾 → 人审 → 入库）
- 项目可交付完成 / 批量队列收尾时，自动汇总黑板事实与漏洞记录，经独立 LLM 配置
  （可用便宜模型）生成**脱敏经验草稿**（IPv4 机械脱敏 + 提示词约束；min_facts 门槛 + 24h 去重）。
- 人工在 `/api/experience/drafts` 审批：批准时可选择写入知识库「经验总结」分类，
  或以「经验补丁」段落追加 / 新建到某个 Skill 的 `SKILL.md`。
- `auto_apply` 恒为 false：**没有任何自动入库路径**，会话中模型也没有写 Skill 的工具。
- 批量队列整队只产出**一份**聚合草稿（子会话不重复触发）。

### 5. 其他增强
- 配置持久化与工具注册的若干可靠性修复（设置保存即落 yaml；应用配置后工具完整重挂）。
- CVE 语料目录按配置文件目录解析相对路径，支持任意工作目录启动。
- `orchestrator.md` / 单代理提示词 / `pentest-blackboard` 等提示词与本地语料、交接纪律对齐。

## 仓库包含

| 目录 | 内容 |
|---|---|
| `cmd/` `internal/` | Go 主程序（Gin + Eino 智能体、MCP 服务端/客户端、C2、知识库、HITL、审计） |
| `web/` | 前端（设置 / 会话 / C2 / CVE 检索 / 经验草稿审批页） |
| `skills/` `agents/` `roles/` | 26+ 渗透 Skills、多代理编排角色、角色配置 |
| `tools/` | 90+ 安全工具封装（nmap/sqlmap/nuclei/…，YAML 定义） |
| `knowledge_base/` | 随库知识内容（注入类方法论等） |
| `mcp-servers/` | 示例外部 MCP（reverse_shell 等） |
| `zvec-grep/` | 内嵌的 zvec-grep 0.2.1 源码（Apache-2.0，`./run.sh` 自动构建） |
| `assets/cve-corpus.tar.gz` | **预构建 CVE 语料包**（36MB，解压后 19 万+ md，见下节） |
| `run.sh` `upgrade.sh` | Linux 一键部署 / 升级脚本 |
| `config.example.yaml` | 配置模板（含新功能全部开关与说明） |

## 部署（Linux）

```bash
git clone https://github.com/Kxiandaoyan/CyberStrikeAI.git
cd CyberStrikeAI
./run.sh            # 首次运行：编译 Go、构建 zvec-grep、解压 CVE 语料、启动服务
```

要求：Go 1.25+、Python 3（知识库依赖）、Node ≥ 22（zvec-grep 检索；无 Node 也可启动，仅无本地 CVE 检索）。

首次启动后：
1. 复制/对照 `config.example.yaml` 生成 `config.yaml`（二进制首次启动也会自动从模板创建）。
2. 按需打开开关：`zvec_grep.enabled`（本地 CVE 检索）、`cve_corpus.enabled`（每日增量同步）、
   `experience.enabled` + `auto_draft`（经验草稿）、`github_c2`（交接配置，可在网页设置页保存）。
3. 浏览器打开 `https://127.0.0.1:8080`（自签证书；`./run.sh --http` 用纯 HTTP），
   首次启动日志会打印管理员初始密码。

### CVE 语料数据（需自行解压）

仓库**不包含**解压后的 `data/` 目录（19 万+ md 文件，直接入库太重），以
`assets/cve-corpus.tar.gz`（36MB）形式分发：

```bash
# 方式一：run.sh 首次部署会自动解压
# 方式二：手动解压
mkdir -p data/corpus
tar xzf assets/cve-corpus.tar.gz -C data/corpus
# 解压后：data/corpus/cve/{2022..2026}/CVE-*.md
```

解压后即可使用 CVE 检索页与 `zvec_grep_search`；首次语义索引由 `run.sh` 在后台构建
（CPU 本地嵌入模型，19 万条约需一段时间，期间 fts 检索可用），之后增量自动维护。

## 与上游同步

上游更新可对照 [Ed1s0nZ/CyberStrikeAI](https://github.com/Ed1s0nZ/CyberStrikeAI) 手动合并；
本仓库新增代码集中在 `internal/cvesync/`、`internal/experience/`、`internal/githubc2sidecar/`、
`internal/app/github_c2_tools.go`、`internal/app/experience_apply.go` 与对应前端页。

## License

Apache-2.0（随上游）。
