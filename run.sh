#!/bin/bash

set -euo pipefail

# CyberStrikeAI one-click deploy and start script
ROOT_DIR="$(cd "$(dirname "$0")" && pwd)"
cd "$ROOT_DIR"

# Color definitions
RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
BLUE='\033[0;34m'
CYAN='\033[0;36m'
NC='\033[0m' # No Color

# Print colored messages
info() { echo -e "${BLUE}ℹ️  $1${NC}"; }
success() { echo -e "${GREEN}✅ $1${NC}"; }
warning() { echo -e "${YELLOW}⚠️  $1${NC}"; }
error() { echo -e "${RED}❌ $1${NC}"; }
note() { echo -e "${CYAN}ℹ️  $1${NC}"; }

# Temporary mirror/proxy settings (only effective in this script)
PIP_INDEX_URL="${PIP_INDEX_URL:-https://pypi.tuna.tsinghua.edu.cn/simple}"
GOPROXY="${GOPROXY:-https://goproxy.cn,direct}"

# Save original env vars (for restoration)
ORIGINAL_PIP_INDEX_URL="${PIP_INDEX_URL:-}"
ORIGINAL_GOPROXY="${GOPROXY:-}"

# Progress display helper
show_progress() {
    local pid=$1
    local message=$2
    local i=0
    local dots=""
    
    # Check if the process exists
    if ! kill -0 "$pid" 2>/dev/null; then
        # Process already finished; return immediately
        return 0
    fi
    
    while kill -0 "$pid" 2>/dev/null; do
        i=$((i + 1))
        case $((i % 4)) in
            0) dots="." ;;
            1) dots=".." ;;
            2) dots="..." ;;
            3) dots="...." ;;
        esac
        printf "\r${BLUE}⏳ %s%s${NC}" "$message" "$dots"
        sleep 0.5
        
        # Re-check whether the process is still running
        if ! kill -0 "$pid" 2>/dev/null; then
            break
        fi
    done
    printf "\r"
}

print_banner() {
    local show_mirrors="${1:-1}"
    echo ""
    echo "=========================================="
    echo "  CyberStrikeAI Deploy & Start Script"
    echo "  (HTTPS with self-signed cert by default; plain HTTP: $0 --http)"
    echo "=========================================="
    echo ""

    if [ "$show_mirrors" -eq 1 ]; then
        # Show temporary mirror/proxy info
        echo ""
        warning "Note: this script uses temporary mirrors to speed up downloads"
        echo ""
        info "Python pip temporary mirror:"
        echo "  ${PIP_INDEX_URL}"
        info "Go temporary proxy:"
        echo "  ${GOPROXY}"
        echo ""
        note "These settings apply only while this script runs and do not change system config"
        echo ""
        sleep 1
    fi
}

CONFIG_FILE="$ROOT_DIR/config.yaml"
EXAMPLE_CONFIG_FILE="$ROOT_DIR/config.example.yaml"
VENV_DIR="$ROOT_DIR/venv"
REQUIREMENTS_FILE="$ROOT_DIR/requirements.txt"
BINARY_NAME="cyberstrike-ai"

# Check Python environment
check_python() {
    if ! command -v python3 >/dev/null 2>&1; then
        error "python3 not found"
        echo ""
        info "Install Python 3.10 or later first:"
        echo "  macOS:   brew install python3"
        echo "  Ubuntu:  sudo apt-get install python3 python3-venv"
        echo "  CentOS:  sudo yum install python3 python3-pip"
        exit 1
    fi
    
    PYTHON_VERSION=$(python3 --version 2>&1 | awk '{print $2}')
    PYTHON_MAJOR=$(echo "$PYTHON_VERSION" | cut -d. -f1)
    PYTHON_MINOR=$(echo "$PYTHON_VERSION" | cut -d. -f2)
    
    if [ "$PYTHON_MAJOR" -lt 3 ] || ([ "$PYTHON_MAJOR" -eq 3 ] && [ "$PYTHON_MINOR" -lt 10 ]); then
        error "Python version too old: $PYTHON_VERSION (requires 3.10+)"
        exit 1
    fi
    
    success "Python check passed: $PYTHON_VERSION"
}

