# Classement et tags sur un NAS QNAP : cadrage

Date : 2026-11-16 · Branche : `feat/nas-qnap-remote` · Statut : **cadrage ; plan de la connexion locale révisé (Mac + SMB), en attente d'approbation** (aucun code écrit)

Plan de la connexion locale : `docs/superpowers/plans/2026-11-17-nas-local-connection-plan.md`.
Plan du serveur Docker (différé) : `docs/superpowers/plans/2026-11-17-nas-docker-server-plan-deferred.md`.

> **Révision du 2026-11-17.** Container Station n'est pas installé, le SSH reste
> désactivé, l'entrée ne contient que quelques centaines de photos et la
> bibliothèque existante est massive. La v1 locale passe donc par l'approche **A'**
> (l'app sur le Mac, le partage monté en SMB avec un compte dédié) : rien à
> installer sur le NAS. L'approche **B** (serveur Docker) est différée ; elle
> reste pertinente pour la classification automatique et l'accès à distance. Les
> sections « Design (approche B) », « Docker et architecture » et « Performance »
> ci-dessous décrivent B et restent valables pour ce plan différé.

## Objectif

Les photos d'iPhone sont sauvegardées automatiquement sur un QNAP, dans un seul
gros dossier. Pouvoir les **classer** (date, lieu, doublons) et les **taguer**
directement sur le NAS, depuis l'app Electron, **en local comme à distance**.
La logique métier reste en Go. Les originaux ne sont jamais modifiés.

## Faits et décisions

Confirmés par l'utilisateur (✔) ou retenus par défaut (hypothèse) :

| # | Sujet | Décision |
| --- | --- | --- |
| F1 | NAS ✔ | **QNAP TS-251D** : Celeron J4005/J4025, **2 cœurs / 2 threads**, x86_64, 2 ou 4 Go de RAM (8 Go maximum). Container Station (Docker) disponible mais **non installé** ✔ ; **SSH désactivé** ✔ (surface d'attaque) |
| F2 | Stockage ✔ | Les photos sont sur un disque du NAS, dans **un seul partage à deux sous-dossiers** (entrée en vrac et bibliothèque classée) ✔. La bibliothèque **existe déjà et est massive** ✔ |
| F3 | Déclenchement ✔ | **Manuel depuis l'app** d'abord ; une case « classer automatiquement » viendra ensuite |
| F4 | Accès distant ✔ | **Aucun en place** : la v1 fonctionne en réseau local. Accès à distance ensuite ; comparatif myQNAPcloud Link, Tailscale et NetBird plus bas, recommandation Tailscale intégré au serveur (à confirmer) |
| F5 | Tags existants ✔ | L'utilisateur utilise le **tagging de QNAP** (QuMagie). Les tags Photosort sont un complément, pas un remplacement |
| F6 | Classement (révisé) | **Copie** de l'entrée vers la bibliothèque. Le lien physique est abandonné : avec `fs.protected_hardlinks=1`, un utilisateur ne peut lier que des fichiers qu'il possède ou sur lesquels il a lecture et écriture, ce qui contredit F8. « Déplacer » n'est pas proposé en v1 ; l'espace disque des photos classées s'ajoute (contrôle avant copie) |
| F7 | Volume ✔ | **Quelques centaines de photos** dans l'entrée ; bibliothèque massive. Le coût à surveiller est le parcours de la bibliothèque (dédoublonnage), pas la copie |
| F8 | Droits ✔ | **Moindre privilège** : utilisateur QTS dédié, **lecture seule** sur le dossier iPhone en vrac, **écriture seulement** sur la destination, aucun autre droit. Un logiciel d'accès (réseau local, Tailscale, NetBird) peut avoir des failles : ce que le compte dédié (et le conteneur, en voie B) peut toucher ne dépend jamais du mode d'accès |

## Approches étudiées

| | Principe | Avantages | Limites |
| --- | --- | --- | --- |
| A | Le Mac traite, NAS monté en SMB avec un compte dédié | Rien à installer sur le NAS, ni SSH ni Docker ; l'app lit déjà un dossier et écrit dans un autre ; tags IA rapides sur le Mac ; aucun secret dans l'app | Les données transitent par le réseau (quelques centaines de photos : sans importance) ; parcours de la bibliothèque massive à contenir ; inutilisable à distance sans VPN ; pas d'automatisation sans le Mac |
| **B** | **Serveur Go en Docker sur le NAS** ; l'app Electron s'y connecte | Traitement à côté des données ; seuls miniatures et progression circulent ; local et distant ; automatisable | Exige Container Station (démon root) ; code serveur à écrire ; CPU du NAS décisif pour l'IA |
| B' | Variante de B : binaire Go statique (sans CGO, sans IA) lancé par QTS | Pas de Docker nécessaire | Pas de tags IA sur le NAS ; sans objet ici, le TS-251D gère Docker |
| C | Tâche planifiée sur le NAS, sans UI | Entièrement automatique | Pas de prévisualisation ni de contrôle |

**Recommandation révisée : A' pour la v1 en réseau local** (A durci : compte dédié aux
droits minimaux, contrôle préalable de la lecture seule, dédoublonnage à portée,
copie atomique, noms système ignorés). **B reste la cible** pour la classification
automatique (case « classer automatiquement ») et l'accès à distance ; C en découle.

Raisons de la révision : B exige Container Station et un démon root que
l'utilisateur n'a pas, pour quelques centaines de photos ; le protocole du
sidecar (une requête JSON, des événements portant l'`id` de la requête) se
transpose néanmoins tel quel sur une connexion TLS le jour où B sera construit.

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

1. **Connexion locale, voie A'** (plan `2026-11-17-nas-local-connection-plan.md`) :
   compte QTS dédié et Advanced Folder Permissions (phase 0b), noms système
   ignorés, dédoublonnage à portée, copie atomique, contrôle préalable
   `nasCheck`, mode NAS dans les préférences, recette sur le NAS, documentation.
   Les tags (métadonnées et IA) et le semainier fonctionnent sans changement :
   la bibliothèque est lue en SMB, le PDF est écrit en local.
2. **Serveur Docker (voie B, différé)** (plan `…-nas-docker-server-plan-deferred.md`) :
   serveur Go, jeton, TLS épinglé, racines fixes, image `linux/amd64` et
   `docker-compose.yml` durci ; mesure sur le Celeron (durée du plan, tag de
   métadonnées, ONNX Runtime). À reprendre pour l'automatisation et l'accès à
   distance, si Container Station est installé.
3. Case « classer automatiquement » : surveillance de l'entrée, fichiers
   stables seulement, reprenable (nécessite un traitement sans le Mac, donc B).
4. Accès à distance : voir ci-dessous. En voie A', le NAS doit être joignable
   par SMB depuis un VPN ; **SMB n'est jamais exposé sur internet**. Avec B, le
   serveur est écrit autour d'une interface `net.Listener` et l'étape ajoute
   l'implémentation « overlay » retenue. Un **test de relais** myQNAPcloud Link
   (conteneur HTTP minimal joint par `qlink.to/<id>` depuis le partage de
   connexion du téléphone) reste possible en parallèle : seul ce test dit si le
   relais accepte un service autre que les applis QTS.

## Accès à distance (plan ultérieur)

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

Décision à confirmer par l'utilisateur avant d'aborder l'accès à distance. Points à vérifier à ce
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

1. ~~Volume de photos~~ : quelques centaines dans l'entrée (F7).
2. ~~Étape 0 sur le NAS~~ : sans objet en voie A' (ni SSH ni Container Station).
3. ~~Lien physique ou copie~~ : copie, imposée par la lecture seule de l'entrée (F6).
4. Accès à distance (second plan) : en voie A', un VPN donnant accès au NAS
   (SMB jamais exposé sur internet) ; en voie B, Tailscale intégré au serveur
   (recommandé), NetBird (open source), ou myQNAPcloud Link (URL publique,
   renforcée par le MFA du compte QNAP, si le test de relais réussit). Le choix
   n'affecte pas les droits du compte dédié (F8).
5. ~~Un ou deux partages~~ : un partage, deux sous-dossiers ; bibliothèque
   existante et massive (F2). Ouvert : suit-elle l'arborescence `AAAA/AAAA-MM`
   de Photosort (conditionne le dédoublonnage à portée) ?
6. ~~Container Station, SSH~~ : ni l'un ni l'autre (F1).
7. Advanced Folder Permissions (réglage global du NAS) : accepté pour séparer
   les droits des deux sous-dossiers ? Sinon, l'entrée n'est protégée que par l'app.
8. QuMagie indexe-t-il l'entrée, la bibliothèque, ou les deux ?
