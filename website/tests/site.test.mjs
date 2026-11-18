import assert from 'node:assert/strict';
import { readFile, readdir } from 'node:fs/promises';
import test from 'node:test';

const languages = ['fr', 'en', 'da'];
const rawBase = process.env.ASTRO_BASE ?? '/';
const base = rawBase.endsWith('/') ? rawBase : `${rawBase}/`;
const escapedBase = base.replace(/[.*+?^${}()|[\]\\/]/g, '\\$&');
const dist = (path) => new URL(`../dist/${path}`, import.meta.url);
const pages = Object.fromEntries(
  await Promise.all(languages.map(async (lang) => [lang, await readFile(dist(`${lang}/index.html`), 'utf8')])),
);
const chooser = await readFile(dist('index.html'), 'utf8');
const count = (html, pattern) => (html.match(pattern) ?? []).length;

test('chaque langue a sa page, sa balise lang, un titre unique et des métadonnées', () => {
  for (const lang of languages) {
    const html = pages[lang];
    assert.match(html, new RegExp(`<html lang="${lang}">`));
    assert.match(html, /<title>[^<]*Photosort/);
    assert.match(html, /<meta name="description" content="[^"]+"/);
    assert.equal(count(html, /<h1\b/g), 1, lang);
    assert.doesNotMatch(html, /undefined|\[object Object\]/, lang);
  }
});

test('les trois langues ont la même structure', () => {
  for (const pattern of [/<h2\b/g, /<details\b/g, /class="specimen"/g, /<figure class="print /g, /<li class="specimen" style="--i:/g, /class="numeral"/g, /<svg\b/g]) {
    const counts = languages.map((lang) => count(pages[lang], pattern));
    assert.ok(counts[0] > 0, String(pattern));
    assert.deepEqual(counts, [counts[0], counts[0], counts[0]], String(pattern));
  }
});

test('hreflang, canonical et sélecteur de langue sont présents', () => {
  for (const lang of languages) {
    const html = pages[lang];
    for (const other of languages) {
      assert.match(html, new RegExp(`<link rel="alternate" hreflang="${other}" href="[^"]*${escapedBase}${other}/"`));
      assert.match(html, new RegExp(`<a href="${escapedBase}${other}/" lang="${other}" hreflang="${other}"`));
    }
    assert.match(html, /<link rel="alternate" hreflang="x-default"/);
    assert.match(html, new RegExp(`<link rel="canonical" href="[^"]*${escapedBase}${lang}/"`));
    assert.equal(count(html, /aria-current="true"/g), 1);
    assert.match(html, new RegExp(`href="${escapedBase}${lang}/" lang="${lang}" hreflang="${lang}" aria-current="true"`));
  }
});

test('la page racine redirige selon la langue du navigateur et reste utilisable sans script', () => {
  assert.equal(count(chooser, /<script\b/g), 1);
  for (const lang of languages) {
    assert.match(chooser, new RegExp(`<a href="${escapedBase}${lang}/" lang="${lang}"`));
  }
});

test('les pages de langue fonctionnent sans JavaScript ni ressource distante', () => {
  for (const lang of languages) {
    assert.doesNotMatch(pages[lang], /<script\b/);
    assert.doesNotMatch(pages[lang], /<iframe\b/);
  }
});

test('les ancres et les liens de marque respectent la base', () => {
  for (const lang of languages) {
    const html = pages[lang];
    assert.doesNotMatch(html, /href="(?:#)?"/);
    const ids = new Set([...html.matchAll(/\bid="([^"]+)"/g)].map((match) => match[1]));
    for (const [, anchor] of html.matchAll(/href="#([^"]+)"/g)) {
      assert.ok(ids.has(anchor), `${lang} : ancre introuvable ${anchor}`);
    }
    const brands = [...html.matchAll(/<a\b[^>]*class="brand"[^>]*>/g)];
    assert.equal(brands.length, 2);
    for (const [link] of brands) {
      assert.match(link, new RegExp(`href="${escapedBase}${lang}/"`));
    }
  }
});