# Check Go environment
check_go() {
    if ! command -v go >/dev/null 2>&1; then
        error "Go not found"
        echo ""
        info "Install Go 1.21 or later first:"
        echo "  macOS:   brew install go"
        echo "  Ubuntu:  sudo apt-get install golang-go"
        echo "  CentOS:  sudo yum install golang"
        echo "  Or visit: https://go.dev/dl/"
        exit 1
    fi
    
    GO_VERSION=$(go version | awk '{print $3}' | sed 's/go//')
    GO_MAJOR=$(echo "$GO_VERSION" | cut -d. -f1)
    GO_MINOR=$(echo "$GO_VERSION" | cut -d. -f2)
    
    if [ "$GO_MAJOR" -lt 1 ] || ([ "$GO_MAJOR" -eq 1 ] && [ "$GO_MINOR" -lt 21 ]); then
        error "Go version too old: $GO_VERSION (requires 1.21+)"
        exit 1
    fi
    
    success "Go check passed: $(go version)"
}

check_go_quiet() {
    if ! command -v go >/dev/null 2>&1; then
        error "Go not found"
        echo ""
        info "Install Go 1.21 or later first:"
        echo "  macOS:   brew install go"
        echo "  Ubuntu:  sudo apt-get install golang-go"
        echo "  CentOS:  sudo yum install golang"
        echo "  Or visit: https://go.dev/dl/"
        exit 1
    fi

    GO_VERSION=$(go version | awk '{print $3}' | sed 's/go//')
    GO_MAJOR=$(echo "$GO_VERSION" | cut -d. -f1)
    GO_MINOR=$(echo "$GO_VERSION" | cut -d. -f2)

    if [ "$GO_MAJOR" -lt 1 ] || ([ "$GO_MAJOR" -eq 1 ] && [ "$GO_MINOR" -lt 21 ]); then
        error "Go version too old: $GO_VERSION (requires 1.21+)"
        exit 1
    fi
}

# Set up Python virtual environment
setup_python_env() {
    if [ ! -d "$VENV_DIR" ]; then
        info "Creating Python virtual environment..."
        python3 -m venv "$VENV_DIR"
        success "Virtual environment created"
    else
        info "Python virtual environment already exists"
    fi
    
    info "Activating virtual environment..."
    # shellcheck disable=SC1091
    source "$VENV_DIR/bin/activate"
    
    if [ -f "$REQUIREMENTS_FILE" ]; then
        echo ""
        note "━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━"
        note "Using temporary pip mirror (this script run only)"
        note "   Mirror URL: ${PIP_INDEX_URL}"
        note "   For a permanent setting, set the PIP_INDEX_URL env var"
        note "━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━"
        echo ""
        
        info "Upgrading pip..."
        pip install --index-url "$PIP_INDEX_URL" --upgrade pip >/dev/null 2>&1 || true
        
        info "Installing Python dependencies..."
        echo ""
        
        # Install deps in background; capture errors and show progress
        PIP_LOG=$(mktemp)
        (
            set +e  # disable errexit in subshell
            pip install --index-url "$PIP_INDEX_URL" -r "$REQUIREMENTS_FILE" >"$PIP_LOG" 2>&1
            echo $? > "${PIP_LOG}.exit"
        ) &
        PIP_PID=$!
        
        # Brief pause so the process can start
        sleep 0.1
        
        # Show progress while still running
        if kill -0 "$PIP_PID" 2>/dev/null; then
            show_progress "$PIP_PID" "Installing dependencies"
        else
            # Process already finished; wait for exit code file
            sleep 0.2
        fi
        
        # Wait for completion; ignore wait exit code
        wait "$PIP_PID" 2>/dev/null || true
        
        PIP_EXIT_CODE=0
        if [ -f "${PIP_LOG}.exit" ]; then
            PIP_EXIT_CODE=$(cat "${PIP_LOG}.exit" 2>/dev/null || echo "1")
            rm -f "${PIP_LOG}.exit" 2>/dev/null || true
        else
            # No exit code file; check log for errors
            if [ -f "$PIP_LOG" ] && grep -q -i "error\|failed\|exception" "$PIP_LOG" 2>/dev/null; then
                PIP_EXIT_CODE=1
            fi
        fi
        
        if [ $PIP_EXIT_CODE -eq 0 ]; then
            success "Python dependencies installed"
        else
            # Check for angr install failure (needs Rust)
            if grep -q "angr" "$PIP_LOG" && grep -q "Rust compiler\|can't find Rust" "$PIP_LOG"; then
                warning "angr install failed (Rust compiler required)"
                echo ""
                info "angr is optional and mainly used for binary analysis tools"
                info "To use angr, install Rust first:"
                echo "  macOS:   curl --proto '=https' --tlsv1.2 -sSf https://sh.rustup.rs | sh"
                echo "  Ubuntu:  curl --proto '=https' --tlsv1.2 -sSf https://sh.rustup.rs | sh"
                echo "  Or visit: https://rustup.rs/"
                echo ""
                info "Other dependencies are installed; you can continue (some tools may be unavailable)"
            else
                warning "Some Python dependencies failed to install, but continuing"
                warning "If you hit issues, check the errors and install missing packages manually"
                # Show last lines of error output
                echo ""
                info "Error details (last 10 lines):"
                tail -n 10 "$PIP_LOG" | sed 's/^/  /'
                echo ""
            fi
        fi
        rm -f "$PIP_LOG"
    else
        warning "requirements.txt not found; skipping Python dependency install"
    fi
}

