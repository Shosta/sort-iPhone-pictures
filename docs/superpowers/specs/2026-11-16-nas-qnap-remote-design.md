# Classement et tags sur un NAS QNAP : cadrage

Date : 2026-11-16 · Branche : `feat/nas-qnap-remote` · Statut : **cadrage, en attente de validation** (aucun code écrit)

## Objectif

Les photos d'iPhone sont sauvegardées automatiquement sur un QNAP, dans un seul
gros dossier. Pouvoir les **classer** (date, lieu, doublons) et les **taguer**
directement sur le NAS, depuis l'app Electron, **en local comme à distance**.
La logique métier reste en Go. Les originaux ne sont jamais modifiés.

## Hypothèses à confirmer

Ces points n'ont pas encore été confirmés par l'utilisateur ; le plan en dépend.

| # | Sujet | Hypothèse | Si faux |
| --- | --- | --- | --- |
| H1 | Processeur du NAS | x86_64 ou ARM64 avec Container Station (Docker) | Sans Docker : binaire Go statique lancé par tâche planifiée QTS (voir option B') |
| H2 | Volume | Quelques dizaines de milliers de photos | Au-delà, l'indexation incrémentale devient prioritaire |
| H3 | Sauvegarde iPhone | Dossier « entrée » alimenté par l'app QNAP (Qfile ou QuMagie), à ne pas perturber | Adapter la détection des fichiers en cours de synchro |
| H4 | Classement | **Copie** vers une bibliothèque classée distincte de l'entrée | Option « déplacer après copie vérifiée » |
| H5 | Accès distant | Par VPN (QVPN, Tailscale ou WireGuard), jamais par un port ouvert sur internet | TLS + jeton obligatoires dans tous les cas |
| H6 | Déclenchement | Manuel depuis l'app d'abord, automatique ensuite | — |

## Approches étudiées

| | Principe | Avantages | Limites |
| --- | --- | --- | --- |
| A | Le Mac traite, NAS monté en SMB | Aucun code, marche déjà | Tout transite par le réseau : lent en local, inutilisable à distance |
| **B** | **Serveur Go en Docker sur le NAS** ; l'app Electron s'y connecte | Traitement à côté des données ; seuls miniatures et progression circulent ; local et distant | Code serveur à écrire ; CPU du NAS décisif pour l'IA |
| B' | Variante de B : binaire Go statique (sans CGO, sans IA) lancé par QTS | Pas de Docker nécessaire | Pas de tags IA sur le NAS |
| C | Tâche planifiée sur le NAS, sans UI | Entièrement automatique | Pas de prévisualisation ni de contrôle |

**Recommandation : B**, puis C comme mode de fonctionnement complémentaire.

Raisons : le protocole du sidecar (une requête JSON, des événements portant l'`id`
de la requête) se transpose tel quel sur WebSocket ; le moteur Go est réutilisé
sans changement de logique ; une image Docker multi-architecture se construit
dans la CI existante.

## Design (approche B)

### Composants

- **`cmd/photosort-server`** (nouveau) : même moteur que `photosort-sidecar`, exposé
  en HTTPS/WebSocket. Un seul service Go, une image Docker `linux/amd64` et
  `linux/arm64`.
- **Transport** : l'app Electron reçoit un second « backend » à côté du sidecar
  local (`sidecar.ts`) : mêmes requêtes, mêmes événements, seul le transport
  change (processus local ou WebSocket).
- **Authentification** : jeton long généré au premier lancement, stocké côté
  Electron dans le trousseau du système (`safeStorage`). TLS obligatoire hors
  réseau local ; certificat fourni par le VPN, le reverse proxy ou un certificat
  auto-signé épinglé.
- **Chemins** : le serveur n'accepte que des chemins sous les racines
  déclarées (entrée, bibliothèque) ; toute tentative de sortie (`..`, liens)
  est refusée.
- **Fichiers volumineux** : seules les miniatures (JPEG réduits) et le PDF du
  semainier sont rapatriés ; les originaux ne quittent pas le NAS.

### Flux de données

1. L'app se connecte (adresse, jeton), choisit un dossier d'entrée et une
   bibliothèque, tous deux sur le NAS.
2. `plan` : le serveur lit les EXIF sur place et diffuse la progression et les
   brouillons (déjà le comportement actuel).
3. `copy` : copie locale au NAS (pas de trafic réseau), reprenable.
4. `tag` : tags `.xmp` écrits à côté des photos, comme aujourd'hui.
5. Semainier : le PDF est généré sur le NAS puis téléchargé dans le dossier
   choisi en préférences.

### Gestion des erreurs et sécurité

- Connexion perdue : les opérations longues continuent sur le serveur ; l'app
  se reconnecte et reprend le suivi par identifiant d'opération.
- Fichiers en cours de synchro : ignorés tant que leur taille ou leur date de
  modification change, ou que leur âge est inférieur à un seuil (60 s).
- Aucune suppression ni modification d'original. Écritures limitées à la
  bibliothèque et aux sidecars `.xmp`.
- Compte système du conteneur sans droits en dehors des deux volumes montés ;
  entrée montée en **lecture seule**.

### Performance

- EXIF et décodage HEIC partiels sur place, à 1/8, comme en local.
- Parallélisme borné selon les CPU du NAS ; tags IA désactivables séparément
  (`tagMeta` seul reste très léger).
- Mesure préalable obligatoire (étape 0) avant de fixer ces bornes.

## Plan par étapes

0. **Mesure (sans code produit)** : `photosort --source <entrée montée en SMB>`
   sans `--apply`, pour chiffrer débit et durée, et valider H1 à H3.
1. Serveur Go, authentification, TLS, accès local ; image Docker.
2. Second backend dans l'app Electron, écran de connexion, navigateur de
   dossiers distants.
3. Semainier distant (PDF rapatrié) et tags sur le NAS.
4. Classement automatique des nouvelles photos de l'entrée (mode C), reprenable.
5. CI : image multi-architecture ; documentation VPN (QVPN, Tailscale).

## Hors périmètre (YAGNI)

Exposition directe sur internet, multi-utilisateurs, synchronisation avec les
albums QuMagie, suppression ou déplacement des originaux (sauf option ultérieure
explicite), reconnaissance de visages nominative.

## Questions ouvertes

1. Modèle du NAS (processeur, RAM) et présence de Container Station.
2. Volume de photos et application de sauvegarde iPhone.
3. Classement manuel depuis l'app, ou automatique dès l'arrivée des photos.
4. Mode d'accès distant déjà en place (VPN, myQNAPcloud, autre).
