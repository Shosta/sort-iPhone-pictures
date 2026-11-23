import type { Lang } from './types';

// Les libellés entre guillemets reprennent mot pour mot ceux de l'application (panneau
// « Destination distante (NAS) », messages d'erreur) : ils sont à tenir à jour avec elle.
export interface NasCopy {
  meta: { title: string; description: string };
  hero: { eyebrow: string; title: string; lead: string; back: string };
  toc: { label: string; items: { id: string; label: string }[] };
  why: {
    title: string;
    lead: string;
    diagram: { alt: string; mac: string; sort: string; rsync: string; nas: string; local: string; sent: string };
    items: { title: string; text: string }[];
  };
  prereq: {
    title: string;
    qnap: { title: string; intro: string; steps: string[]; note: string };
    ssh: { title: string; text: string };
    mac: { title: string; text: string; check: string; other: string };
  };
  steps: {
    title: string;
    lead: string;
    open: string;
    shotCaption: string;
    shotAlt: string;
    pending: string;
    fields: { label: string; text: string }[];
    sortTitle: string;
    sortText: string;
  };
  test: { title: string; text: string; ok: string; pathMissing: string; error: string };
  dry: { title: string; text: string; result: string; then: string };
  safety: { title: string; items: { title: string; text: string }[] };
  trouble: {
    title: string;
    lead: string;
    columns: { message: string; fix: string };
    items: { message: string; fix: string }[];
  };
  faq: { title: string; items: { q: string; a: string }[] };
  cli: { title: string; text: string };
}