# Build Go project
build_go_project() {
    echo ""
    note "━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━"
    note "Using temporary Go proxy (this script run only)"
    note "   Proxy URL: ${GOPROXY}"
    note "   For a permanent setting, set the GOPROXY env var"
    note "━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━"
    echo ""
    
    info "Downloading Go dependencies..."
    GO_DOWNLOAD_LOG=$(mktemp)
    (
        set +e  # disable errexit in subshell
        export GOPROXY="$GOPROXY"
        go mod download >"$GO_DOWNLOAD_LOG" 2>&1
        echo $? > "${GO_DOWNLOAD_LOG}.exit"
    ) &
    GO_DOWNLOAD_PID=$!
    
    # Brief pause so the process can start
    sleep 0.1
    
    # Show progress while still running
    if kill -0 "$GO_DOWNLOAD_PID" 2>/dev/null; then
        show_progress "$GO_DOWNLOAD_PID" "Downloading Go dependencies"
    else
        # Process already finished; wait for exit code file
        sleep 0.2
    fi
    
    # Wait for completion; ignore wait exit code
    wait "$GO_DOWNLOAD_PID" 2>/dev/null || true
    
    GO_DOWNLOAD_EXIT_CODE=0
    if [ -f "${GO_DOWNLOAD_LOG}.exit" ]; then
        GO_DOWNLOAD_EXIT_CODE=$(cat "${GO_DOWNLOAD_LOG}.exit" 2>/dev/null || echo "1")
        rm -f "${GO_DOWNLOAD_LOG}.exit" 2>/dev/null || true
    else
        # No exit code file; check log for errors
        if [ -f "$GO_DOWNLOAD_LOG" ] && grep -q -i "error\|failed" "$GO_DOWNLOAD_LOG" 2>/dev/null; then
            GO_DOWNLOAD_EXIT_CODE=1
        fi
    fi
    rm -f "$GO_DOWNLOAD_LOG" 2>/dev/null || true
    
    if [ $GO_DOWNLOAD_EXIT_CODE -ne 0 ]; then
        error "Go dependency download failed"
        exit 1
    fi
    success "Go dependencies downloaded"
    
    info "Building project..."
    GO_BUILD_LOG=$(mktemp)
    (
        set +e  # disable errexit in subshell
        export GOPROXY="$GOPROXY"
        go build -o "$BINARY_NAME" cmd/server/main.go >"$GO_BUILD_LOG" 2>&1
        echo $? > "${GO_BUILD_LOG}.exit"
    ) &
    GO_BUILD_PID=$!
    
    # Brief pause so the process can start
    sleep 0.1
    
    # Show progress while still running
    if kill -0 "$GO_BUILD_PID" 2>/dev/null; then
        show_progress "$GO_BUILD_PID" "Building project"
    else
        # Process already finished; wait for exit code file
        sleep 0.2
    fi
    
    # Wait for completion; ignore wait exit code
    wait "$GO_BUILD_PID" 2>/dev/null || true
    
    GO_BUILD_EXIT_CODE=0
    if [ -f "${GO_BUILD_LOG}.exit" ]; then
        GO_BUILD_EXIT_CODE=$(cat "${GO_BUILD_LOG}.exit" 2>/dev/null || echo "1")
        rm -f "${GO_BUILD_LOG}.exit" 2>/dev/null || true
    else
        # No exit code file; check log for errors
        if [ -f "$GO_BUILD_LOG" ] && grep -q -i "error\|failed" "$GO_BUILD_LOG" 2>/dev/null; then
            GO_BUILD_EXIT_CODE=1
        fi
    fi
    
    if [ $GO_BUILD_EXIT_CODE -eq 0 ]; then
        success "Build complete: $BINARY_NAME"
        rm -f "$GO_BUILD_LOG"
    else
        error "Build failed"
        # Show build errors
        echo ""
        info "Build error details:"
        cat "$GO_BUILD_LOG" | sed 's/^/  /'
        echo ""
        rm -f "$GO_BUILD_LOG"
        exit 1
    fi
}

