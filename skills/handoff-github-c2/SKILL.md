---
name: handoff-github-c2
description: >-
  CS 内置 C2 没有自启动和维权。Beacon 会话一旦在线，按设置页已保存的下载地址投递人编好的 GitHub-C2 Agent，再按主机名+新出现判定上线，然后不再使用该信道。
  Use when a CyberStrikeAI Beacon session is online and github_c2.payload_url
  has been saved by the operator; CS only delivers and checks check-in.
metadata:
  tags: [交接, handoff, GitHub-C2, 权限维持]
---

# CS Beacon → GitHub-C2 交接（人编包+人保存下载地址，CS 投递+验上线）

CS Beacon 只服务「这次评估、CS 还开着」。要留下目标侧长期线：人先在 GitHub-C2 控制台生成，再把 **目标可达的下载地址** 填进 CS「设置 → C2 → GitHub-C2 交接」并保存。

**生成端不在本 Skill。下载地址也不由本 Skill 填写。** 禁止 builder，禁止改 URL。

## 前提

- `c2_session` get：status 为 active 或 sleeping，hostname 不是空/unknown。
- `github_c2_handoff_source`（或配置）里已有 payload_url。没有 → 停，请人去设置页保存。
- GitHub-C2 信道已由人配好。

## 按序做

1. `c2_session` get，钉死 session_id、hostname、username、os。
2. 读已保存的 payload_url / drop_path。不要编、不要改。
   - 操作员在对话里口头给了**当次**下载地址时：只允许写进黑板 `persist/handoff-url-override`（`upsert_project_fact`，confidence=confirmed），投递时它**优先于**配置里的 URL。仍然禁止模型自己编造或改写任何地址。
3. `github_c2_list_agents` 拍 baseline（实现会先 refresh）。
4. `c2_task` 按该 URL 投递并拉起（HITL）。不要发明隐蔽安装。现网 `c2_file` 不能上传，不要去调它的 upload。
5. 在 wait_seconds（默认 180）内每约 15s 再 list。判定：
   - hostname 小写全等（禁止 unknown==unknown）
   - username / OS 能解析时一致
   - 且 (channel,id) 或 agent_uuid 不在 baseline，或 last_seen ≥ 投递时刻
   - 不要要求 last_reply_ago<300（新登记常常还没回过命令，MCP 会显示 unknown）
6. 命中 → upsert_project_fact key=`persist/handoff-<session_id>` confirmed，body 记 g_id/g_uuid/hostname/channel 与 CS session 对上，CS 不再使用该信道。
7. **停止**调用 github_c2_*。评估继续只用 c2_*。

## 不要做

- **不要调 `c2_task` 的 `task_type=persist`**——那是 CS Beacon 自带的临时自启动（cron/bashrc/registry），不可靠且和 G-C2 维权重复。维权一律走本 Skill 的 GitHub-C2 交接
- 没有 Beacon 或没有已保存 URL 就去种
- 调 builder，或让模型填/改下载地址、回连地址
- 用 github_c2_run_command 当工作通道
- 用 c2_task 打 GitHub-C2 的 agent id；用 s_ 去对 G-C2 的 id
- 把 baseline 里的旧 Agent 当成成功
- 登记失败后死循环重投