test('les logos et favicons sont optimisés', async () => {
  const html = pages.fr;
  const logos = [...html.matchAll(/<img\b[^>]*\balt\b[^>]*>/g)].map((match) => match[0]);
  assert.equal(logos.length, 2);
  for (const logo of logos) {
    assert.match(logo, new RegExp(`src="${escapedBase}_astro/photosort[^" ]+\\.webp"`));
    const srcset = logo.match(/srcset="([^"]+)"/)?.[1];
    assert.ok(srcset && / 2x/.test(srcset) && / 3x/.test(srcset));
  }
  for (const [asset, size] of [['favicon-16.png', 16], ['favicon-32.png', 32]]) {
    const png = await readFile(dist(asset));
    assert.deepEqual(png.subarray(0, 8), Buffer.from([137, 80, 78, 71, 13, 10, 26, 10]));
    assert.equal(png.readUInt32BE(16), size);
    assert.ok(html.includes(`<link rel="icon" href="${base}${asset}" type="image/png" sizes="${size}x${size}">`));
  }
});

test('le semainier, le tampon et les limites sont annoncés honnêtement', () => {
  for (const lang of languages) {
    const html = pages[lang];
    assert.match(html, /id="semainier"/);
    assert.match(html, /<svg class="stamp[^"]*" viewBox="0 0 200 200" role="img"/);
    assert.equal(count(html, /<svg class="stamp/g), 2);
    assert.doesNotMatch(html, /\bNAS\b/);
  }
  assert.match(pages.fr, /Semainier|semainier/);
  assert.match(pages.en, /[Ss]crapbook/);
});

test('les liens du dépôt ne sont pas inventés', () => {
  const repositoryUrl = process.env.PUBLIC_REPOSITORY_URL?.trim();
  for (const lang of languages) {
    const html = pages[lang];
    if (repositoryUrl) {
      assert.ok(html.includes(`href="${repositoryUrl}/releases/latest"`));
      assert.ok(html.includes(`href="${repositoryUrl}"`));
      assert.doesNotMatch(html, /class="availability-note"/);
    } else {
      assert.match(html, /class="availability-note"/);
      assert.doesNotMatch(html, /href="https:\/\/github\.com\//);
    }
  }
});

test('sans capture, un emplacement propre remplace l’image', () => {
  for (const lang of languages) {
    const html = pages[lang];
    const placeholders = count(html, /data-placeholder="/g);
    const figures = count(html, /<figure class="print /g);
    const photos = count(html, /<figure class="print [\s\S]*?<img\b/g);
    assert.equal(figures, 4);
    assert.ok(placeholders + photos >= figures);
    assert.doesNotMatch(html, /<img\b[^>]*src=""/);
  }
});

test('les ressources locales existent et le CSS ne charge rien à distance', async () => {
  const assets = [...pages.fr.matchAll(/(?:href|src)="(\/[^"]+)"/g)].map((match) => match[1]).filter((asset) => !asset.endsWith('/'));
  const css = assets.filter((asset) => asset.endsWith('.css'));
  assert.ok(css.length > 0);
  for (const asset of assets) {
    const content = await readFile(dist(asset.slice(base.length)));
    assert.ok(content.length > 0, asset);
  }
  for (const asset of css) {
    const content = await readFile(dist(asset.slice(base.length)), 'utf8');
    assert.doesNotMatch(content, /@import\s+(?:url\()?["']?https?:/);
    assert.doesNotMatch(content, /url\(\s*["']?https?:/);
    const fonts = [...content.matchAll(/url\(["']?([^"')]+\.woff2)/g)].map((match) => match[1]);
    assert.ok(fonts.length >= 6, 'polices embarquées');
    for (const font of fonts) {
      const file = font.startsWith(base) ? font.slice(base.length) : font.replace(/^\//, '');
      await readFile(dist(file));
    }
  }
  const files = await readdir(dist('_astro'));
  assert.ok(files.some((file) => file.endsWith('.woff2')));
});
