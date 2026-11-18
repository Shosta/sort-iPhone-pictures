import assert from 'node:assert/strict';
import { readFile } from 'node:fs/promises';
import test from 'node:test';

const html = await readFile(new URL('../dist/index.html', import.meta.url), 'utf8');
const rawBase = process.env.ASTRO_BASE ?? '/';
const base = rawBase.endsWith('/') ? rawBase : `${rawBase}/`;
const escapedBase = base.replace(/[.*+?^${}()|[\]\\/]/g, '\\$&');

test('la page est en français et contient ses métadonnées', () => {
  assert.match(html, /<html lang="fr">/);
  assert.match(html, /<title>Photosort/);
  assert.match(html, /<meta name="description" content="[^"]+"/);
  assert.equal((html.match(/<h1\b/g) ?? []).length, 1);
});

test('tous les liens internes ont une destination', () => {
  assert.doesNotMatch(html, /href="(?:#)?"/);
  const ids = new Set([...html.matchAll(/\bid="([^"]+)"/g)].map((match) => match[1]));
  for (const [, anchor] of html.matchAll(/href="#([^"]+)"/g)) {
    assert.ok(ids.has(anchor), `Ancre introuvable : ${anchor}`);
  }
  const brandLinks = [...html.matchAll(/<a\b[^>]*class="brand"[^>]*>/g)];
  assert.equal(brandLinks.length, 2);
  for (const [link] of brandLinks) {
    assert.match(link, new RegExp(`href="${escapedBase}"`));
  }
});

test('les logos sont optimisés avec des dimensions et des variantes haute densité', async () => {
  const logos = [...html.matchAll(/<img\b[^>]*>/g)].map((match) => match[0]);
  assert.equal(logos.length, 2);
  assert.doesNotMatch(html, new RegExp(`src="${escapedBase}photosort\\.png"`));
  for (const [index, logo] of logos.entries()) {
    const size = index === 0 ? 36 : 30;
    assert.match(logo, new RegExp(`width="${size}"`));
    assert.match(logo, new RegExp(`height="${size}"`));
    assert.match(logo, new RegExp(`src="${escapedBase}_astro/photosort[^" ]+\\.webp"`));
    assert.match(logo, index === 0 ? /loading="eager"/ : /loading="lazy"/);
    const srcset = logo.match(/srcset="([^"]+)"/)?.[1];
    assert.ok(srcset);
    assert.match(srcset, / 2x(?:,|$)/);
    assert.match(srcset, / 3x(?:,|$)/);
    for (const variant of srcset.split(',')) {
      const [asset] = variant.trim().split(/\s+/);
      const image = await readFile(new URL(`../dist/${asset.slice(base.length)}`, import.meta.url));
      assert.equal(image.toString('ascii', 0, 4), 'RIFF');
      assert.equal(image.toString('ascii', 8, 12), 'WEBP');
    }
  }
});

test('les favicons conservent les icônes PNG de Photosort', async () => {
  assert.doesNotMatch(html, /favicon\.svg/);
  for (const [asset, size] of [['favicon-16.png', 16], ['favicon-32.png', 32]]) {
    const png = await readFile(new URL(`../dist/${asset}`, import.meta.url));
    assert.deepEqual(png.subarray(0, 8), Buffer.from([137, 80, 78, 71, 13, 10, 26, 10]));
    assert.equal(png.readUInt32BE(16), size);
    assert.equal(png.readUInt32BE(20), size);
    assert.ok(html.includes(`<link rel="icon" href="${base}${asset}" type="image/png" sizes="${size}x${size}">`));
  }
});

test('les limites et la confidentialité sont visibles sans JavaScript', () => {
  assert.equal((html.match(/<details\b/g) ?? []).length, 6);
  assert.match(html, /ne se connecte pas directement à votre iPhone/);
  assert.match(html, /Elle ne détecte pas encore les doublons de contenu/);
  assert.match(html, /aucune photo envoyée dans le cloud/);
  assert.match(html, /n’est pas notarisé par Apple/);
  assert.doesNotMatch(html, /<script\b/);
  assert.doesNotMatch(html, /<iframe\b/);
});

test('les liens du dépôt ne sont pas inventés', () => {
  const repositoryUrl = process.env.PUBLIC_REPOSITORY_URL?.trim();
  if (repositoryUrl) {
    const escapedUrl = repositoryUrl
      .replaceAll('&', '&amp;')
      .replaceAll('"', '&quot;');
    assert.ok(html.includes(`href="${escapedUrl}"`));
    assert.ok(html.includes('Accéder au projet et aux instructions'));
    assert.ok(!html.includes('Publication du dépôt à venir.'));
  } else {
    assert.ok(html.includes('Publication du dépôt à venir.'));
    assert.doesNotMatch(html, /href="https:\/\/github\.com\//);
  }
});

test('les ressources locales existent et le CSS ne charge rien à distance', async () => {
  const assets = [...html.matchAll(/(?:href|src)="(\/[^"]+)"/g)].map((match) => match[1]).filter((asset) => !asset.endsWith('/'));
  assert.ok(assets.some((asset) => asset.endsWith('.css')));
  for (const asset of assets) {
    const content = await readFile(new URL(`../dist/${asset.slice(base.length)}`, import.meta.url), 'utf8');
    assert.ok(content.length > 0);
    if (asset.endsWith('.css')) {
      assert.doesNotMatch(content, /@import|https?:\/\//);
    }
  }
});
