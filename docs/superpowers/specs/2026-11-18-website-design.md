# Site de présentation Photosort – design

Date : 2026-11-18 · Branche : `feat/website` · Statut : à valider

## Objectif

Page de vente trilingue (fr, en, da) pour Photosort, hébergée dans le dépôt de
l'application (`website/`) et publiée automatiquement sur GitHub Pages à
`https://shosta.github.io/sort-iPhone-pictures/`. Le contenu décrit uniquement ce
qui existe aujourd'hui.

## Périmètre

Inclus : refonte visuelle, contenu réel, trois langues, emplacements pour les
captures d'écran, liens de téléchargement vers les releases, tests.
Exclus : analytics, formulaire, cookies, JavaScript applicatif, annonce du NAS
(en planification), signature des paquets.

## Contenu (même structure dans les 3 langues)

1. **Accroche** : trier les photos d'iPhone par date et par lieu, sans cloud,
   puis générer un PDF de scrapbook à imprimer et découper pour coller la
   semaine dans son semainier. Visuel : une planche A4 en perspective avec ses
   tirages et ses filets de coupe, et le tampon « sans cloud ». Boutons
   Télécharger et Découvrir le scrapbook.
2. **Scrapbook / semainier** (section dédiée juste après l'accroche) : choisir
   une semaine (lundi à dimanche) dans un calendrier, cocher les photos, obtenir
   un PDF A4 paysage (A4 portrait ou Letter en option), photos de 6,5 cm sur le
   grand côté, au moins 5 mm entre les photos pour la découpe, ouverture
   automatique du PDF. Les originaux ne sont jamais modifiés.
3. **3 étapes** : choisir le dossier source, vérifier l'aperçu, copier (les
   originaux ne sont jamais modifiés).
4. **Fonctionnalités** (présentées comme des étiquettes d'herbier numérotées) :
   - classement par date EXIF (`AAAA/AAAA-MM`) ;
   - classement par lieu, villes et monuments, hors ligne ;
   - doublons et quasi-doublons ;
   - tags IA locaux (ONNX Runtime) et tags libres ;
   - scrapbook PDF hebdomadaire ;
   - préférences : langues fr / en-GB / da, thèmes, ⌘,.
5. **Captures** : 3 à 4 tirages (emplacements, voir plus bas).
6. **Téléchargements** : macOS (dmg arm64 et Intel), Linux (AppImage x86_64),
   Windows (exe portable), tous vers `…/releases/latest`.
7. **Limites honnêtes** : paquets non signés (notarisation macOS, SmartScreen
   Windows) ; tags IA indisponibles sur Mac Intel ; date EXIF requise.
8. **FAQ** (`<details>`, sans JavaScript) et pied de page avec licence MIT.

## Architecture

- `src/i18n/{fr,en,da}.ts` : un dictionnaire typé par langue (même forme, vérifiée
  par TypeScript et par un test de clés identiques).
- `src/pages/[lang]/index.astro` : un seul gabarit, `getStaticPaths` pour les
  3 langues, `<html lang>`, `hreflang` alternates, sélecteur de langue (liens).
- `src/pages/index.astro` : redirection racine. Une courte redirection inline
  choisit la langue selon `navigator.language` (repli `fr`), avec
  `<meta http-equiv="refresh">` et un lien `<noscript>`. Seule page autorisée à
  contenir un script.
- Liens internes toujours construits à partir de `import.meta.env.BASE_URL`.
- `astro.config.mjs` : `site`, `base` via `ASTRO_SITE` / `ASTRO_BASE` (déjà en place).
- `PUBLIC_REPOSITORY_URL` est injectée par le workflow ; l'URL attendue est
  `https://github.com/shosta/sort-iPhone-pictures`.
- Le danois est rédigé par l'assistant et doit être relu par le propriétaire.

## Direction esthétique : « album photo »

- **Ton** : éditorial, chaleureux, artisanal. Une page qui ressemble à un album
  de tirages posés sur une table, pas à un SaaS.
- **Palette** (variables CSS) : papier crème `#f4ecdd`, encre `#1d1a16`, gris
  chaud `#6b6257`, accent unique rouge tampon `#b3362b`, filet `#d9cdb6`.
  Aucun dégradé violet, aucun vert.
- **Typographie** (fichiers `woff2` copiés dans `src/assets/fonts/`, aucun appel externe) :
  Young Serif pour les titres (Fraunces écartée : trop répandue), Newsreader pour
  le texte courant, IBM Plex Mono pour les légendes et métadonnées, Caveat
  uniquement pour les annotations manuscrites des tirages.
- **Atmosphère** : grain de papier léger (bruit SVG inline en fond), ombres
  portées douces sous les tirages, coins photo en SVG, bandes de masking tape
  translucides qui tiennent les tirages.
- **Herbier** : en fond, très discrètement (opacité 6 à 12 %, teinte encre
  brun-sépia), des plantes séchées dessinées en SVG inline (fougère, ginkgo,
  feuille de chêne, brin de graminée), posées dans les marges, pivotées et
  parfois coupées par le bord de page. Aucune ne passe derrière du texte long :
  la lisibilité prime. Les étiquettes de fonctionnalités reprennent les
  étiquettes d'herbier (numéro, nom, mono).
- **Tampon « sans cloud »** : différenciateur de l'application. Tampon rond rouge
  encre, légèrement pivoté et irrégulier (filtre SVG de bruit), dans l'accroche
  et repris, plus petit, sur la section Téléchargements.
- **Composition** : asymétrique ; titre décalé sur une grille de 12 colonnes,
  tirages qui débordent de la grille et se chevauchent, rotations de ±1 à 4°
  (neutralisées sous 640 px pour la lisibilité).
- **Mouvement** : un seul moment, les tirages se « déposent » au chargement
  (translation + rotation, 600 ms, décalage échelonné). Tout est désactivé sous
  `prefers-reduced-motion`.
- **Accessibilité** : contraste AA minimum, focus visible, `alt` réels dans
  chaque langue, structure `h1` unique, navigation clavier du sélecteur de langue.

## Captures d'écran

Fournies par le propriétaire dans `website/src/assets/screenshots/`
(`apercu.png`, `lieux.png`, `doublons.png`, `scrapbook.png`). Tant qu'un fichier
manque, le composant `Print.astro` affiche un tirage vide à légende
« capture à venir », sans image cassée. Les images passent par le composant
`Image` d'Astro (WebP, largeur et hauteur déclarées).

## Workflow et licence

- `.github/workflows/pages.yml` (déjà créé) : push sur `main` touchant
  `website/**` ou `workflow_dispatch` ; `npm ci`, `npm test`, déploiement.
- Réglage unique à faire par le propriétaire : *Settings > Pages > Source :
  GitHub Actions*, dépôt public.
- `LICENSE` MIT, titulaire « Shosta » (à confirmer).

## Tests (`npm test`, aussi avec `ASTRO_BASE=/sort-iPhone-pictures`)

- Les 3 pages existent, avec le bon `<html lang>`, un seul `h1` et des `hreflang`.
- Les dictionnaires ont exactement les mêmes clés.
- Ancres, ressources locales et liens de marque respectent la base.
- Pas de `<script>` sur les pages de langue, aucune ressource distante.
- Aucune mention du NAS ni de fonctionnalité inexistante.
- Les boutons du dépôt n'apparaissent que si `PUBLIC_REPOSITORY_URL` est définie.
- Les emplacements de capture manquants ne produisent pas d'image cassée.

## Hors de ce lot

Mise à jour du README racine (encore décrit en Fyne), annonce du NAS une fois livré.
