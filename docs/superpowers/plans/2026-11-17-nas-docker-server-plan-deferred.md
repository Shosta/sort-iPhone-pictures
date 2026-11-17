# Plan : classement et tags sur le NAS, connexion en réseau local

Date : 2026-11-17 · Branche : `feat/nas-qnap-remote` · Statut : **différé** (aucun code écrit)
Spec parente : `docs/superpowers/specs/2026-11-16-nas-qnap-remote-design.md`

> **Différé.** Ce plan (serveur Go dans Docker) exige Container Station, que
> l'utilisateur n'a pas installé, et un accès SSH qu'il refuse d'activer. Avec
> quelques centaines de photos à traiter, il n'est pas nécessaire pour la
> connexion locale : voir `2026-11-17-nas-local-connection-plan.md` (Mac + SMB,
> rien à installer sur le NAS). Ce plan reste la base pour la **classification
> automatique** (traitement sans le Mac) et pour l'accès à distance par overlay.
> Les droits à imposer au conteneur (D1, D3, D4, durcissement) restent valables.

## Objectif

Depuis l'app Electron du Mac, sur le même réseau local, se connecter à un
`photosort-server` qui tourne dans Docker sur le QNAP TS-251D : classer
l'entrée en vrac, taguer la bibliothèque classée et produire le PDF du
semainier. **Le conteneur ne peut rien toucher d'autre** : même compromis, il ne
modifie ni le dossier iPhone ni le reste du NAS. L'accès à distance viendra
dans un second plan, sans changer ce que le conteneur a le droit de faire.

## Contraintes

- Logique métier en Go ; moteur existant réutilisé (`sorter`, `internal/tagger`, `internal/scrapbook`).
- **Entrée jamais écrite** : trois verrous indépendants (droits QTS de l'utilisateur dédié, montage Docker `:ro`, garde au démarrage du serveur).
- Utilisateur QTS dédié `photosort` : lecture seule sur l'entrée, lecture-écriture sur la bibliothèque, **aucun droit ailleurs**.
- Conteneur durci : non-root, `cap_drop: ALL`, `no-new-privileges`, racine en lecture seule, ni `privileged`, ni réseau hôte, ni `docker.sock`.
- Réseau local seulement ; le serveur écoute via une interface `net.Listener` (remplaçable plus tard par un overlay).
- TS-251D : 2 cœurs, 2 à 4 Go partagés avec QTS et QuMagie → parallélisme et mémoire bornés.
- Branche `feat/nas-qnap-remote`, aucun merge dans `main` sans demande, trailer `Co-authored-by` sur chaque commit.
- Aucune nouvelle dépendance côté Electron si possible (`node:tls`, `safeStorage`).

## Décisions (retenues par défaut, à confirmer)

| # | Décision | Raison |
| --- | --- | --- |
| D1 | **Copie, pas de lien physique.** Le lien physique est abandonné | Avec `fs.protected_hardlinks=1`, un utilisateur ne peut lier que des fichiers qu'il possède ou sur lesquels il a lecture **et écriture** ([doc noyau](https://docs.kernel.org/admin-guide/sysctl/fs.html)) : incompatible avec une entrée en lecture seule. Contrepartie : l'espace disque des photos classées s'ajoute (contrôle d'espace avant copie) |
| D2 | Transport : **TLS + JSON ligne par ligne**, le protocole du sidecar tel quel | `serve(ctx, io.Reader)` et `send` existent déjà ; Electron le parle avec `node:tls`. Pas de WebSocket : pas de dépendance, et le `WebSocket` global de Node 20 (Electron 33.4.11 embarque Node 20.18.3) n'est pas assuré |
| D3 | **Deux racines fixes** : `/inbox` (lecture seule) et `/library` (écriture) | Surface d'attaque minimale ; pas de navigateur de dossiers distants en v1 |
| D4 | Authentification : jeton de 256 bits (le serveur n'en garde que l'empreinte SHA-256) + certificat auto-signé **épinglé par empreinte**, vérifiée **avant** d'envoyer le jeton | Le jeton ne circule jamais vers un serveur non reconnu |
| D5 | Déconnexion = **annulation** des opérations en cours ; reprise en relançant le plan (les doublons déjà copiés sont ignorés, à tester) | Même sémantique que le sidecar ; opérations détachées et reprise par identifiant reportées au plan « accès distant » |
| D6 | PDF : généré dans le dossier de travail du serveur puis rapatrié par une opération `fetch` (blocs base64) ; le renderer ne change pas | Le PDF arrive dans le dossier choisi en préférences (Téléchargements par défaut) |
| D7 | Image livrée en **archive `docker save`** (aucun registre requis) | Le dépôt n'a pas de remote ; import dans Container Station |
| D8 | Tags de métadonnées activés ; **tags IA désactivés tant que l'étape NAS n'a pas mesuré** | Celeron probablement 10 à 30 fois plus lent ; AVX non garanti |

