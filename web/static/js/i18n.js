// 前端国际化：语言包 JSON 始终加载。i18next 仅作可选增强，缺失时用内置查找，避免露出 projects.statusActive 一类 key。
(function () {
    const DEFAULT_LANG = 'zh-CN';
    const STORAGE_KEY = 'csai_lang';
    const RESOURCES_PREFIX = '/static/i18n';

    const loadedLangs = {};
    const bundles = {};
    let currentLang = DEFAULT_LANG;

    let i18nReadyResolve;
    window.i18nReady = new Promise(function (resolve) {
        i18nReadyResolve = resolve;
    });

    function detectInitialLang() {
        try {
            const stored = localStorage.getItem(STORAGE_KEY);
            if (stored) {
                return stored;
            }
        } catch (e) {
            console.warn('无法读取语言设置:', e);
        }

        const navLang = (navigator.language || navigator.userLanguage || '').toLowerCase();
        if (navLang.startsWith('zh')) {
            return 'zh-CN';
        }
        if (navLang.startsWith('en')) {
            return 'en-US';
        }
        return DEFAULT_LANG;
    }

    function lookup(obj, key) {
        if (!obj || !key) return undefined;
        const parts = String(key).split('.');
        let cur = obj;
        for (let i = 0; i < parts.length; i++) {
            if (cur == null || typeof cur !== 'object' || !(parts[i] in cur)) {
                return undefined;
            }
            cur = cur[parts[i]];
        }
        return typeof cur === 'string' ? cur : undefined;
    }

    function interpolate(str, opts) {
        if (!opts || typeof str !== 'string') return str;
        return str.replace(/\{\{\s*(\w+)\s*\}\}/g, function (_, name) {
            if (opts[name] == null) return '';
            return String(opts[name]);
        });
    }

    function translateFromBundles(key, opts) {
        const primary = lookup(bundles[currentLang], key);
        if (primary) return interpolate(primary, opts);
        if (currentLang !== DEFAULT_LANG) {
            const fallback = lookup(bundles[DEFAULT_LANG], key);
            if (fallback) return interpolate(fallback, opts);
        }
        return '';
    }

    function translate(key, opts) {
        if (!key) return '';
        if (typeof i18next !== 'undefined' && typeof i18next.t === 'function') {
            const fromEngine = i18next.t(key, opts);
            if (fromEngine && fromEngine !== key) {
                return fromEngine;
            }
        }
        const fromBundle = translateFromBundles(key, opts);
        return fromBundle || key;
    }

    async function loadLanguageResources(lang) {
        if (loadedLangs[lang]) {
            return;
        }
        try {
            const resp = await fetch(RESOURCES_PREFIX + '/' + lang + '.json', {
                cache: 'no-cache'
            });
            if (!resp.ok) {
                console.warn('加载语言包失败:', lang, resp.status);
                return;
            }
            const data = await resp.json();
            bundles[lang] = data;
            if (typeof i18next !== 'undefined') {
                i18next.addResourceBundle(lang, 'translation', data, true, true);
            }
            loadedLangs[lang] = true;
        } catch (e) {
            console.error('加载语言包异常:', lang, e);
        }
    }

    function applyTranslations(root) {
        const container = root || document;
        if (!container) return;

        const elements = container.querySelectorAll('[data-i18n]');
        elements.forEach(function (el) {
            const key = el.getAttribute('data-i18n');
            if (!key) return;
            const skipText = el.getAttribute('data-i18n-skip-text') === 'true';
            const isFormControl = (el.tagName === 'INPUT' || el.tagName === 'TEXTAREA');
            const attrList = el.getAttribute('data-i18n-attr');
            const translated = translate(key);
            const text = translated && translated !== key ? translated : '';
            const hasNoElementChildren = !el.querySelector('*');
            if (!skipText && !isFormControl && hasNoElementChildren && text && typeof text === 'string') {
                el.textContent = text;
            }

            if (attrList) {
                const titleKey = el.getAttribute('data-i18n-title');
                attrList.split(',').map(function (s) { return s.trim(); }).forEach(function (attr) {
                    if (!attr) return;
                    var val = text;
                    if (attr === 'title' && titleKey) {
                        var titleText = translate(titleKey);
                        if (titleText && titleText !== titleKey && typeof titleText === 'string') val = titleText;
                    }
                    if (val && typeof val === 'string') {
                        el.setAttribute(attr, val);
                    }
                });
            }
        });

        try {
            const chatInput = document.getElementById('chat-input');
            if (chatInput && chatInput.tagName === 'TEXTAREA') {
                const ph = (chatInput.getAttribute('placeholder') || '').trim();
                if (ph && chatInput.value.trim() === ph) {
                    chatInput.value = '';
                }
            }
        } catch (e) { /* ignore */ }

        try {
            if (document && document.documentElement) {
                document.documentElement.lang = currentLang || DEFAULT_LANG;
            }
        } catch (e) {
            // ignore
        }
    }

    function updateLangLabel() {
        const label = document.getElementById('current-lang-label');
        if (!label) return;
        const lang = (currentLang || DEFAULT_LANG).toLowerCase();
        if (lang.indexOf('zh') === 0) {
            label.textContent = translate('lang.zhCN');
        } else {
            label.textContent = translate('lang.enUS');
        }
    }

    function closeLangDropdown() {
        const dropdown = document.getElementById('lang-dropdown');
        if (dropdown) {
            dropdown.style.display = 'none';
        }
    }

    function handleGlobalClickForLangDropdown(ev) {
        const dropdown = document.getElementById('lang-dropdown');
        const btn = document.querySelector('.lang-switcher-btn');
        if (!dropdown || dropdown.style.display !== 'block') return;
        const target = ev.target;
        if (btn && btn.contains(target)) {
            return;
        }
        if (!dropdown.contains(target)) {
            closeLangDropdown();
        }
    }

    async function changeLanguage(lang) {
        if (lang === currentLang && loadedLangs[lang]) return;
        await loadLanguageResources(lang);
        if (lang !== DEFAULT_LANG) {
            await loadLanguageResources(DEFAULT_LANG);
        }
        currentLang = lang;
        if (typeof i18next !== 'undefined' && typeof i18next.t === 'function') {
            await i18next.changeLanguage(lang);
        }
        try {
            localStorage.setItem(STORAGE_KEY, lang);
        } catch (e) {
            console.warn('无法保存语言设置:', e);
        }
        applyTranslations(document);
        updateLangLabel();
        if (typeof window.refreshThemeToggleLabel === 'function') {
            window.refreshThemeToggleLabel();
        }
        try {
            window.__locale = lang;
        } catch (e) { /* ignore */ }
        try {
            document.dispatchEvent(new CustomEvent('languagechange', { detail: { lang: lang } }));
        } catch (e) { /* ignore */ }
    }

    function exportGlobals() {
        window.t = translate;
        window.changeLanguage = changeLanguage;
        window.applyTranslations = applyTranslations;
        window.toggleLangDropdown = function () {
            const dropdown = document.getElementById('lang-dropdown');
            if (!dropdown) return;
            if (dropdown.style.display === 'block') {
                dropdown.style.display = 'none';
            } else {
                dropdown.style.display = 'block';
            }
        };
        window.onLanguageSelect = function (lang) {
            changeLanguage(lang);
            closeLangDropdown();
        };
    }

    async function initI18n() {
        exportGlobals();
        const initialLang = detectInitialLang();
        currentLang = initialLang;

        if (typeof i18next !== 'undefined') {
            await i18next.init({
                lng: initialLang,
                fallbackLng: DEFAULT_LANG,
                debug: false,
                resources: {},
                // 文案用 {{count}} 插值，不要按复数后缀去找 statsFacts_other
                compatibilityJSON: 'v3'
            });
        }

        await loadLanguageResources(initialLang);
        if (initialLang !== DEFAULT_LANG) {
            await loadLanguageResources(DEFAULT_LANG);
        }
        applyTranslations(document);
        updateLangLabel();
        if (typeof window.refreshThemeToggleLabel === 'function') {
            window.refreshThemeToggleLabel();
        }
        try {
            window.__locale = currentLang;
        } catch (e) { /* ignore */ }

        document.addEventListener('click', handleGlobalClickForLangDropdown);

        try {
            if (typeof refreshSystemReadyMessageBubbles === 'function') {
                refreshSystemReadyMessageBubbles();
            }
        } catch (e) { /* ignore */ }

        if (typeof i18nReadyResolve === 'function') i18nReadyResolve();
    }

    exportGlobals();

    document.addEventListener('DOMContentLoaded', function () {
        initI18n().catch(function (e) {
            console.error('初始化国际化失败:', e);
            exportGlobals();
            if (typeof i18nReadyResolve === 'function') i18nReadyResolve();
        });
    });
})();
