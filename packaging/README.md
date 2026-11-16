# Packaging Photosort

Ce dossier isole la generation des applications de la logique de tri et de
l'interface. Il reutilise le module Go racine, sans dupliquer ses dependances.
Les commandes peuvent etre lancees depuis n'importe quel dossier avec le chemin
absolu du script. Les exemples ci-dessous partent de la racine du depot.
Les sorties dans `dist/` sont ignorees par Git.

## Icone

L'image source est `internal/appicon/photosort.png`, embarquee dans le binaire
pour l'icone Fyne et reutilisee par le packaging. Le generateur `packaging/icon`
conserve la transparence et produit un PNG de 512 pixels pour Linux et un ICNS
macOS avec les resolutions 16, 32, 64, 128, 256, 512 et 1024 pixels.
Remplacer l'image source puis relancer les builds pour actualiser les icones.

## Taches VS Code

Dans `Terminal > Run Build Task...`, choisir :

- `Photosort: build macOS (.app)`
- `Photosort: build Linux (AppImage x86_64)`

## macOS

Prerequis : macOS, Go a la version demandee par `go.mod` ou plus recente, et les
Command Line Tools Apple (`xcode-select --install` si necessaire).

```sh
bash packaging/build.sh macos
open dist/macos/Photosort.app
```

Le script compile avec CGO pour l'architecture du Mac hote (ARM64 ou AMD64),
assemble le bundle, genere son icone et applique une signature ad hoc locale.
Le resultat est `dist/macos/Photosort.app`, copiable dans Applications.
Il ne s'agit pas d'un binaire universel. La signature n'est pas une signature
Developer ID et le bundle n'est pas notarise : une distribution a d'autres
machines demandera une procedure Apple de signature et de notarisation.
Le numero de version se trouve dans `macos/Info.plist`.

## Linux AppImage

Prerequis : Docker avec BuildKit (fourni par Docker Desktop), moteur demarre,
conteneurs Linux et acces Internet pour les images, paquets et outils.
Docker Desktop doit pouvoir executer des conteneurs `linux/amd64` ; sur un Mac
ARM64, cette execution utilise l'emulation et peut etre sensiblement plus lente.

```sh
docker info
bash packaging/build.sh linux
```

Le script prend la version de Go dans `go.mod`. La compilation CGO et le
packaging s'executent dans un conteneur Debian Bookworm x86_64, avec les
dependances graphiques de Fyne. Aucun compilateur Linux n'est installe sur le Mac.
Le stage final exporte seulement `dist/linux/Photosort-x86_64.AppImage`.
Les couches Docker sont conservees pour accelerer les builds suivants.

`linuxdeploy` rassemble les bibliotheques redistribuables et cree l'AppImage.
L'outil provient de la release officielle `continuous` de linuxdeploy ; celle-ci
est evolutive, donc le build n'est pas strictement reproductible. L'argument
Docker `LINUXDEPLOY_URL` permet de choisir une autre release. Le packaging extrait
les outils AppImage sans FUSE et verifie que l'artefact contient son lanceur et
le binaire Photosort. Il n'execute pas de session graphique Linux.

Sur la machine Linux cible :

```sh
chmod +x Photosort-x86_64.AppImage
./Photosort-x86_64.AppImage
```

Si FUSE n'est pas disponible :

```sh
./Photosort-x86_64.AppImage --appimage-extract-and-run
```

La cible est Linux x86_64 avec glibc 2.36 ou plus recente (Debian 12 et systemes
compatibles), une session graphique X11 ou XWayland et les pilotes OpenGL du
systeme. L'AppImage n'embarque ni le noyau, ni les pilotes GPU, ni toutes les
bibliotheques de base. Ce n'est pas une garantie de compatibilite avec toute
distribution Linux ; un essai graphique sur les distributions cibles reste requis.
Le format ARM64 Linux n'est pas genere par cette tache.

## Verification

```sh
bash -n packaging/build.sh
go test ./...
```

Le build macOS verifie egalement la signature locale avec `codesign`.
Pour un test manuel, ouvrir l'application produite, choisir un dossier de photos
de test et une destination temporaire, puis verifier les apercus et la copie.