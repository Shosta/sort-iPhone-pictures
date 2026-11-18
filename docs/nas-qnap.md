# Préparer le NAS QNAP pour Photosort (réseau local et Tailscale)

Procédure de la **phase 0b** du plan [`2026-11-17-nas-local-connection-plan.md`](superpowers/plans/2026-11-17-nas-local-connection-plan.md). Seul Tailscale est installé sur le NAS : on crée un compte aux droits minimaux, on limite l'accès par les ACL Tailscale, puis on monte le partage. Les étapes 1 à 6 et 9 à 10 ne changent pas ; l'étape 7bis remplace le montage par le Finder quand on passe par Tailscale.

**Durée** : 30 à 45 minutes. **Prérequis** : accès administrateur à QTS (navigateur), Mac sur le même réseau que le NAS.

## Principe

Photosort tourne sur le Mac. Le compte `photosort` peut :

| Dossier | Droit |
| --- | --- |
| Entrée (photos en vrac de l'iPhone) | **Lecture seule** |
| Bibliothèque (photos classées) | **Lecture et écriture** |
| Reste du partage et autres partages | **Refusé** |

Même si le compte était compromis, il ne pourrait ni modifier les originaux ni toucher au reste du NAS.

Dans la suite, remplacez `<PARTAGE>`, `<ENTRÉE>`, `<BIBLIOTHÈQUE>` par vos vrais noms, et `<NAS>` par l'adresse IP du NAS (ex. `192.168.1.20`).

> Les menus ci-dessous suivent la documentation QNAP (QTS 5.x). Les libellés exacts peuvent varier légèrement selon votre version ; les points marqués **(à confirmer)** n'ont pas pu être vérifiés sans accès à votre NAS.

## Étape 1 — Sauvegarder la configuration

*Panneau de configuration > Système > Sauvegarde / Restauration > Sauvegarder les paramètres système* **(à confirmer)**.

Conservez le fichier hors du NAS. L'étape 4 change un réglage global : cette sauvegarde permet de revenir en arrière.

## Étape 2 — Vérifier que SMB est actif

*Panneau de configuration > Services réseau et fichiers > Win/Mac/NFS/WebDAV > Réseaux Microsoft* (anglais : *Network & File Services > Win/Mac/NFS/WebDAV > Microsoft Networking*).

- [ ] **Activer le service de fichiers pour les réseaux Microsoft** est coché.
- [ ] Authentification : **Serveur autonome** (comptes locaux du NAS).

Si la case était déjà cochée, ne changez rien d'autre ici pour l'instant.

## Étape 3 — Créer le compte `photosort`

*Panneau de configuration > Privilège > Utilisateurs > Créer > Créer un utilisateur.*

- [ ] **Nom d'utilisateur** : `photosort`.
- [ ] **Mot de passe** : long (20 caractères ou plus), unique, de préférence généré par un gestionnaire de mots de passe. QTS accepte 64 caractères ASCII au plus.
- [ ] **Groupe d'utilisateurs** : aucun. Surtout **pas** `administrators`.
- [ ] **Autorisations de dossiers partagés** : laisser vide pour l'instant (étape 5).
- [ ] **Privilèges d'application** > *Modifier* : ne garder que **Réseaux Microsoft** (SMB). Refuser tout le reste : File Station, FTP, SSH/SFTP, WebDAV, AFP, NFS, Qsync, applications multimédia **(à confirmer : liste exacte selon votre version)**. QNAP recommande de refuser tout ce dont l'utilisateur n'a pas besoin.
- [ ] **Quota** : aucun.
- [ ] Cliquez sur **Créer**.

## Étape 4 — Activer les permissions avancées de dossier

*Panneau de configuration > Privilège > Dossiers partagés > Permissions avancées* :

- [ ] cochez **Activer les permissions avancées de dossier** (*Enable Advanced Folder Permissions*) ;
- [ ] **Appliquer**.

C'est un réglage **global** du NAS : il active les droits par sous-dossier pour tous les partages. Les droits existants des partages ne sont pas modifiés, mais le contrôle 5 (étape 9) vérifie que rien n'a cassé.

## Étape 5 — Définir les droits sur le partage

*Panneau de configuration > Privilège > Dossiers partagés > Dossiers partagés*, ligne `<PARTAGE>` > **Permissions de dossier**.

Règle QNAP à connaître : un droit **lecture seule** sur la racine s'impose à tous les sous-dossiers, et un **refus** sur la racine bloque tout. Pour avoir « lecture seule » sur l'entrée et « écriture » sur la bibliothèque, on donne donc **lecture/écriture à la racine**, puis on restreint dans les sous-dossiers.

**5a. Racine du partage** (sélectionnez `<PARTAGE>` dans l'arbre de gauche) :

- [ ] **Ajouter** `photosort` en **Lecture/Écriture**.
- [ ] Cochez **Seul l'administrateur peut créer des fichiers et des dossiers** (*Only admin can create files and folders*) : `photosort` ne peut plus rien créer au premier niveau du partage.
- [ ] Cochez **Seul le propriétaire peut supprimer le contenu** (*Only the owner can delete the contents*) et vérifiez que le **propriétaire** est `admin` : `photosort` ne peut pas supprimer ni renommer les dossiers de premier niveau.
- [ ] **Ne cochez jamais** *Permission spéciale* : elle donne un accès total, ignorant tous les autres droits.
- [ ] Laissez *Accès invité* sur **Refuser**.
- [ ] **Appliquer**.

**5b. Entrée** (`<PARTAGE>/<ENTRÉE>`) :

- [ ] **Ajouter** `photosort` en **Lecture seule**.
- [ ] **Appliquer**.

**5c. Bibliothèque** (`<PARTAGE>/<BIBLIOTHÈQUE>`) :

- [ ] **Ajouter** `photosort` en **Lecture/Écriture**.
- [ ] **Appliquer**.

**5d. Tous les autres sous-dossiers de premier niveau de `<PARTAGE>`** :

- [ ] **Ajouter** `photosort` en **Refuser**, un par un.

**5e. Tous les autres partages du NAS** (*Dossiers partagés*, ligne par ligne > **Permissions de dossier**) :

- [ ] **Ajouter** `photosort` en **Refuser**.
- [ ] Faites de même pour les partages créés plus tard.

Conseils :
- N'allez pas au-delà du premier ou deuxième niveau de sous-dossiers (recommandation QNAP).
- Évitez *Appliquer et remplacer toutes les permissions existantes de ce dossier, fichiers et sous-dossiers* sur la racine : cela écraserait les droits existants des autres comptes sur votre bibliothèque massive.
- Facultatif, durcissement : dans la liste déroulante *Accès à l'hôte Réseaux Microsoft* de la même fenêtre, n'autorisez que l'adresse IP de votre Mac.

## Étape 6 — Options SMB (facultatif)

*Panneau de configuration > Services réseau et fichiers > Win/Mac/NFS/WebDAV > Réseaux Microsoft > Options avancées.*

Ces options sont **globales** : elles s'appliquent aussi à vos autres appareils (TV, scanner, vieux PC). Ne les changez que si vous savez qu'ils sont compatibles.

- [ ] **Restreindre l'accès des utilisateurs anonymes aux dossiers SMB** : coché.
- [ ] **Version SMB la plus basse** : SMB 3 (inclut 3.1 et 3.1.1). macOS le gère. Si un appareil ancien perd l'accès, remettez la valeur par défaut.
- [ ] **Signature du serveur** : *Signer si le client l'accepte*. N'imposez la signature (*Imposer la signature*) que si tous vos appareils la gèrent.
- [ ] Ne cochez pas **Activer le démon SMB en mode noyau** si vous voulez garder le chiffrement SMB disponible.

## Étape 7 — Monter le partage sur le Mac

1. Finder > **Aller > Se connecter au serveur…** (`⌘K`).
2. Adresse : `smb://<NAS>/<PARTAGE>` puis **Se connecter**.
3. Choisissez **Utilisateur enregistré**, entrez `photosort` et son mot de passe.
4. Cochez **Mémoriser ce mot de passe dans mon trousseau**.
5. Le partage apparaît dans le Finder et sous `/Volumes/<PARTAGE>`.

Pièges connus :
- macOS refuse souvent un deuxième montage du même serveur avec un autre compte. Si vous êtes déjà connecté au NAS avec votre compte principal, éjectez ce volume d'abord, ou connectez-vous avec l'adresse IP pour un compte et le nom d'hôte pour l'autre.
- Photosort ne monte rien lui-même et ne voit jamais le mot de passe : si le partage n'est pas monté, il s'arrête avec un message.
- Facultatif, pour ne pas laisser de fichiers `.DS_Store` sur le NAS : dans le Terminal, `defaults write com.apple.desktopservices DSDontWriteNetworkStores -bool true`, puis redémarrez le Finder.

## Étape 7bis — Monter le partage par Tailscale (Linux)

Prérequis : le plan [phase 0c](superpowers/plans/2026-11-17-nas-local-connection-plan.md) est faite (NAS et Linux dans le tailnet, clé du NAS sans expiration, ACL limitées au port 445).

1. Vérifiez la connexion : `tailscale ping <NAS-MAGICDNS>` doit répondre en **direct**. Un relais DERP marche mais ralentit fortement SMB.
2. Option **GVFS** (sans droits administrateur) : Dolphin > `smb://<NAS-MAGICDNS>/<PARTAGE>`, compte `photosort`. Le chemin est sous `/run/user/<uid>/gvfs/`.
3. Option **cifs** (droits administrateur) : `sudo mount -t cifs //<NAS-MAGICDNS>/<PARTAGE> /mnt/photos -o username=photosort,vers=3.0,uid=$(id -u),gid=$(id -g),file_mode=0644,dir_mode=0755` (le mot de passe est demandé ; ne l'écrivez pas dans l'historique).
4. Faites ensuite l'**étape 9** depuis ce point de montage (remplacez `/Volumes/<PARTAGE>` par le chemin réel). **(à confirmer)** : GVFS et la création exclusive de fichiers.

Règles Tailscale : pas de routes de sous-réseau ni de nœud de sortie depuis le NAS, ni Serve ni Funnel ; seul l'appareil qui lance Photosort atteint le NAS, et seulement sur le port 445.

## Étape 8 — Monter automatiquement au démarrage (facultatif)

Réglages Système > Général > **Ouverture** : faites glisser le volume monté dans la liste **Éléments d'ouverture**.

## Étape 9 — Vérifier les droits (5 contrôles)

Préparation : avec votre **compte administrateur** (File Station), déposez un fichier texte `test-lecture.txt` dans `<ENTRÉE>`. **N'essayez jamais de supprimer une vraie photo** pour ce test.

Dans le Terminal, avec le partage monté par `photosort` :

```sh
cd "/Volumes/<PARTAGE>"

# 1. doit ÉCHOUER (Permission denied / Read-only)
touch "<ENTRÉE>/essai.txt"

# 2. doit ÉCHOUER
rm "<ENTRÉE>/test-lecture.txt"
mv "<ENTRÉE>/test-lecture.txt" "<ENTRÉE>/renomme.txt"

# 3. doit RÉUSSIR
touch "<BIBLIOTHÈQUE>/.essai-photosort" && rm "<BIBLIOTHÈQUE>/.essai-photosort"

# 4. doit ÉCHOUER (accès refusé)
ls "<AUTRE-SOUS-DOSSIER>"
touch "essai-racine.txt"
```

Puis :

- [ ] **1. Création dans l'entrée** : refusée.
- [ ] **2. Suppression et renommage dans l'entrée** : refusés.
- [ ] **3. Création puis suppression dans la bibliothèque** : réussies.
- [ ] **4. Autres sous-dossiers, racine du partage et autres partages** : refusés (essayez aussi `smb://<NAS>/<AUTRE-PARTAGE>` dans le Finder).
- [ ] **5. Comptes existants** : la sauvegarde automatique de l'iPhone continue (lancez-en une et vérifiez qu'une nouvelle photo arrive), QuMagie affiche toujours vos photos, et votre compte principal accède toujours à ses dossiers.

**Tous les contrôles doivent passer.** Si le 1 ou le 2 réussit, le compte a trop de droits : ne lancez pas Photosort, corrigez l'étape 5 (le plus souvent, un droit hérité de la racine ou d'un groupe).