## Phase 0 — Documentation (inventaire des API)

Faite en ligne : API stables de la bibliothèque standard et code du dépôt, pas de sous-agents.

**API autorisées**
- Go (1.27.3) : `crypto/tls` (`MinVersion: tls.VersionTLS13`, `tls.NewListener`), `crypto/x509.CreateCertificate`, `crypto/ecdsa` + `elliptic.P256`, `crypto/sha256` (empreinte de `cert.Raw`), `crypto/subtle.ConstantTimeCompare`, `crypto/rand`, `net.Listener`, `syscall.Access` et `syscall.Statfs` (build tag `unix`), `os.OpenRoot` (déjà utilisé : `sorter/sorter.go:415`).
- Electron 33.4.11 / Node 20.18.3 (vérifié dans le binaire) : `tls.connect`, `socket.getPeerCertificate().fingerprint256`, `readline` (déjà utilisé : `main/index.ts:4`), `safeStorage.isEncryptionAvailable/encryptString/decryptString`.
- Docker 29.8.2 en local ; clés compose : `user`, `read_only`, `tmpfs`, `cap_drop`, `security_opt: no-new-privileges:true`, `mem_limit`, `cpus`, `pids_limit`, volumes `:ro`.
- ONNX Runtime : `photosort-electron/scripts/fetch-ai-assets.mjs` sait déjà récupérer `linux-amd64` (sha256 épinglé) : à réutiliser dans l'image.

**À ne pas faire**
- Ne pas exposer `aiDir`, `outDir`, `fallbackDir` aux clients du réseau : le serveur impose les siens.
- Ne pas présenter `os.Root` comme un confinement du moteur : il n'est utilisé que pour la copie ; la vraie barrière est le système (utilisateur, montage), la validation des chemins n'est que la seconde.
- Ne pas utiliser `rejectUnauthorized:false` sans vérifier l'empreinte avant toute écriture ; ne jamais stocker le jeton en clair.
- Ne pas lancer le conteneur en root, ni monter le socket Docker.

**Non vérifié (à lever à la phase 7)** : application des droits QTS aux montages Docker ; compose dans Container Station ; UID/GID de l'utilisateur QTS ; AVX et ONNX Runtime sur le Celeron ; lisibilité des copies par QuMagie.

## Phase 1 — Refactor sans changement de comportement

**Sortie** : le serveur du sidecar devient un paquet réutilisable.
**Vérification** : `go test ./cmd/photosort-sidecar/... ./internal/rpc/...` : tests existants inchangés et verts ; le sidecar se lance et répond comme avant.

- [ ] Déplacer `server`, `thumbCache` et les opérations (`cmd/photosort-sidecar/{main,cache,scrapbook}.go`) vers `internal/rpc` (`NewServer`, `Serve(ctx, in, out)`), avec une interface `Policy` appelée avant chaque requête (identité par défaut : tout est permis, comme le sidecar)
- [ ] Rendre configurables (options) le dossier de sortie du PDF et le nombre de workers
- [ ] `cmd/photosort-sidecar/main.go` : appel mince, comportement inchangé
- [ ] Déplacer les tests avec le code (`main_test.go`, `scrapbook_test.go`)