build_go_project_quiet() {
    info "Building $BINARY_NAME..."

    GO_DOWNLOAD_LOG=$(mktemp)
    if ! GOPROXY="$GOPROXY" go mod download >"$GO_DOWNLOAD_LOG" 2>&1; then
        error "Go dependency download failed"
        echo ""
        info "Download error details:"
        cat "$GO_DOWNLOAD_LOG" | sed 's/^/  /'
        echo ""
        rm -f "$GO_DOWNLOAD_LOG"
        exit 1
    fi
    rm -f "$GO_DOWNLOAD_LOG"

    GO_BUILD_LOG=$(mktemp)
    if ! GOPROXY="$GOPROXY" go build -o "$BINARY_NAME" cmd/server/main.go >"$GO_BUILD_LOG" 2>&1; then
        error "Build failed"
        echo ""
        info "Build error details:"
        cat "$GO_BUILD_LOG" | sed 's/^/  /'
        echo ""
        rm -f "$GO_BUILD_LOG"
        exit 1
    fi
    rm -f "$GO_BUILD_LOG"
}

# Check whether a rebuild is needed
need_rebuild() {
    if [ ! -f "$BINARY_NAME" ]; then
        return 0  # needs build
    fi
    
    # Check if source changed since last build
    if [ "$BINARY_NAME" -ot cmd/server/main.go ] || \
       [ "$BINARY_NAME" -ot go.mod ] || \
       find internal cmd -name "*.go" -newer "$BINARY_NAME" 2>/dev/null | grep -q .; then
        return 0  # needs rebuild
    fi
    
    return 1  # no rebuild needed
}

# ── zvec-grep setup ──────────────────────────────────────────────────────────
setup_zvec_grep() {
    local ZG_DIR="$ROOT_DIR/zvec-grep"
    local ZG_CLI="$ZG_DIR/dist/cli/index.js"
    if [ ! -d "$ZG_DIR" ]; then
        info "zvec-grep: source not found, skipping"
        return 0
    fi
    if [ -f "$ZG_CLI" ]; then
        success "zvec-grep: CLI already built"
        return 0
    fi
    info "zvec-grep: building (npm ci + npm run build)..."
    if ! command -v node >/dev/null 2>&1; then
        warning "zvec-grep: Node.js not found, skipping"
        return 0
    fi
    local NODE_VER
    NODE_VER=$(node --version 2>/dev/null | sed 's/v//' | cut -d. -f1)
    if [ "${NODE_VER:-0}" -lt 22 ]; then
        warning "zvec-grep: Node >= 22 required (found v$NODE_VER), skipping"
        return 0
    fi
    ( cd "$ZG_DIR" && { [ -d node_modules ] || npm ci --silent; } && npm run build --silent )
    if [ -f "$ZG_CLI" ]; then
        success "zvec-grep: build OK"
    else
        warning "zvec-grep: build failed, CS will start without local CVE search"
    fi
    mkdir -p "$ROOT_DIR/data/corpus/cve" "$ROOT_DIR/data/corpus/playbooks" \
             "$ROOT_DIR/data/corpus/poc" "$ROOT_DIR/data/corpus/raw" "$ROOT_DIR/data/zvec-home"
}

