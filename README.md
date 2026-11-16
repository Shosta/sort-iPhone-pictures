# Photosort

Application locale de classement de photos en Go, avec interface Fyne et CLI.
Compatible macOS et Linux, avec Go 1.27 ou plus recent, sans serveur.

## Interface Graphique

Lancer depuis le terminal, sans debugger ni droits administrateur :

```sh
go run ./cmd/photosort-gui
```

Ou compiler un executable puis le lancer :

```sh
go build -o bin/photosort-gui ./cmd/photosort-gui
./bin/photosort-gui
```

- Deposer un ou plusieurs dossiers dans la fenetre, ou utiliser `Ajouter`.
- L'arborescence de gauche contient uniquement les photos classables et leurs
  dossiers parents. `Vider` retire les sources sans supprimer de fichiers.
- Modifier le chemin au-dessus du panneau droit, ou utiliser `Choisir`.
  Le chemin peut designer un dossier qui n'existe pas encore.
  La derniere destination analysee avec succes est memorisee localement.
- `Trier` lance les copies. Le panneau droit affiche uniquement les copies
  reussies de cette session, pas le contenu preexistant du dossier destination.
- `Annuler` arrete l'analyse ou la copie ; les copies deja terminees restent
  presentes. Les originaux sont toujours conserves.
- Les fichiers ignores et les echecs de miniatures figurent dans `Signalements`.

Les miniatures se chargent en arriere-plan ; les dossiers ont une icone bleue.
JPEG, PNG, TIFF, HEIC/HEIF et AVIF disposent de decodeurs. Les formats RAW
utilisent leur apercu JPEG embarque lorsqu'il existe. Une image endommagee,
sans apercu decodable, ou de plus de 100 megapixels conserve une icone de
fichier et genere un signalement, sans bloquer sa copie si ses EXIF sont valides.

