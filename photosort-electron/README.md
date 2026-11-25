# Photosort Electron

Version Electron (macOS, Windows, Linux) de Photosort. La logique métier reste
en Go : l'application lance le binaire `photosort-sidecar` (`../cmd/photosort-sidecar`)
et dialogue avec lui en JSON ligne par ligne via stdin/stdout.

## Prérequis

- Node.js 20+ et npm
- Go (version demandée par `../go.mod`) dans le `PATH`
- Un compilateur C est requis sur tous les OS : le sidecar utilise CGO pour ONNX Runtime (tags IA). clang/gcc sous macOS et Linux, MinGW-w64 (`gcc`) sous Windows

## Commandes

```sh
npm install
npm run dev        # compile le sidecar Go puis lance l'application avec rechargement à chaud
npm run typecheck
npm test           # tests unitaires (validations IPC, lanceur Linux, empreintes)
npm run build      # compile le sidecar et l'interface dans out/
npm run dist       # installeurs dans release/ + release/SHA256SUMS (dmg, exe portable, AppImage/deb selon l'OS)
```

`dist:mac`, `dist:win` et `dist:linux` ciblent un OS précis. Le sidecar est
compilé pour l'OS courant ; pour une autre cible, définir `GOOS`/`GOARCH`
(`scripts/build-sidecar.mjs`). Les installeurs sont à générer sur chaque OS
(voir « Packaging via GitHub Actions »).

## GitHub Actions : CI, paquets et releases

Trois workflows dans `.github/workflows/` (actions épinglées par SHA, permissions
minimales par job, aucun secret exposé aux PR ; `dependabot.yml` propose les mises
à jour des épinglages).

| Workflow | Déclencheurs | Rôle |
| --- | --- | --- |
| `ci.yml` « CI » | PR, lundi 05:17 UTC, manuel, appelé par `package.yml` | contrôles de qualité, sécurité et performance (ci-dessous) |
| `package.yml` « Package Photosort » | push sur `main`, tag `v*`, manuel | CI puis installeurs, `SHA256SUMS`, release |
| `pages.yml` | push sur `main` touchant `website/` | publie le site |

