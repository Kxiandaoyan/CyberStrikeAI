---
name: experience-closeout
description: >-
  收尾时把可复用方法论交给系统经验草稿，而不是手写 SKILL.md。
  Use when a project/queue/conversation is wrapping up and the user asks to
  summarize lessons; facts and vulns must already be on the blackboard.
metadata:
  tags: [经验总结, 收尾, knowledge-management]
---

# 收尾与经验

会话中继续边打边记：upsert_project_fact / record_vulnerability（见 pentest-blackboard）。

可复用的检测顺序、误报、换路，由系统在「可交付完成」后生成经验草稿，人审后才进入 skills/ 或知识库「经验总结」。

## 你不要做

- 不要用 write_file 改 skills/ 目录
- 不要编造 write_skill 工具
- 不要把目标 IP、Cookie、内网路径写进任何「通用经验」表述
- 一次性目标信息只留在黑板

## 你要做

- 收尾前列出：哪些 fact_key 值得蒸馏、哪些只是这个项目的
- 若用户问「写进 Skill 了吗」：说明需到经验草稿接口批准
- 方法论文检索仍用 search_knowledge_base（批准并索引之后）
