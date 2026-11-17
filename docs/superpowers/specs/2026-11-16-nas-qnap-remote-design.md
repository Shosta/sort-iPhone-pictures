# Classement et tags sur un NAS QNAP : cadrage

Date : 2026-11-16 · Branche : `feat/nas-qnap-remote` · Statut : **cadrage, en attente de validation** (aucun code écrit)

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
| F6 | Classement (hypothèse) | **Lien physique (hardlink)** de l'entrée vers la bibliothèque : aucun espace disque en plus, l'entrée reste intacte. Repli sur une **copie** si le lien est impossible (volumes différents). « Déplacer » n'est pas proposé en v1 |
| F7 | Volume (inconnu) | Non communiqué ; la mesure de l'étape 0 le donnera |

## Approches étudiées

| | Principe | Avantages | Limites |
| --- | --- | --- | --- |
| A | Le Mac traite, NAS monté en SMB | Aucun code, marche déjà | Tout transite par le réseau : lent en local, inutilisable à distance |
| **B** | **Serveur Go en Docker sur le NAS** ; l'app Electron s'y connecte | Traitement à côté des données ; seuls miniatures et progression circulent ; local et distant | Code serveur à écrire ; CPU du NAS décisif pour l'IA |
| B' | Variante de B : binaire Go statique (sans CGO, sans IA) lancé par QTS | Pas de Docker nécessaire | Pas de tags IA sur le NAS ; sans objet ici, le TS-251D gère Docker |
| C | Tâche planifiée sur le NAS, sans UI | Entièrement automatique | Pas de prévisualisation ni de contrôle |

**Recommandation : B**, puis C comme mode de fonctionnement complémentaire.

Raisons : le protocole du sidecar (une requête JSON, des événements portant l'`id`
de la requête) se transpose tel quel sur WebSocket ; le moteur Go est réutilisé
sans changement de logique ; une image Docker multi-architecture se construit
dans la CI existante.

## Design (approche B)

### Composants

- **`cmd/photosort-server`** (nouveau) : même moteur que `photosort-sidecar`, exposé
  en HTTPS/WebSocket. Un seul service Go, dans une image Docker `linux/amd64`.
- **Transport** : l'app Electron reçoit un second « backend » à côté du sidecar
  local (`sidecar.ts`) : mêmes requêtes, mêmes événements, seul le transport
  change (processus local ou WebSocket).
- **Authentification** : jeton long généré au premier lancement, stocké côté
  Electron dans le trousseau du système (`safeStorage`). TLS obligatoire hors
  réseau privé chiffré ; en réseau local, certificat auto-signé épinglé.
  Le serveur écoute via une interface `net.Listener` interchangeable (réseau
  local, puis réseau privé Tailscale ou NetBird, voir « Accès à distance »).
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
3. `copy` : lien physique local au NAS (repli : copie), sans trafic réseau,
   reprenable. Un lien physique échoue entre deux montages Docker, même d'un
   même volume : le conteneur monte donc **un seul dossier parent** (par exemple
   `/photos`) qui contient l'entrée et la bibliothèque.
4. `tag` : tags `.xmp` écrits à côté des photos, comme aujourd'hui.
5. Semainier : le PDF est généré sur le NAS puis téléchargé dans le dossier
   choisi en préférences.

### Gestion des erreurs et sécurité

- Connexion perdue : les opérations longues continuent sur le serveur ; l'app
  se reconnecte et reprend le suivi par identifiant d'opération.
- Fichiers en cours de synchro : ignorés tant que leur taille ou leur date de
  modification change, ou que leur âge est inférieur à un seuil (60 s).
- Aucune suppression ni modification d'original. Écritures limitées à la
  bibliothèque et aux sidecars `.xmp` ; l'entrée n'est jamais écrite, ce que le
  serveur impose lui-même puisque le montage unique est en lecture-écriture.
- Un lien physique partage le contenu de l'original : modifier une photo de la
  bibliothèque sur place modifierait aussi l'entrée. Photosort n'écrit jamais
  dans les photos ; la copie complète reste disponible en option.
- Compte système du conteneur sans droits en dehors du dossier monté.

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
- Le classement (EXIF, doublons, liens physiques) et les tags de métadonnées
  restent légers : **activés par défaut sur le NAS**. Les tags IA
  (scènes, moment, personnes) sont **optionnels**, pensés pour tourner en
  tâche de fond. QuMagie fournit déjà sa propre reconnaissance.
- Parallélisme borné à 1 ou 2 workers ; EXIF et décodage HEIC partiels sur place,
  à 1/8, comme en local.
- Il faut vérifier qu'ONNX Runtime tourne sur ce Celeron (sans AVX) : test
  de l'étape 0.

### Coexistence avec QuMagie

- QuMagie ne doit indexer que la **bibliothèque classée**, pas l'entrée : avec
  des liens physiques, indexer les deux créerait des doublons dans ses albums.
- Les `.xmp` de Photosort sont écrits à côté des photos ; QuMagie les ignore,
  ses tags restent indépendants. Un affichage des tags Photosort dans l'app
  sert à la recherche et au semainier.
- La sauvegarde automatique de l'iPhone n'est jamais modifiée : aucun fichier
  de l'entrée n'est écrit, déplacé ni supprimé.

## Plan par étapes

0. **Mesure (peu de code)** : compiler `photosort` pour Linux amd64 (en
   conteneur `golang` ou via la CI), le lancer dans un conteneur Docker de test
   sur le NAS, sur un échantillon de quelques centaines de photos copiées
   ailleurs, en simulation (sans `--apply`) : durée du plan, du tag de
   métadonnées et d'un tag IA ; vérifier qu'ONNX Runtime démarre sur le
   Celeron. Cette étape fixe les bornes de parallélisme et dit si les tags IA
   sont réalistes sur le NAS.
1. Serveur Go en réseau local : jeton, TLS, chemins confinés au dossier monté,
   mode lien physique avec repli copie, écoute via une interface
   `net.Listener` ; image Docker `linux/amd64` et `docker-compose.yml` prêt
   pour Container Station.
2. Second backend dans l'app Electron : écran de connexion, navigateur de
   dossiers distants, choix entrée et bibliothèque.
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

## Hors périmètre (YAGNI)

Exposition directe sur internet, multi-utilisateurs, synchronisation avec les
albums QuMagie, déplacement ou suppression des originaux, architecture ARM,
reconnaissance de visages nominative.

## Questions ouvertes

1. Volume de photos approximatif (pour dimensionner les durées).
2. L'étape 0 peut-elle se faire sur le NAS (accès SSH ou Container Station) ?
3. Le lien physique convient-il, ou faut-il une copie complète (QuMagie et
   sauvegardes de la bibliothèque peuvent mieux supporter une copie réelle) ?
4. Accès à distance : valides-tu Tailscale intégré au serveur (recommandé),
   ou préfères-tu NetBird (open source) ?
