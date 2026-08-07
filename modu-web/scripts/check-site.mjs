import { readFile } from 'node:fs/promises';
import { join } from 'node:path';

const dist = new URL('../dist/', import.meta.url);
const pages = [
  ['index.html', 'zh-CN', 'https://modu.crosszan.com/', 'https://modu.crosszan.com/en/'],
  ['docs.html', 'zh-CN', 'https://modu.crosszan.com/docs', 'https://modu.crosszan.com/en/docs'],
  ['pricing.html', 'zh-CN', 'https://modu.crosszan.com/pricing', 'https://modu.crosszan.com/en/pricing'],
  ['privacy.html', 'zh-CN', 'https://modu.crosszan.com/privacy', 'https://modu.crosszan.com/en/privacy'],
  ['terms.html', 'zh-CN', 'https://modu.crosszan.com/terms', 'https://modu.crosszan.com/en/terms'],
  ['en/index.html', 'en', 'https://modu.crosszan.com/en/', 'https://modu.crosszan.com/'],
  ['en/docs.html', 'en', 'https://modu.crosszan.com/en/docs', 'https://modu.crosszan.com/docs'],
  ['en/pricing.html', 'en', 'https://modu.crosszan.com/en/pricing', 'https://modu.crosszan.com/pricing'],
  ['en/privacy.html', 'en', 'https://modu.crosszan.com/en/privacy', 'https://modu.crosszan.com/privacy'],
  ['en/terms.html', 'en', 'https://modu.crosszan.com/en/terms', 'https://modu.crosszan.com/terms'],
  ['docs/cli.html', 'zh-CN', 'https://modu.crosszan.com/docs/cli', 'https://modu.crosszan.com/en/docs/cli'],
  ['docs/models.html', 'zh-CN', 'https://modu.crosszan.com/docs/models', 'https://modu.crosszan.com/en/docs/models'],
  ['en/docs/cli.html', 'en', 'https://modu.crosszan.com/en/docs/cli', 'https://modu.crosszan.com/docs/cli'],
  ['en/docs/models.html', 'en', 'https://modu.crosszan.com/en/docs/models', 'https://modu.crosszan.com/docs/models'],
  ['guides/go-agent-framework.html', 'zh-CN', 'https://modu.crosszan.com/guides/go-agent-framework', 'https://modu.crosszan.com/en/guides/go-agent-framework'],
  ['en/guides/go-agent-framework.html', 'en', 'https://modu.crosszan.com/en/guides/go-agent-framework', 'https://modu.crosszan.com/guides/go-agent-framework']
];

const failures = [];
const htmlByFile = new Map();
const titles = new Map();
const descriptions = new Map();
const assert = (condition, message) => {
  if (!condition) failures.push(message);
};

for (const [file, lang, canonical, counterpart] of pages) {
  const html = await readFile(new URL(file, dist), 'utf8');
  htmlByFile.set(file, html);
  assert(html.includes(`<html lang="${lang}">`), `${file}: incorrect html lang`);
  assert(html.includes(`<link rel="canonical" href="${canonical}"`), `${file}: missing canonical URL`);
  assert(html.includes('hreflang="zh-CN"'), `${file}: missing zh-CN alternate`);
  assert(html.includes('hreflang="en"'), `${file}: missing English alternate`);
  assert(
    (html.match(/<link\s+rel="alternate"\s+hreflang="zh-CN"/g) ?? []).length === 1,
    `${file}: expected one zh-CN alternate`
  );
  assert(
    (html.match(/<link\s+rel="alternate"\s+hreflang="en"/g) ?? []).length === 1,
    `${file}: expected one English alternate`
  );
  assert(
    (html.match(/<link\s+rel="alternate"\s+hreflang="x-default"/g) ?? []).length === 1,
    `${file}: expected one x-default alternate`
  );
  assert(html.includes(`href="${counterpart}"`), `${file}: missing language counterpart`);
  assert(html.includes('property="og:title"'), `${file}: missing Open Graph title`);
  assert(html.includes('property="og:description"'), `${file}: missing Open Graph description`);
  assert(html.includes('name="twitter:card"'), `${file}: missing Twitter card`);
  assert(html.includes('property="og:image:width"'), `${file}: missing og:image dimensions`);
  assert(html.includes('property="og:image:alt"'), `${file}: missing og:image alt text`);
  assert(!html.includes('data-en='), `${file}: contains obsolete partial translation attributes`);
  assert(!/href="\/[^"#?]*\.html/.test(html), `${file}: contains a public .html link`);

  const title = html.match(/<title>([^<]+)<\/title>/)?.[1]?.trim();
  const description = html.match(/<meta\s+name="description"\s+content="([^"]+)"/s)?.[1]?.trim();
  assert(Boolean(title), `${file}: missing title text`);
  assert(Boolean(description), `${file}: missing meta description text`);
  if (title) {
    assert(!titles.has(title), `${file}: duplicate title also used by ${titles.get(title)}`);
    titles.set(title, file);
  }
  if (description) {
    assert(
      !descriptions.has(description),
      `${file}: duplicate meta description also used by ${descriptions.get(description)}`
    );
    descriptions.set(description, file);
  }

  for (const match of html.matchAll(/<script type="application\/ld\+json">([\s\S]*?)<\/script>/g)) {
    try {
      JSON.parse(match[1]);
    } catch (error) {
      failures.push(`${file}: JSON-LD does not parse (${error.message})`);
    }
  }
}