# ── CVE corpus setup ────────────────────────────────────────────────────────
setup_cve_corpus() {
    local CORPUS_DIR="$ROOT_DIR/data/corpus"
    local TAR_FILE="$ROOT_DIR/assets/cve-corpus.tar.gz"

    # Already has data?
    if [ -d "$CORPUS_DIR/cve" ] && [ "$(find "$CORPUS_DIR/cve" -name '*.md' 2>/dev/null | head -1)" ]; then
        success "CVE corpus: already present ($(find "$CORPUS_DIR/cve" -name '*.md' 2>/dev/null | wc -l) files)"
        return 0
    fi

    # Extract from pre-built asset (36MB, ~5 seconds)
    if [ -f "$TAR_FILE" ]; then
        info "CVE corpus: extracting pre-built data (36MB)..."
        mkdir -p "$CORPUS_DIR"
        tar xzf "$TAR_FILE" -C "$CORPUS_DIR"
        local TOTAL=$(find "$CORPUS_DIR/cve" -name '*.md' 2>/dev/null | wc -l)
        success "CVE corpus: $TOTAL CVE records ready"
        return 0
    fi

    warning "CVE corpus: assets/cve-corpus.tar.gz not found, skipping"
    info "CVE corpus: run 'python tools/convert_cve.py <cvelistV5-path> data' to rebuild"
}

# ── POC stock (public PoC index + wiki POC articles, one-time download) ─────
# 存量不从仓库分发（SourByte Wiki 无 license 不可再分发；PocOrExp 为上游衍生数据），
# 部署时直接从源仓库下载快照导入。幂等：已有数据则跳过。SKIP_POC_STOCK=1 可整体跳过。
setup_poc_stock() {
    if [ "${SKIP_POC_STOCK:-0}" = "1" ]; then
        info "POC stock: SKIP_POC_STOCK=1, skipping"
        return 0
    fi
    local TMPD
    # ① pocindex 层：CVE → 公开 PoC 仓库索引（MIT，~5MB 快照）
    if [ -z "$(ls "$ROOT_DIR/data/corpus/pocindex" 2>/dev/null | head -1)" ]; then
        info "POC stock: fetching PocOrExp index snapshot (~5MB)…"
        TMPD=$(mktemp -d)
        if curl -sL --max-time 300 -o "$TMPD/src.zip" \
             "https://codeload.github.com/ycdxsb/PocOrExp_in_Github/zip/refs/heads/main" \
           && { unzip -q "$TMPD/src.zip" -d "$TMPD" 2>/dev/null || python3 -m zipfile -e "$TMPD/src.zip" "$TMPD"; }; then
            if python3 tools/import_pocindex.py "$TMPD/PocOrExp_in_Github-main" \
                 "$ROOT_DIR/data/corpus/pocindex" \
                 --source-name "github.com/ycdxsb/PocOrExp_in_Github" >/dev/null; then
                success "POC stock: pocindex ready ($(ls "$ROOT_DIR/data/corpus/pocindex" | wc -l) CVEs)"
            else
                warning "POC stock: pocindex import failed — 可稍后手动运行 tools/import_pocindex.py"
            fi
        else
            warning "POC stock: pocindex download failed — CS 照常启动，可稍后手动导入"
        fi
        rm -rf "$TMPD"
    else
        success "POC stock: pocindex already present"
    fi
    # ② 实战层存量：Wiki POC 复现文章（快照 ~305MB 一次性；源仓无 license，仅本机使用）
    if [ -z "$(find "$ROOT_DIR/data/corpus/poc" -name '*.md' 2>/dev/null | head -1)" ]; then
        info "POC stock: fetching Vulnerability-Wiki-PoC articles (~305MB one-time)…"
        TMPD=$(mktemp -d)
        if curl -sL --max-time 900 -o "$TMPD/src.zip" \
             "https://codeload.github.com/SourByte05/Vulnerability-Wiki-PoC/zip/refs/heads/main" \
           && { unzip -q "$TMPD/src.zip" -d "$TMPD" 2>/dev/null || python3 -m zipfile -e "$TMPD/src.zip" "$TMPD"; }; then
            if python3 tools/import_poc_repo.py "$TMPD/Vulnerability-Wiki-PoC-main" \
                 "$ROOT_DIR/data/corpus/poc" \
                 --source-name "github.com/SourByte05/Vulnerability-Wiki-PoC" >/dev/null; then
                success "POC stock: wiki POCs ready ($(ls "$ROOT_DIR/data/corpus/poc" | wc -l) articles)"
            else
                warning "POC stock: wiki import failed — 可稍后手动运行 tools/import_poc_repo.py"
            fi
        else
            warning "POC stock: wiki download failed — CS 照常启动，可稍后手动导入"
        fi
        rm -rf "$TMPD"
    else
        success "POC stock: combat POC layer already present"
    fi
}

