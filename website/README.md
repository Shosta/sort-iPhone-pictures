# Photosort – site de présentation

Site Astro statique, en français, anglais et danois, publié sur GitHub Pages. Il vit dans `website/`,
indépendamment de l'application (Go + Electron).

## Développement

Node.js 22 (`.nvmrc`) et npm.

```sh
npm ci
npm run dev       # http://localhost:4321
npm test          # compile puis vérifie ancres, ressources, métadonnées, limites
npm run preview
```

## Variables

| Variable | Rôle | Défaut |
| --- | --- | --- |
| `PUBLIC_REPOSITORY_URL` | URL HTTPS du dépôt (active les boutons) | absente : « Publication du dépôt à venir » |
| `ASTRO_SITE` | origine du site | `https://shosta.github.io` |
| `ASTRO_BASE` | sous-chemin (`/<dépôt>` sur Pages) | `/` |

Les liens internes passent par `import.meta.env.BASE_URL`. Les tests lisent
`ASTRO_BASE` : lancer `ASTRO_BASE=/mon-depot npm test` pour simuler Pages.

## Déploiement (GitHub Pages)

Le workflow `.github/workflows/pages.yml` publie à chaque push sur `main` qui
touche `website/**`, ou à la demande (*Run workflow*). Il compile, teste, puis
déploie. `ASTRO_BASE` et `PUBLIC_REPOSITORY_URL` sont déduits du dépôt.

Réglage unique : **Settings > Pages > Build and deployment > Source : GitHub Actions**.
Le dépôt doit être public pour utiliser Pages gratuitement.

Adresse : `https://shosta.github.io/<dépôt>/` (ou `https://shosta.github.io/`
si le dépôt s'appelle `shosta.github.io`).

## Contenu

- Pages : `/fr/`, `/en/`, `/da/` (gabarit `src/pages/[lang]/index.astro`). La racine `/`
  choisit la langue du navigateur, avec un choix manuel si JavaScript est absent.
- Textes : `src/i18n/{fr,en,da}.ts`, typés par `Copy` (`types.ts`). Modifier une langue
  impose de modifier les deux autres. Le danois est à relire par une personne native.
- Composants : `src/components/` (herbier, tampon, planche, tirage).
- Styles : `src/styles/global.css`. Logo : `src/assets/photosort.png` ; favicons : `public/favicon-*.png`.
- Polices (Young Serif, Newsreader, IBM Plex Mono, Caveat) : fichiers `woff2` dans
  `src/assets/fonts/`, licences OFL incluses. Aucune requête externe, aucun analytics,
  cookie ou formulaire.

## Captures d'écran

Déposer les images dans `src/assets/screenshots/` (png, jpg ou webp), nommées :
`apercu`, `lieux`, `doublons`, `scrapbook`. Tant qu'un fichier manque, la page affiche
un emplacement « Capture à venir ».
