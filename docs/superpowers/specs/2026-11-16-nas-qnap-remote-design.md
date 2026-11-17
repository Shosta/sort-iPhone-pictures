# Classement et tags sur un NAS QNAP : cadrage

Date : 2026-11-16 · Branche : `feat/nas-qnap-remote` · Statut : **cadrage ; plan de la connexion locale proposé, en attente d'approbation** (aucun code écrit)

Plan de la connexion locale : `docs/superpowers/plans/2026-11-17-nas-local-connection-plan.md`.
Il prime sur ce cadrage pour le transport, les droits et la copie (décisions D1 à D8).

## Objectif

Les photos d'iPhone sont sauvegardées automatiquement sur un QNAP, dans un seul
gros dossier. Pouvoir les **classer** (date, lieu, doublons) et les **taguer**
directement sur le NAS, depuis l'app Electron, **en local comme à distance**.
La logique métier reste en Go. Les originaux ne sont jamais modifiés.

## Faits et décisions

Confirmés par l'utilisateur (✔) ou retenus par défaut (hypothèse) :

| # | Sujet | Décision |
| --- | --- | --- |
| F1 | NAS ✔ | **QNAP TS-251D** : Celeron J4005/J4025, **2 cœurs / 2 threads**, x86_64, 2 ou 4 Go de RAM (8 Go maximum). Container Station (Docker) disponible |
| F2 | Stockage ✔ | Les photos sont dans **un dossier d'un disque du NAS** : même fonctionnement qu'en local, mais sur le disque du NAS. Entrée et bibliothèque sur le **même volume** |
| F3 | Déclenchement ✔ | **Manuel depuis l'app** d'abord ; une case « classer automatiquement » viendra ensuite |
| F4 | Accès distant ✔ | **Aucun en place** : la v1 fonctionne en réseau local. Accès à distance ensuite ; comparatif myQNAPcloud Link, Tailscale et NetBird plus bas, recommandation Tailscale intégré au serveur (à confirmer) |
| F5 | Tags existants ✔ | L'utilisateur utilise le **tagging de QNAP** (QuMagie). Les tags Photosort sont un complément, pas un remplacement |
| F6 | Classement (révisé) | **Copie** de l'entrée vers la bibliothèque. Le lien physique est abandonné : avec `fs.protected_hardlinks=1`, un utilisateur ne peut lier que des fichiers qu'il possède ou sur lesquels il a lecture et écriture, ce qui contredit F8. « Déplacer » n'est pas proposé en v1 ; l'espace disque des photos classées s'ajoute (contrôle avant copie) |
| F7 | Volume (inconnu) | Non communiqué ; la mesure de l'étape 0 le donnera |
| F8 | Droits ✔ | **Moindre privilège** : utilisateur QTS dédié, **lecture seule** sur le dossier iPhone en vrac, **écriture seulement** sur la destination, aucun autre droit. Un logiciel d'accès (réseau local, Tailscale, NetBird) peut avoir des failles : ce que le conteneur peut toucher ne dépend jamais du mode d'accès |

## Approches étudiées

| | Principe | Avantages | Limites |
| --- | --- | --- | --- |
| A | Le Mac traite, NAS monté en SMB | Aucun code, marche déjà | Tout transite par le réseau : lent en local, inutilisable à distance |
| **B** | **Serveur Go en Docker sur le NAS** ; l'app Electron s'y connecte | Traitement à côté des données ; seuls miniatures et progression circulent ; local et distant | Code serveur à écrire ; CPU du NAS décisif pour l'IA |
| B' | Variante de B : binaire Go statique (sans CGO, sans IA) lancé par QTS | Pas de Docker nécessaire | Pas de tags IA sur le NAS ; sans objet ici, le TS-251D gère Docker |
| C | Tâche planifiée sur le NAS, sans UI | Entièrement automatique | Pas de prévisualisation ni de contrôle |

**Recommandation : B**, puis C comme mode de fonctionnement complémentaire.

Raisons : le protocole du sidecar (une requête JSON, des événements portant l'`id`
de la requête) se transpose tel quel sur une connexion TLS ; le moteur Go est réutilisé
sans changement de logique ; une image Docker multi-architecture se construit
dans la CI existante.

## Design (approche B)

### Composants

