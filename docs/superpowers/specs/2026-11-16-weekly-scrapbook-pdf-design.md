# Planche scrapbooking hebdomadaire (PDF) : spécification

Date : 2026-11-16 · Branche : `feat/weekly-scrapbook-pdf` · Statut : **à valider**

## Objectif

Depuis le dossier racine classé (`Racine/2026/2026-05/…jpg`), choisir une semaine
dans un calendrier, voir les photos de cette semaine, en sélectionner, et obtenir
un PDF imprimable : une grille de photos à découper, à coller dans le semainier.
Le PDF s'ouvre automatiquement à la fin. Aucun original n'est modifié.

Nouveau bouton 📅 dans l'en-tête, à côté de 🏷️ (raccourci `Cmd/Ctrl+P`), qui ouvre
un panneau dédié, sur le modèle de `TagsPanel`. L'écran principal ne gagne qu'un
bouton : la règle des 3 actions maximum est respectée.

## Hypothèses retenues (à confirmer ou corriger)

Les questions n'ont pas pu être posées, ces choix sont mes décisions par défaut :

| # | Sujet | Choix par défaut |
| --- | --- | --- |
| 1 | Taille des photos | **Grand côté = 6,5 cm, ratio d'origine conservé** (indiqué par vous) : portrait 6,5 cm de haut, paysage 6,5 cm de large (soit 4,9 × 6,5 cm ou 6,5 × 4,9 cm pour un iPhone 3:4). Valeur réglable (5 à 8 cm), mémorisée |
| 2 | Recadrage | Aucun : chaque photo garde son cadrage et sa forme, imprimée sans déformation (orientation EXIF appliquée) |
| 3 | Début de semaine | Lundi (ISO) ; réglable en préférence |
| 4 | Date d'une photo | Préfixe `AAAA-MM-JJ` du nom de fichier (déjà écrit par le tri), sinon date EXIF, sinon date de modification |
| 5 | Sélection initiale | Aucune photo cochée ; boutons « tout cocher / décocher » |
| 6 | Format de page | A4 portrait, marges 7 mm ; A4 paysage et Letter proposés ; échelle 100 % rappelée avant impression |
| 7 | Aides à la découpe | Pas d'espace entre photos (moins de coupes) ; filet gris fin le long du contour de chaque photo ; espace de 0 à 3 mm réglable |
| 8 | Légende | Petite date (« mar. 12 ») sous chaque photo, désactivable |
| 9 | Dossier de sortie | `Racine/Semainiers/Semaine 2026-S20 (11-17 mai).pdf`, dossier créé au besoin |
| 10 | Exclusions | Les dossiers « … - Almost Duplicates » et les fichiers non image sont ignorés |

Hors périmètre (YAGNI, possibles plus tard) : taille propre à chaque photo
(S/M/L), panoramas plus larges (ils suivent la règle du grand côté dans la v1),
recadrage intelligent sur les visages (YuNet existe déjà), filtrage par tags,
mise en page manuelle par glisser-déposer.

## Approches étudiées

- **A. PDF en Go (recommandé)** : moteur de mise en page et génération PDF dans le
  cœur Go avec `github.com/go-pdf/fpdf` (MIT, v0.9.0). Cohérent avec « la logique
  métier reste en Go », parallélisable (goroutines), faible mémoire, aucune
  dépendance runtime (pas de Chromium).
- **B. `printToPDF` d'Electron (HTML/CSS)** : mise en page facile, mais la logique
  quitte Go, les images transitent par le renderer, mémoire élevée, rendu
  dépendant de Chromium.
- **C. Export PPTX** : reproduit l'ancien processus, mais n'automatise pas
  l'impression et ajoute une dépendance lourde.

→ **A**.

## Architecture

```text
internal/scrapbook/            (nouveau, Go pur, sans I/O réseau)
  week.go      Week(date, firstDay) -> [début, fin] ; étiquettes (« 11-17 mai »)
  scan.go      ScanWeek(root, week) -> []Photo ; DayCounts(root, month) -> map[jour]n
  layout.go    Layout(page, opts, []Size) -> [][]Placement   (pur, testé : placement par rangées)
  render.go    Render(photos, opts, out, progress)    (fpdf + redimensionnement)
cmd/photosort-sidecar/main.go  nouvelles opérations du protocole JSON
photosort-electron/src/renderer/components/ScrapbookPanel.tsx (+ Calendar.tsx)
```

### Opérations du sidecar (JSON par ligne, comme `tag`)

- `weekDays` `{root, month}` → `{days: {"2026-05-12": 7, …}}` : nombre de photos par
  jour, pour marquer le calendrier. Parcours limité à `Racine/AAAA/AAAA-MM`.