Supprimez ensuite `test-lecture.txt` avec le compte administrateur.

## Étape 10 — Après la première copie réelle

Quand Photosort aura copié ses premières photos (phase 6 du plan), vérifiez depuis votre **compte principal** (pas `photosort`) :

- [ ] vous ouvrez et lisez les photos copiées dans `<BIBLIOTHÈQUE>` ;
- [ ] QuMagie les indexe.

Si elles sont illisibles, c'est que les fichiers sont créés avec le mode `0600` : dites-le-moi, le plan prévoit une option pour créer les fichiers en `0644`.

## Règles à retenir

- **Ne jamais exposer SMB (port 445) sur internet** ni le rediriger depuis la box. L'accès à distance passe par Tailscale uniquement (ACL limitées au port 445), jamais par une redirection de port ; ni Funnel, ni Serve, ni routes de sous-réseau.
- Le compte `photosort` ne sert qu'à Photosort. Ne le réutilisez pas ailleurs.
- Changez son mot de passe en cas de doute, puis mettez à jour le trousseau.

## Retour arrière

- Retirer `photosort` des permissions de chaque dossier, ou **supprimer le compte** (*Privilège > Utilisateurs*).
- Pour revenir à l'état d'avant : restaurer le fichier de l'étape 1, ou décocher *Activer les permissions avancées de dossier* (étape 4). La documentation QNAP signale que cela peut modifier les droits au niveau des sous-dossiers : contrôlez ensuite vos accès existants.

## Sources

- QNAP, [Shared folders and permissions (QTS)](https://www.qnap.com/en/how-to/faq/article/how-do-i-set-up-shared-folders-and-the-permission-on-the-nas-running-qts) : permissions avancées, règles lecture seule / refus, options de la racine.
- QNAP, [Creating a local user](https://docs.qnap.com/operating-system/qts/4.4.x/en-us/GUID-A3835C8C-0950-4554-8E5A-11F8C3C60CCE.html) : privilèges d'application.
- QNAP, [Configuring Samba (Microsoft networking) settings (QTS 5.2.x)](https://docs.qnap.com/operating-system/qts/5.2.x/en-us/configuring-samba-microsoft-networking-settings-7447174D.html) : version SMB, signature, accès anonyme.
