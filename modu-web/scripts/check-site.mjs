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
  ['en/terms.html', 'en', 'https://modu.crosszan.com/en/terms', 'https://modu.crosszan.com/terms']
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
  assert(!html.includes('data-en='), `${file}: contains obsolete partial translation attributes`);
  assert(!/href="\/[^"#?]*\.html/.test(html), `${file}: contains a public .html link`);
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
