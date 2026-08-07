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
  ['en/docs/models.html', 'en', 'https://modu.crosszan.com/en/docs/models', 'https://modu.crosszan.com/docs/models']
];

const failures = [];
const assert = (condition, message) => {
  if (!condition) failures.push(message);
};

for (const [file, lang, canonical, counterpart] of pages) {
  const html = await readFile(new URL(file, dist), 'utf8');
  assert(html.includes(`<html lang="${lang}">`), `${file}: incorrect html lang`);
  assert(html.includes(`<link rel="canonical" href="${canonical}"`), `${file}: missing canonical URL`);
  assert(html.includes('hreflang="zh-CN"'), `${file}: missing zh-CN alternate`);
  assert(html.includes('hreflang="en"'), `${file}: missing English alternate`);
  assert(html.includes(`href="${counterpart}"`), `${file}: missing language counterpart`);
  assert(html.includes('property="og:title"'), `${file}: missing Open Graph title`);
  assert(html.includes('property="og:description"'), `${file}: missing Open Graph description`);
  assert(html.includes('name="twitter:card"'), `${file}: missing Twitter card`);
  assert(html.includes('property="og:image:width"'), `${file}: missing og:image dimensions`);
  assert(html.includes('property="og:image:alt"'), `${file}: missing og:image alt text`);
  assert(!html.includes('data-en='), `${file}: contains obsolete partial translation attributes`);
  assert(!/href="\/[^"#?]*\.html/.test(html), `${file}: contains a public .html link`);
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

const robots = await readFile(new URL('robots.txt', dist), 'utf8');
assert(robots.includes('Allow: /'), 'robots.txt: site is not crawlable');
assert(robots.includes('https://modu.crosszan.com/sitemap.xml'), 'robots.txt: missing sitemap');

const sitemap = await readFile(new URL('sitemap.xml', dist), 'utf8');
for (const [, , canonical] of pages) {
  assert(sitemap.includes(`<loc>${canonical}</loc>`), `sitemap.xml: missing ${canonical}`);
}

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

console.log(`Site checks passed for ${pages.length} localized pages, robots.txt, sitemap.xml, and 404.html.`);
