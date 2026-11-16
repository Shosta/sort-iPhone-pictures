# Planche scrapbooking hebdomadaire (PDF) : spécification

Date : 2026-11-16 · Branche : `feat/weekly-scrapbook-pdf` · Statut : **à valider** (v2, après retours de l'utilisateur)

## Objectif

Depuis le dossier racine classé (`Racine/2026/2026-05/…jpg`), choisir une semaine
dans un calendrier, voir les photos de cette semaine, en sélectionner, et obtenir
un PDF imprimable : une grille de photos à découper, à coller dans le semainier.
Le PDF s'ouvre automatiquement à la fin. Aucun original n'est modifié.

Nouveau bouton 📅 dans l'en-tête, à côté de 🏷️ (raccourci `Cmd/Ctrl+P`), qui ouvre
un panneau dédié, sur le modèle de `TagsPanel`. L'écran principal ne gagne qu'un
bouton : la règle des 3 actions maximum est respectée.

## Décisions

Confirmées par l'utilisateur (✔) ou retenues par défaut (hypothèse) :

| # | Sujet | Décision |
| --- | --- | --- |
| 1 | Taille des photos ✔ | **Grand côté = 6,5 cm, ratio d'origine conservé** : portrait 6,5 cm de haut, paysage 6,5 cm de large (soit 4,9 × 6,5 cm ou 6,5 × 4,9 cm pour un iPhone 3:4). Valeur réglable (5 à 8 cm) |
| 2 | Espace entre photos ✔ | **Au moins 5 mm** entre deux photos, horizontalement et verticalement, pour faciliter la découpe. Valeur par défaut 5 mm, réglable de 5 à 10 mm (jamais en dessous de 5 mm) |
| 3 | Début de semaine ✔ | Lundi → dimanche |
| 4 | Sélection initiale ✔ | Aucune photo cochée ; boutons « tout cocher / tout décocher » |
| 5 | Format de page ✔ | **A4 paysage** (297 × 210 mm) par défaut, marges 10 mm (zone non imprimable des imprimantes) ; A4 portrait et Letter en option ; rappel d'imprimer à 100 % |
| 6 | Dossier de sortie ✔ | Choisi dans les **préférences** (⚙️), **Téléchargements** par défaut ; nom `Semaine 2026-S20 (11-17 mai).pdf`, suffixe ` (2)` si le fichier existe déjà |
| 7 | Quasi-doublons ✔ | **Inclus** : les dossiers « … - Almost Duplicates » sont parcourus comme les autres ; seuls les fichiers non image sont ignorés |
| 8 | Recadrage (hypothèse) | Aucun : chaque photo garde son cadrage et sa forme, imprimée sans déformation (orientation EXIF appliquée) |
| 9 | Date d'une photo (hypothèse) | Préfixe `AAAA-MM-JJ` du nom de fichier (déjà écrit par le tri), sinon date EXIF, sinon date de modification |
| 10 | Aide à la découpe (hypothèse) | Filet gris fin autour de chaque photo ; l'espace entre les photos sert de zone de coupe |

Hors périmètre (YAGNI, possibles plus tard) : légende de date (inutile avec un
espace de coupe de 5 mm), taille propre à chaque photo (S/M/L), panoramas plus
larges (ils suivent la règle du grand côté dans la v1), recadrage intelligent sur
les visages (YuNet existe déjà), filtrage par tags, mise en page manuelle par
glisser-déposer.

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
               (récursif sous Racine/AAAA/AAAA-MM, sous-dossiers de lieu et
               « Almost Duplicates » compris)
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
  en mm. `gutter` est ramené à 5 mm minimum côté Go. Source unique de vérité :
  l'aperçu de l'UI dessine ces positions, le PDF aussi.
- `scrapbook` `{files, options, outDir, name}` → événements `progress {done,total}`
  puis `done {path, pages}`. `outDir` vient des préférences (Téléchargements par
  défaut), créé s'il manque. Annulable (`cancel`), comme les autres opérations.

### Mise en page (`layout.go`)

Chaque photo est mise à l'échelle pour que son grand côté vaille `longSide`
(6,5 cm), sans recadrage. Pour que la découpe reste simple (coupes de bord à bord
d'abord en rangées, puis verticales), le placement se fait **par rangées**, avec
un espace d'au moins `gutter` (5 mm) entre deux photos voisines et entre deux
rangées :

1. Les photos sont séparées par orientation (portrait / paysage / carré), l'ordre
   chronologique étant conservé dans chaque groupe.
2. Les rangées se remplissent de gauche à droite ; la hauteur d'une rangée est
   celle de sa photo la plus haute. Les photos restantes de chaque groupe sont
   réunies dans une dernière rangée mixte pour limiter le gaspillage de papier.
3. Les rangées s'empilent page après page ; une rangée ne coupe jamais une page.

Capacité en A4 paysage (zone utile 277 × 190 mm, espace 5 mm) pour des photos 3:4 :
5 portraits de 48,75 mm par rangée sur 2 rangées, soit **10 par page** ; ou
4 paysages de 65 mm par rangée sur 3 rangées de 48,75 mm, soit **12 par page**.
Une page peut mêler rangées de portraits et de paysages (par exemple 1 rangée de
portraits + 2 rangées de paysages = 172,5 mm de haut). Le nombre de pages est
affiché avant la génération.

### Génération du PDF

1. Pool de workers : décodage (JPEG/HEIC/PNG déjà gérés, orientation EXIF
   appliquée), réduction à **300 dpi** de la taille imprimée, ré-encodage JPEG q90.
   Évite des PDF de 200 Mo et garde une impression nette.
2. Assemblage séquentiel des pages ; filet de découpe gris fin autour de chaque
   photo (il reste dans l'espace de coupe, la photo n'est jamais rognée).
3. Écriture atomique (`.tmp` puis renommage) dans le dossier de sortie, puis
   `shell.openPath` côté Electron à la réception de `done`.

### Interface

Le panneau se déroule en un seul écran, sans étapes bloquantes :

1. **Dossier racine** : prérempli avec le dossier de destination des préférences.
2. **Calendrier mensuel** : jours avec photos marqués d'un point ; un clic sur un
   jour surligne toute la semaine ; flèches mois précédent/suivant ; « Cette semaine ».
3. **Photos de la semaine** : `VirtualGrid` + `Thumb` existants, cases à cocher,
   compteur « 9 sur 38 ».
4. **Réglages** (repliés) : taille du grand côté (6,5 cm), format de page, espace
   entre photos (5 mm minimum).
5. **Aperçu de page** à droite avec les photos à leur vraie proportion, mis à jour
   en direct (nombre de pages affiché), puis bouton **Créer le PDF**. Une ligne
   rappelle le dossier de sortie (« Enregistré dans : Téléchargements »). Rappel :
   imprimer à 100 % (« taille réelle »), sans « adapter à la page ».

**Préférences (⚙️)** : nouveau champ « Dossier des PDF de semainier » avec
sélecteur de dossier (réutilise `dialog:pickFolder`). Vide = Téléchargements,
résolu côté main par `app.getPath('downloads')` (nouvel IPC `app:downloadsDir`,
valable macOS, Windows et Linux).

i18n fr/en/da ; préférences persistées dans `prefs.ts` (`scrapOutputDir`,
`scrapLongSide`, `scrapPage`, `scrapGutter`).

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
- Typographie : pile système de l'application ; numéros d'ordre et étiquette de
  semaine en chasse fixe (`ui-monospace`) pour évoquer un tampon dateur.
- Pas de flou en direct ni d'ombres coûteuses (contraintes de performance existantes).

## Gestion des erreurs et cas limites

- Semaine sans photo : message clair et bouton désactivé.
- Photo illisible ou HEIC non décodable : ignorée avec avertissement dans le résumé,
  les autres pages sont générées.
- Plus de photos que de place sur une page : pagination automatique.
- Dossier racine invalide ou sans `AAAA/AAAA-MM` : message guidant vers le bon dossier.
- Dossier de sortie absent ou non inscriptible : repli sur Téléchargements avec
  avertissement ; si l'écriture échoue encore, erreur explicite et aucun fichier partiel.
- Semaine à cheval sur deux mois ou deux années : le scan couvre les deux dossiers.
- Annulation en cours : aucun fichier partiel laissé.

## Tests

- Go : `week_test.go` (début de semaine, chevauchement mois/année, 53ᵉ semaine),
  `scan_test.go` (arbre temporaire, fichiers non image ignorés, « Almost Duplicates »
  inclus, repli sur EXIF/mtime), `layout_test.go` (grand côté exact à 0,1 mm,
  portrait/paysage/carré, rangées mixtes, **espace ≥ 5 mm entre toutes les paires de
  photos**, même avec `gutter` demandé < 5 mm, aucun débordement des marges,
  10 portraits / 12 paysages par page A4 paysage, pagination), `render_test.go`
  (PDF valide : en-tête `%PDF`, nombre de pages, taille raisonnable, annulation,
  suffixe ` (2)` en cas de collision de nom).
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
