# Photosort showcase website – design

Date: 2026-11-18 · Branch: `feat/website` · Status: to be validated

## Goal

Trilingual (fr, en, da) sales page for Photosort, hosted in the application's
repository (`website/`) and published automatically to GitHub Pages at
`https://shosta.github.io/sort-iPhone-pictures/`. The content describes only what
exists today.

## Scope

Included: visual overhaul, real content, three languages, placeholders for
screenshots, download links to the releases, tests.
Excluded: analytics, form, cookies, application JavaScript, NAS announcement
(being planned), package signing.

## Content (same structure in all 3 languages)

Message priority: **bulk-sort loose iPhone photos by capture date, then find them
again thanks to tags**. The scrapbook follows from that: it is a bonus, presented
after the sorting.

1. **Hero**: "Thousands of loose iPhone photos, sorted by date and easy to find",
   no cloud. Visual: a taped A4 sheet with the "no cloud" stamp, whose cells
   contain small, discreet illustrations (portrait, landscapes, animals, city,
   friends) to suggest photos, captioned "Bonus · the weekly planner as a PDF".
   Buttons: See the sorting and Download.
2. **Sorting** ("The essentials"): 3 steps, choose the source folder,
   check the preview, copy (originals are never modified).
3. **Features** (numbered herbarium labels), in this order:
   - sorting by EXIF date (`YYYY/YYYY-MM`);
   - local AI tags (ONNX Runtime) and free-form tags, to find your photos;
   - sorting by place, cities and landmarks, offline;
   - duplicates and near-duplicates;
   - preferences: languages fr / en-GB / da, themes, ⌘, ;
   - weekly PDF scrapbook (last).
4. **Scrapbook / weekly planner** ("And then"): pick a week (Monday to
   Sunday), tick the photos, get an A4 landscape PDF (A4 portrait or Letter
   as an option), photos of 6.5 cm on the long side, at least 5 mm between photos
   for cutting, PDF opened automatically.
5. **Screenshots**: 3 to 4 prints (placeholders, see below).
6. **Downloads**: macOS (dmg arm64 and Intel), Linux (AppImage x86_64),
   Windows (portable exe), all pointing to `…/releases/latest`.
7. **Honest limitations**: unsigned packages (macOS notarisation, Windows
   SmartScreen); AI tags unavailable on Intel Macs; EXIF date required.
8. **FAQ** (`<details>`, no JavaScript) and footer with the MIT licence.

## Architecture

- `src/i18n/{fr,en,da}.ts`: one typed dictionary per language (same shape, checked
  by TypeScript and by an identical-keys test).
- `src/pages/[lang]/index.astro`: a single template, `getStaticPaths` for the
  3 languages, `<html lang>`, `hreflang` alternates, language selector (links).
- `src/pages/index.astro`: root redirect. A short inline redirect
  picks the language from `navigator.language` (fallback `fr`), with
  `<meta http-equiv="refresh">` and a `<noscript>` link. The only page allowed to
  contain a script.
- Internal links are always built from `import.meta.env.BASE_URL`.
- `astro.config.mjs`: `site`, `base` via `ASTRO_SITE` / `ASTRO_BASE` (already in place).
- `PUBLIC_REPOSITORY_URL` is injected by the workflow; the expected URL is
  `https://github.com/shosta/sort-iPhone-pictures`.
- The Danish is written by the assistant and must be proofread by the owner.

## Aesthetic direction: "photo album"

- **Tone**: editorial, warm, handcrafted. A page that looks like an album
  of prints laid out on a table, not like a SaaS.
- **Palette** (CSS variables): cream paper `#f4ecdd`, ink `#1d1a16`, warm
  grey `#6b6257`, single accent stamp red `#b3362b`, rule `#d9cdb6`.
  No purple gradient, no green.
- **Typography** (`woff2` files copied into `src/assets/fonts/`, no external calls):
  Young Serif for headings (Fraunces ruled out: too common), Newsreader for
  body text, IBM Plex Mono for captions and metadata, Caveat
  only for handwritten annotations on the prints.
- **Atmosphere**: light paper grain (inline SVG noise as background), soft
  drop shadows under the prints, SVG photo corners, translucent masking tape
  strips holding the prints.
- **Herbarium**: in the background, very discreetly (6 to 12% opacity, sepia-brown
  ink tint), dried plants drawn as inline SVG (fern, ginkgo,
  oak leaf, blade of grass), placed in the margins, rotated and
  sometimes cut off by the page edge. None goes behind long text:
  readability comes first. The feature labels reuse the herbarium
  labels (number, name, mono).
- **"No cloud" stamp**: the application's differentiator. Round ink-red stamp,
  slightly rotated and irregular (SVG noise filter), in the hero
  and repeated, smaller, in the Downloads section.
- **Layout**: asymmetric; heading offset on a 12-column grid,
  prints that overflow the grid and overlap, rotations of ±1 to 4°
  (neutralised below 640 px for readability).
- **Motion**: a single moment, the prints "settle" on load
  (translation + rotation, 600 ms, staggered delay). Everything is disabled under
  `prefers-reduced-motion`.
- **Accessibility**: AA contrast minimum, visible focus, real `alt` text in
  each language, single `h1` structure, keyboard navigation of the language selector.

## Screenshots

Provided by the owner in `website/src/assets/screenshots/`
(`apercu.png`, `lieux.png`, `doublons.png`, `scrapbook.png`). As long as a file
is missing, the `Print.astro` component displays an empty print with the caption
"capture à venir" (screenshot coming), with no broken image. Images go through Astro's
`Image` component (WebP, declared width and height).

## Workflow and licence

- `.github/workflows/pages.yml` (already created): push to `main` touching
  `website/**` or `workflow_dispatch`; `npm ci`, `npm test`, deploy.
- One-time setting for the owner to make: *Settings > Pages > Source:
  GitHub Actions*, public repository.
- MIT `LICENSE`, holder "Shosta" (to be confirmed).

## Tests (`npm test`, also with `ASTRO_BASE=/sort-iPhone-pictures`)

- The 3 pages exist, with the correct `<html lang>`, a single `h1` and `hreflang` links.
- The dictionaries have exactly the same keys.
- Anchors, local resources and brand links respect the base.
- No `<script>` on the language pages, no remote resources.
- No mention of the NAS or of any non-existent feature.
- The repository buttons appear only if `PUBLIC_REPOSITORY_URL` is set.
- Missing screenshot placeholders do not produce a broken image.

## Out of this batch

Update of the root README (still described as Fyne), NAS announcement once delivered.
