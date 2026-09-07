// Package githubc2sidecar — startup detection of an embedded GitHub-C2
// controller (spec §6.8 / §3 step 4).
//
// This phase does NOT spawn processes: the Flask controller keeps running in
// its original repo (`python app.py`) and CS talks to it over HTTP only
// (deliver via c2_task, verify via the built-in github_c2_list_agents tool).
// The sidecar therefore only reports presence:
//   - github_c2/ without app.py  → one "not embedded, skipping" log line
//   - github_c2/ present         → instructions to start Flask manually
//
// When process management is implemented in a later phase, PrepareAndStart is
// the single place to add it (venv + C2_ALWAYS_POLL=1 + FastMCP registration).
package githubc2sidecar

import (
	"os"
	"path/filepath"

	"cyberstrike-ai/internal/config"

	"go.uber.org/zap"
)

// PrepareAndStart reports the embedded-controller state. Never fatal.
func PrepareAndStart(cfg config.GitHubC2EmbedConfig, configDir string, logger *zap.Logger) {
	if !cfg.Enabled {
		logger.Debug("GitHub-C2 内嵌未启用（github_c2.enabled=false），跳过 sidecar")
		return
	}
	sourceDir := cfg.SourceDir
	if sourceDir == "" {
		sourceDir = "github-c2"
	}
	if !filepath.IsAbs(sourceDir) {
		sourceDir = filepath.Join(configDir, sourceDir)
	}
	if _, err := os.Stat(filepath.Join(sourceDir, "app.py")); err != nil {
		logger.Info("GitHub-C2 未内嵌（缺少 "+sourceDir+"/app.py），跳过 sidecar；控制器仍在原仓单独运行",
			zap.String("sourceDir", sourceDir))
		return
	}
	// Directory exists but this phase does not manage the process.
	logger.Warn("检测到内嵌 GitHub-C2 目录，但本阶段 CS 不代为启动 Flask；"+
		"请手动运行 `python app.py`（工作目录指向运行态目录，C2_ALWAYS_POLL=1），"+
		"并在 CS 设置页「GitHub-C2 交接」保存控制器地址与账号。CS 只投递 + 验上线。",
		zap.String("sourceDir", sourceDir))
}