const fr: NasCopy = {
  meta: {
    title: 'Photosort — Trier ses photos directement vers un NAS (guide pas à pas)',
    description:
      'Comment envoyer vos photos triées vers un NAS (QNAP) avec Photosort : le tri est calculé chez vous puis envoyé par rsync, avec reprise après coupure, vérification optionnelle et aucun écrasement. Guide pas à pas et dépannage.',
  },
  hero: {
    eyebrow: 'Guide · Destination distante',
    title: 'Tri distant vers un NAS',
    lead: 'Photosort peut classer vos photos puis les envoyer directement sur le NAS de la maison, sans rien copier d’abord sur votre ordinateur. Ce guide explique comment le régler, pas à pas, sans jargon.',
    back: '← Retour à l’accueil',
  },
  toc: {
    label: 'Sur cette page',
    items: [
      { id: 'pourquoi', label: 'Ce que ça fait' },
      { id: 'prerequis', label: 'Avant de commencer' },
      { id: 'pas-a-pas', label: 'Pas à pas' },
      { id: 'tester', label: 'Tester la connexion' },
      { id: 'simuler', label: 'Simuler' },
      { id: 'depannage', label: 'Dépannage' },
      { id: 'faq', label: 'FAQ' },
    ],
  },
  why: {
    title: 'Ce que fait cette fonction, et pourquoi',
    lead: 'D’habitude, Photosort range vos photos dans un dossier de votre ordinateur. Avec la destination distante, il fait le même tri, mais le résultat part directement vers le NAS (le disque réseau de la maison) : la bibliothèque classée arrive au bon endroit sans occuper de place sur votre ordinateur.',
    diagram: {
      alt: 'Schéma : vos photos sur l’ordinateur sont triées en local (Photosort calcule le plan de classement), puis envoyées par rsync vers le NAS, dans des dossiers année puis mois.',
      mac: 'Vos photos',
      sort: '1. Tri calculé en local',
      rsync: '2. Envoi par rsync',
      nas: 'Votre NAS',
      local: 'sur votre ordinateur',
      sent: 'AAAA / AAAA-MM',
    },
    items: [
      { title: 'Le tri reste local', text: 'Photosort lit vos photos et décide où chacune doit aller, exactement comme pour un dossier normal. Rien n’est copié sur votre ordinateur : seul le plan de classement est préparé, puis envoyé.' },
      { title: 'Reprise automatique', text: 'Si la connexion se coupe (Wi-Fi, veille, câble débranché), rien n’est perdu : un message l’indique et le bouton « Reprendre » relance l’envoi, qui repart là où il s’était arrêté. Les fichiers déjà arrivés ne sont pas renvoyés.' },
      { title: 'Vérification optionnelle', text: 'L’option « Vérifier l’intégrité (checksum) après l’envoi » compare chaque fichier avec celui du NAS. C’est plus lent, mais c’est la preuve que la copie est identique à l’original.' },
      { title: 'Jamais d’écrasement', text: 'Ce qui existe déjà sur le NAS n’est jamais réécrit. Si un fichier du même nom mais au contenu différent s’y trouve déjà, il est laissé tel quel et simplement signalé.' },
    ],
  },
  prereq: {
    title: 'Avant de commencer',
    qnap: {
      title: 'Sur un NAS QNAP : activer le serveur rsync',
      intro: 'C’est le mode recommandé : le NAS se comporte comme un « serveur rsync » protégé par un nom d’utilisateur et un mot de passe. À faire une seule fois, dans l’interface web de votre QNAP (QTS) :',
      steps: [
        'Ouvrez l’interface d’administration du NAS dans votre navigateur et connectez-vous en administrateur.',
        'Ouvrez la configuration du serveur rsync (selon la version de QTS : « Panneau de configuration », « Backup Station » ou « Gestionnaire de sauvegarde », rubrique « Serveur Rsync ») et activez-le. Notez le port : 873 par défaut.',
        'Créez (ou autorisez) un utilisateur rsync avec un mot de passe, puis donnez-lui le droit d’écrire dans le dossier partagé qui recevra vos photos (par exemple « Photos »).',
        'Vérifiez le nom du module : il correspond au nom du dossier partagé exposé par le serveur rsync. C’est ce nom exact qu’il faudra saisir dans Photosort, majuscules comprises.',
        'Gardez sous la main : l’adresse du NAS (par exemple nas.local ou 192.168.1.20), le nom d’utilisateur, le mot de passe et le nom du module.',
      ],
      note: 'Les intitulés exacts changent d’une version de QTS à l’autre : cherchez « rsync » dans la recherche de l’interface si vous ne trouvez pas la page.',
    },
    ssh: {
      title: 'Alternative : rsync via SSH',
      text: 'Si votre NAS (ou un autre serveur) accepte les connexions SSH, Photosort sait aussi envoyer les photos par rsync à travers SSH. Il utilise alors votre clé SSH ou votre agent SSH : aucun mot de passe n’est demandé, et on indique le dossier du NAS plutôt qu’un module (par exemple /share/Photos). Ce mode s’adresse plutôt aux utilisateurs à l’aise avec SSH.',
    },
    mac: {
      title: 'Sur votre ordinateur : rsync',
      text: 'Photosort s’appuie sur le programme rsync. Linux en dispose en général ; sur macOS, Apple fournit une ancienne version (openrsync) qui fonctionne, mais dont la progression est moins précise. Pour une version récente, installez rsync avec Homebrew, dans le Terminal :',
      check: 'Vous n’avez pas Homebrew ? Voyez brew.sh pour l’installer. Si rsync est introuvable, Photosort le dit clairement (voir le dépannage).',
      other: 'Linux : sudo apt install rsync. Windows : installez rsync, par exemple via WSL ou Git for Windows.',
    },
  },
  steps: {
    title: 'Pas à pas dans Photosort',
    lead: 'Tout se règle dans une seule fenêtre.',
    open: 'Dans la zone de destination de l’application, cliquez sur le lien « ☁️ Destination distante (NAS)… ». La fenêtre « Destination distante (NAS) » s’ouvre. Cochez ensuite « Envoyer le tri vers un NAS (rsync) » : sans cette case, les champs restent grisés. Voici chaque champ :',
    shotCaption: 'La fenêtre « Destination distante (NAS) »',
    shotAlt: 'Capture de la fenêtre « Destination distante (NAS) » de Photosort.',
    pending: 'Capture à venir',
    fields: [
      { label: 'Mode de connexion', text: 'Choisissez « Serveur rsync du NAS (module + mot de passe) » pour le cas QNAP décrit plus haut, ou « rsync via SSH (clé ou agent SSH) » pour l’alternative SSH.' },
      { label: 'Adresse du NAS', text: 'Le nom ou l’adresse IP du NAS, par exemple « nas.local » ou « 192.168.1.20 ». À côté, le champ « Port » peut rester vide : Photosort utilise alors le port habituel (873 pour le serveur rsync, 22 pour SSH). Ne le remplissez que si votre NAS utilise un autre port.' },
      { label: 'Utilisateur', text: 'Le nom de l’utilisateur rsync (ou SSH) créé sur le NAS.' },
      { label: 'Module rsync', text: 'Mode serveur rsync uniquement. Le nom du module exposé par le NAS, par exemple « Photos ». Il doit être identique à celui du NAS.' },
      { label: 'Sous-dossier dans le module (facultatif)', text: 'Si vous voulez ranger la bibliothèque dans un sous-dossier du module, indiquez-le ici. Laissez vide pour envoyer à la racine du module. Si le dossier n’existe pas encore, il est créé lors de l’envoi. En mode SSH, ce champ s’appelle « Dossier sur le NAS » (par exemple /share/Photos).' },
      { label: 'Mot de passe', text: 'Le mot de passe de l’utilisateur rsync. Cochez « Mémoriser le mot de passe de façon sécurisée (trousseau du système) » pour ne pas le ressaisir : il est alors gardé dans le trousseau de votre ordinateur. Sinon, il est demandé à chaque lancement et n’est jamais enregistré. Si le stockage sécurisé n’est pas disponible sur votre système, l’application le précise et le mot de passe est à saisir à chaque fois.' },
      { label: 'Clé SSH (facultatif)', text: 'Mode SSH uniquement, à la place du mot de passe : choisissez votre fichier de clé, ou laissez vide pour utiliser votre agent SSH.' },
      { label: 'Vérifier l’intégrité (checksum) après l’envoi', text: 'Optionnel. Plus lent : compare chaque fichier avec celui du NAS. Un fichier différent déjà présent n’est jamais écrasé, il est seulement signalé.' },
    ],
    sortTitle: 'Lancer le tri',
    sortText: 'Fermez la fenêtre, choisissez vos dossiers sources, puis lancez l’analyse comme d’habitude : la destination affichée est maintenant celle du NAS, et le bouton « Revenir au dossier local » permet de repartir sur un dossier de l’ordinateur. Pendant l’envoi, la barre d’état indique « Envoi vers le NAS » avec la progression ; à la fin : « photos sur le NAS (envoyées, déjà présentes) ».',
  },
  test: {
    title: 'Tester la connexion',
    text: 'Avant de trier quoi que ce soit, cliquez sur « Tester la connexion ». Photosort demande simplement au NAS la liste de ce qu’il voit (sans rien écrire). Le bouton reste grisé tant que l’adresse et le module (ou, en mode SSH, le dossier) ne sont pas remplis.',
    ok: '✅ Connexion réussie (n éléments visibles).',
    pathMissing: '✅ Connexion réussie. Le dossier n’existe pas encore : il sera créé lors de l’envoi.',
    error: 'En cas de problème, un message en français vous dit quoi vérifier : voir le dépannage ci-dessous.',
  },
  dry: {
    title: 'Simuler l’envoi',
    text: 'Le bouton « Simuler l’envoi » montre ce qui serait envoyé, sans rien écrire sur le NAS. C’est le bon réflexe pour un premier essai ou après avoir changé de dossier de destination.',
    result: 'À la fin, l’application indique : « Simulation terminée : X fichiers seraient envoyés, Y déjà présents. Rien n’a été écrit. »',
    then: 'Si le résultat vous convient, lancez le vrai tri.',
  },
  safety: {
    title: 'Ce qui est garanti',
    items: [
      { title: 'Vos originaux ne bougent pas', text: 'Photosort ne modifie ni ne supprime vos photos d’origine.' },
      { title: 'Rien n’est réécrit sur le NAS', text: 'Un fichier déjà présent est ignoré ; s’il est différent, il est signalé : « n fichier(s) existent déjà sur le NAS avec un contenu différent (non écrasés) ».' },
      { title: 'Le mot de passe reste discret', text: 'Il n’apparaît dans aucun message d’erreur et n’est enregistré que si vous cochez la mémorisation sécurisée.' },
    ],
  },
  trouble: {
    title: 'Dépannage',
    lead: 'Quand quelque chose ne va pas, Photosort affiche un message en clair, parfois suivi d’un détail technique entre parenthèses. Voici les plus courants, avec ce qu’il faut faire.',
    columns: { message: 'Message dans l’application', fix: 'Que faire' },
    items: [
      { message: 'Identifiants refusés : mot de passe ou nom d’utilisateur incorrect (ou clé SSH non acceptée).', fix: 'Ressaisissez le mot de passe (attention aux majuscules et aux espaces) et vérifiez le nom d’utilisateur. Si vous venez de changer le mot de passe sur le NAS, mettez-le aussi à jour dans Photosort. En mode SSH, vérifiez que la clé est bien autorisée sur le NAS.' },
      { message: 'Le module rsync indiqué n’existe pas sur le NAS. Vérifiez son nom dans le service rsync du NAS.', fix: 'Le nom du module doit être identique à celui du NAS, majuscules comprises (« Photos » n’est pas « photos »). Relisez-le dans la configuration du serveur rsync du QNAP.' },
      { message: 'Adresse du NAS introuvable. Vérifiez son nom ou son adresse IP et que vous êtes sur le même réseau.', fix: 'Essayez l’adresse IP du NAS plutôt que son nom, et assurez-vous que l’ordinateur est sur le même réseau (même box, pas un Wi-Fi invité ni un VPN).' },
      { message: 'Le NAS ne répond pas (éteint, hors réseau ou bloqué par un pare-feu).', fix: 'Vérifiez que le NAS est allumé et branché au réseau, puis réessayez. Un pare-feu peut aussi bloquer le port 873 (22 pour SSH).' },
      { message: 'Connexion refusée : le service rsync n’est pas démarré sur le NAS, ou le port est incorrect (873 pour rsync, 22 pour SSH).', fix: 'Activez le serveur rsync dans l’interface du NAS (voir « Avant de commencer ») et vérifiez le port. Cette erreur est reprenable : une fois réglée, relancez avec « Reprendre ».' },
      { message: 'Accès refusé à ce module ou dossier : cet utilisateur n’a pas le droit d’y écrire, ou votre adresse n’est pas autorisée.', fix: 'Sur le NAS, donnez à l’utilisateur le droit d’écriture sur le dossier, et vérifiez que le serveur rsync accepte l’adresse de votre ordinateur.' },
      { message: 'rsync est introuvable sur cet ordinateur. Installez-le (macOS : brew install rsync ; …).', fix: 'Installez rsync (voir « Sur votre ordinateur : rsync »), puis relancez Photosort.' },
      { message: 'Ce Mac utilise l’ancien rsync d’Apple (openrsync) : l’envoi fonctionne mais la progression est moins précise. Pour mieux : brew install rsync', fix: 'Ce n’est pas une erreur : l’envoi fonctionne. Pour une progression précise, installez un rsync récent avec Homebrew (brew install rsync).' },
      { message: 'L’envoi a été interrompu (connexion coupée). … Relancez « Reprendre » : l’envoi repart là où il s’est arrêté.', fix: 'Rétablissez la connexion, puis cliquez sur « Reprendre ». Seuls les fichiers manquants sont envoyés.' },
      { message: 'Le NAS n’a plus assez d’espace libre.', fix: 'Libérez de la place sur le NAS (ou changez de volume), puis relancez.' },
      { message: 'L’identité SSH du NAS a changé ou n’est pas reconnue. …', fix: 'Mode SSH uniquement. Si vous n’avez pas réinstallé le NAS, soyez prudent ; sinon supprimez l’ancienne entrée du NAS dans le fichier known_hosts.' },
    ],
  },
  faq: {
    title: 'Questions fréquentes',
    items: [
      { q: 'Dois-je avoir de la place sur mon ordinateur pour trier vers le NAS ?', a: 'Non. Photosort ne fait pas de copie intermédiaire des photos : il prépare un plan de classement en local, puis les envoie telles quelles. Il vous faut simplement de la place sur le NAS.' },
      { q: 'Que se passe-t-il si je relance un envoi déjà terminé ?', a: 'Rien de plus : les fichiers déjà présents sur le NAS sont ignorés. Le message final indique combien ont été envoyés et combien étaient déjà là.' },
      { q: 'Photosort peut-il écraser ou supprimer des photos du NAS ?', a: 'Non. Il n’écrase jamais un fichier existant et ne supprime rien sur le NAS.' },
      { q: 'Faut-il activer la vérification par checksum ?', a: 'Pas obligatoirement. Elle rassure pour une grosse bibliothèque importante mais rallonge l’opération. Un fichier différent est signalé, jamais remplacé.' },
      { q: 'Mon NAS n’est pas un QNAP, est-ce possible ?', a: 'Oui, si votre NAS propose un serveur rsync (module + mot de passe) ou l’accès SSH. Les réglages sont les mêmes ; seule l’interface d’administration diffère. Ce guide ne détaille que QNAP.' },
      { q: 'Y a-t-il une version en ligne de commande ?', a: 'Oui : l’outil « photosort » accepte --remote rsync://utilisateur@hote/module (ou ssh://…), avec --remote-test, --remote-dry-run et --remote-verify. Le mot de passe se fournit par un fichier (--remote-password-file) ou la variable PHOTOSORT_REMOTE_PASSWORD, jamais en clair sur la ligne de commande.' },
      { q: 'Les tags de l’IA fonctionnent-ils avec un NAS ?', a: 'Pas pour l’instant : les tags s’écrivent à côté des photos locales et ne se combinent pas avec une destination distante.' },
    ],
  },
  cli: { title: 'Pour les utilisateurs avancés', text: 'Exemple en ligne de commande :' },
};

