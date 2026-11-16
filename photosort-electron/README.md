# Photosort Electron

Version Electron (macOS, Windows, Linux) de Photosort. La logique métier reste
en Go : l'application lance le binaire `photosort-sidecar` (`../cmd/photosort-sidecar`)
et dialogue avec lui en JSON ligne par ligne via stdin/stdout.

## Prérequis

- Node.js 20+ et npm
- Go (version demandée par `../go.mod`) dans le `PATH`
- Un compilateur C (clang/gcc) est requis sauf sous Windows : le sidecar utilise CGO pour ONNX Runtime (tags IA)

## Commandes

```sh
npm install
npm run dev        # compile le sidecar Go puis lance l'application avec rechargement à chaud
npm run typecheck
npm run build      # compile le sidecar et l'interface dans out/
npm run dist       # installeurs dans release/ (dmg/zip, nsis, AppImage/deb selon l'OS)
```

`dist:mac`, `dist:win` et `dist:linux` ciblent un OS précis. Le sidecar est
compilé pour l'OS courant ; pour une autre cible, définir `GOOS`/`GOARCH`
(`scripts/build-sidecar.mjs`). Les installeurs sont à générer sur chaque OS
(ou via une CI en matrice).

Derrière un proxy d'entreprise, si `npm install` échoue avec
`UNABLE_TO_GET_ISSUER_CERT_LOCALLY` : `NODE_OPTIONS=--use-system-ca npm install`.
Cette variable ne doit servir qu'à `npm install` : Electron la refuse au
lancement (`--use-system-ca is not allowed in NODE_OPTIONS`), donc ne pas
l'exporter pour `npm run dev`. Aucun droit administrateur n'est nécessaire
(tout s'installe dans le dossier du projet et `~/.local`).

## Architecture

```text
src/main/       processus principal : fenêtre, dialogues, gestion du sidecar
src/preload/    pont sécurisé (contextIsolation) exposé en window.photosort
src/renderer/   interface React + Framer Motion
resources/bin/  binaire Go compilé (ignoré par Git, embarqué par electron-builder)
```

### Protocole du sidecar

Requêtes (une par ligne sur stdin) :

```json
{"id":"1","op":"plan","sources":["/photos"],"destination":"/classement"}
{"id":"2","op":"copy","planId":"1"}
{"id":"3","op":"thumbnail","path":"/photos/a.heic","size":160}
{"id":"4","op":"tag","folder":"/classement","tagScenes":true,"tagMoment":true,"tagPeople":true}
{"id":"5","op":"cancel","target":"2"}
```

Événements (un par ligne sur stdout, avec l'`id` de la requête) : `draft`,
`status`, `plan`, `progress`, `done`, `thumbnail`, `tagged`, `error`. Les règles
de classement sont celles décrites dans le [README racine](../README.md).

- `plan` envoie des `draft` au fil de la lecture EXIF (destination provisoire de
  chaque photo, doublons trouvés) puis des `status` par étape ; le plan final
  arrive dans l'événement `plan`.
- `tag` accepte un dossier (`folder`) ou une liste de fichiers (`files`), ignore
  les photos qui ont déjà un `.xmp` et envoie un `tagged` par photo.
- Pendant un `plan` ou une `copy`, au plus 2 miniatures sont décodées en même
  temps pour laisser les cœurs et le disque au moteur.

## Classement par lieu

L'interrupteur « Classer par lieu » (option `byLocation` de la requête `plan`)
ajoute un dossier de ville dans le mois : `2026/2026-06/Rouen/...`. La ville est
déduite des coordonnées GPS avec une base GeoNames embarquée, hors ligne
(CC BY 4.0, <https://www.geonames.org>). Sans GPS, la photo reste à la racine du mois.

L'interrupteur « Nommer les monuments » (`landmarks`, activé par défaut) remplace
la ville par le nom d'un musée, château, palais, stade ou salle situé à proximité
(~250 m, ~120 m pour un monument), p. ex. `2026/2026-06/Musée du Louvre/...`.
Base hors ligne pour l'Europe, le Canada et les États-Unis ; en dehors ou sans
correspondance, on garde la ville.

## Préférences et langues

Le bouton ⚙️ en haut à droite ouvre les préférences : langue (🇫🇷 Français,
🇬🇧 English, 🇩🇰 Dansk), thème de couleur (Aurore, Monochrome, Analogue,
Complémentaire, Triadique, Stonewash Côtier, Drame Laiton Minuit, Retraite en Lin Chaud ; palettes dans `src/renderer/themes.ts`), dossier de sortie et options de géolocalisation. Elles sont
enregistrées automatiquement (stockage local de l'application) et restaurées au
lancement suivant. La langue initiale suit celle du système (anglais par défaut).
Les textes sont dans `src/renderer/i18n.tsx`.

## Tags automatiques (IA locale)

L'analyse est **séparée du tri**, car elle reste plus lente que le tri (~0,06-0,1 s par photo sur un M2 au repos) : le tri reste rapide et on tague quand on veut. Elle s'ouvre avec le bouton 🏷️ en haut à droite (`Cmd/Ctrl+T`, menu *Tags*) ou le lien « Taguer ces photos » affiché après un tri, qui cible les photos copiées. Sinon, on choisit un dossier. Les photos qui ont déjà un `.xmp` sont ignorées.

Le panneau montre la progression, le temps restant, les compteurs par tag et les derniers résultats. On peut le fermer : l'analyse continue et une pastille sur 🏷️ indique l'avancement. Tout se fait **hors ligne** : les mots-clés sont écrits dans un fichier `<photo>.xmp` à côté de la copie (`dc:subject`, lu par Lightroom, digiKam, etc.). Les originaux ne sont jamais modifiés.

| Famille | Tags | Méthode |
| --- | --- | --- |
| Lieux | plage, montagne, ville, campagne, château, église | Places365 (ResNet18, ONNX) |
| Lieux (suite) | forêt, lac, rivière, cascade, neige, désert, jardin, parc ; intérieur, cuisine, chambre, restaurant, marché, supermarché ; piscine, camping, stade, scène, parc d'attractions, aire de jeux, aquarium, musée ; gare, aéroport, pont | mêmes probabilités Places365, sans calcul de plus : plusieurs catégories sont regroupées par tag (seuil 0,30) ; « intérieur » vient de la liste intérieur/extérieur officielle (seuil 0,60) |
| Moment | Lever de soleil, Coucher de soleil, Nuit, Soleil, Nuage | calcul solaire (date, GPS, fuseau EXIF) ; ciel pour Soleil et Nuage |
| Personnes | People, Portrait, Groupe | détection de visages YuNet (pas de reconnaissance) ; Groupe = au moins 4 visages visibles |
| Infos de la photo | Printemps, Été, Automne, Hiver (inversées dans l'hémisphère sud d'après le GPS), année (« 2025 »), Week-end, Jour de l'an, Selfie, Capture d'écran, Panorama, Zoom, Macro, Faible luminosité | EXIF et nom de fichier, **sans modèle** ni décodage : instantané, aucun fichier d'IA nécessaire |

Règles « Infos de la photo » : **Selfie** = objectif « front » (iPhone), retiré si l'analyse des personnes ne voit aucun visage ; **Capture d'écran** = commentaire EXIF « Screenshot » (iOS) ou nom de fichier commençant par `Screenshot` / `Capture d…` ; **Panorama** = côté long ≥ 2,5 × côté court ; **Zoom** = focale équivalente 35 mm ≥ 70 mm ; **Macro** = distance au sujet ≤ 15 cm ou objectif « macro » (l'iPhone n'écrit pas la distance : rarement détecté) ; **Faible luminosité** = valeur de luminosité EXIF ≤ 0 (ou ISO ≥ 1600 sans valeur). Un tag « zoo » n'existe pas dans Places365 et n'est pas proposé.| Moment | Lever de soleil, Coucher de soleil, Nuit, Soleil, Nuage | calcul solaire (date, GPS, fuseau EXIF) ; ciel pour Soleil et Nuage |
| Personnes | People, Portrait | détection de visages YuNet (pas de reconnaissance) |

Les modèles n'ont pas besoin de pleine résolution : les photos d'au moins 3584 px sont décodées à 1/8 (DC seul, ~504 px pour un iPhone), soit ~2 fois plus vite qu'un décodage complet pour des tags identiques (écart de 3 « Portrait » limites sur 173 tags, contre 2 entre 640 et 1024 px).

La pluie n'est pas détectée de façon fiable et n'est pas proposée. Sans GPS, les tags de moment solaires sont omis.

Les modèles (~46 Mo) et ONNX Runtime (~45 Mo) ne sont pas dans Git : `npm run ai-assets` les télécharge (sha256 vérifié) dans `ai-assets/` pour l'OS courant (`GOOS`/`GOARCH` pour un autre), et `electron-builder` les copie dans `resources/ai`. ONNX Runtime 1.31 n'existe pas pour macOS Intel : l'étiquetage y est désactivé avec un avertissement.

Le sidecar utilise CGO (sauf Windows) : il faut le compiler sur chaque OS (matrice CI macOS, Linux, Windows) avec `npm run sidecar`. CLI : `--tag-scenes --tag-moment --tag-people --tag-meta [--ai-dir …]`.

## Performances et fluidité

- Analyse en flux : la colonne « Classées » se remplit dès qu'une destination est
  connue, sans attendre la fin du plan.
- Empreintes de doublons calculées en une seule lecture par fichier, décodage
  JPEG à 1/8 (DC seul) pour le hash perceptuel, et quasi-doublons recherchés
  uniquement entre photos d'un même dossier prises à moins de 10 min d'écart.
- Grilles virtualisées (seules les miniatures visibles sont montées) et cache LRU
  des miniatures ; les requêtes hors écran sont annulées.
- Copie par 4 workers en parallèle ; les photos qui s'envolent vers « Classées »
  réutilisent les miniatures déjà chargées, sans décodage supplémentaire.
- Fond animé sans filtre de flou en direct, animations limitées à `transform` et
  `opacity` : l'application consomme quelques % de CPU au repos.
- Sur macOS, le mode économie d'énergie et les antivirus qui analysent chaque
  lecture ralentissent nettement l'analyse.

## Limites

- Copie uniquement, aucun original modifié (comportement du moteur Go).
- Les installeurs ne sont pas signés ni notarisés.
- Pas encore de tests automatisés ni de CI.
