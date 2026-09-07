// Package persistencec2sidecar — startup detection of an embedded custom
// persistence-C2 controller.
//
// This phase does NOT spawn processes: the operator's persistence C2 keeps
// running wherever they deployed it and CS talks to it over HTTP only
// (deliver via c2_task, verify via the built-in persistence_c2_list_agents
// tool). The sidecar therefore only reports presence:
//   - no app.py under the source dir  → one "not embedded, skipping" log line
//   - source dir present              → instructions to start it manually
//
// When process management is implemented in a later phase, PrepareAndStart is
// the single place to add it.
package persistencec2sidecar

import (
	"os"
	"path/filepath"

	"cyberstrike-ai/internal/config"

	"go.uber.org/zap"
)

// PrepareAndStart reports the embedded-controller state. Never fatal.
func PrepareAndStart(cfg config.PersistenceC2EmbedConfig, configDir string, logger *zap.Logger) {
	if !cfg.Enabled {
		logger.Debug("自定义维权 C2 内嵌未启用（persistence_c2.enabled=false），跳过 sidecar")
		return
	}
	sourceDir := cfg.SourceDir
	if sourceDir == "" {
		sourceDir = "persistence-c2"
	}
	if !filepath.IsAbs(sourceDir) {
		sourceDir = filepath.Join(configDir, sourceDir)
	}
	if _, err := os.Stat(filepath.Join(sourceDir, "app.py")); err != nil {
		logger.Info("自定义维权 C2 未内嵌（缺少 "+sourceDir+"/app.py），跳过 sidecar；控制器仍在原处单独运行",
			zap.String("sourceDir", sourceDir))
		return
	}
	// Directory exists but this phase does not manage the process.
	logger.Warn("检测到内嵌自定义维权 C2 目录，但本阶段 CS 不代为启动；"+
		"请自行启动控制器，并在 CS 设置页「自定义维权 C2 交接」保存控制器地址与账号。CS 只投递 + 验上线。",
		zap.String("sourceDir", sourceDir))
}