const en: NasCopy = {
  meta: {
    title: 'Photosort — Sort your photos straight to a NAS (step-by-step guide)',
    description:
      'How to send your sorted photos to a NAS (QNAP) with Photosort: the sorting is computed on your computer, then sent with rsync, with automatic resume, optional checksum verification and no overwriting. Step-by-step guide and troubleshooting.',
  },
  hero: {
    eyebrow: 'Guide · Remote destination',
    title: 'Remote sorting to a NAS',
    lead: 'Photosort can sort your photos and send them straight to the NAS at home, without copying them to your computer first. This guide explains how to set it up, step by step, without jargon.',
    back: '← Back to home',
  },
  toc: {
    label: 'On this page',
    items: [
      { id: 'pourquoi', label: 'What it does' },
      { id: 'prerequis', label: 'Before you start' },
      { id: 'pas-a-pas', label: 'Step by step' },
      { id: 'tester', label: 'Test the connection' },
      { id: 'simuler', label: 'Simulate' },
      { id: 'depannage', label: 'Troubleshooting' },
      { id: 'faq', label: 'FAQ' },
    ],
  },
  why: {
    title: 'What this feature does, and why',
    lead: 'Usually Photosort files your photos into a folder on your computer. With the remote destination it does the same sorting, but the result goes straight to the NAS (the network drive at home): the sorted library lands in the right place without using space on your computer.',
    diagram: {
      alt: 'Diagram: your photos on the computer are sorted locally (Photosort works out the filing plan), then sent with rsync to the NAS, into year then month folders.',
      mac: 'Your photos',
      sort: '1. Sorting computed locally',
      rsync: '2. Sent with rsync',
      nas: 'Your NAS',
      local: 'on your computer',
      sent: 'YYYY / YYYY-MM',
    },
    items: [
      { title: 'Sorting stays local', text: 'Photosort reads your photos and decides where each one goes, exactly as for a normal folder. Nothing is copied to your computer: only the filing plan is prepared, then sent.' },
      { title: 'Automatic resume', text: 'If the connection drops (Wi-Fi, sleep, unplugged cable), nothing is lost: a message says so and the “Resume” button restarts sending, which continues where it stopped. Files that already arrived are not sent again.' },
      { title: 'Optional verification', text: 'The “Verify integrity (checksum) after sending” option compares every file with the one on the NAS. It is slower, but proves the copy is identical to the original.' },
      { title: 'Never overwrites', text: 'What already exists on the NAS is never rewritten. If a file with the same name but different content is already there, it is left untouched and simply reported.' },
    ],
  },
  prereq: {
    title: 'Before you start',
    qnap: {
      title: 'On a QNAP NAS: turn on the rsync server',
      intro: 'This is the recommended mode: the NAS acts as an “rsync server” protected by a user name and a password. Do it once, in your QNAP’s web interface (QTS):',
      steps: [
        'Open the NAS administration interface in your browser and sign in as administrator.',
        'Open the rsync server settings (depending on the QTS version: “Control Panel”, “Backup Station” or “Backup Manager”, “Rsync Server” section) and turn it on. Note the port: 873 by default.',
        'Create (or allow) an rsync user with a password, then give it permission to write to the shared folder that will receive your photos (for example “Photos”).',
        'Check the module name: it matches the name of the shared folder exposed by the rsync server. This exact name is what you will type in Photosort, capitals included.',
        'Keep at hand: the NAS address (for example nas.local or 192.168.1.20), the user name, the password and the module name.',
      ],
      note: 'The exact labels change between QTS versions: search for “rsync” in the interface search if you cannot find the page.',
    },
    ssh: {
      title: 'Alternative: rsync over SSH',
      text: 'If your NAS (or another server) accepts SSH connections, Photosort can also send the photos with rsync through SSH. It then uses your SSH key or SSH agent: no password is asked, and you give a folder on the NAS instead of a module (for example /share/Photos). This mode is meant for users comfortable with SSH.',
    },
    mac: {
      title: 'On your computer: rsync',
      text: 'Photosort relies on the rsync program. Linux usually has it; on macOS, Apple ships an old version (openrsync) that works but reports progress less precisely. For a recent version, install rsync with Homebrew, in Terminal:',
      check: 'No Homebrew yet? See brew.sh to install it. If rsync cannot be found, Photosort says so clearly (see troubleshooting).',
      other: 'Linux: sudo apt install rsync. Windows: install rsync, for example via WSL or Git for Windows.',
    },
  },
  steps: {
    title: 'Step by step in Photosort',
    lead: 'Everything is set in a single window.',
    open: 'In the destination area of the app, click the “☁️ Remote destination (NAS)…” link. The “Remote destination (NAS)” window opens. Then tick “Send the sorted photos to a NAS (rsync)”: without it the fields stay greyed out. Here is each field:',
    shotCaption: 'The “Remote destination (NAS)” window',
    shotAlt: 'Screenshot of Photosort’s “Remote destination (NAS)” window.',
    pending: 'Screenshot coming soon',
    fields: [
      { label: 'Connection mode', text: 'Choose “NAS rsync server (module + password)” for the QNAP case described above, or “rsync over SSH (key or SSH agent)” for the SSH alternative.' },
      { label: 'NAS address', text: 'The NAS name or IP address, for example “nas.local” or “192.168.1.20”. Next to it, the “Port” field can stay empty: Photosort then uses the usual port (873 for the rsync server, 22 for SSH). Only fill it in if your NAS uses another port.' },
      { label: 'User', text: 'The name of the rsync (or SSH) user created on the NAS.' },
      { label: 'rsync module', text: 'rsync server mode only. The name of the module exposed by the NAS, for example “Photos”. It must be identical to the one on the NAS.' },
      { label: 'Folder inside the module (optional)', text: 'If you want to keep the library in a subfolder of the module, enter it here. Leave empty to send to the module root. If the folder does not exist yet, it is created when sending. In SSH mode this field is called “Folder on the NAS” (for example /share/Photos).' },
      { label: 'Password', text: 'The rsync user’s password. Tick “Remember the password securely (system keychain)” to avoid typing it again: it is then kept in your computer’s keychain. Otherwise it is asked every time and never saved. If secure storage is not available on your system, the app says so and the password must be typed every time.' },
      { label: 'SSH key (optional)', text: 'SSH mode only, instead of the password: choose your key file, or leave empty to use your SSH agent.' },
      { label: 'Verify integrity (checksum) after sending', text: 'Optional. Slower: compares every file with the one on the NAS. A different file already there is never overwritten, only reported.' },
    ],
    sortTitle: 'Start the sort',
    sortText: 'Close the window, pick your source folders, then start the analysis as usual: the destination shown is now the NAS, and the “Back to a local folder” button takes you back to a folder on the computer. While sending, the status bar shows “Sending to the NAS” with progress; at the end: “photos on the NAS (sent, already there)”.',
  },
  test: {
    title: 'Test the connection',
    text: 'Before sorting anything, click “Test connection”. Photosort simply asks the NAS for the list of what it can see (without writing anything). The button stays greyed out until the address and the module (or, in SSH mode, the folder) are filled in.',
    ok: '✅ Connection successful (n items visible).',
    pathMissing: '✅ Connection successful. The folder does not exist yet: it will be created when sending.',
    error: 'If something is wrong, a plain-language message tells you what to check: see troubleshooting below.',
  },
  dry: {
    title: 'Simulate the sending',
    text: 'The “Simulate sending” button shows what would be sent, without writing anything to the NAS. It is a good habit for a first try or after changing the destination folder.',
    result: 'At the end, the app says: “Simulation finished: X files would be sent, Y already there. Nothing was written.”',
    then: 'If the result looks right, start the real sort.',
  },
  safety: {
    title: 'What is guaranteed',
    items: [
      { title: 'Your originals do not move', text: 'Photosort never modifies or deletes your original photos.' },
      { title: 'Nothing is rewritten on the NAS', text: 'A file that is already there is skipped; if it differs, it is reported: “n file(s) already exist on the NAS with different content (not overwritten)”.' },
      { title: 'The password stays private', text: 'It appears in no error message and is only saved if you tick secure remembering.' },
    ],
  },
  trouble: {
    title: 'Troubleshooting',
    lead: 'When something goes wrong, Photosort shows a plain message, sometimes followed by a technical detail in brackets. Here are the most common ones, and what to do.',
    columns: { message: 'Message in the app', fix: 'What to do' },
    items: [
      { message: 'Credentials refused: wrong password or user name (or SSH key not accepted).', fix: 'Type the password again (mind capitals and spaces) and check the user name. If you just changed the password on the NAS, update it in Photosort too. In SSH mode, check that the key is authorised on the NAS.' },
      { message: 'The rsync module does not exist on the NAS. Check its name in the NAS rsync service.', fix: 'The module name must be identical to the one on the NAS, capitals included (“Photos” is not “photos”). Read it again in the QNAP rsync server settings.' },
      { message: 'NAS address not found. Check its name or IP address and that you are on the same network.', fix: 'Try the NAS IP address instead of its name, and make sure the computer is on the same network (same router, not a guest Wi-Fi or a VPN).' },
      { message: 'The NAS is not responding (turned off, off the network or blocked by a firewall).', fix: 'Check that the NAS is on and plugged into the network, then try again. A firewall may also block port 873 (22 for SSH).' },
      { message: 'Connection refused: the rsync service is not running on the NAS, or the port is wrong (873 for rsync, 22 for SSH).', fix: 'Turn on the rsync server in the NAS interface (see “Before you start”) and check the port. This error is resumable: once fixed, restart with “Resume”.' },
      { message: 'Access denied to this module or folder: this user may not write there, or your address is not allowed.', fix: 'On the NAS, give the user write permission on the folder, and check that the rsync server accepts your computer’s address.' },
      { message: 'rsync was not found on this computer. Install it (macOS: brew install rsync; …).', fix: 'Install rsync (see “On your computer: rsync”), then restart Photosort.' },
      { message: 'This Mac uses Apple’s old rsync (openrsync): sending works but progress is less precise. For better results: brew install rsync', fix: 'This is not an error: sending works. For precise progress, install a recent rsync with Homebrew (brew install rsync).' },
      { message: 'Sending was interrupted (connection lost). … Press “Resume”: sending continues where it stopped.', fix: 'Restore the connection, then click “Resume”. Only the missing files are sent.' },
      { message: 'The NAS has no free space left.', fix: 'Free up space on the NAS (or pick another volume), then start again.' },
      { message: 'The SSH identity of the NAS changed or is not recognised. …', fix: 'SSH mode only. If you did not reinstall the NAS, be careful; otherwise remove the old NAS entry from the known_hosts file.' },
    ],
  },
  faq: {
    title: 'Frequently asked questions',
    items: [
      { q: 'Do I need free space on my computer to sort to the NAS?', a: 'No. Photosort makes no intermediate copy of the photos: it prepares a filing plan locally, then sends them as they are. You only need free space on the NAS.' },
      { q: 'What happens if I run a sending that already finished?', a: 'Nothing more: files already on the NAS are skipped. The final message says how many were sent and how many were already there.' },
      { q: 'Can Photosort overwrite or delete photos on the NAS?', a: 'No. It never overwrites an existing file and deletes nothing on the NAS.' },
      { q: 'Should I turn on the checksum verification?', a: 'Not necessarily. It is reassuring for a large, important library but makes the operation longer. A different file is reported, never replaced.' },
      { q: 'My NAS is not a QNAP, is it possible?', a: 'Yes, if your NAS offers an rsync server (module + password) or SSH access. The settings are the same; only the administration interface differs. This guide only details QNAP.' },
      { q: 'Is there a command-line version?', a: 'Yes: the “photosort” tool accepts --remote rsync://user@host/module (or ssh://…), with --remote-test, --remote-dry-run and --remote-verify. The password is given through a file (--remote-password-file) or the PHOTOSORT_REMOTE_PASSWORD variable, never in clear on the command line.' },
      { q: 'Do AI tags work with a NAS?', a: 'Not for now: tags are written next to local photos and cannot be combined with a remote destination.' },
    ],
  },
  cli: { title: 'For advanced users', text: 'Command-line example:' },
};