# ── zvec-grep playbooks seed (3 starter playbooks per spec §9) ──────────────
setup_zvec_playbooks() {
    local PB_DIR="$ROOT_DIR/data/corpus/playbooks"
    mkdir -p "$PB_DIR"
    [ "$(find "$PB_DIR" -name '*.md' 2>/dev/null | head -1)" ] && return 0
    cat > "$PB_DIR/local-cve-lookup.md" <<'EOF'
# 本地 CVE 语料检索

1. 先 skill local-corpus-cve，再 zvec_grep_search（root=语料仓绝对路径）。
2. 已知编号：fts=["CVE-YYYY-NNNN"]；产品+现象：query="Jenkins CLI file read"。
3. 命中 → upsert_project_fact key=intel/<cve-id>，confidence=tentative。
4. 本地 0 命中 → component-vuln-intel 外网序列。
EOF
    cat > "$PB_DIR/persistence-c2-handoff.md" <<'EOF'
# 自定义维权 C2 交接（Beacon 立足后）

1. c2_session get：status ∈ {active, sleeping} 且 hostname 非 unknown。
2. persistence_c2_handoff_source：payload_url 为空 → 停，请人去设置页保存。
3. persistence_c2_list_agents 拍 baseline（内部已 refresh）。
4. c2_task 按已保存 URL 投递并拉起（HITL 看一眼）。
5. wait_seconds 内每 15s list：hostname 全等 + (channel,id)/uuid 新出现。
6. 命中 → persist/handoff-<session_id> confirmed，停止 persistence_c2_*。
EOF
    cat > "$PB_DIR/experience-closeout.md" <<'EOF'
# 项目收尾与经验沉淀

1. 收尾前核对黑板：值得蒸馏的 fact_key 与一次性项目信息分开。
2. 可交付完成后系统自动生成经验草稿（LLM + 脱敏）。
3. 人在 /api/experience/drafts 批准后才写入 Skill 或知识库。
4. 会话中禁止 write_file 改 skills/ 目录。
EOF
    success "zvec-grep: seeded 3 starter playbooks"
}

# ── zvec-grep server (agent toolset) + first index ──────────────────────────
ZVEC_LISTEN="127.0.0.1:7999"

zvec_config_enabled() {
    # Reads the enabled: value inside the top-level zvec_grep: section.
    local f="$1"
    [ -f "$f" ] || return 1
    awk '/^zvec_grep:/{s=1;next} s && /^[^ #]/{s=0} s && $1=="enabled:"{print $2; exit}' "$f"
}

