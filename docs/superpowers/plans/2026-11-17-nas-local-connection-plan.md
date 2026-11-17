# Plan : classement et tags sur le NAS, connexion en réseau local (Mac + SMB)

Date : 2026-11-17 · Branche : `feat/nas-qnap-remote` · Statut : **plan révisé, en attente d'approbation** (aucun code écrit)
Spec parente : `docs/superpowers/specs/2026-11-16-nas-qnap-remote-design.md`
Plan différé (serveur Docker) : `docs/superpowers/plans/2026-11-17-nas-docker-server-plan-deferred.md`

## Pourquoi ce plan remplace le précédent

Réponses de l'utilisateur (2026-11-17) :
- entrée et bibliothèque sont **deux sous-dossiers d'un même partage** ;
- la bibliothèque existe déjà et est **massive** ;
- **Container Station n'est pas installé** et le **SSH reste désactivé** (surface d'attaque) ;
- l'entrée contient **quelques centaines de photos**.

Conséquences : le serveur Docker (Container Station + import d'image) ajouterait
au NAS un démon root et de la maintenance pour traiter quelques centaines de
photos. L'app sait déjà lire un dossier d'entrée et écrire dans un dossier de
destination : un partage SMB monté sur le Mac suffit. **Rien à installer sur le
NAS, aucun port nouveau.** Le Mac (Apple M2) rend en plus les tags IA
réalistes, ce que le Celeron du NAS ne garantissait pas.

## Objectif

Depuis l'app Electron du Mac, sur le même réseau local : classer l'entrée en
vrac dans la bibliothèque du NAS, taguer la bibliothèque et produire le PDF du
semainier, avec un compte QTS dédié aux droits minimaux. L'accès à distance et
le traitement sans le Mac viendront dans des plans ultérieurs.

## Contraintes

- Logique métier en Go ; moteur existant réutilisé (`sorter`, `internal/tagger`, `internal/scrapbook`).
- **Entrée jamais écrite**, bibliothèque seul dossier inscriptible.
- Compte QTS dédié `photosort` : lecture seule sur l'entrée, lecture-écriture sur la bibliothèque, **aucun droit ailleurs**, SMB seulement.
- **Rien d'installé sur le NAS**, ni SSH, ni Container Station, ni QPKG tiers.
- Le comportement local actuel ne change pas par défaut (toute nouveauté est une option ou un durcissement neutre).
- Aucun secret dans l'app : le montage est fait par l'OS (trousseau macOS).
- Branche `feat/nas-qnap-remote`, aucun merge dans `main` sans demande, trailer `Co-authored-by` sur chaque commit.

## Décisions (retenues par défaut, à confirmer)

| # | Décision | Raison |
| --- | --- | --- |
| N1 | **Voie A' : l'app sur le Mac, le partage monté en SMB** avec le compte dédié. Le serveur Docker est différé | Aucune installation ni SSH ; quelques centaines de photos transitent en secondes sur le LAN |
| N2 | Droits : **Advanced Folder Permissions** de QTS (droits par sous-dossier : lecture, lecture-écriture, refus) | Documenté par QNAP ; seul moyen de séparer deux sous-dossiers d'un même partage sans Docker. Réglage global du NAS : voir risques |
| N3 | Montage par l'**OS** (Finder `smb://`, lecteur réseau Windows, `mount.cifs`) ; l'app ne monte rien et ne voit jamais le mot de passe | Le trousseau gère le secret ; aucune dépendance |
| N4 | **Contrôle préalable** `nasCheck` (Go) : l'entrée doit refuser l'écriture, la bibliothèque doit l'accepter | Seconde barrière : si l'ACL QTS est mal réglée, l'app refuse de lancer l'opération (échoue fermé) |
| N5 | **Copie** (pas de lien physique) vers la bibliothèque | Un lien physique ne passe pas sur un montage SMB et contredit la lecture seule de l'entrée |
| N6 | Dédoublonnage **à portée** (option) : ne parcourir dans la bibliothèque que les mois concernés par le plan | `loadDestination` (`sorter/dedupe.go:44`) parcourt toute la destination avec un `lstat` par fichier : coûteux sur SMB avec une bibliothèque massive |
| N7 | Ignorer les dossiers et fichiers **système** (`.@__thumb`, `@Recycle`, `@Recently-Snapshot`, `@eaDir`, `._*`, `.DS_Store`) | `Plan` (`sorter.go:122`) et `deduper` ne sautent aucun dossier caché alors que `tags.go:39` et `scrapbook/scan.go:101` le font déjà ; QTS crée des `.@__thumb` partout |
| N8 | Copie **atomique** : écriture dans `<cible>.part` puis renommage | Une coupure réseau laisserait sinon un fichier tronqué au nom final (`copyEntry`, `sorter.go:415`), pris ensuite pour une photo valide |
| N9 | Semainier et tags inchangés : lecture de la bibliothèque en SMB, **PDF écrit localement** (Téléchargements) | Rien à rapatrier |
| N10 | Tags IA **autorisés** (calculés sur le Mac) | Le blocage du Celeron disparaît |

## Phase 0 — Documentation (inventaire des API)

Faite en ligne : API stables de la bibliothèque standard et code du dépôt, pas de sous-agents.

**Vérifié dans le code**
- `Plan` parcourt les sources avec `WalkDir` sans sauter les dossiers cachés (`sorter.go:122`) ; `deduper.loadDestination` parcourt toute la destination (`dedupe.go:44-60`) et ne hache que les fichiers de **même taille** qu'une source (`dedupe.go:69`) ; la destination n'est comparée qu'**octet pour octet** (`register`, `dedupe.go:90`).
- `copyEntry` crée le fichier final en `O_EXCL`, mode 0600 (`sorter.go:435`) et ne nettoie qu'en cas d'erreur rattrapée.
- `copyWorkers = 4` (`sorter.go:330`) : adapté à un disque réseau.
- `tags.go:39` et `scrapbook/scan.go:101` sautent les dossiers dont le nom commence par `.`.

**Vérifié dans la documentation QNAP** : *Control Panel > Privilege > Shared Folders > Advanced Permissions > Enable Advanced Folder Permissions* permet de fixer lecture, lecture-écriture ou refus par utilisateur pour chaque dossier et sous-dossier ([QTS 5.2](https://docs.qnap.com/operating-system/qts/5.2.x/en-us/configuring-advanced-folder-permissions-16E9201B.html), [FAQ QNAP](https://www.qnap.com/en/how-to/faq/article/should-i-enable-advanced-folder-permissions-or-windows-acl-support)). « Windows ACL » est une alternative, configurée depuis l'Explorateur Windows : non retenue.

**À ne pas faire**
- Ne jamais tester l'écriture de l'entrée autrement que par une tentative qui **doit échouer** ; si elle réussit, supprimer la sonde et refuser.
- Ne pas stocker de mot de passe dans l'app ; ne pas monter de partage depuis l'app.
- Ne pas exposer SMB (port 445) sur internet, ni le rediriger sur la box.
- Ne pas changer le comportement local par défaut (dédoublonnage complet de la destination conservé).

**Non vérifié (à lever à la phase 6)** : effet de l'Advanced Folder Permissions sur les accès existants ; mode des fichiers créés (0600) vu par QuMagie et les autres comptes ; options de sécurité SMB disponibles dans la version de QTS installée (version SMB minimale, signature) ; accès du compte aux applications QTS (restriction « Application Privilege ») ; durée réelle du parcours de la bibliothèque sur SMB ; normalisation Unicode des noms de lieux accentués ; organisation de la bibliothèque existante (voir questions).

## Phase 0b — Préparer le NAS `[utilisateur, aucune ligne de code]`

**Sortie** : un compte `photosort` qui ne peut que lire l'entrée et écrire la bibliothèque, et un partage monté sur le Mac.
**Vérification** (manuelle, 5 contrôles) : avec ce compte monté sur le Mac, (1) créer un fichier dans l'entrée échoue, (2) supprimer ou renommer une photo de l'entrée échoue, (3) créer puis supprimer un fichier dans la bibliothèque réussit, (4) les autres dossiers du partage et les autres partages sont refusés, (5) les comptes existants (la sauvegarde de l'iPhone, QuMagie) fonctionnent comme avant.

Procédure pas à pas : [`docs/nas-qnap.md`](../../nas-qnap.md). Résumé :
- [ ] Sauvegarder la configuration QTS (*Backup / Restore*) avant tout changement.
- [ ] Créer l'utilisateur `photosort`, mot de passe long et unique, **hors** groupe `administrators`.
- [ ] Activer *Advanced Folder Permissions* (réglage global).
- [ ] Sur le partage : `photosort` en lecture-écriture sur la bibliothèque, **lecture seule** sur l'entrée, **refus** sur le reste ; refus sur tous les autres partages. Règle QNAP : un droit lecture seule ou un refus sur la racine s'impose aux sous-dossiers ; la racine reçoit donc lecture-écriture avec *Only admin can create files and folders* et *Only the owner can delete the contents* (propriétaire `admin`).
- [ ] Limiter les applications accessibles au compte (SMB seulement ; pas de File Station, FTP, SSH).
- [ ] Options SMB (à confirmer dans QTS) : version minimale SMB 3, signature activée.
- [ ] Monter `smb://<nas>/<partage>` depuis le Finder avec ce compte, enregistrer le mot de passe dans le trousseau.

## Phase 1 — Moteur : ignorer les noms système

**Sortie** : un point unique décide quels dossiers et fichiers sont ignorés, utilisé par les quatre parcours.
**Vérification** : `go test ./sorter/... ./internal/scrapbook/...` ; test avec un arbre contenant `.@__thumb/x.jpg`, `@Recycle/y.jpg`, `._IMG_1.HEIC`, `.DS_Store`, `vrai/IMG_2.HEIC` : seul `vrai/IMG_2.HEIC` est vu par `Plan`, `deduper`, `PhotosToTag` et `scanMonth`.

Tâches :
- [ ] Ajouter `SkipDir(name)` et `SkipFile(name)` dans `sorter` (nom caché `.`, préfixe `@`, `._*`, `.DS_Store`, `Thumbs.db`).
- [ ] Les utiliser dans `Plan` (`sorter.go:122`), `deduper.loadDestination` (`dedupe.go:44`), `PhotosToTag` (`tags.go:39`) et `scanMonth` (`scan.go:101`, qui garde sa règle actuelle plus la nouvelle).
- [ ] Tests (`sorter_test.go`, `tags_test.go`, `scan_test.go`).

## Phase 2 — Moteur : dédoublonnage à portée `[après 1 : mêmes fichiers]`

**Sortie** : option `Engine.ScopedDestination` ; par défaut, comportement inchangé.
**Vérification** : `go test ./sorter/...` ; test avec 24 mois de fichiers factices et 2 mois dans le plan : le parcours ne visite que ces 2 mois (compteur de fichiers visités, pas de chronomètre), et un doublon octet pour octet d'un de ces mois est toujours détecté. Sans l'option, tous les mois sont visités.

Tâches :
- [ ] `newDeduper(destination, scope)` : si `scope` est non vide, `loadDestination` ne parcourt que `<destination>/<AAAA>/<AAAA-MM>` pour les mois présents dans le plan ; la liste vient de l'étape 1 de `Plan` (le dossier de chaque photo est déjà calculé avant les empreintes).
- [ ] Documenter l'hypothèse : un fichier identique a la même date EXIF, donc le même mois, **si la bibliothèque suit l'arborescence de Photosort**. Sinon, laisser l'option désactivée.
- [ ] Exposer `scopedDestination` dans la requête `plan` du sidecar (booléen optionnel, défaut `false`).

## Phase 3 — Moteur : copie atomique et reprise sur coupure `[après 2]`

**Sortie** : `copyEntry` écrit dans `<cible>.part` puis renomme ; une coupure ne laisse jamais de fichier tronqué au nom final.
**Vérification** : `go test ./sorter/...` ; test : source tronquée en cours de copie (lecteur qui échoue) → la cible finale n'existe pas, le `.part` est supprimé ; un `.part` orphelin d'une exécution précédente est écrasé ; une cible existante n'est jamais remplacée ; une copie réussie garde le contenu exact.

Tâches :
- [ ] `copyEntry` (`sorter.go:415`) : ouverture de `<cible>.part` en `O_CREATE|O_TRUNC|O_WRONLY`, `Sync`, vérification que la cible n'existe pas (`root.Lstat`), `root.Rename`.
- [ ] Nouvelle tentative (3 fois, attente croissante) sur les erreurs réseau transitoires de la copie (`EIO`, `ETIMEDOUT`, `ENOTCONN`, `ESTALE`, `EHOSTDOWN`) ; échec clair ensuite, la reprise consiste à relancer le plan (doublons déjà copiés ignorés).
- [ ] **Conditionnel** : si la recette (phase 6) montre que le mode 0600 rend les copies illisibles pour QuMagie ou d'autres comptes, ajouter `CopyOptions{FileMode}` (0644 sur le NAS, 0600 en local).

## Phase 4 — Contrôle préalable `nasCheck` `[parallel avec 1 à 3 : autres fichiers]`

**Sortie** : opération `nasCheck` du sidecar (même enveloppe de requête que `plan`).
**Vérification** : `go test ./cmd/photosort-sidecar/...` ; tests sur des dossiers temporaires : entrée en `0555` → `inputReadOnly: true` ; entrée inscriptible → `inputReadOnly: false` **et sonde supprimée** ; bibliothèque inscriptible → `libraryWritable: true` ; chemin absent ou non dossier → erreur claire.

Tâches :
- [ ] Paquet `internal/nascheck` : `Check(input, library string) (Result, error)` avec `{mounted, inputReadOnly, libraryWritable, problems[]}`. La sonde d'entrée est une création de fichier qui **doit** échouer (`EACCES`, `EPERM`, `EROFS`) ; toute autre erreur est un échec fermé ; si elle réussit, suppression immédiate et `inputReadOnly: false`. La sonde de bibliothèque est un fichier caché `.photosort-probe-<aléatoire>` créé puis supprimé.
- [ ] Opération `nasCheck` dans `cmd/photosort-sidecar/main.go` et test dans `main_test.go`.
- [ ] Note : sous Windows, `EACCES` se traduit par `ERROR_ACCESS_DENIED` ; le paquet normalise via `errors.Is(err, fs.ErrPermission)`.

## Phase 5 — Electron : mode NAS `[après 2 et 4 ; checkpoint : revue visuelle]`

**Sortie** : dans les préférences, un bloc « NAS » (dossier d'entrée, bibliothèque, case « bibliothèque rangée par Photosort » = dédoublonnage à portée, case « vérifier la lecture seule de l'entrée » cochée par défaut) ; avant chaque classement ou tag en mode NAS, un contrôle `nasCheck` avec message clair en cas d'échec ; entrée et bibliothèque mémorisées comme dossiers par défaut.
**Vérification** : `npm run typecheck` ; capture d'écran de l'écran de préférences et du message d'échec ; test manuel : entrée inscriptible → classement refusé avec l'explication ; NAS démonté → message « dossier introuvable, montez le partage ».

Tâches :
- [ ] Préférences et types (`photosort-electron/src/renderer/…`), traductions fr/en/da.
- [ ] Appel de `nasCheck` avant `plan`, et propagation de `scopedDestination`.
- [ ] Messages d'erreur : partage non monté, entrée inscriptible, bibliothèque en lecture seule.
- [ ] Revue visuelle (thèmes clair/sombre).

## Phase 6 — Recette sur le NAS `[checkpoint : intervention de l'utilisateur]`

**Sortie** : un compte rendu chiffré.
**Vérification** : exécuter sur les vraies photos et noter :
- [ ] les 5 contrôles de la phase 0b ;
- [ ] durée de `plan` avec et sans dédoublonnage à portée, sur la vraie bibliothèque ;
- [ ] durée de la copie de quelques centaines de photos ;
- [ ] une photo en double octet pour octet est bien ignorée ;
- [ ] un lieu accentué (ex. `Orléans`) ne crée pas deux dossiers ;
- [ ] QuMagie voit les copies après réindexation, les `.xmp` ne le perturbent pas ;
- [ ] mode des fichiers créés (0600) : lisible par les autres comptes ? sinon, tâche conditionnelle de la phase 3 ;
- [ ] tags de métadonnées puis IA sur un mois, lecture et écriture des `.xmp` par SMB ;
- [ ] coupure volontaire du Wi-Fi pendant une copie : aucun fichier tronqué, reprise propre.

## Phase 7 — Documentation `[parallel]`

- [ ] `docs/nas-qnap.md` (rédigé, sans captures) : l'ajuster après la recette (menus réellement observés, captures éventuelles) et le lier depuis le README.
- [ ] `README.md` : section « NAS QNAP en réseau local », renvoi vers ce document ; notes Windows (lecteur réseau) et Linux (`mount.cifs`).
- [ ] Spec : statut mis à jour.

## Risques

- **Advanced Folder Permissions est global** → sauvegarde de la configuration avant, contrôle 5 de la phase 0b ; retour arrière possible en le désactivant (la doc QNAP signale des effets sur les ACL existantes).
- **Seulement deux verrous sur l'entrée** (droits QTS, contrôle préalable) au lieu de trois avec Docker → contrôle exécuté à chaque lancement, échec fermé, compte sans autre droit, jamais d'ouverture en écriture hors de la destination dans le moteur.
- **Performances SMB non mesurées** → dédoublonnage à portée, copie à 4 travailleurs, progression déjà affichée ; chiffres relevés à la phase 6.
- **Mac en veille ou Wi-Fi coupé en cours de copie** → copie atomique, relance idempotente.
- **Le compte `photosort` fuite** → il ne peut lire que l'entrée et écrire la bibliothèque ; pas d'application QTS, pas de SSH.
- **Bibliothèque organisée autrement que par Photosort** → l'option de portée reste désactivée ; parcours complet avec progression.

## Hors périmètre

- Serveur Docker sur le NAS, classification automatique (case à cocher), accès à distance (Tailscale, NetBird, myQNAPcloud Link) : plans ultérieurs ; l'accès à distance ne devra jamais élargir les droits du compte.
- Contrôle d'espace libre avant copie (quelques centaines de photos).
- Déplacement ou suppression des originaux, navigateur de dossiers distants, montage automatique depuis l'app.

## Questions ouvertes

1. Valides-tu la **voie sans installation sur le NAS** (Mac + SMB) pour la connexion locale, le serveur Docker étant gardé pour l'automatisation ?
2. Acceptes-tu d'activer **Advanced Folder Permissions** (réglage global du NAS) ? À défaut, l'entrée ne serait protégée que par l'app (moins strict).
3. La bibliothèque existante suit-elle déjà l'arborescence `AAAA/AAAA-MM` de Photosort ? (conditionne le dédoublonnage à portée)
4. QuMagie indexe-t-il l'entrée, la bibliothèque, ou les deux ?
