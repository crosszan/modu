// Generates the per-topic documentation pages.
//
// These pages share one shell (head metadata, header, sidebar, footer) that
// differs only by language and by which sidebar entry is current. Writing
// each page by hand would mean ten near-identical copies drifting apart —
// most obviously the sidebar, which every new page has to appear in. So the
// shell lives here once and the pages are generated from the content below.
//
// Run via `npm run docs` (also runs as part of `npm run build`).
import { mkdir, writeFile } from 'node:fs/promises';
import { dirname, resolve } from 'node:path';
import { fileURLToPath } from 'node:url';

const root = resolve(dirname(fileURLToPath(import.meta.url)), '..');
const origin = 'https://modu.crosszan.com';

const escapeHtml = (value) =>
  value.replace(/&/g, '&amp;').replace(/</g, '&lt;').replace(/>/g, '&gt;').replace(/"/g, '&quot;');

const strings = {
  'zh-CN': {
    docsWord: '文档',
    navDocs: '文档',
    navRuntime: 'Agent Runtime',
    navTeams: '多 Agent 协作',
    navExamples: '示例',
    skip: '跳到文档正文',
    navLabel: '文档导航',
    tocLabel: '文档目录',
    filter: '筛选文档目录',
    browse: '浏览文档目录',
    searchEmpty: '没有匹配的文档',
    openNav: '打开导航',
    theme: '切换深色模式',
    copy: '复制',
    home: '首页',
    privacy: '隐私协议',
    terms: '服务条款',
    tagline: 'Go-native building blocks for agent applications.',
    copied: '已复制到剪贴板',
    langToggle: 'Switch to English',
    langCode: 'EN'
  },
  en: {
    docsWord: 'Docs',
    navDocs: 'Docs',
    navRuntime: 'Agent runtime',
    navTeams: 'Multi-agent teams',
    navExamples: 'Examples',
    skip: 'Skip to documentation',
    navLabel: 'Documentation navigation',
    tocLabel: 'Documentation contents',
    filter: 'Filter contents',
    browse: 'Browse contents',
    searchEmpty: 'No matching pages',
    openNav: 'Open navigation',
    theme: 'Toggle dark mode',
    copy: 'Copy',
    home: 'Home',
    privacy: 'Privacy',
    terms: 'Terms',
    tagline: 'Go-native building blocks for agent applications.',
    copied: 'Copied to clipboard',
    langToggle: '切换到中文',
    langCode: '中文'
  }
};

// The sidebar is shared by every docs page, so it is declared once. `path`
// is the language-neutral part of the URL.
const sidebar = [
  {
    heading: { 'zh-CN': '开始', en: 'Start' },
    items: [
      { path: '/docs', label: { 'zh-CN': '快速开始', en: 'Quickstart' } },
      { path: '/docs/cli', label: { 'zh-CN': '命令行 (modu_code)', en: 'CLI (modu_code)' } },
      { path: '/docs/models', label: { 'zh-CN': '模型与 Provider 配置', en: 'Models & providers' } }
    ]
  },
  {
    heading: { 'zh-CN': 'Agent Runtime', en: 'Agent runtime' },
    items: [
      { path: '/docs#runtime', label: { 'zh-CN': 'Agent Loop', en: 'Agent loop' } },
      { path: '/docs#recovery', label: { 'zh-CN': '会话恢复', en: 'Session recovery' } },
      { path: '/docs#events', label: { 'zh-CN': '事件与状态', en: 'Events & state' } }
    ]
  },
  {
    heading: { 'zh-CN': '多 Agent 协作', en: 'Multi-agent' },
    items: [
      { path: '/docs#mailbox', label: { 'zh-CN': 'Mailbox Teams', en: 'Mailbox teams' } },
      { path: '/docs#architecture', label: { 'zh-CN': '协作架构', en: 'Architecture' } }
    ]
  }
];

const localizedHref = (path, lang) => (lang === 'en' ? `/en${path}` : path);

const renderSidebar = (lang, currentPath) =>
  sidebar
    .map((group) => {
      const links = group.items
        .map((item) => {
          const href = localizedHref(item.path, lang);
          const active = item.path === currentPath ? ' class="active"' : '';
          return `            <a${active} href="${href}">${escapeHtml(item.label[lang])}</a>`;
        })
        .join('\n');
      return `          <div>\n            <strong>${escapeHtml(group.heading[lang])}</strong>\n${links}\n          </div>`;
    })
    .join('\n');

const renderSection = (section, t) => {
  const body = section.blocks
    .map((block) => {
      if (block.p) return `          <p>${block.p}</p>`;
      if (block.note) {
        return [
          '          <div class="article-note">',
          '            <i class="ti ti-info-circle" aria-hidden="true"></i>',
          `            <p>${block.note}</p>`,
          '          </div>'
        ].join('\n');
      }
      if (block.list) {
        const items = block.list.map((entry) => `            <li>${entry}</li>`).join('\n');
        return `          <ul>\n${items}\n          </ul>`;
      }
      if (block.code) {
        const lines = block.code
          .split('\n')
          .map((line) => (block.shell && line.trim() ? `<span class="syntax-muted">$</span> ${escapeHtml(line)}` : escapeHtml(line)))
          .join('\n');
        return [
          '          <div class="article-code">',
          '            <div class="article-code-bar">',
          `              <span>${escapeHtml(block.caption ?? (block.shell ? 'terminal' : 'code'))}</span>`,
          `              <button class="copy-button" type="button" data-copy="${escapeHtml(block.code)}">`,
          `                <span data-copy-label>${t.copy}</span>`,
          '                <i class="ti ti-copy" aria-hidden="true"></i>',
          '              </button>',
          '            </div>',
          `            <pre><code>${lines}</code></pre>`,
          '          </div>'
        ].join('\n');
      }
      return '';
    })
    .join('\n');
  return `        <section id="${section.id}">\n          <h2>${escapeHtml(section.title)}</h2>\n${body}\n        </section>`;
};

const renderPage = (page) => {
  const { lang } = page;
  const t = strings[lang];
  const zhUrl = `${origin}${page.path}`;
  const enUrl = `${origin}/en${page.path}`;
  const selfUrl = lang === 'en' ? enUrl : zhUrl;
  const counterpart = lang === 'en' ? zhUrl : enUrl;
  const ogLocale = lang === 'en' ? 'en_US' : 'zh_CN';
  const homeUrl = lang === 'en' ? `${origin}/en/` : `${origin}/`;
  const imageAlt =
    lang === 'en'
      ? 'Modu — Go toolkit for building agent applications'
      : 'Modu — 用于构建 Agent 应用的 Go 工具箱';

  const jsonLd = {
    '@context': 'https://schema.org',
    '@graph': [
      {
        '@type': 'TechArticle',
        headline: page.title,
        description: page.description,
        inLanguage: lang,
        url: selfUrl,
        mainEntityOfPage: selfUrl,
        isPartOf: { '@id': `${origin}/#website` },
        publisher: { '@id': `${origin}/#organization` },
        about: { '@type': 'SoftwareSourceCode', name: 'Modu', programmingLanguage: 'Go' }
      },
      {
        '@type': 'BreadcrumbList',
        itemListElement: [
          { '@type': 'ListItem', position: 1, name: t.home, item: homeUrl },
          { '@type': 'ListItem', position: 2, name: t.docsWord, item: localizedHref('/docs', lang).replace(/^/, origin) },
          { '@type': 'ListItem', position: 3, name: page.crumb, item: selfUrl }
        ]
      }
    ]
  };

  return `<!doctype html>
<html lang="${lang}">
  <head>
    <meta charset="UTF-8" />
    <meta name="viewport" content="width=device-width, initial-scale=1.0" />
    <meta name="description" content="${escapeHtml(page.description)}" />
    <link rel="canonical" href="${selfUrl}" />
    <link rel="alternate" hreflang="zh-CN" href="${zhUrl}" />
    <link rel="alternate" hreflang="en" href="${enUrl}" />
    <link rel="alternate" hreflang="x-default" href="${zhUrl}" />
    <link rel="icon" type="image/png" href="/logo.png" />
    <link rel="stylesheet" href="/site.css" />
    <meta property="og:type" content="article" />
    <meta property="og:site_name" content="Modu" />
    <meta property="og:locale" content="${ogLocale}" />
    <meta property="og:title" content="${escapeHtml(page.title)}" />
    <meta property="og:description" content="${escapeHtml(page.description)}" />
    <meta property="og:url" content="${selfUrl}" />
    <meta property="og:image" content="${origin}/logo.png" />
    <meta property="og:image:width" content="1024" />
    <meta property="og:image:height" content="1024" />
    <meta property="og:image:alt" content="${escapeHtml(imageAlt)}" />
    <meta name="twitter:card" content="summary" />
    <meta name="twitter:title" content="${escapeHtml(page.title)}" />
    <meta name="twitter:description" content="${escapeHtml(page.description)}" />
    <meta name="twitter:image" content="${origin}/logo.png" />
    <script type="application/ld+json">
${JSON.stringify(jsonLd, null, 2)
  .split('\n')
  .map((line) => `      ${line}`)
  .join('\n')}
    </script>
    <title>${escapeHtml(page.title)}</title>
  </head>
  <body class="site-page docs-page">
    <a class="skip-link" href="#docs-content">${t.skip}</a>

    <header class="site-header">
      <nav class="nav-shell" aria-label="${t.navLabel}">
        <a class="brand" href="${lang === 'en' ? '/en/' : '/'}">
          <img src="/logo.png" width="34" height="34" alt="" />
          <span>Modu</span>
          <em>${t.docsWord}</em>
        </a>

        <button class="icon-button mobile-menu-button" type="button" aria-label="${t.openNav}" aria-expanded="false">
          <i class="ti ti-menu-2" aria-hidden="true"></i>
        </button>

        <div class="nav-menu">
          <div class="nav-links">
            <a class="active" href="${localizedHref('/docs', lang)}">${t.navDocs}</a>
            <a href="${lang === 'en' ? '/en/#runtime' : '/#runtime'}">${t.navRuntime}</a>
            <a href="${lang === 'en' ? '/en/#teams' : '/#teams'}">${t.navTeams}</a>
            <a href="https://github.com/openmodu/modu/tree/main/examples" target="_blank" rel="noreferrer">${t.navExamples}</a>
            <a href="https://github.com/openmodu/modu" target="_blank" rel="noreferrer">GitHub</a>
          </div>
          <div class="nav-actions">
            <button class="icon-button theme-toggle" type="button" aria-label="${t.theme}">
              <i class="ti ti-moon" aria-hidden="true"></i>
            </button>
            <a class="language-toggle" href="${lang === 'en' ? page.path : `/en${page.path}`}" lang="${lang === 'en' ? 'zh-CN' : 'en'}" hreflang="${lang === 'en' ? 'zh-CN' : 'en'}" aria-label="${t.langToggle}">${t.langCode}</a>
          </div>
        </div>
      </nav>
    </header>

    <div class="docs-layout shell">
      <aside class="docs-sidebar" aria-label="${t.tocLabel}">
        <label class="docs-search">
          <i class="ti ti-search" aria-hidden="true"></i>
          <input type="search" placeholder="${t.filter}" aria-label="${t.filter}" />
          <kbd>⌘ K</kbd>
        </label>
        <button class="docs-index-toggle" type="button" aria-expanded="false">
          <span>${t.browse}</span>
          <i class="ti ti-chevron-right" aria-hidden="true"></i>
        </button>
        <nav class="docs-nav">
${renderSidebar(lang, page.path)}
          <p class="search-empty" hidden>${t.searchEmpty}</p>
        </nav>
      </aside>

      <main class="docs-article" id="docs-content">
        <p class="breadcrumb">${t.docsWord} <i class="ti ti-chevron-right" aria-hidden="true"></i> ${escapeHtml(page.crumb)}</p>
        <header>
          <p class="eyebrow">${escapeHtml(page.eyebrow)}</p>
          <h1>${escapeHtml(page.heading)}</h1>
          <p class="article-lede">${page.lede}</p>
        </header>

${page.sections.map((section) => renderSection(section, t)).join('\n\n')}
      </main>
    </div>

    <footer class="site-footer docs-footer">
      <div class="shell footer-content">
        <div class="footer-brand">
          <a class="brand" href="${lang === 'en' ? '/en/' : '/'}">
            <img src="/logo.png" width="34" height="34" alt="" />
            <span>Modu</span>
          </a>
          <p>${t.tagline}</p>
        </div>
        <div class="footer-links">
          <a href="${lang === 'en' ? '/en/' : '/'}">${t.home}</a>
          <a href="https://github.com/openmodu/modu" target="_blank" rel="noreferrer">GitHub</a>
          <a href="${localizedHref('/privacy', lang)}">${t.privacy}</a>
          <a href="${localizedHref('/terms', lang)}">${t.terms}</a>
        </div>
      </div>
    </footer>

    <div class="copy-toast" role="status" aria-live="polite">${t.copied}</div>
    <script type="module" src="/main.js"></script>
  </body>
</html>
`;
};

const { pages } = await import('./docs-content.mjs');

for (const page of pages) {
  const file = resolve(root, page.lang === 'en' ? `en${page.path}.html` : `${page.path.slice(1)}.html`);
  await mkdir(dirname(file), { recursive: true });
  await writeFile(file, renderPage(page), 'utf8');
  console.log(`generated ${file.replace(`${root}/`, '')}`);
}