- **`cmd/photosort-server`** (nouveau) : même moteur que `photosort-sidecar`, exposé
  en TLS avec le protocole JSON ligne par ligne du sidecar. Un seul service Go,
  dans une image Docker `linux/amd64`.
- **Transport** : l'app Electron reçoit un second « backend » dans son processus
  principal (`node:tls`) : mêmes requêtes, mêmes événements, seul le transport
  change (processus local ou connexion TLS). Pas de WebSocket.
- **Authentification** : jeton de 256 bits généré au premier lancement (le serveur
  n'en garde que l'empreinte), stocké côté Electron dans le trousseau du système
  (`safeStorage`). TLS obligatoire ; en réseau local, certificat auto-signé
  **épinglé par empreinte**, vérifiée avant l'envoi du jeton.
  Le serveur écoute via une interface `net.Listener` interchangeable (réseau
  local, puis réseau privé Tailscale ou NetBird, voir « Accès à distance »).
- **Chemins** : deux racines fixes dans le conteneur, `/inbox` (lecture seule) et
  `/library` (écriture) ; le serveur refuse tout chemin hors de ces racines
  (`..`, liens). Cette validation double les droits du système, elle ne les
  remplace pas.
- **Fichiers volumineux** : seules les miniatures (JPEG réduits) et le PDF du
  semainier sont rapatriés ; les originaux ne quittent pas le NAS.

### Flux de données

1. L'app se connecte (adresse, jeton) ; l'entrée et la bibliothèque sont les deux
   racines fixes du serveur.
2. `plan` : le serveur lit les EXIF sur place et diffuse la progression et les
   brouillons (déjà le comportement actuel).
3. `copy` : copie locale au NAS, sans trafic réseau. Entrée (`:ro`) et
   bibliothèque sont deux montages distincts. En cas de coupure, l'opération est
   annulée sans fichier partiel ; relancer le plan reprend le travail.
4. `tag` : tags `.xmp` écrits à côté des photos, comme aujourd'hui.
5. Semainier : le PDF est généré sur le NAS puis téléchargé dans le dossier
   choisi en préférences.

### Gestion des erreurs et sécurité

- Connexion perdue : les opérations en cours sont annulées (v1) ; l'app se
  reconnecte et relance le plan. Opérations détachées et reprise par identifiant :
  plan « accès distant ».
- Fichiers en cours de synchro : ignorés tant que leur taille ou leur date de
  modification change, ou que leur âge est inférieur à un seuil (60 s).
- Aucune suppression ni modification d'original. Écritures limitées à la
  bibliothèque (copies, sidecars `.xmp`) et au dossier d'état du conteneur.
- **L'entrée n'est jamais écrite**, par trois verrous indépendants : droits QTS de
  l'utilisateur dédié (F8), montage Docker `:ro`, et contrôle au démarrage du
  serveur, qui refuse de démarrer si l'entrée est inscriptible.
- Conteneur non-root, sans capacités, racine en lecture seule, sans `privileged`
  ni socket Docker, avec le seul accès aux deux dossiers et à son dossier d'état.

### Docker et architecture

Docker n'émule pas le processeur : le conteneur utilise le CPU et la RAM du NAS.
L'architecture compte donc encore, mais le TS-251D est en **x86_64** : l'image
`linux/amd64` convient. Ce qui pèse vraiment, c'est la puissance (2 cœurs) et la
mémoire (2 à 4 Go, partagés avec QTS et QuMagie). Le conteneur est limité en
mémoire et en CPU pour ne pas gêner QuMagie.

### Performance (estimations à mesurer)

- Sur le Mac M2, le tagging IA prend 0,06 à 0,1 s par photo. Le Celeron à
  2 cœurs sera nettement plus lent (ordre de grandeur : 10 à 30 fois, soit
  1 à 3 s par photo, à confirmer) : plusieurs heures pour des dizaines de
  milliers de photos.
- Le classement (EXIF, doublons, copies) et les tags de métadonnées
  restent légers : **activés par défaut sur le NAS**. Les tags IA
  (scènes, moment, personnes) sont **optionnels**, pensés pour tourner en
  tâche de fond. QuMagie fournit déjà sa propre reconnaissance.
- Parallélisme borné à 1 ou 2 workers ; EXIF et décodage HEIC partiels sur place,
  à 1/8, comme en local.
- Il faut vérifier qu'ONNX Runtime tourne sur ce Celeron (sans AVX) : test
  de l'étape 0.

