# Modu website design QA

## Comparison target

- Source visual truth:
  `/Users/bytedance/.codex/generated_images/019fc7ad-a128-7fb3-9058-ba6161b6a791/exec-bc6c2c8c-c92b-4846-985a-19a7b27f2fc1.png`
- Source pixels: `864 x 1821`.
- Implementation routes:
  `http://127.0.0.1:4173/` and `http://127.0.0.1:4173/docs.html`.
- Implementation desktop evidence:
  `qa-home-desktop.png` (`1425 x 3404` full page),
  `qa-home-desktop-top.png` (`1440 x 1024` viewport), and
  `qa-docs-desktop.png` (`1425 x 5226` full page).
- Implementation mobile evidence:
  `qa-home-mobile.png` and `qa-docs-mobile.png` at a `390 x 844` CSS viewport.
- State: Chinese, light theme, signed-out public website.
- Browser density: CSS pixel capture at the connected browser's default
  device scale.

## Normalization and evidence

- Full-view comparison: `qa-compare-home-full.png`.
  The source and implementation were each normalized to `720px` width and
  placed in the same comparison image. The shorter source was padded only
  below its footer so the content itself was not stretched.
- Focused above-the-fold comparison: `qa-compare-home-top.png`.
  The source was cropped to its first `864 x 614` pixels; the implementation
  `1440 x 1024` viewport was normalized to the same `864 x 614` comparison
  size. This region was required because the hero typography, code panel,
  navigation, CTA sizes, and section transition were too small to judge in the
  full-page comparison.
- The documentation page has no independent source frame beyond the
  documentation preview inside the chosen visual. `qa-docs-desktop.png` and
  `qa-docs-mobile.png` were therefore reviewed against that preview's sidebar,
  article, code-block, and table-of-contents treatment.

## Required fidelity surfaces

- Fonts and typography: the implementation uses a Chinese serif display stack
  and a system sans/mono stack matching the source's editorial hierarchy.
  Hero wrapping was corrected to two deliberate lines on desktop; mobile
  wrapping remains readable without orphaned single characters.
- Spacing and layout rhythm: the split hero, alternating capability rows,
  thin dividers, code panels, documentation preview, CTA, and footer follow the
  source order and grouping. Desktop margins and mobile section spacing remain
  consistent and no page-level horizontal overflow was observed.
- Colors and tokens: warm off-white paper, charcoal code surfaces, muted ink,
  thin warm-gray rules, and a restrained blue-violet accent map directly to
  the source. Dark theme uses the same semantic token structure and retains
  readable contrast.
- Image quality and asset fidelity: the project logo is the supplied raster
  asset. All interface icons come from local Tabler SVG assets; there are no
  inline SVGs, handmade SVGs, placeholder illustrations, or CSS-drawn icon
  substitutes. The selected visual contains no additional photographic or
  illustrative asset that needed generation.
- Copy and content: the source's generic snippets were replaced with working
  Modu package names, commands, and examples from the repository. The landing
  page retains the selected promise and capability order; the documentation
  page expands it into accurate install, runtime, coding-agent, and mailbox
  guidance.

## Comparison history

### Iteration 1

- [P1] Desktop hero title left a single-character final line.
  Fix: reduced the desktop display scale and rebalanced the line length so the
  title resolves to the two-line composition shown in the source.
- [P2] An additional examples section made the landing page materially longer
  than the selected visual.
  Fix: removed the extra marketing section and linked the navigation item
  directly to the repository's runnable examples.
- [P2] The mobile documentation sidebar occupied the first viewport before the
  article began.
  Fix: converted the mobile directory to a compact, horizontally scrollable
  navigation row beneath search.
- [P2] Replacing the shared script initially removed the reveal behavior used
  by the existing legal pages.
  Fix: restored the legacy observer and verified the terms article remains
  visible.

Post-fix evidence: `qa-compare-home-top.png`,
`qa-compare-home-full.png`, `qa-home-mobile.png`, and
`qa-docs-mobile.png`.

## Interaction and runtime checks

- Install-command copy action reached its visible success state.
- Light/dark theme toggle updated the document theme.
- Chinese/English primary-copy toggle updated the hero and navigation.
- Documentation search for `Mailbox` returned only `Mailbox Teams`.
- Mobile navigation opened and exposed every primary route.
- Desktop and mobile browser console checks returned no errors.
- Existing terms page stayed visible after the shared-script change.
- `npm run build` completed successfully and emitted all five HTML entries.

## Findings

No actionable P0, P1, or P2 differences remain.

## Follow-up polish

- [P3] The implementation uses slightly more vertical breathing room between
  capability groups than the generated source. This is acceptable at a real
  `1440px` browser viewport and improves long-form readability.
- [P3] The documentation page intentionally contains substantially more real
  product content than the small preview in the source visual.

## Final result

final result: passed
