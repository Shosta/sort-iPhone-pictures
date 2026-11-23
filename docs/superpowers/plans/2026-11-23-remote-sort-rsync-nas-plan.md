# Plan : tri distant vers le NAS QNAP par rsync (sélecteur de dossier, dédoublonnage)

Date : 2026-11-23 · Statut : **à valider** (aucun code écrit pour cette étape)
Remplace, pour le transport, `2026-11-17-nas-local-connection-plan.md` (SMB) qui reste en repli.

## Contexte et décisions actées

| # | Décision |
| --- | --- |
| D1 | **rsync** est le transport (reprise `--partial`, vérification `--checksum` optionnelle, progression). Pas de réimplémentation. |
| D2 | **Les sources restent sur le PC.** Analyse, tri, tags, doublons, flou, captures : en local, comme aujourd'hui. Le NAS n'est qu'une **destination** proposée au moment de la copie. |
| D3 | Le tri local est écrit dans un arbre de liens symboliques temporaire, puis `rsync -aL --partial --ignore-existing` l'envoie. Aucune copie intermédiaire. |
| D4 | **Jamais de suppression** : ni `--delete`, ni `--remove-source-files`. Les originaux ne sont jamais touchés. |
| D5 | QNAP : **un seul utilisateur rsync**, non limitable à un dossier. La sécurité vient donc du programme, pas des droits NAS. |
| D6 | Modes : daemon rsync (module + mot de passe, jamais en clair dans les journaux ni les arguments) et rsync par SSH (SSH est refusé par l'utilisateur sur le NAS : le daemon est le mode principal). |
| D7 | macOS : préférer le rsync Homebrew (`/opt/homebrew/bin`, `/usr/local/bin`) ; le `/usr/bin/rsync` (openrsync) est limité, avec repli et avertissement. |
| D8 | Guide pour l'utilisateur sur le **site statique** (pas le wiki). |
| D9 | Abandonnés : import USB depuis l'iPhone, dossier surveillé. |

Déjà livré dans `main` : `internal/remote` (interface `Transport`, rsync, mise en scène des liens), CLI, sidecar, `RemotePanel` (test de connexion, simulation, progression, erreurs FR/EN/DA), guide `tri-distant`.

## Objectif de cette étape

1. Choisir la destination sur le NAS **sans taper de chemin** (découverte des dossiers).
2. Ne pas renvoyer ce que le NAS possède déjà (dédoublonnage vers la bibliothèque distante).
3. Envoyer aussi les tags créés en local.
4. Mettre le guide à jour.

## Phase 1 — Sélecteur de dossier distant

- [ ] `internal/remote` : `ListDir(ctx, target, path)` fondé sur `rsync --list-only` (sans récursion, `-d`) ; analyse de la sortie (nom, type, taille, date) ; premier niveau = liste des modules du daemon.
- [ ] Op sidecar `remoteList` ; erreurs réutilisant les messages existants (mot de passe, module inconnu, hôte injoignable).
- [ ] UI : navigateur niveau par niveau dans `RemotePanel` (ouverture à la demande, chemin courant, bouton « Choisir ce dossier »), indicateur de chargement, annulation.
- [ ] **Dossier favori** : dernier dossier mémorisé dans les préférences.
- [ ] **Création de dossier** : un dossier de départ à plusieurs niveaux demande `--mkpath` (rsync ≥ 3.2.3) ; sinon, message clair invitant à installer le rsync Homebrew.
- [ ] Tests : sortie rsync simulée (faux exécutable) ; i18n FR/EN/DA.

**Point à vérifier sur le NAS** : si le module QNAP pointe sur tout le partage, le sélecteur voit tout ce qui est dessous. Conseil : un module qui ne pointe que sur la bibliothèque.

## Phase 2 — Dédoublonnage vers la bibliothèque distante

- [ ] Liste de la bibliothèque : `rsync --list-only -r` sur le dossier choisi (nom, taille, date) ; mise en cache locale, avec une durée de validité.
- [ ] Une photo est **ignorée** si la bibliothèque contient déjà un fichier au même chemin de destination, ou du même nom, de la même taille et de la même date de prise de vue (empreinte plus fine plus bas).
- [ ] **Index d'empreintes** : fichier léger (SHA-256, taille, date) des photos envoyées, tenu en local et copié sur le NAS ; détecte un doublon rangé ailleurs ou renommé.
- [ ] Rapport avant envoi : à envoyer / déjà présentes / ignorées, visible aussi en simulation.
- [ ] Tests : bibliothèque simulée avec chemins identiques, renommés, déplacés.

## Phase 3 — Tags

- [ ] Vérifier d'abord **où les tags sont stockés** aujourd'hui (`internal/tagger`).
- [ ] Les envoyer avec les photos, par rsync : fichiers à côté des photos (XMP) ou fichier d'index, au choix après vérification.
- [ ] Ne jamais écraser des tags déjà présents sur le NAS sans le dire.

## Phase 4 — Garde-fous (D4, D5)

- [ ] Fonction unique qui construit les arguments rsync ; liste blanche d'options.
- [ ] **Test automatique** : aucune commande générée ne contient `--delete*` ni `--remove-source-files`.
- [ ] La simulation (`--dry-run`) est proposée avant le premier envoi vers une nouvelle destination.
- [ ] Écriture uniquement sous le dossier choisi comme bibliothèque.

## Phase 5 — Guide du site

- [ ] Mettre à jour la page `tri-distant` (FR/EN/DA) : choix du dossier distant, dédoublonnage, tags, garde-fous, module QNAP conseillé, limite d'un seul utilisateur.

## Phase 6 — Recette sur le QNAP `[intervention de l'utilisateur]`

- [ ] Test de connexion, parcours des dossiers, simulation, envoi réel d'un petit lot.
- [ ] Relance : rien n'est renvoyé. Reprise après coupure : l'envoi repart.
- [ ] Vérification dans QuMagie.
- [ ] Remonter les messages d'erreur incompris pour le guide.

## Risques

- Le rsync système de macOS est ancien : certaines options sont absentes (progression, `--mkpath`) → détection et repli.
- Liste d'une bibliothèque de 200 000 photos par `--list-only -r` : lente à distance → cache et rafraîchissement manuel.
- Comportement du serveur rsync de QTS non vérifié (limites du module, visibilité des dossiers) → tester à la phase 6.

## Hors périmètre

Lecture ou tri depuis le NAS, import USB depuis l'iPhone, dossier surveillé, serveur Docker sur le NAS, SFTP/WebDAV.