### Coexistence avec QuMagie

- QuMagie ne doit indexer que la **bibliothèque classée**, pas l'entrée : les
  photos y existeraient deux fois (l'original et sa copie), d'où des doublons
  dans ses albums.
- Les `.xmp` de Photosort sont écrits à côté des photos ; QuMagie les ignore,
  ses tags restent indépendants. Un affichage des tags Photosort dans l'app
  sert à la recherche et au semainier.
- La sauvegarde automatique de l'iPhone n'est jamais modifiée : aucun fichier
  de l'entrée n'est écrit, déplacé ni supprimé.

## Plan par étapes

0. **Mesure (peu de code ; reprise à la phase 7 du plan, avec le serveur)** : compiler `photosort` pour Linux amd64 (en
   conteneur `golang` ou via la CI), le lancer dans un conteneur Docker de test
   sur le NAS, sur un échantillon de quelques centaines de photos copiées
   ailleurs, en simulation (sans `--apply`) : durée du plan, du tag de
   métadonnées et d'un tag IA ; vérifier qu'ONNX Runtime démarre sur le
   Celeron. Cette étape fixe les bornes de parallélisme et dit si les tags IA
   sont réalistes sur le NAS.
1. Serveur Go en réseau local : jeton, TLS épinglé, chemins confinés aux deux
   racines, copie, écoute via une interface `net.Listener` ; image Docker
   `linux/amd64` et `docker-compose.yml` durci pour Container Station.
2. Second backend dans l'app Electron : écran de connexion et mode NAS
   (racines fixes, sans navigateur de dossiers distants).

Le détail, les phases et leurs vérifications sont dans le plan de la connexion
locale (voir en tête de document).
3. Semainier distant (PDF rapatrié) et tags sur le NAS.
4. Case « classer automatiquement » : surveillance de l'entrée, fichiers
   stables seulement, reprenable.
5. Accès à distance : voir ci-dessous. Le serveur est écrit dès l'étape 1 autour
   d'une interface `net.Listener` ; l'étape 5 ajoute l'implémentation
   « overlay » retenue. Un **test de relais** myQNAPcloud Link (conteneur HTTP
   minimal joint par `qlink.to/<id>` depuis le partage de connexion du
   téléphone) reste possible en parallèle : seul ce test dit si le relais
   accepte un service autre que les applis QTS.

## Accès à distance (étape 5)

Objectif de l'utilisateur : **le moins de configuration possible sur le NAS** et
la plus grande simplicité d'usage. Aucun accès distant n'existe aujourd'hui.

### Comparaison

| Critère | myQNAPcloud Link | Tailscale | NetBird |
| --- | --- | --- | --- |
| Principe | Relais QNAP `qlink.to/<id>`, compte QNAP | Réseau privé WireGuard maillé, coordination par Tailscale Inc. | Idem ; client open source, plan de contrôle auto-hébergeable |
| Sur le NAS | Appli QNAP + compte QNAP | Appli QPKG officielle, **ou aucune** si intégré au serveur (`tsnet`) | Pas de QPKG ; conteneur Docker (`netbirdio/netbird`), **ou aucune** si intégré (`client/embed`) |
| Sur la box internet | Rien | Rien | Rien |
| Sur le Mac | Rien (si le relais convient) | Appli Tailscale + compte | Appli NetBird + compte (Google, GitHub... via OIDC) |
| Exposition | **URL publique** : tout internet atteint la page du service, d'où jeton et TLS obligatoires | Privée (visible des seuls appareils du compte) | Privée |
| Coût | Inclus avec le NAS | Gratuit en usage personnel (limites exactes non vérifiées) | Free : 5 utilisateurs, 100 machines (vérifié) |
| Licence | Fermée | Clients BSD-3 ; coordination propriétaire (Headscale, communautaire, comme alternative) | Client BSD-3 ; gestion, signal et relais en AGPLv3 |
| Intégration Go | — | **`tsnet`** : mature, serveur HTTPS avec certificat automatique | **`client/embed`** : `Dial` et `ListenTCP`, plus récent, dépendances plus lourdes (à mesurer) |
| TLS | Fourni par le relais | Certificat automatique (HTTPS à activer une fois dans la console Tailscale) | Aucun certificat automatique ; le tunnel WireGuard chiffre déjà, jeton conservé |
| Auto-hébergement du plan de contrôle | Non | Non (Headscale non officiel) | Oui, mais exige une VM publique, un domaine et les ports 443 et 3478 : contraire à la simplicité voulue, donc NetBird Cloud |
| Principal risque | Relais d'un service non-QTS **non vérifié** | Dépendance à un éditeur | Écosystème plus jeune ; aucune intégration QNAP officielle |

QVPN et le DDNS myQNAPcloud (`id.myqnapcloud.com`) sont écartés : ils demandent
d'ouvrir un port sur la box (inutilisable derrière un CGNAT, service exposé
directement sur internet).

