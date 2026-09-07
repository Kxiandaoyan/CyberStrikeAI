// Experience Drafts — 经验草稿审批页面
(function () {
    'use strict';

    let currentDraft = null;

    // Hook into switchPage
    const origSwitch = window.switchPage || function () {};
    window.switchPage = function (page) {
        origSwitch(page);
        if (page === 'experience-drafts') {
            loadExperienceDrafts();
        }
    };

    async function loadExperienceDrafts() {
        const list = document.getElementById('experience-drafts-list');
        const status = document.getElementById('experience-status-filter')?.value || 'draft';
        if (list) list.innerHTML = '<div style="padding:24px;text-align:center;color:var(--text-secondary)">加载中…</div>';
        try {
            const r = await apiFetch('/api/experience/drafts?status=' + encodeURIComponent(status));
            if (!r.ok) {
                if (list) list.innerHTML = '<div style="padding:24px;text-align:center;color:var(--text-secondary)">API ' + r.status + ' — 经验功能未启用或无数据</div>';
                return;
            }
            const d = await r.json();
            renderDrafts(d.drafts || []);
            loadStats();
        } catch (e) {
            if (list) list.innerHTML = '<div style="padding:24px;text-align:center;color:var(--text-secondary)">加载失败: ' + esc(e.message) + '</div>';
        }
    }

    async function loadStats() {
        try {
            const r = await apiFetch('/api/experience/stats');
            if (!r.ok) return;
            const d = await r.json();
            const c = d.counts || {};
            const el = document.getElementById('experience-stats');
            if (el) {
                el.textContent = `草稿 ${c.draft || 0} · 已批准 ${c.approved || 0} · 已拒绝 ${c.rejected || 0}`;
            }
        } catch (e) { /* ignore */ }
    }

    function esc(s) {
        const d = document.createElement('div');
        d.textContent = s || '';
        return d.innerHTML;
    }

    function renderDrafts(drafts) {
        const list = document.getElementById('experience-drafts-list');
        if (!list) return;
        if (!drafts.length) {
            list.innerHTML = '<div style="padding:24px;text-align:center;color:var(--text-secondary)">无草稿</div>';
            return;
        }
        list.innerHTML = drafts.map(d => `
            <div style="border:1px solid var(--border);border-radius:8px;padding:14px 16px;cursor:pointer"
                 onclick="showExperienceDetail(${d.id})">
                <div style="display:flex;justify-content:space-between;align-items:center">
                    <span style="font-weight:600;font-size:15px">${esc(d.title)}</span>
                    <span style="font-size:11px;padding:2px 8px;border-radius:10px;font-weight:600
                        ${d.status === 'draft' ? 'background:rgba(255,193,7,.15);color:#ffc107'
                        : d.status === 'approved' ? 'background:rgba(40,167,69,.15);color:#28a745'
                        : 'background:rgba(220,53,69,.15);color:#dc3535'}">
                        ${d.status === 'draft' ? '待审核' : d.status === 'approved' ? '已批准' : '已拒绝'}
                    </span>
                </div>
                <div style="font-size:12px;color:var(--text-secondary);margin-top:6px">
                    ${esc(d.category)} · ${new Date(d.created_at).toLocaleString()} · 来源: ${esc(d.source)}
                </div>
                <div style="font-size:13px;margin-top:8px;overflow:hidden;text-overflow:ellipsis;white-space:nowrap;max-height:20px">
                    ${esc((d.content || '').substring(0, 120))}…
                </div>
            </div>
        `).join('');
    }

    async function showExperienceDetail(id) {
        try {
            const r = await apiFetch('/api/experience/drafts/' + id);
            if (!r.ok) return;
            const d = await r.json();
            currentDraft = d;

            document.getElementById('experience-detail-title').textContent = d.title;
            document.getElementById('experience-detail-meta').textContent =
                `分类: ${d.category} · 状态: ${d.status} · 创建: ${new Date(d.created_at).toLocaleString()}` +
                (d.reviewer ? ` · 审核人: ${d.reviewer}` : '');
            document.getElementById('experience-detail-content').textContent = d.content;

            // Show approve/reject buttons only for drafts
            const actions = document.getElementById('experience-detail-actions');
            if (d.status === 'draft') {
                actions.innerHTML = `
                    <button class="btn-primary" onclick="approveExperience(${d.id})">✓ 批准</button>
                    <button class="btn-secondary" style="color:#dc3535" onclick="rejectExperience(${d.id})">✗ 拒绝</button>
                `;
            } else {
                actions.innerHTML = '';
            }

            document.getElementById('experience-detail-modal').style.display = 'flex';
        } catch (e) {
            alert('获取详情失败: ' + e.message);
        }
    }

    function closeExperienceDetail() {
        document.getElementById('experience-detail-modal').style.display = 'none';
    }

    async function approveExperience(id) {
        // POC 草稿默认预填 poc:CVE-ID（写入本地实战库 data/corpus/poc/）；
        // 方法论草稿默认留空 = 知识库「经验总结」，或填 skill:技能名。
        let def = '';
        let hint = '应用到哪（留空 = 知识库「经验总结」；或填 skill:技能名 追加进对应 SKILL.md，如 skill:web-attack-methods）:';
        if (currentDraft && currentDraft.category === 'poc') {
            try {
                const fk = JSON.parse(currentDraft.fact_keys || '[]');
                if (fk.length > 0 && fk[0]) def = 'poc:' + fk[0];
            } catch (e) { /* ignore */ }
            hint = 'POC 沉淀 — 写入本地实战库 data/corpus/poc/（同步不覆盖、不入公开仓库）。回车确认，或改成 skill:技能名 / 留空入知识库:';
        }
        const appliedTo = prompt(hint, def) || '';
        try {
            const r = await apiFetch(`/api/experience/drafts/${id}/approve`, {
                method: 'POST',
                headers: { 'Content-Type': 'application/json' },
                body: JSON.stringify({ applied_to: appliedTo })
            });
            if (!r.ok) {
                const e2 = await r.json().catch(() => ({}));
                alert('批准失败: ' + (e2.error || r.status));
                return;
            }
            const d = await r.json().catch(() => ({}));
            if (d.applied_to) alert('已批准并写入: ' + d.applied_to);
            closeExperienceDetail();
            loadExperienceDrafts();
        } catch (e) {
            alert('批准失败: ' + e.message);
        }
    }

    async function rejectExperience(id) {
        if (!confirm('确认拒绝此草稿？')) return;
        try {
            const r = await apiFetch(`/api/experience/drafts/${id}/reject`, { method: 'POST' });
            if (!r.ok) { alert('拒绝失败: ' + r.status); return; }
            closeExperienceDetail();
            loadExperienceDrafts();
        } catch (e) {
            alert('拒绝失败: ' + e.message);
        }
    }

    // Close on backdrop click
    document.getElementById('experience-detail-modal')?.addEventListener('click', function (e) {
        if (e.target === this) closeExperienceDetail();
    });

    window.loadExperienceDrafts = loadExperienceDrafts;
    window.showExperienceDetail = showExperienceDetail;
    window.closeExperienceDetail = closeExperienceDetail;
    window.approveExperience = approveExperience;
    window.rejectExperience = rejectExperience;
})();