## Phase 2 — Moteur : options de copie et contrôle d'espace `[parallel avec 1]`

**Sortie** : la copie accepte un mode de fichier ; l'espace libre se contrôle.
**Vérification** : `go test ./sorter/...` ; `CopyWithProgress` garde le mode 0600 par défaut (comportement local inchangé).

- [ ] `sorter.CopyOptions{FileMode}` + `CopyWithOptions` ; `CopyWithProgress` en devient l'enveloppe (aujourd'hui `copyEntry` écrit en 0600, ce qui rendrait les copies illisibles pour QuMagie et SMB : `sorter/sorter.go:435`)
- [ ] Fonction `FreeSpace(dir)` (`syscall.Statfs`, build tag `unix`) et comparaison avec la taille du plan
- [ ] Tests : mode 0644 appliqué ; échec propre si l'espace manque ; copie interrompue sans fichier partiel (comportement existant, à verrouiller par un test)

## Phase 3 — Serveur `photosort-server` `[checkpoint]`

**Sortie** : binaire `cmd/photosort-server` + paquet `internal/remote`.
**Vérification** : `go test ./internal/remote/... ./cmd/photosort-server/...` (intégration sur `127.0.0.1:0`, dossiers temporaires, client TLS réel).

- [ ] Contrat : `docs/nas-protocol.md` (`hello`, `status`, `ping`, `fetch`, codes d'erreur, version de protocole)
- [ ] **Racines et chemins** : `Roots{Inbox, Library}`, `Resolve(path)` (refus de `..`, des chemins hors racines, du préfixe voisin `/inbox2`, des liens symboliques qui sortent), racines non imbriquées
- [ ] **Politique par opération** : `plan` (sources sous `/inbox`, destination forcée à `/library`), `copy` (par `planId` du serveur : le client n'injecte aucun chemin), `thumbnail` (sous `/inbox` ou `/library`), `tag` et `weekDays/weekPhotos/scrapbook*` (sous `/library` seulement), `fetch` (dossier de travail seulement), `cancel`, `ping`, `status` ; toute autre opération est refusée
- [ ] **Contrôle au démarrage (échec fermé)** : refus si uid 0 ; refus si `/inbox` est inscriptible (`syscall.Access(W_OK)`, qui tient compte de `EROFS` et des ACL) ; `/library` et le dossier d'état doivent accepter l'écriture (fichier témoin créé puis supprimé) ; sous-commande `selfcheck` pour le NAS
- [ ] **TLS et jeton** : au premier démarrage, certificat ECDSA P-256 (SAN depuis `PHOTOSORT_HOSTS`), jeton aléatoire affiché une seule fois dans les journaux avec l'empreinte du certificat ; seul le hachage du jeton est écrit (`0600`)
- [ ] **Connexion** : `hello` attendu en 5 s, ligne limitée à 4 Ko avant authentification, comparaison à temps constant, temporisation après échecs par adresse, **un seul client actif** (le second reçoit `busy`), `ping` toutes les 15 s et fermeture après 45 s de silence, opérations annulées à la fermeture
- [ ] `status` : résultats du contrôle de démarrage, version, espace libre, IA disponible ; `fetch` : blocs de 256 Ko de fichiers du dossier de travail uniquement ; nettoyage des PDF après rapatriement ou au bout d'1 h
- [ ] Journal d'audit sur la sortie standard (heure, adresse, opération, résultat ; jamais le jeton)
- [ ] Mode IA : voir risque 1 (sonde dans un processus enfant)
- [ ] Tests : mauvais jeton, pas de `hello`, ligne trop longue, second client, opération interdite, évasions de chemin, `plan` puis `copy` avec fichiers en 0644 et **entrée identique avant/après** (somme de l'arborescence), `fetch` octet pour octet, coupure en pleine copie sans fichier partiel

## Phase 4 — Image Docker et compose `[parallel avec 5]`

**Sortie** : `Dockerfile`, `docker-compose.yml`, `scripts/build-nas-image.sh`, archive `dist/photosort-server-amd64.tar`.
**Vérification** (Docker local) : conteneur durci démarré avec une entrée `:ro` ; `docker exec … touch /inbox/x` échoue (`Read-only file system`) ; `docker inspect` ne montre ni `Privileged` ni capacités ; test d'intégration de la phase 3 rejoué contre le conteneur ; taille de l'image notée.

- [ ] Dockerfile multi-étapes (Go avec CGO, `libonnxruntime.so` et modèles via `fetch-ai-assets` pour la plateforme visée, image d'exécution mince, `USER` numérique)
- [ ] Compose : `user: "<uid>:<gid>"`, `/inbox:ro`, `/library`, dossier d'état, `read_only`, `tmpfs: /tmp`, `cap_drop: [ALL]`, `no-new-privileges`, `mem_limit`, `cpus`, `pids_limit`, port publié sur l'adresse LAN du NAS (`IP:8443:8443`), jamais `privileged` ni réseau hôte
- [ ] Script : construction `linux/amd64` (émulée sur le M2) et `linux/arm64` (tests locaux), export `docker save` compressé
- [ ] Verrouiller par un test ce qui est déjà vrai dans le code : `tagger.New` n'initialise ONNX Runtime que pour les scènes et les personnes (`internal/tagger/tagger.go:117`), donc les tags de métadonnées et de moment fonctionnent sans `libonnxruntime.so`

## Phase 5 — Electron, processus principal `[parallel avec 4]`

**Sortie** : second backend réseau derrière la même interface que le sidecar.
**Vérification** : `npm run typecheck` ; `node --test` (type stripping de Node 24, sans dépendance) contre un vrai `photosort-server` lancé en local.

- [ ] `src/main/backend.ts` : interface `Backend` (`send`, événements) ; `LocalSidecar` (code actuel) et `RemoteServer` ; `ipcMain.on('sidecar:send')` bascule selon le mode
- [ ] `src/main/remote.ts` : `probe(host, port)` (lit l'empreinte sans rien envoyer) ; `connect` avec vérification de `fingerprint256` à `secureConnect`, **avant** d'écrire le `hello` ; battement de cœur ; événement `error` synthétique à la perte de connexion ; reconnexion manuelle
- [ ] Rapatriement du PDF : à l'événement `done` d'un `scrapbook`, téléchargement par `fetch` dans le dossier choisi en préférences, puis réécriture de `path` en chemin local : le renderer n'est pas modifié
- [ ] `src/main/secrets.ts` : jeton chiffré par `safeStorage` dans `userData` ; si le chiffrement est indisponible, ne rien écrire et redemander le jeton à chaque session
- [ ] IPC et `preload` : `nas.probe`, `nas.connect`, `nas.forget`, `nas.status`, `backend.use`
- [ ] Tests : une empreinte différente est refusée **sans qu'un seul octet soit envoyé** (serveur TLS factice qui compte les octets reçus) ; `hello` ; miniature ; `fetch` avec somme de contrôle identique

## Phase 6 — Electron, interface `[checkpoint : revue visuelle]`

**Sortie** : écran de connexion et mode NAS.
**Vérification** : `npm run typecheck`, `npm run build`, parcours manuel contre le conteneur local, capture d'écran.

- [ ] Écran de connexion : adresse, port, jeton ; étape « Comparer l'empreinte avec celle des journaux du conteneur » avant validation
- [ ] Carte de statut : liste de sécurité animée (entrée en lecture seule ✔, bibliothèque inscriptible ✔, non-root ✔, TLS épinglé ✔, espace libre, version) ; rouge et explicite si un contrôle échoue
- [ ] Mode NAS : sélecteur « Ce Mac / NAS » ; entrée et bibliothèque affichées comme racines fixes, sans sélecteurs de dossiers ni dépôt par glisser-déposer ; case « tags IA » masquée tant que le serveur ne l'annonce pas
- [ ] Bandeau de déconnexion avec « Se reconnecter » ; notification du PDF rapatrié
- [ ] Chaînes i18n fr/en/da ; animations dans le style existant (`framer-motion`)

## Phase 7 — Recette sur le NAS `[checkpoint : intervention de l'utilisateur]`

**Sortie** : `docs/nas/installation.md` (guide pas à pas en français) et `docs/nas/measurements.md` (mesures).
**Vérification** : liste de recette signée par l'utilisateur.

- [ ] Guide : créer l'utilisateur QTS `photosort` (non administrateur, aucun accès aux applications), matrice de droits sur les deux dossiers partagés (entrée : lecture seule ; bibliothèque : lecture-écriture ; tous les autres : aucun accès), dossier d'état, import de l'archive dans Container Station, compose, lecture du jeton et de l'empreinte
- [ ] Recette : `status` entièrement vert ; **preuve négative** : donner temporairement l'écriture sur l'entrée, le conteneur doit refuser de démarrer, puis rétablir ; relever `id photosort` et `fs.protected_hardlinks`
- [ ] QuMagie lit les copies (droits 0644) ; n'indexer que la bibliothèque
- [ ] Mesures sur un échantillon : durée du plan, tags de métadonnées, un tag IA, démarrage d'ONNX Runtime, pic mémoire (`docker stats`), CPU ; décision sur les tags IA et les bornes de workers

## Phase 8 — Documentation et CI `[parallel]`

**Vérification** : YAML valide ; l'archive produite se charge avec `docker load`.

- [ ] README (section NAS) et mise à jour de la spec (décisions D1 à D8)
- [ ] Étape CI optionnelle de construction de l'image `linux/amd64` en artefact (`workflow_dispatch`)

## Risques

1. **ONNX Runtime sans AVX sur le Celeron** : un `SIGILL` tuerait le serveur et ne se rattrape pas dans le processus → sonde d'initialisation dans un **processus enfant** au démarrage ; en cas d'échec, `status` annonce `ai:false` et les tags IA restent indisponibles, le reste fonctionne.
2. **Droits QTS non appliqués aux montages Docker comme attendu** → les verrous sont indépendants (montage `:ro` et garde au démarrage échouent fermés) ; vérifié par la preuve négative de la phase 7.
3. **Disque** : la copie double l'espace des photos classées → contrôle d'espace avant copie, estimation donnée à l'utilisateur.
4. **Mac en veille en pleine copie** → annulation propre (sans fichier partiel), reprise par un nouveau plan.
5. **Image malveillante dans l'entrée visant un décodeur** : atténué par le but même du plan (non-root, sans capacités, sans écriture sur l'entrée, rien d'autre de monté).
6. **Jeton et empreinte à la première connexion** : l'utilisateur doit comparer l'empreinte aux journaux ; le jeton ne part jamais avant cette validation.
7. **Container Station** : prise en charge de compose et rôle de `user:` non vérifiés → phase 7.

## Hors périmètre

Accès distant (second plan : Tailscale `tsnet` ou NetBird, ou myQNAPcloud Link avec MFA ; l'invariant ci-dessus s'y applique tel quel), case « classer automatiquement », lien physique, navigateur de dossiers distants, découverte automatique (mDNS), multi-utilisateurs, opérations détachées avec reprise par identifiant, déplacement ou suppression d'originaux.

## Questions ouvertes

1. Entrée et bibliothèque sont-elles **deux dossiers partagés distincts** (droits simples) ou deux sous-dossiers d'un même partage (ACL avancées de QTS) ?
2. La bibliothèque classée existe-t-elle déjà, ou sera-t-elle créée vide ? Avec quel espace libre sur le disque ?
3. Container Station est-il installé et l'accès SSH permis (pour relever `id photosort` ; sinon la phase 7 passe par l'interface) ?
4. Ordre de grandeur du nombre de photos dans l'entrée ?