**CI (`ci.yml`).** `go build/vet/test` et `govulncheck` ; `tsc`, `npm test` et
`npm audit` côté Electron ; `npm audit --omit=dev` et tests du site ; `actionlint`
sur les workflows. Le contrôle d'audit npm (`npm run audit:gate`) échoue pour toute
vulnérabilité de **production** et pour **Electron** (qui est une devDependency, donc
invisible à `--omit=dev`) à partir de « moderate » ; les avis sur l'outillage de build
(electron-builder, vite…) sont affichés en avertissement sans bloquer.
Garde-fou performance : les tests de structure qui bornent la mémoire (budget des
décodages HEIC/AVIF, plan envoyé au sidecar par morceaux) tournent dans un job
dédié, sans seuil de temps (seulement un délai global généreux) pour ne pas être
instables. Ce qui n'est **pas** mesuré : débit de tri, consommation mémoire réelle,
temps de démarrage (aucun benchmark n'existe dans le dépôt).

**Paquets (`package.yml`).** Rien n'est construit si la CI échoue.

| Cible | Runner | Fichiers produits |
| --- | --- | --- |
| macOS Apple Silicon | `macos-15` | `Photosort-<version>-mac-arm64.dmg` |
| macOS Intel | `macos-15-intel` | `Photosort-<version>-mac-x64.dmg` |
| Linux x64 | `ubuntu-22.04` | `Photosort-<version>-linux-x86_64.AppImage`, `Photosort-<version>-linux-amd64.deb` |
| Windows x64 | `windows-latest` | `Photosort-<version>-win-x64.exe` (portable : un seul .exe) |

- **Push sur `main`** : paquets non signés, en *Artifacts* du run (30 jours), avec la
  version de `package.json`. Les changements limités à `docs/`, aux `.md`, à `website/`
  ou à `pages.yml`/`dependabot.yml` ne déclenchent rien.
- **Tag `vX.Y.Z` ou `vX.Y.Z-rc.1` sur un commit de `main`** :
  `git tag v1.2.3 && git push origin v1.2.3`. Le tag doit être du SemVer strict (sans
  `+build`) et pointer sur un commit de `main`, sinon le workflow échoue avant de
  consommer des minutes de runner. **La version de l'application est celle du tag**
  (`electron-builder -c.extraMetadata.version=…`) : `package.json` n'a pas besoin
  d'être modifié avant de taguer. Un tag avec `-` (`-rc.1`, `-beta.2`) publie une
  *pré-version* GitHub. Les paquets sont signés si les secrets ci-dessous existent,
  puis `SHA256SUMS` est généré, vérifié (`--check`) et publié avec la release (notes
  générées). Relancer le workflow sur le tag (*Run workflow*, ref = le tag) remplace
  les fichiers d'une release existante.
- **Manuel** : *Actions* > *Package Photosort* > *Run workflow* (artefacts seulement,
  sauf depuis un tag).
- **Un job macOS reste en file puis est annulé** (« The job was not acquired by Runner of
  type hosted… », annotation « capacity constraints ») : c'est une saturation des runners
  macOS Apple Silicon de GitHub, pas une erreur du workflow. Relancer avec *Re-run failed
  jobs* ; la release d'un tag ne part que quand les quatre plateformes sont construites.

Les installeurs non signés : macOS demande une validation (clic droit > Ouvrir, ou
`xattr -cr /Applications/Photosort.app`) et Windows affiche SmartScreen
(« Informations complémentaires > Exécuter quand même »). L'AppImage doit être
rendue exécutable (`chmod +x`) ; le `.deb` s'installe avec `sudo apt install ./Photosort-<version>-linux-amd64.deb`.
Sur macOS Intel, ONNX Runtime n'existe pas : les tags IA y sont désactivés.

Derrière un proxy d'entreprise, si `npm install` échoue avec
`UNABLE_TO_GET_ISSUER_CERT_LOCALLY` : `NODE_OPTIONS=--use-system-ca npm install`.
Cette variable ne doit servir qu'à `npm install` : Electron la refuse au
lancement (`--use-system-ca is not allowed in NODE_OPTIONS`), donc ne pas
l'exporter pour `npm run dev`. Aucun droit administrateur n'est nécessaire
(tout s'installe dans le dossier du projet et `~/.local`).

## Signature, notarisation et empreintes

Les installeurs publiés doivent être signés, et les macOS notarisés. La
configuration est prête (`electron-builder.yml`, `build/entitlements.mac*.plist`),
mais elle ne s'active qu'avec des identifiants que le dépôt ne contient pas.
Sans eux, `electron-builder` produit un installeur non signé (`CSC_IDENTITY_AUTO_DISCOVERY=false`
en local ; le workflow ne le force à `false` que lorsque le certificat macOS est absent).
Attention : une variable `CSC_LINK` *définie mais vide* fait échouer `electron-builder` sur
macOS (`… not a file` : la chaîne vide est résolue comme le répertoire courant). Le
workflow n'exporte donc `CSC_LINK`, `CSC_KEY_PASSWORD`, `APPLE_*` et `WIN_CSC_*` (via
`GITHUB_ENV`, valeurs masquées) que si le secret existe et n'est pas vide ; en local,
laisser ces variables non définies (`unset CSC_LINK`) plutôt que vides.

**Secrets GitHub** (*Settings > Secrets and variables > Actions*). Ils ne sont lus que
sur un tag `v*` ; tous sont optionnels, et leur absence donne un paquet non signé sans
faire échouer le workflow :

| Secret | Contenu | Effet |
| --- | --- | --- |
| `CSC_LINK`, `CSC_KEY_PASSWORD` | certificat « Developer ID Application » `.p12` (base64) et son mot de passe | signature macOS (vérifiée par `codesign` dans le job) |
| `APPLE_ID`, `APPLE_APP_SPECIFIC_PASSWORD`, `APPLE_TEAM_ID` | compte Apple Developer, mot de passe d'app, identifiant d'équipe | notarisation macOS (vérifiée par `spctl` et `stapler`) |
| `WIN_CSC_LINK`, `WIN_CSC_KEY_PASSWORD` | certificat Authenticode `.pfx` (base64) et son mot de passe | signature Windows (vérifiée par `Get-AuthenticodeSignature`) |

La clé d'API App Store Connect (`APPLE_API_KEY`…) n'est pas branchée dans le workflow
(elle suppose un fichier `.p8` sur le runner) ; seule la voie Apple ID l'est.

**macOS (runtime durci + notarisation).** Fournir à `electron-builder`, en
variables d'environnement :

| Variable | Contenu |
| --- | --- |
| `CSC_LINK`, `CSC_KEY_PASSWORD` | certificat « Developer ID Application » exporté en `.p12` (chemin ou base64) et son mot de passe |
| `APPLE_ID`, `APPLE_APP_SPECIFIC_PASSWORD`, `APPLE_TEAM_ID` | compte Apple Developer, mot de passe d'app (appleid.apple.com) et identifiant d'équipe |

À la place des trois `APPLE_*`, on peut utiliser une clé App Store Connect
(`APPLE_API_KEY` = chemin du `.p8`, `APPLE_API_KEY_ID`, `APPLE_API_ISSUER`).
`electron-builder` signe tous les binaires imbriqués avec le runtime durci puis
soumet l'app à Apple (`notarize: true`) et agrafe le ticket. Sans identifiants
de notarisation, l'étape est ignorée avec un avertissement.

Les droits (`entitlements`) sont volontairement minimaux : `allow-jit` pour V8 ;
`disable-library-validation` seulement pour les processus enfants (dont le
sidecar Go), qui chargent `libonnxruntime.dylib` téléchargée au premier
lancement (non signée par nous ; son sha256 est vérifié avant extraction).
Vérification locale d'un build : `codesign --verify --deep --strict Photosort.app`
et `codesign -d --entitlements :- Photosort.app` ; après notarisation,
`spctl -a -vv Photosort.app` doit répondre `source=Notarized Developer ID`.

**Windows.** `CSC_LINK`/`CSC_KEY_PASSWORD` (ou `WIN_CSC_LINK`/`WIN_CSC_KEY_PASSWORD`
pour un certificat différent de celui de macOS) avec un certificat Authenticode,
ou Azure Trusted Signing (`win.azureSignOptions`). Les exécutables sont signés
automatiquement dès que la variable est présente.

**Empreintes SHA-256.** `npm run dist*` écrit `release/SHA256SUMS` (un
`<sha256>  <fichier>` par installeur : `.dmg`, `.exe`, `.AppImage`, `.deb`,
`.zip`). `npm run checksums` le régénère, `npm run checksums:verify` le contrôle.
Publier `SHA256SUMS` avec les installeurs ; un utilisateur vérifie son
téléchargement avec :

```sh
shasum -a 256 -c SHA256SUMS --ignore-missing   # macOS
sha256sum -c SHA256SUMS --ignore-missing       # Linux
Get-FileHash .\Photosort-<version>-win-x64.exe -Algorithm SHA256   # Windows (comparer à SHA256SUMS)
```

Le job `release` de `package.yml` exécute `node scripts/checksums.mjs dist` puis
`--check` avant de publier, et joint `SHA256SUMS` à la release.

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
{"id":"6","op":"weekPhotos","root":"/classement","from":"2026-11-16","to":"2026-11-22"}
{"id":"7","op":"scrapbook","files":["/classement/2026/2026-11/a.heic"],"page":"a4-landscape","longSide":65,"gutter":5,"outDir":"/Downloads","name":"Semaine 2026-S47"}
```

Événements (un par ligne sur stdout, avec l'`id` de la requête) : `draft`,
`status`, `plan`, `progress`, `done`, `thumbnail`, `tagged`, `weekDays`, `photos`,
`layout`, `error`. Les règles
de classement sont celles décrites dans le [README racine](../README.md).

- `plan` envoie des `draft` au fil de la lecture EXIF (destination provisoire de
  chaque photo, doublons trouvés) puis des `status` par étape ; le plan final
  arrive dans l'événement `plan`.
- `tag` accepte un dossier (`folder`) ou une liste de fichiers (`files`), ignore
  les photos qui ont déjà un `.xmp` et envoie un `tagged` par photo.
- Pendant un `plan` ou une `copy`, au plus 2 miniatures sont décodées en même
  temps pour laisser les cœurs et le disque au moteur.
- Semainier : `weekDays` (`root`, `from`, `to`) renvoie le nombre de photos par
  jour ; `weekPhotos` renvoie un seul événement `photos` (chemin et date, triés
  par date) ; `scrapbookLayout` (`files`, `page`, `longSide`, `gutter`) renvoie
  `layout` (positions en mm, utilisées telles quelles par l'aperçu) ; `scrapbook`
  envoie des `progress` puis `done` (`path`, `pages`, `photos`, `failed`,
  `fallback`). Les dates sont en `AAAA-MM-JJ`, bornes incluses, plage limitée à
  366 jours.

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

Les modèles (~46 Mo) et ONNX Runtime (~45 Mo) ne sont pas dans Git : l'application les télécharge au premier lancement (sha256 vérifié, progression dans l'onglet Tags) dans le dossier de données de l'application (`userData/ai`). Les paquets (dmg, AppImage, portable Windows) ne les embarquent donc pas. `npm run ai-assets` reste disponible pour les pré-télécharger dans `ai-assets/` en développement : ce dossier est utilisé en priorité s'il est complet. ONNX Runtime 1.31 n'existe pas pour macOS Intel : l'étiquetage y est désactivé avec un avertissement.

Le sidecar utilise CGO sur tous les OS (MinGW-w64 sous Windows) : il faut le compiler sur chaque OS avec `npm run sidecar` (c'est ce que fait le workflow « Package Photosort », voir plus haut). CLI : `--tag-scenes --tag-moment --tag-people --tag-meta [--ai-dir …]`.

## Semainier : planche de photos à découper (PDF)

Pour un semainier papier : le bouton 📅 en haut à droite (`Cmd/Ctrl+P`, menu *Semainier*) ouvre un panneau qui part du **dossier classé racine** (celui qui contient `2026/`, prérempli avec le dossier de sortie des préférences).

1. On choisit une semaine (**lundi → dimanche**) dans le calendrier ; les jours qui ont des photos affichent leur nombre. La semaine courante est présélectionnée.
2. Les photos de la semaine apparaissent (les « Almost Duplicates » comprises). **Aucune n'est cochée au départ** ; on coche celles à garder (ou « Tout cocher »).
3. L'aperçu montre les pages en direct, avec le nombre de pages ; « Réglages de la page » permet de changer le papier (A4 paysage par défaut, A4 portrait, Letter) ; la taille du grand côté (6,5 cm) et l'espace entre photos (5 mm minimum).
4. « Créer le PDF » écrit la planche dans le **dossier des PDF** (préférences, *Téléchargements* par défaut) sous le nom `Semaine 2026-S47 (16–22 novembre).pdf` (suffixe ` (2)` si le fichier existe) puis l'ouvre.

Chaque photo garde sa forme, sans recadrage : son grand côté fait 6,5 cm (portrait : 6,5 cm de haut ; paysage : 6,5 cm de large), avec au moins 5 mm entre deux photos pour découper. Sur A4 paysage, il tient 10 portraits ou 12 paysages 3:4 par page. Les images sont réduites à 300 dpi, d'où un PDF léger. **À l'impression, choisir « Taille réelle » (100 %)**, pas « Ajuster à la page ». Les originaux ne sont jamais modifiés ; un dossier de sortie inutilisable est remplacé par *Téléchargements* avec un avertissement. Le moteur (`internal/scrapbook`, PDF avec `go-pdf/fpdf`) est en Go pur.

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
- Les installeurs ne sont signés et notarisés que si les secrets de la section
  « Signature, notarisation et empreintes » sont configurés sur GitHub.
- Tests automatisés : `npm test` (validations IPC, lanceur Linux, empreintes, audit npm,
  plan par morceaux) ; la CI exécute en plus les tests Go, `govulncheck` et les gardes
  de performance (voir « GitHub Actions »).