zvec_port_open() {
    local host="${ZVEC_LISTEN%%:*}" port="${ZVEC_LISTEN##*:}"
    (exec 3<>"/dev/tcp/$host/$port") 2>/dev/null && { exec 3>&- 3<&- 2>/dev/null; return 0; } || return 1
}

start_zvec_server() {
    local ZG_DIR="$ROOT_DIR/zvec-grep"
    local ZG_CLI="$ZG_DIR/dist/cli/index.js"
    [ -f "$ZG_CLI" ] || { info "zvec-grep: CLI not built, skipping server"; return 0; }

    local enabled
    enabled="$(zvec_config_enabled "$CONFIG_FILE" || true)"
    if [ "$enabled" != "true" ]; then
        info "zvec-grep: enabled != true in config.yaml, skipping server (set zvec_grep.enabled: true to enable)"
        return 0
    fi

    if zvec_port_open; then
        success "zvec-grep: server already listening on $ZVEC_LISTEN"
        return 0
    fi

    info "zvec-grep: starting server (agent toolset) on $ZVEC_LISTEN ..."
    mkdir -p "$ROOT_DIR/data/zvec-home" "$ROOT_DIR/logs"
    (
        cd "$ZG_DIR" || exit 1
        ZVEC_GREP_HOME="$ROOT_DIR/data/zvec-home" ZVEC_GREP_DEVICE=cpu \
            nohup node "$ZG_CLI" server run --listen "$ZVEC_LISTEN" --mcp-toolset agent \
            >> "$ROOT_DIR/logs/zvec-server.log" 2>&1 &
    )
    local i
    for i in $(seq 1 15); do
        zvec_port_open && break
        sleep 1
    done
    if zvec_port_open; then
        success "zvec-grep: server ready (log: logs/zvec-server.log)"
    else
        warning "zvec-grep: server not responding yet — CS 照常启动，外部 MCP 连接会自动重试"
    fi
}

setup_zvec_first_index() {
    local ZG_DIR="$ROOT_DIR/zvec-grep"
    local ZG_CLI="$ZG_DIR/dist/cli/index.js"
    local CORPUS_DIR="$ROOT_DIR/data/corpus"
    [ -f "$ZG_CLI" ] || return 0
    if [ ! -d "$CORPUS_DIR/cve" ] || [ -z "$(find "$CORPUS_DIR/cve" -name '*.md' 2>/dev/null | head -1)" ]; then
        info "zvec-grep: no CVE corpus yet, skipping first index"
        return 0
    fi
    if [ -d "$CORPUS_DIR/.zvec-grep" ]; then
        success "zvec-grep: workspace index already present (增量由检索时 autoUpdate 维护)"
        note "若索引建立早于 POC 实战层/pocindex 索引层，需重建一次才能语义检索：cd $CORPUS_DIR && node $ZG_CLI index . --rebuild -g 'cve/**' -g 'playbooks/**' -g 'poc/**' -g 'pocindex/**' --embedding local/potion-code-16m-v2 --mode direct"
        return 0
    fi
    mkdir -p "$ROOT_DIR/logs"
    info "zvec-grep: building first index in background (190K+ files, CPU local embedding — logs/zvec-index.log)"
    (
        cd "$CORPUS_DIR" || exit 1
        ZVEC_GREP_HOME="$ROOT_DIR/data/zvec-home" ZVEC_GREP_DEVICE=cpu \
            nohup node "$ZG_CLI" index . -g 'cve/**' -g 'playbooks/**' -g 'poc/**' -g 'pocindex/**' \
            --embedding local/potion-code-16m-v2 --mode direct \
            >> "$ROOT_DIR/logs/zvec-index.log" 2>&1 &
    )
    note "首次索引完成前 zvec_grep_search 可能只返回部分结果；日常增量由 cvesync + 检索时 autoUpdate 维护"
}

