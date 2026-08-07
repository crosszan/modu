# Modu website design QA

## Comparison target

- Source visual truth:
  - `/Users/ityike/Code/go/src/github.com/openmodu/modu/modu-web/qa-docs-desktop.png`
  - `/Users/ityike/.codex/visualizations/2026/08/06/019fd7b5-3956-7793-9191-738efc0a08c6/modu-seo-mobile-audit/02-docs-mobile-zh.png`
  - `/Users/ityike/.codex/visualizations/2026/08/06/019fd7b5-3956-7793-9191-738efc0a08c6/modu-seo-mobile-audit/03-docs-mobile-en-partial.png`
  - `/Users/ityike/.codex/visualizations/2026/08/06/019fd7b5-3956-7793-9191-738efc0a08c6/modu-seo-mobile-audit/06-pricing-mobile-zh-only.png`
- Rendered implementation:
  - `/Users/ityike/.codex/visualizations/2026/08/06/019fd7b5-3956-7793-9191-738efc0a08c6/modu-refactor-qa/qa-refactor-docs-desktop.png`
  - `/Users/ityike/.codex/visualizations/2026/08/06/019fd7b5-3956-7793-9191-738efc0a08c6/modu-refactor-qa/qa-refactor-docs-mobile-top.png`
  - `/Users/ityike/.codex/visualizations/2026/08/06/019fd7b5-3956-7793-9191-738efc0a08c6/modu-refactor-qa/qa-refactor-docs-mobile-en-top.png`
  - `/Users/ityike/.codex/visualizations/2026/08/06/019fd7b5-3956-7793-9191-738efc0a08c6/modu-refactor-qa/qa-refactor-pricing-mobile-top.png`
  - `/Users/ityike/.codex/visualizations/2026/08/06/019fd7b5-3956-7793-9191-738efc0a08c6/modu-refactor-qa/qa-refactor-docs-mobile-index-open.png`
- Local URL: `http://127.0.0.1:4173/`
- State: production build served by `vite preview`, light theme, public unauthenticated pages.

The source is the previous production implementation rather than a separate mock. The visual goal was to retain its warm editorial system while intentionally replacing the mobile documentation chip strip and the legacy dark legal layout.

## Viewport and normalization

- Mobile source and implementation: 390 × 844 CSS px, 390 × 844 screenshot pixels, density 1.
- Desktop source: 1425 px wide full-page capture; implementation: 1440 × 1000 CSS viewport and 1440 px wide full-page capture, density 1. The 15 px width difference was treated as a viewport normalization constraint, so desktop judgment focused on composition, grid behavior, type hierarchy, and overflow rather than pixel-perfect alignment.
- All mobile comparison images use equal-size, side-by-side source and implementation crops:
  - `/Users/ityike/.codex/visualizations/2026/08/06/019fd7b5-3956-7793-9191-738efc0a08c6/modu-refactor-qa/qa-compare-refactor-docs-mobile.jpg`
  - `/Users/ityike/.codex/visualizations/2026/08/06/019fd7b5-3956-7793-9191-738efc0a08c6/modu-refactor-qa/qa-compare-refactor-docs-mobile-en-top.jpg`
  - `/Users/ityike/.codex/visualizations/2026/08/06/019fd7b5-3956-7793-9191-738efc0a08c6/modu-refactor-qa/qa-compare-refactor-pricing-mobile.jpg`

## Full-view comparison

- Desktop documentation preserves the three-column information architecture, sticky index, article width, right-side table of contents, code surfaces, and editorial rhythm. The full page has no horizontal overflow at 1440 px.
- The 390 px documentation page has no horizontal overflow. The mobile index is now an explicit disclosure instead of an off-screen chip rail, and the article begins without requiring a horizontal swipe.
- The legal page now uses the same paper, typography, divider, header, footer, and responsive navigation tokens as the rest of the site. Its document and body scroll widths both equal the 390 px viewport.
- The English documentation is a complete document route. Its article contains no CJK text, while the language control correctly links back to `/docs`.

## Focused comparison

Focused mobile top-of-page comparisons were required because navigation, type scale, notices, language completeness, and legal-page density are not readable in a full-page desktop capture.

- Typography: the serif display hierarchy and sans-serif body hierarchy remain consistent with the source. Mobile code text is now 12 px instead of 9.5 px and scrolls within its own code surface.
- Spacing and layout rhythm: 16 px mobile page gutters, section dividers, legal-page vertical rhythm, and 44 px navigation tap targets are consistent and unclipped.
- Colors and tokens: the existing paper, ink, indigo accent, dark code surface, border, and shadow tokens are reused. No unrelated palette was introduced.
- Image quality: the original raster logo is reused with explicit intrinsic dimensions. No placeholder imagery, CSS drawing, inline SVG substitute, or generated asset was introduced.
- Copy and content: Chinese remains on the root routes; all five `/en/` routes contain complete English page copy. The obsolete partial `data-en` substitution is gone.
- Icons: the existing local icon set remains aligned with the controls; the documentation disclosure icon now rotates with its expanded state.

## Findings

No actionable P0, P1, or P2 findings remain.

- P3: the English legal copy is a faithful translation of the current project policy, but it is not a substitute for jurisdiction-specific legal review.
- P3: social previews currently use the square project logo. A future 1200 × 630 branded social card would improve link-preview presentation without affecting crawlability.

## Interaction and accessibility checks

- Mobile main navigation opens, changes its accessible label to “关闭导航”, locks body scroll, and returns body overflow to normal after navigation.
- The documentation index opens with `aria-expanded="true"`, rotates its icon, caps at 460 px, and scrolls internally when its content is taller.
- Documentation filtering with `Mailbox` leaves only the matching group and link visible.
- The Chinese “Switch to English” link navigates to `/en/docs`; the resulting page reports `lang="en"` and the English H1.
- English copy buttons show “Copied” and “Copied to clipboard”.
- Search controls, menu buttons, disclosure buttons, skip links, semantic headings, and reduced-motion rules remain present.
- Browser console errors checked after navigation and interactions: none.

## Comparison history

1. Initial mobile comparison found one P2 issue: the expanded documentation index measured 642 px and consumed almost the full 844 px viewport.
2. Fixed `.docs-nav.open` with `max-height: min(56vh, 460px)` and internal vertical scrolling, and rotated the disclosure icon when expanded.
3. Post-fix evidence: `/Users/ityike/.codex/visualizations/2026/08/06/019fd7b5-3956-7793-9191-738efc0a08c6/modu-refactor-qa/qa-refactor-docs-mobile-index-open.png`. Measured height is 460 px, overflow is `auto`, and page scroll width remains 390 px.

## Validation

- `npm run check`: passed.
- 10 localized pages checked for language, canonical URL, reciprocal hreflang, Open Graph, Twitter card, clean public links, and absence of partial-translation attributes.
- `robots.txt`, `sitemap.xml`, and `404.html`: present in the production build.

final result: passed