### Constat déterminant : l'overlay s'intègre dans le serveur Go

Tailscale (`tsnet`) comme NetBird (`client/embed`) existent en **bibliothèques
Go** : le serveur `photosort-server` peut rejoindre le réseau privé **lui-même**,
sans appli QNAP, sans conteneur supplémentaire, sans configuration réseau du NAS.
La mise en place sur le NAS se réduit à **un `docker-compose.yml` et une clé
d'authentification** à saisir une fois. Avantages : aucun port exposé, un seul
conteneur, aucune dépendance au QPKG de l'éditeur.

Côté Mac, l'utilisateur installe l'appli du service choisi (v1 de l'accès
distant). Une évolution possible (hors v1) : intégrer la même bibliothèque dans
le sidecar Go du Mac, pour qu'il n'y ait **rien à installer** et qu'un code
d'appairage suffise.

### Recommandation

1. **v1 : réseau local seulement.** TLS avec certificat auto-signé épinglé, jeton.
2. **Accès à distance : Tailscale par `tsnet` dans le serveur.** Raisons :
   bibliothèque la plus mature, certificat HTTPS automatique, repli officiel par
   QPKG, appli Mac et mobile très répandues. La configuration NAS se limite à la
   clé d'authentification dans le compose.
3. **NetBird reste un échange facile** : le serveur n'a besoin que d'un
   `net.Listener`, le client que d'un `Dialer`. À privilégier si l'open source du
   plan de contrôle prime sur la simplicité ; il ne se justifie pas sinon.
4. **myQNAPcloud Link : option secondaire**, seulement si le test de relais
   réussit ; son URL publique est son principal défaut.

Décision à confirmer par l'utilisateur avant l'étape 5. Points à vérifier à ce
moment-là : limites du plan gratuit Tailscale, expiration des clés (désactiver
l'expiration du nœud NAS dans la console), taille de l'image Docker avec la
bibliothèque choisie.

### Sécurité de l'accès distant (remarques de l'utilisateur)

- **MFA** : l'URL publique de myQNAPcloud Link est un défaut, mais l'authentification
  multifacteur du compte QNAP renforce l'accès ; à peser contre l'absence
  d'exposition d'un réseau privé.
- **Failles** : Tailscale, NetBird (comme tout logiciel d'accès) peuvent en avoir.
  Invariant de conception : la sécurité du NAS **ne dépend pas** du mode d'accès.
  Un compte QTS dédié (lecture seule sur l'entrée, écriture seulement sur la
  bibliothèque, rien d'autre) et un conteneur durci (F8) limitent les dégâts si
  le serveur ou l'overlay est compromis. L'overlay n'ajoute aucun droit.

## Hors périmètre (YAGNI)

Exposition directe sur internet, multi-utilisateurs, synchronisation avec les
albums QuMagie, déplacement ou suppression des originaux, architecture ARM,
reconnaissance de visages nominative.

## Questions ouvertes

1. Volume de photos approximatif (pour dimensionner les durées).
2. L'étape 0 peut-elle se faire sur le NAS (accès SSH ou Container Station) ?
3. ~~Lien physique ou copie~~ : copie, imposée par la lecture seule de l'entrée (F6).
4. Accès à distance (second plan) : Tailscale intégré au serveur (recommandé),
   NetBird (open source), ou myQNAPcloud Link (URL publique, renforcée par le MFA
   du compte QNAP, si le test de relais réussit). Le choix n'affecte pas les
   droits du conteneur (F8).
5. Entrée et bibliothèque : deux dossiers partagés distincts, ou deux sous-dossiers
   d'un même partage ? La bibliothèque existe-t-elle déjà ? Espace libre ?
6. Container Station installé, accès SSH permis ?