# Main flow
# Default: HTTPS (--https passed to binary); --http forces plain HTTP even if config.yaml enables TLS.
main() {
    USE_HTTPS=1
    RESET_ADMIN_PASSWORD=0
    FORWARD_ARGS=()
    for arg in "$@"; do
        if [ "$arg" = "--http" ]; then
            USE_HTTPS=0
            continue
        fi
        if [ "$arg" = "--https" ]; then
            USE_HTTPS=1
            continue
        fi
        if [ "$arg" = "--reset-admin-password" ]; then
            RESET_ADMIN_PASSWORD=1
            continue
        fi
        FORWARD_ARGS+=("$arg")
    done

    if [ "$RESET_ADMIN_PASSWORD" -eq 1 ]; then
        if [ ! -f "$CONFIG_FILE" ] && [ ! -f "$EXAMPLE_CONFIG_FILE" ]; then
            error "config.yaml not found, and config.example.yaml is missing"
            info "The server binary creates config.yaml from config.example.yaml on first start"
            exit 1
        fi
        check_go_quiet
        if need_rebuild; then
            build_go_project_quiet
            echo ""
        fi
        if [ "${#FORWARD_ARGS[@]}" -gt 0 ]; then
            exec "./$BINARY_NAME" -config "$CONFIG_FILE" --reset-admin-password "${FORWARD_ARGS[@]}"
        else
            exec "./$BINARY_NAME" -config "$CONFIG_FILE" --reset-admin-password
        fi
    fi

    print_banner 1

    # Environment checks
    info "Checking runtime environment..."
    check_python
    if [ ! -f "$CONFIG_FILE" ] && [ ! -f "$EXAMPLE_CONFIG_FILE" ]; then
        error "config.yaml not found, and config.example.yaml is missing"
        info "The server binary creates config.yaml from config.example.yaml on first start"
        exit 1
    fi
    check_go
    echo ""
    
    # Python setup
    info "Setting up Python environment..."
    setup_python_env
    echo ""
    
    # Go build
    if need_rebuild; then
        info "Preparing to build project..."
        build_go_project
    else
        success "Binary is up to date; skipping build"
    fi
    echo ""

    # zvec-grep build (if enabled in config and dist/ missing)
    setup_zvec_grep
    echo ""

    # CVE corpus initial data (pre-built 36MB asset; cvesync handles daily deltas)
    setup_cve_corpus
    echo ""

    # POC stock: public PoC index (~5MB) + wiki POC articles (~305MB one-time),
    # downloaded straight from the source repos (idempotent; SKIP_POC_STOCK=1 to opt out)
    setup_poc_stock
    echo ""

    # zvec-grep runtime: seed playbooks, start server (agent toolset), first index
    setup_zvec_playbooks
    start_zvec_server
    setup_zvec_first_index
    echo ""
    
    # Start server
    success "All setup complete!"
    echo ""
    if [ "$USE_HTTPS" -eq 1 ]; then
        info "Starting CyberStrikeAI server (HTTPS + HTTP/2, self-signed cert)..."
        note "For plain HTTP, use: $0 --http"
    else
        info "Starting CyberStrikeAI server (HTTP)..."
    fi
    echo "=========================================="
    echo ""

    # Always pass config.yaml from project root so cwd does not matter; extra args still apply (e.g. -config override; last Go flag wins).
    if [ "$USE_HTTPS" -eq 1 ]; then
        if [ "${#FORWARD_ARGS[@]}" -gt 0 ]; then
            exec "./$BINARY_NAME" -config "$CONFIG_FILE" --https "${FORWARD_ARGS[@]}"
        else
            exec "./$BINARY_NAME" -config "$CONFIG_FILE" --https
        fi
    else
        if [ "${#FORWARD_ARGS[@]}" -gt 0 ]; then
            exec "./$BINARY_NAME" -config "$CONFIG_FILE" --http "${FORWARD_ARGS[@]}"
        else
            exec "./$BINARY_NAME" -config "$CONFIG_FILE" --http
        fi
    fi
}

# Run main (supports args, e.g. ./run.sh --http, ./run.sh --reset-admin-password)
main "$@"