const da: NasCopy = {
  meta: {
    title: 'Photosort — Sortér dine billeder direkte til et NAS (trin for trin)',
    description:
      'Sådan sender du sorterede billeder til et NAS (QNAP) med Photosort: sorteringen beregnes lokalt og sendes med rsync, med automatisk genoptagelse, valgfri checksum-kontrol og uden overskrivning. Vejledning og fejlfinding.',
  },
  hero: {
    eyebrow: 'Vejledning · Fjerndestination',
    title: 'Fjernsortering til et NAS',
    lead: 'Photosort kan sortere dine billeder og sende dem direkte til NAS’et derhjemme, uden først at kopiere dem til din computer. Her er en enkel vejledning, trin for trin og uden fagsprog.',
    back: '← Tilbage til forsiden',
  },
  toc: {
    label: 'På denne side',
    items: [
      { id: 'pourquoi', label: 'Hvad den gør' },
      { id: 'prerequis', label: 'Før du går i gang' },
      { id: 'pas-a-pas', label: 'Trin for trin' },
      { id: 'tester', label: 'Test forbindelsen' },
      { id: 'simuler', label: 'Simulér' },
      { id: 'depannage', label: 'Fejlfinding' },
      { id: 'faq', label: 'FAQ' },
    ],
  },
  why: {
    title: 'Hvad funktionen gør, og hvorfor',
    lead: 'Normalt lægger Photosort dine billeder i en mappe på din computer. Med fjerndestinationen sorterer den på samme måde, men resultatet sendes direkte til NAS’et (netværksdisken derhjemme): det sorterede bibliotek havner det rigtige sted uden at fylde på din computer.',
    diagram: {
      alt: 'Diagram: dine billeder på computeren sorteres lokalt (Photosort beregner placeringsplanen) og sendes derefter med rsync til NAS’et i mapper for år og måned.',
      mac: 'Dine billeder',
      sort: '1. Sortering beregnet lokalt',
      rsync: '2. Sendt med rsync',
      nas: 'Dit NAS',
      local: 'på din computer',
      sent: 'ÅÅÅÅ / ÅÅÅÅ-MM',
    },
    items: [
      { title: 'Sorteringen er lokal', text: 'Photosort læser dine billeder og afgør, hvor hvert enkelt skal hen, præcis som for en almindelig mappe. Intet kopieres til din computer: kun placeringsplanen forberedes og sendes.' },
      { title: 'Automatisk genoptagelse', text: 'Hvis forbindelsen afbrydes (Wi-Fi, dvale, kabel trukket ud), går intet tabt: en besked fortæller det, og knappen “Genoptag” starter afsendelsen igen, hvor den stoppede. Filer, der allerede er nået frem, sendes ikke igen.' },
      { title: 'Valgfri kontrol', text: 'Valgmuligheden “Kontrollér integritet (checksum) efter afsendelse” sammenligner hver fil med den på NAS’et. Det er langsommere, men beviser, at kopien er identisk med originalen.' },
      { title: 'Aldrig overskrivning', text: 'Det, der allerede findes på NAS’et, skrives aldrig om. Hvis en fil med samme navn men andet indhold allerede er der, lades den være og bliver blot rapporteret.' },
    ],
  },
  prereq: {
    title: 'Før du går i gang',
    qnap: {
      title: 'På et QNAP NAS: slå rsync-serveren til',
      intro: 'Det er den anbefalede tilstand: NAS’et fungerer som en “rsync-server” beskyttet af brugernavn og adgangskode. Gøres én gang i QNAP’ens webgrænseflade (QTS):',
      steps: [
        'Åbn NAS’ets administrationsgrænseflade i din browser, og log ind som administrator.',
        'Åbn indstillingerne for rsync-serveren (afhængigt af QTS-versionen: “Kontrolpanel”, “Backup Station” eller “Backup Manager”, afsnittet “Rsync Server”), og slå den til. Notér porten: 873 som standard.',
        'Opret (eller tillad) en rsync-bruger med adgangskode, og giv brugeren skriveadgang til den delte mappe, der skal modtage dine billeder (fx “Photos”).',
        'Kontrollér modulnavnet: det svarer til navnet på den delte mappe, som rsync-serveren udstiller. Det præcise navn skal du indtaste i Photosort, med store og små bogstaver.',
        'Hav ved hånden: NAS’ets adresse (fx nas.local eller 192.168.1.20), brugernavnet, adgangskoden og modulnavnet.',
      ],
      note: 'De præcise betegnelser ændrer sig mellem QTS-versioner: søg efter “rsync” i grænsefladens søgefelt, hvis du ikke finder siden.',
    },
    ssh: {
      title: 'Alternativ: rsync via SSH',
      text: 'Hvis dit NAS (eller en anden server) accepterer SSH-forbindelser, kan Photosort også sende billederne med rsync gennem SSH. Den bruger så din SSH-nøgle eller SSH-agent: der spørges ikke om adgangskode, og du angiver en mappe på NAS’et i stedet for et modul (fx /share/Photos). Tilstanden er til brugere, der er fortrolige med SSH.',
    },
    mac: {
      title: 'På din computer: rsync',
      text: 'Photosort bruger programmet rsync. Linux har det som regel; på macOS leverer Apple en gammel version (openrsync), som virker, men viser fremdriften mindre præcist. For en nyere version installerer du rsync med Homebrew i Terminal:',
      check: 'Har du ikke Homebrew? Se brew.sh. Hvis rsync ikke findes, siger Photosort det tydeligt (se fejlfinding).',
      other: 'Linux: sudo apt install rsync. Windows: installér rsync, fx via WSL eller Git for Windows.',
    },
  },
  steps: {
    title: 'Trin for trin i Photosort',
    lead: 'Alt indstilles i ét vindue.',
    open: 'I programmets destinationsområde klikker du på linket “☁️ Fjerndestination (NAS)…”. Vinduet “Fjerndestination (NAS)” åbner. Sæt derefter kryds i “Send de sorterede billeder til et NAS (rsync)”: uden det er felterne nedtonede. Her er hvert felt:',
    shotCaption: 'Vinduet “Fjerndestination (NAS)”',
    shotAlt: 'Skærmbillede af Photosorts vindue “Fjerndestination (NAS)”.',
    pending: 'Skærmbillede følger',
    fields: [
      { label: 'Forbindelsestype', text: 'Vælg “NAS’ets rsync-server (modul + adgangskode)” til QNAP-tilfældet ovenfor, eller “rsync via SSH (nøgle eller SSH-agent)” til SSH-alternativet.' },
      { label: 'NAS-adresse', text: 'NAS’ets navn eller IP-adresse, fx “nas.local” eller “192.168.1.20”. Ved siden af kan feltet “Port” stå tomt: Photosort bruger så den sædvanlige port (873 til rsync-serveren, 22 til SSH). Udfyld det kun, hvis dit NAS bruger en anden port.' },
      { label: 'Bruger', text: 'Navnet på den rsync- (eller SSH-) bruger, du oprettede på NAS’et.' },
      { label: 'rsync-modul', text: 'Kun i rsync-servertilstand. Navnet på det modul, NAS’et udstiller, fx “Photos”. Det skal være identisk med det på NAS’et.' },
      { label: 'Mappe i modulet (valgfri)', text: 'Hvis biblioteket skal ligge i en undermappe i modulet, angiv den her. Lad feltet stå tomt for at sende til modulets rod. Findes mappen ikke endnu, oprettes den ved afsendelse. I SSH-tilstand hedder feltet “Mappe på NAS’et” (fx /share/Photos).' },
      { label: 'Adgangskode', text: 'rsync-brugerens adgangskode. Sæt kryds i “Husk adgangskoden sikkert (systemets nøglering)” for ikke at skulle skrive den igen: den gemmes så i computerens nøglering. Ellers spørges der hver gang, og den gemmes aldrig. Hvis sikker lagring ikke er tilgængelig, fortæller programmet det, og adgangskoden skal indtastes hver gang.' },
      { label: 'SSH-nøgle (valgfri)', text: 'Kun SSH-tilstand, i stedet for adgangskoden: vælg din nøglefil, eller lad feltet stå tomt for at bruge din SSH-agent.' },
      { label: 'Kontrollér integritet (checksum) efter afsendelse', text: 'Valgfri. Langsommere: sammenligner hver fil med den på NAS’et. En anden fil, der allerede findes, overskrives aldrig, men rapporteres.' },
    ],
    sortTitle: 'Start sorteringen',
    sortText: 'Luk vinduet, vælg dine kildemapper, og start analysen som sædvanligt: destinationen er nu NAS’et, og knappen “Tilbage til lokal mappe” fører dig tilbage til en mappe på computeren. Under afsendelsen viser statuslinjen “Sender til NAS’et” med fremdrift; til sidst: “billeder på NAS’et (sendt, fandtes allerede)”.',
  },
  test: {
    title: 'Test forbindelsen',
    text: 'Før du sorterer noget, klik på “Test forbindelsen”. Photosort beder blot NAS’et om listen over det, det kan se (uden at skrive noget). Knappen er nedtonet, indtil adresse og modul (eller i SSH-tilstand mappen) er udfyldt.',
    ok: '✅ Forbindelsen virker (n elementer synlige).',
    pathMissing: '✅ Forbindelsen virker. Mappen findes ikke endnu: den oprettes ved afsendelse.',
    error: 'Hvis noget er galt, fortæller en forståelig besked, hvad du skal kontrollere: se fejlfinding nedenfor.',
  },
  dry: {
    title: 'Simulér afsendelsen',
    text: 'Knappen “Simulér afsendelse” viser, hvad der ville blive sendt, uden at skrive noget til NAS’et. Det er en god vane ved første forsøg eller efter skift af destinationsmappe.',
    result: 'Til sidst siger programmet: “Simulering færdig: X filer ville blive sendt, Y findes allerede. Intet blev skrevet.”',
    then: 'Ser resultatet rigtigt ud, så start den rigtige sortering.',
  },
  safety: {
    title: 'Hvad der er garanteret',
    items: [
      { title: 'Dine originaler rører sig ikke', text: 'Photosort ændrer eller sletter aldrig dine originale billeder.' },
      { title: 'Intet overskrives på NAS’et', text: 'En fil, der allerede findes, springes over; er den anderledes, rapporteres den: “n fil(er) findes allerede på NAS’et med andet indhold (ikke overskrevet)”.' },
      { title: 'Adgangskoden forbliver diskret', text: 'Den vises i ingen fejlbesked og gemmes kun, hvis du vælger sikker lagring.' },
    ],
  },
  trouble: {
    title: 'Fejlfinding',
    lead: 'Når noget går galt, viser Photosort en forståelig besked, nogle gange efterfulgt af en teknisk detalje i parentes. Her er de hyppigste, og hvad du skal gøre.',
    columns: { message: 'Besked i programmet', fix: 'Hvad du skal gøre' },
    items: [
      { message: 'Loginoplysninger afvist: forkert adgangskode eller brugernavn (eller SSH-nøglen blev ikke accepteret).', fix: 'Skriv adgangskoden igen (pas på store bogstaver og mellemrum), og kontrollér brugernavnet. Har du lige ændret adgangskoden på NAS’et, så opdatér den også i Photosort. I SSH-tilstand: kontrollér, at nøglen er godkendt på NAS’et.' },
      { message: 'rsync-modulet findes ikke på NAS’et. Kontrollér navnet i NAS’ets rsync-tjeneste.', fix: 'Modulnavnet skal være identisk med det på NAS’et, med store og små bogstaver (“Photos” er ikke “photos”). Læs det igen i QNAP’ens rsync-serverindstillinger.' },
      { message: 'NAS-adressen blev ikke fundet. Kontrollér navn eller IP-adresse, og at du er på samme netværk.', fix: 'Prøv NAS’ets IP-adresse i stedet for navnet, og sørg for, at computeren er på samme netværk (samme router, ikke gæste-Wi-Fi eller VPN).' },
      { message: 'NAS’et svarer ikke (slukket, ikke på netværket eller blokeret af en firewall).', fix: 'Kontrollér, at NAS’et er tændt og tilsluttet netværket, og prøv igen. En firewall kan også blokere port 873 (22 til SSH).' },
      { message: 'Forbindelse afvist: rsync-tjenesten kører ikke på NAS’et, eller porten er forkert (873 for rsync, 22 for SSH).', fix: 'Slå rsync-serveren til i NAS’ets grænseflade (se “Før du går i gang”), og kontrollér porten. Fejlen kan genoptages: når den er rettet, start igen med “Genoptag”.' },
      { message: 'Adgang nægtet til dette modul eller denne mappe: brugeren må ikke skrive dér, eller din adresse er ikke tilladt.', fix: 'Giv brugeren skriveadgang til mappen på NAS’et, og kontrollér, at rsync-serveren accepterer din computers adresse.' },
      { message: 'rsync blev ikke fundet på denne computer. Installér det (macOS: brew install rsync; …).', fix: 'Installér rsync (se “På din computer: rsync”), og start Photosort igen.' },
      { message: 'Denne Mac bruger Apples gamle rsync (openrsync): afsendelse virker, men fremdriften er mindre præcis. Bedre: brew install rsync', fix: 'Det er ikke en fejl: afsendelsen virker. For præcis fremdrift installér en nyere rsync med Homebrew (brew install rsync).' },
      { message: 'Afsendelsen blev afbrudt (forbindelsen blev afbrudt). … Tryk “Genoptag”: afsendelsen fortsætter, hvor den stoppede.', fix: 'Genopret forbindelsen, og klik på “Genoptag”. Kun de manglende filer sendes.' },
      { message: 'NAS’et har ikke mere ledig plads.', fix: 'Frigør plads på NAS’et (eller vælg et andet volumen), og prøv igen.' },
      { message: 'NAS’ets SSH-identitet er ændret eller genkendes ikke. …', fix: 'Kun SSH-tilstand. Har du ikke nyinstalleret NAS’et, så vær forsigtig; ellers fjern NAS’ets gamle post i filen known_hosts.' },
    ],
  },
  faq: {
    title: 'Ofte stillede spørgsmål',
    items: [
      { q: 'Skal jeg have ledig plads på min computer for at sortere til NAS’et?', a: 'Nej. Photosort laver ingen mellemkopi af billederne: den forbereder en placeringsplan lokalt og sender dem, som de er. Du skal kun bruge plads på NAS’et.' },
      { q: 'Hvad sker der, hvis jeg kører en afsendelse, der allerede er færdig?', a: 'Intet mere: filer, der allerede er på NAS’et, springes over. Den afsluttende besked fortæller, hvor mange der blev sendt, og hvor mange der allerede var der.' },
      { q: 'Kan Photosort overskrive eller slette billeder på NAS’et?', a: 'Nej. Den overskriver aldrig en eksisterende fil og sletter intet på NAS’et.' },
      { q: 'Skal jeg slå checksum-kontrol til?', a: 'Ikke nødvendigvis. Den er betryggende til et stort, vigtigt bibliotek, men gør forløbet længere. En anden fil rapporteres, aldrig erstattet.' },
      { q: 'Mit NAS er ikke en QNAP, er det muligt?', a: 'Ja, hvis dit NAS tilbyder en rsync-server (modul + adgangskode) eller SSH-adgang. Indstillingerne er de samme; kun administrationsgrænsefladen er forskellig. Vejledningen beskriver kun QNAP.' },
      { q: 'Findes der en kommandolinjeversion?', a: 'Ja: værktøjet “photosort” accepterer --remote rsync://bruger@vært/modul (eller ssh://…), med --remote-test, --remote-dry-run og --remote-verify. Adgangskoden gives via en fil (--remote-password-file) eller variablen PHOTOSORT_REMOTE_PASSWORD, aldrig i klartekst på kommandolinjen.' },
      { q: 'Virker AI-tags med et NAS?', a: 'Ikke for øjeblikket: tags skrives ved siden af lokale billeder og kan ikke kombineres med en fjerndestination.' },
    ],
  },
  cli: { title: 'For avancerede brugere', text: 'Eksempel på kommandolinjen:' },
};

export const nasCopies: Record<Lang, NasCopy> = { fr, en, da };
