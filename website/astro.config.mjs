import { defineConfig } from 'astro/config';

// ASTRO_BASE vaut `/<depot>` sur GitHub Pages (site de projet) et `/` en local.
export default defineConfig({
  output: 'static',
  site: process.env.ASTRO_SITE ?? 'https://shosta.github.io',
  base: process.env.ASTRO_BASE ?? '/',
});
