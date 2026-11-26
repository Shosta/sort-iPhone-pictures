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
| `PUBLIC_HAS_RELEASE` | `true` si une release est publiée : le bouton mène à `/releases/latest` avec détection du système. Sinon il mène à `/releases` avec « Bientôt disponible » | absente (= `false`) |
| `PUBLIC_HAS_PRERELEASE` | `true` si seule une préversion (`-rc.N`) est publiée (sans effet si `PUBLIC_HAS_RELEASE=true`) : le bouton mène à `/releases` avec « Voir la préversion », jamais présentée comme le téléchargement stable | absente (= `false`) |
| `ASTRO_SITE` | origine du site | `https://shosta.github.io` |
| `ASTRO_BASE` | sous-chemin (`/<dépôt>` sur Pages) | `/` |

Les liens internes passent par `import.meta.env.BASE_URL`. Les tests lisent
`ASTRO_BASE` : lancer `ASTRO_BASE=/mon-depot npm test` pour simuler Pages.

## Déploiement (GitHub Pages)

Le workflow `.github/workflows/pages.yml` publie à chaque push sur `main` qui
touche `website/**`, ou à la demande (*Run workflow*). Il compile, teste, puis
déploie. `ASTRO_BASE` et `PUBLIC_REPOSITORY_URL` sont déduits du dépôt ; `PUBLIC_HAS_RELEASE`
vient d'un appel à `gh api releases/latest` (qui ignore les préversions) et `PUBLIC_HAS_PRERELEASE` de la liste des releases. Le workflow se relance aussi à la publication ou
suppression d'une release (`release`), ce qui fait apparaître le téléchargement sans commit.

Réglage unique : **Settings > Pages > Build and deployment > Source : GitHub Actions**.
Le dépôt doit être public pour utiliser Pages gratuitement.

Adresse : `https://shosta.github.io/<dépôt>/` (ou `https://shosta.github.io/`
si le dépôt s'appelle `shosta.github.io`).

## Contenu

- Guide « Tri distant vers un NAS » : `/<lang>/tri-distant/` (gabarit `src/pages/[lang]/tri-distant.astro`, textes FR/EN/DA dans `src/i18n/nas.ts`, à tenir à jour avec les libellés et messages de l'application). La capture `src/assets/screenshots/tri-distant-fenetre.png` remplacera l'emplacement dès qu'elle existe.

- Accueil « page produit » (style Infuse, identité scrapbook) : `/fr/`, `/en/`, `/da/`
  (gabarit `src/pages/[lang]/index.astro`, textes `src/i18n/home.ts`) : accroche, bouton de
  téléchargement, six sections alternées avec capture, grille de fonctions, bloc « gratuit,
  sans publicité, sans suivi ». Un seul petit script en ligne adapte le libellé du bouton au
  système (macOS/Windows/Linux) ; sans JavaScript, le libellé générique reste.
- Ancien accueil, intact : `/<lang>/presentation/` (gabarit `src/pages/[lang]/presentation/index.astro`,
  textes `src/i18n/{fr,en,da}.ts`). **Pour revenir à l'ancien accueil** : `git mv` du gabarit
  `presentation/index.astro` vers `src/pages/[lang]/index.astro` (corriger les imports `../../../`
  → `../../`, `pageUrl` et `SiteHeader path`), et retirer la page produit.
- Racine : La racine `/`
  choisit la langue du navigateur, avec un choix manuel si JavaScript est absent.
- Textes : `src/i18n/{fr,en,da}.ts`, typés par `Copy` (`types.ts`). Modifier une langue
  impose de modifier les deux autres. Le danois est à relire par une personne native.
- Composants : `src/components/` (herbier, tampon, planche, tirage).
- Styles : `src/styles/global.css`. Logo : `src/assets/photosort.png` ; favicons : `public/favicon-*.png`.
- Polices (Young Serif, Newsreader, IBM Plex Mono, Caveat) : fichiers `woff2` dans
  `src/assets/fonts/`, licences OFL incluses. Aucune requête externe, aucun analytics,
  cookie ou formulaire.

## Captures d'écran

Les images sont dans `src/assets/screenshots/`. Pour en remplacer une, déposer un fichier de
même nom (png, jpg ou webp ; garder un seul fichier par nom) :

| Nom | Où |
| --- | --- |
| `hero` | grande image d'accueil et section « Vie privée » (écran de fin de tri) |
| `tri` | tri par date et lieu |
| `iphone` | import depuis un iPhone (DCIM) |
| `nas` | envoi vers un NAS (adresse fictive `nas.local`) |
| `cartes` | cartes de voyage |
| `doublons` | doublons (accueil et présentation) |

Recommandations : largeur 1600 px, WebP (~100 Ko max), **sans EXIF** (ex. `cwebp -metadata none`),
sans chemin personnel, nom d'utilisateur ni adresse réelle, et uniquement des photos libres de droits sans
visage. Les textes alternatifs sont dans `src/i18n/home.ts`. Les captures actuelles ont été prises
sur des photos de démonstration CC0 (voir `src/assets/screenshots/CREDITS.md`). Tant qu'un fichier manque, la page
affiche un emplacement « Capture à venir ».