Fyne ajoute des dependances graphiques natives : les Command Line Tools Apple
sur macOS ; un compilateur C, pkg-config et les bibliotheques de developpement
OpenGL/X11 sur Linux (voir les [prerequis Fyne](https://docs.fyne.io/started/)).
Le decodage HEIC/AVIF utilise une bibliotheque locale si disponible, sinon un
decodeur WASM embarque, sans televersement ni outil externe obligatoire.
Le depot externe de dossiers doit encore etre verifie manuellement sur Linux.

## Applications Distribuables

Les taches VS Code `Photosort: build macOS (.app)` et
`Photosort: build Linux (AppImage x86_64)` generent les applications dans `dist/`.
Le packaging est isole dans [packaging](packaging/README.md).

```sh
bash packaging/build.sh macos
bash packaging/build.sh linux
```

Le build macOS s'execute sur un Mac. Le build Linux utilise Docker avec un moteur
demarre, y compris depuis macOS ARM64. Voir les prerequis et limites de
distribution dans la documentation du packaging.

## Utilisation

Depuis la racine du projet, simuler un classement sans rien ecrire :

```sh
go run ./cmd/photosort --source "/chemin/photos" --destination "/chemin/classement"
```

Copier les photos apres verification de la simulation :

```sh
go run ./cmd/photosort --source "/chemin/photos" --destination "/chemin/classement" --apply
```

Plusieurs sources et une sortie JSON :

```sh
go run ./cmd/photosort --source "/chemin/photos" --source "/chemin/autres-photos" --destination "/chemin/classement" --json
```

`--json` peut aussi etre combine avec `--apply`. `--help` affiche les options.
Le code de sortie est 1 en cas d'erreur bloquante, 0 sinon. Les fichiers ignores
sont signales dans le bilan, sans rendre toute l'operation bloquante.
Les arguments de Go doivent preceder ceux de l'application.

Compiler un executable :

```sh
go build -o bin/photosort ./cmd/photosort
./bin/photosort --help
```

## Regles de classement

Exemple de destination :

```text
2026/2026-10/2026-10-20 - iPhone - 1.png
2026/2026-10/2026-10-20 - iPhone - 2.png
```

- Parcours recursif, avec ordre de traitement stable par chemin source.
- Date issue exclusivement de la date EXIF de prise de vue (`OriginalDate`).
  Aucune substitution par la date de modification du fichier.
- Modele d'appareil issu des EXIF, sinon `Unknown camera`.
- Caracteres dangereux du modele remplaces ; modele limite a 40 caracteres.
- Numerotation a partir de 1 par jour et modele, en evitant les noms deja
  presents dans la destination et ceux reserves dans le plan courant.
- Option `--by-location` (CLI) : si la photo contient un GPS, un sous-dossier de
  ville est ajoute dans le mois (`2026/2026-06/Rouen/...`). Sans GPS ou sans ville
  a moins de 50 km, la photo reste a la racine du mois. La ville est trouvee hors
  ligne dans une base GeoNames embarquee (`internal/geo`, noms locaux, licence
  CC BY 4.0, https://www.geonames.org) ; aucune coordonnee ne quitte la machine.
  Les quartiers et banlieues proches sont regroupes sous la ville la plus peuplee
  (rayon de 6 km autour de la localite la plus proche). Desactive par defaut.
  Option `--landmarks` : si la photo est a moins de ~120 m d'un monument ou
  ~250 m d'un musee, chateau, palais, stade, salle ou theatre (Europe, Canada,
  Etats-Unis, base GeoNames `internal/geo/places.tsv.gz`), le dossier porte le nom
  du lieu (`.../Musee du Louvre/...`) ; sinon c'est la ville (best effort).
  Regenerer les lieux : `go run ./internal/geo/gen poi allCountries.txt internal/geo/places.tsv.gz`.
  Regenerer la base : `go run ./internal/geo/gen cities1000.txt internal/geo/cities.tsv.gz`.
- Extension conservee, normalisee en minuscules ; aucune conversion d'image.
- Sources qui se recouvrent dedupliquees par chemin canonique. Les doublons
  de contenu dans des fichiers distincts ne sont pas detectes.
- Liens symboliques rencontres pendant le parcours ignores. Un dossier source
  explicitement fourni via un lien est resolu avant le parcours.
- Source et destination doivent etre disjointes, y compris via un lien symbolique.
- Images sans date, illisibles ou avec EXIF invalides ignorees avec une raison.
  Les fichiers d'extensions non reconnues ne sont pas examines.
- Copie uniquement : les originaux ne sont ni renommes, ni deplaces, ni effaces.
  Le contenu binaire et les EXIF sont conserves, mais pas les dates et permissions
  du systeme de fichiers. Les copies sont creees avec des permissions privees.
- Aucun fichier existant n'est ecrase, meme s'il apparait apres la planification.
  La copie s'arrete sur une erreur ; les copies precedentes restent presentes.
  Un fichier partiellement copie est supprime en cas d'erreur ou d'annulation
  geree. Un arret brutal du processus peut laisser un fichier incomplet.
- Une nouvelle execution recopie les photos avec de nouveaux numeros :
  il ne s'agit pas encore d'une synchronisation ou d'une deduplication.

Le decodeur [imagemeta](https://github.com/evanoberholster/imagemeta) est une
dependance Go ; la CLI ne requiert ni ExifTool, ni moteur graphique, ni compilateur C.
Extensions examinees : JPEG, PNG, TIFF, HEIC/HEIF, AVIF, DNG, CR2/CR3, NEF,
ARW, RW2 et CRW. Leur exploitation depend de la presence d'EXIF pris en charge
par le decodeur. Les PNG sans bloc EXIF, les videos, Live Photos et les fichiers
annexes XMP/AAE ne sont pas geres. Les formats reels de tes appareils restent
a tester ; les tests integres utilisent un petit TIFF EXIF genere en memoire.

## Architecture Et Tests

`sorter.Engine.Plan(ctx, sources, destination)` prepare un plan sans ecriture.
`sorter.Copy(ctx, plan)` execute les copies et renvoie leur nombre, meme si une
erreur survient ensuite. Les erreurs de lecture individuelles figurent dans
`plan.Skipped`. Les deux operations acceptent un contexte d'annulation.
L'interface `internal/desktop` et la CLI reutilisent le meme package.
`sorter.CopyWithProgress` ajoute une notification par copie reussie pour l'UI.
`Engine.ReadMetadata` permet de substituer le lecteur EXIF dans les tests.
Le plan est produit et consomme dans la meme execution ; la CLI ne recharge
pas de plans JSON. Les sources ne doivent pas changer entre analyse et copie.

```sh
go test ./...
go test -race ./...
go vet ./...
```

Dans VS Code, installer l'extension Go officielle si necessaire. Les profils
`Photosort: preview` et `Photosort: copy` demandent les chemins au lancement.
Placer un point d'arret dans `Engine.Plan`, `ReadMetadata` ou `Copy`, puis F5.
Le profil `copy` ecrit reellement les fichiers.

## GitHub Entreprise

Le depot Git est deja initialise localement. Aucun commit ni remote n'est cree.
Ne pas ajouter tes photos personnelles au depot ; utiliser des dossiers sources
et de destination exterieurs au projet.

Quand tu souhaiteras publier : creer un depot vide dans GitHub Entreprise,
sans README, licence ou gitignore genere cote serveur, puis executer :

```sh
git add .
git commit -m "Initial photo sorting engine"
git remote add origin <URL_DU_DEPOT_ENTREPRISE>
git push -u origin main
```

Le nom du module Go est actuellement local (`sort-iphone-pictures`) et pourra
etre adapte a l'adresse du depot quand elle sera connue.