- `weekPhotos` `{root, from, to}` → événements `photo` en flux `{path, date, w, h}`
  (la liste s'affiche dès la première photo ; `w`×`h` = dimensions après
  orientation EXIF, lues dans l'en-tête sans décodage complet), puis `done`. Les
  miniatures réutilisent l'op `thumbnail` existante (cache, limitation de débit).
- `scrapbookLayout` `{page, longSide, gutter, sizes:[{w,h}…]}` → pages de positions
  en mm. Source unique de vérité : l'aperçu de l'UI dessine ces positions, le PDF aussi.
- `scrapbook` `{files, options, out}` → événements `progress {done,total}` puis
  `done {path, pages}`. Annulable (`cancel`), comme les autres opérations.

### Mise en page (`layout.go`)

Chaque photo est mise à l'échelle pour que son grand côté vaille `longSide`
(6,5 cm), sans recadrage. Pour que la découpe reste simple (coupes de bord à bord
d'abord en rangées, puis verticales), le placement se fait **par rangées** :

1. Les photos sont séparées par orientation (portrait / paysage / carré), l'ordre
   chronologique étant conservé dans chaque groupe.
2. Les rangées se remplissent de gauche à droite ; la hauteur d'une rangée est
   celle de sa photo la plus haute. Les photos restantes de chaque groupe sont
   réunies dans une dernière rangée mixte pour limiter le gaspillage de papier.
3. Les rangées s'empilent page après page ; une rangée ne coupe jamais une page.

Capacité indicative en A4 portrait pour des photos 3:4 (marges 7 mm, sans espace) :
4 portraits de 48,8 mm par rangée (4 rangées, soit 16 par page) ou 3 paysages de
65 mm par rangée (5 rangées, soit 15 par page) ; une page peut mêler rangées de portraits et de paysages. Le nombre de
pages est affiché avant la génération.

### Génération du PDF

1. Pool de workers : décodage (JPEG/HEIC/PNG déjà gérés, orientation EXIF
   appliquée), réduction à **300 dpi** de la taille imprimée, ré-encodage JPEG q90.
   Évite des PDF de 200 Mo et garde une impression nette.
2. Assemblage séquentiel des pages ; filet de découpe gris fin autour de chaque
   photo ; légende de date optionnelle dans l'espace libre sous une photo paysage
   (sinon omise : on ne grignote jamais la photo).
3. Écriture atomique (`.tmp` puis renommage), puis `shell.openPath` côté Electron
   à la réception de `done`.

### Interface

Le panneau se déroule en un seul écran, sans étapes bloquantes :

1. **Dossier racine** : prérempli avec le dossier de destination des préférences.
2. **Calendrier mensuel** : jours avec photos marqués d'un point ; un clic sur un
   jour surligne toute la semaine ; flèches mois précédent/suivant ; « Cette semaine ».
3. **Photos de la semaine** : `VirtualGrid` + `Thumb` existants, cases à cocher,
   compteur « 9 sur 38 ».
4. **Réglages** (repliés) : taille du grand côté (6,5 cm), format de page, espace
   entre photos, légende de date.
5. **Aperçu de page** à droite avec les photos à leur vraie proportion, mis à jour
   en direct (nombre de pages affiché), puis bouton **Créer le PDF**. Rappel :
   imprimer à 100 % (« taille réelle »), sans « adapter à la page ».

i18n fr/en/da ; préférences persistées dans `prefs.ts` (`scrapLongSide`, `scrapPage`,
`scrapGutter`, `scrapCaption`, `weekStart`).

## Direction esthétique

« **Carnet de papeterie** » : s'appuie sur les thèmes existants (variables CSS) pour
rester cohérent, avec un ton chaleureux de papier et de ruban adhésif.

- La semaine sélectionnée apparaît comme une **bande de washi tape** translucide
  qui se pose sur la rangée du calendrier (rotation de ~1°, bord dentelé en CSS).
- Les photos cochées prennent un léger **cadre blanc façon polaroïd** et un numéro
  d'ordre (tampon) ; décocher le retire en douceur.
- **Un moment fort** : à « Créer le PDF », les miniatures cochées s'envolent vers
  l'aperçu de page (réutilise `Flights.tsx`), la feuille se remplit, puis glisse
  hors de l'écran à l'ouverture du PDF. Le reste reste sobre : transitions
  `transform`/`opacity` uniquement, comme dans le reste de l'application.
- Typographie : pile système de l'application ; la légende de date en
  chasse fixe (`ui-monospace`) pour évoquer un tampon dateur.
- Pas de flou en direct ni d'ombres coûteuses (contraintes de performance existantes).

## Gestion des erreurs et cas limites

- Semaine sans photo : message clair et bouton désactivé.
- Photo illisible ou HEIC non décodable : ignorée avec avertissement dans le résumé,
  les autres pages sont générées.
- Plus de photos que de place sur une page : pagination automatique.
- Dossier racine invalide ou sans `AAAA/AAAA-MM` : message guidant vers le bon dossier.
- Semaine à cheval sur deux mois ou deux années : le scan couvre les deux dossiers.
- Annulation en cours : aucun fichier partiel laissé.

## Tests

- Go : `week_test.go` (début de semaine, chevauchement mois/année, 53ᵉ semaine),
  `scan_test.go` (arbre temporaire, exclusions, repli sur EXIF/mtime), `layout_test.go`
  (grand côté exact à 0,1 mm, portrait/paysage/carré, rangées mixtes, aucune
  superposition ni débordement des marges, pagination), `render_test.go` (PDF valide :
  en-tête `%PDF`, nombre de pages, taille raisonnable, annulation).
- Test de bout en bout du sidecar sur une **copie** d'un échantillon de
  `~/Pictures` (jamais l'original).
- UI : `npm run typecheck`, `electron-vite build`, parcours manuel (calendrier,
  sélection, aperçu, ouverture du PDF) et impression d'une planche à l'échelle 100 %
  pour vérifier les cotes (règle).

## Découpage en étapes

1. Cœur Go : `week`, `scan`, `layout` + tests.
2. Rendu PDF + opérations sidecar + test e2e.
3. Calendrier et sélection dans le panneau (sans aperçu).
4. Aperçu en direct, réglages, préférences, i18n.
5. Animations « carnet de papeterie », README, packaging inchangé (pas de nouvelle
   dépendance native : fpdf est en Go pur, compatible CGO/Windows du workflow CI).