// Every docs page should carry TechArticle + BreadcrumbList, and the JSON-LD
// must parse — a malformed block is silently ignored by crawlers.
for (const file of ['docs.html', 'docs/cli.html', 'docs/models.html', 'en/docs.html', 'en/docs/cli.html', 'en/docs/models.html']) {
  const html = await readFile(new URL(file, dist), 'utf8');
  const match = html.match(/<script type="application\/ld\+json">([\s\S]*?)<\/script>/);
  if (!match) {
    failures.push(`${file}: missing JSON-LD`);
    continue;
  }
  try {
    const parsed = JSON.parse(match[1]);
    const types = (parsed['@graph'] ?? []).map((node) => node['@type']);
    assert(types.includes('TechArticle'), `${file}: JSON-LD missing TechArticle`);
    assert(types.includes('BreadcrumbList'), `${file}: JSON-LD missing BreadcrumbList`);
  } catch (error) {
    failures.push(`${file}: JSON-LD does not parse (${error.message})`);
  }
}

for (const file of ['guides/go-agent-framework.html', 'en/guides/go-agent-framework.html']) {
  const html = htmlByFile.get(file);
  assert(html.includes('"@type": "TechArticle"'), `${file}: JSON-LD missing TechArticle`);
  assert(html.includes('"datePublished": "2026-08-07"'), `${file}: JSON-LD missing publication date`);
  assert((html.match(/<h1\b/g) ?? []).length === 1, `${file}: expected exactly one h1`);
}

assert(
  htmlByFile.get('index.html').includes('href="/guides/go-agent-framework"'),
  'index.html: missing crawlable link to the Chinese Go agent guide'
);
assert(
  htmlByFile.get('docs.html').includes('href="/guides/go-agent-framework"'),
  'docs.html: missing crawlable link to the Chinese Go agent guide'
);
assert(
  htmlByFile.get('en/index.html').includes('href="/en/guides/go-agent-framework"'),
  'en/index.html: missing crawlable link to the English Go agent guide'
);
assert(
  htmlByFile.get('en/docs.html').includes('href="/en/guides/go-agent-framework"'),
  'en/docs.html: missing crawlable link to the English Go agent guide'
);

const robots = await readFile(new URL('robots.txt', dist), 'utf8');
assert(robots.includes('Allow: /'), 'robots.txt: site is not crawlable');
assert(robots.includes('https://modu.crosszan.com/sitemap.xml'), 'robots.txt: missing sitemap');

const sitemap = await readFile(new URL('sitemap.xml', dist), 'utf8');
for (const [, , canonical] of pages) {
  assert(sitemap.includes(`<loc>${canonical}</loc>`), `sitemap.xml: missing ${canonical}`);
}

const redirects = await readFile(new URL('_redirects', dist), 'utf8');
assert(/^\/zh\s+\/\s+301$/m.test(redirects), '_redirects: missing /zh to / permanent redirect');
assert(/^\/zh\/docs\s+\/docs\s+301$/m.test(redirects), '_redirects: missing /zh/docs to /docs permanent redirect');

const notFound = await readFile(new URL('404.html', dist), 'utf8');
assert(notFound.includes('noindex,follow'), '404.html: missing noindex');
assert(notFound.includes('class="not-found'), '404.html: missing standalone 404 content');

await Promise.all(
  ['logo.png', 'site.css', 'main.js'].map(async (asset) => {
    const source = asset === 'logo.png' ? new URL(asset, dist) : join(new URL('assets/', dist).pathname, asset);
    if (asset !== 'logo.png') return;
    await readFile(source);
  })
);

if (failures.length) {
  console.error(`Site checks failed:\n- ${failures.join('\n- ')}`);
  process.exit(1);
}

console.log(
  `Site checks passed for ${pages.length} localized pages, unique metadata, guide discovery, redirects, robots.txt, sitemap.xml, and 404.html.`
);
