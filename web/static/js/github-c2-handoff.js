// GitHub-C2 Handoff Settings — 保存/加载交接配置（独立于主设置表单）
// Uses apiFetch (auth.js) for authenticated API calls.
(function () {
    'use strict';

    async function saveGithubC2Handoff() {
        const url = (document.getElementById('github-c2-payload-url')?.value || '').trim();
        const dropPath = (document.getElementById('github-c2-drop-path')?.value || '').trim();
        const waitSec = parseInt(document.getElementById('github-c2-wait-seconds')?.value || '180', 10);
        const listen = (document.getElementById('github-c2-listen')?.value || '').trim();
        const webUser = (document.getElementById('github-c2-web-user')?.value || '').trim();
        const webPass = (document.getElementById('github-c2-web-pass')?.value || '').trim();

        // Validate
        if (url && !url.startsWith('http://') && !url.startsWith('https://')) {
            alert('下载地址必须以 http:// 或 https:// 开头');
            return;
        }
        if (url && (url.includes(' ') || url.includes('\n'))) {
            alert('下载地址不能包含空格或换行');
            return;
        }
        if (url.length > 2048) {
            alert('下载地址过长（>2048 字符）');
            return;
        }
        if (dropPath.includes('..')) {
            alert('落盘路径不能包含 .. ');
            return;
        }
        if (waitSec < 60) {
            alert('等待秒数最小 60');
            return;
        }
        if (listen && !/^[A-Za-z0-9._\-]+:\d{1,5}$/.test(listen)) {
            alert('控制器地址必须是 host:port 形式，例如 127.0.0.1:8082');
            return;
        }

        const body = {
            payload_url: url,
            drop_path: dropPath,
            wait_seconds: waitSec,
        };
        if (listen) body.listen = listen;
        if (webUser) body.web_user = webUser;
        // 密码只写不回显：留空 = 沿用已保存值（后端收到空串会清空，因此仅在填写时提交）
        if (webPass) body.web_pass = webPass;

        try {
            const r = await apiFetch('/api/config', {
                method: 'PUT',
                headers: { 'Content-Type': 'application/json' },
                body: JSON.stringify({ github_c2: body })
            });
            if (!r.ok) {
                const d = await r.json().catch(() => ({}));
                alert('保存失败: ' + (d.error || r.status));
                return;
            }
            const r2 = await apiFetch('/api/config/apply', { method: 'POST' });
            if (!r2.ok) {
                alert('应用配置失败: ' + r2.status);
                return;
            }
            document.getElementById('github-c2-web-pass').value = '';
            alert('GitHub-C2 交接配置已保存');
        } catch (e) {
            alert('保存失败: ' + e.message);
        }
    }

    async function loadGithubC2Handoff() {
        try {
            const r = await apiFetch('/api/config');
            if (!r.ok) return;
            const d = await r.json();
            const gc2 = d.github_c2 || {};
            const urlEl = document.getElementById('github-c2-payload-url');
            const dpEl = document.getElementById('github-c2-drop-path');
            const wsEl = document.getElementById('github-c2-wait-seconds');
            const listenEl = document.getElementById('github-c2-listen');
            const userEl = document.getElementById('github-c2-web-user');
            const passEl = document.getElementById('github-c2-web-pass');
            if (urlEl) urlEl.value = gc2.payload_url || '';
            if (dpEl) dpEl.value = gc2.drop_path || '';
            if (wsEl) wsEl.value = gc2.wait_seconds || 180;
            if (listenEl) listenEl.value = gc2.listen || '';
            if (userEl) userEl.value = gc2.web_user || '';
            // 密码只写不回显：仅提示是否已设置
            if (passEl) passEl.placeholder = gc2.has_pass ? '已设置（留空 = 不修改）' : '未设置';
        } catch (e) { /* ignore */ }
    }

    // C2 会话页只读回显（说明书 §6.4）：不第二套存储，仅显示设置页已保存的配置
    async function loadGithubC2HandoffEcho() {
        const box = document.getElementById('c2-handoff-echo');
        const urlEl = document.getElementById('c2-handoff-echo-url');
        if (!box || !urlEl) return;
        try {
            const r = await apiFetch('/api/config');
            if (!r.ok) { box.style.display = 'none'; return; }
            const d = await r.json();
            const gc2 = d.github_c2 || {};
            if (gc2.payload_url) {
                let text = gc2.payload_url;
                if (gc2.drop_path) text += '  →  ' + gc2.drop_path;
                text += '（验上线等待 ' + (gc2.wait_seconds || 180) + 's）';
                urlEl.textContent = text;
                box.style.display = '';
            } else if (gc2.has_pass) {
                urlEl.textContent = '凭据已配置，但未保存下载地址 — 在线后无法交接';
                box.style.display = '';
            } else {
                box.style.display = 'none';
            }
        } catch (e) {
            box.style.display = 'none';
        }
    }

    // Load on page init and when settings tab is opened
    const origSwitch = window.switchPage || function () {};
    window.switchPage = function (page) {
        origSwitch(page);
        if (page === 'settings') {
            loadGithubC2Handoff();
        }
        if (page === 'c2-sessions') {
            loadGithubC2HandoffEcho();
        }
    };

    window.saveGithubC2Handoff = saveGithubC2Handoff;
    window.loadGithubC2Handoff = loadGithubC2Handoff;
})();
