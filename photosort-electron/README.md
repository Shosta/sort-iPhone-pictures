# Photosort Electron

Version Electron (macOS, Windows, Linux) de Photosort. La logique métier reste
en Go : l'application lance le binaire `photosort-sidecar` (`../cmd/photosort-sidecar`)
et dialogue avec lui en JSON ligne par ligne via stdin/stdout.

## Prérequis

- Node.js 20+ et npm
- Go (version demandée par `../go.mod`) dans le `PATH`
- Aucun compilateur C : le sidecar est compilé avec `CGO_ENABLED=0`

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
{"id":"4","op":"cancel","target":"2"}
```

Événements (un par ligne sur stdout, avec l'`id` de la requête) : `plan`,
`progress`, `done`, `thumbnail`, `error`. Les règles de classement sont celles
décrites dans le [README racine](../README.md).

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

## Limites

- Copie uniquement, aucun original modifié (comportement du moteur Go).
- Les installeurs ne sont pas signés ni notarisés.
- Pas encore de tests automatisés ni de CI.
