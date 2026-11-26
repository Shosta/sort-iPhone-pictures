import type { Lang } from './types';

export type ShotKey = 'hero' | 'tri' | 'iphone' | 'nas' | 'cartes' | 'doublons';

export interface HomeCopy {
  meta: { title: string; description: string };
  nav: { features: string; download: string; details: string };
  hero: { eyebrow: string; title: string; subtitle: string; note: string; alt: string; caption: string };
  sections: { id: string; shot: ShotKey; eyebrow: string; title: string; text: string; bullets: string[]; alt: string; caption: string }[];
  features: { eyebrow: string; title: string; items: { name: string; text: string }[] };
  free: { eyebrow: string; title: string; text: string; items: string[] };
  download: {
    eyebrow: string;
    title: string;
    lead: string;
    generic: string;
    forOs: { mac: string; windows: string; linux: string };
    soon: string;
    soonNote: string;
    others: string;
    platforms: { key: 'mac' | 'windows' | 'linux'; os: string; detail: string }[];
    source: string;
    pendingRepo: string;
    unsigned: string;
  };
  more: { title: string; text: string; cta: string };
}

export const home: Record<Lang, HomeCopy> = {
  fr: {
    meta: {
      title: 'Photosort – Oubliez le fastidieux travail de classement de vos photos',
      description:
        'Photosort classe vos photos par date et par lieu, sur disque ou NAS, sans cloud ni abonnement. Vos originaux ne sont jamais modifiés. Gratuit et open source.',
    },
    nav: { features: 'Fonctions', download: 'Télécharger', details: 'Présentation détaillée' },
    hero: {
      eyebrow: 'Open source · local · sans compte',
      title: 'Oubliez le fastidieux travail de classement de vos photos.',
      subtitle: 'Par date, par lieu, sur disque ou NAS. Sans cloud ni abonnement, et vos originaux ne sont jamais modifiés.',
      note: 'macOS · Windows · Linux',
      alt: 'Fenêtre de Photosort après un tri : les photos sont rangées en dossiers année et mois, avec le nom du lieu.',
      caption: 'Photosort après un tri',
    },
    sections: [
      {
        id: 'tri',
        shot: 'tri',
        eyebrow: 'Classement',
        title: 'Classées par date et par lieu, toutes seules.',
        text: 'Déposez vos dossiers : Photosort lit la date de prise de vue et range chaque photo dans AAAA / AAAA-MM. Si elle est géolocalisée, il ajoute la ville, voire le monument, grâce à une base de lieux embarquée.',
        bullets: ['Arborescence année / mois, lieu en option', 'Aperçu complet avant de trier', 'Vos originaux restent où ils sont'],
        alt: 'Photosort affiche les photos à trier à gauche et, à droite, l’arborescence prévue par année, mois et lieu.',
        caption: 'L’aperçu avant de trier',
      },
      {
        id: 'iphone',
        shot: 'iphone',
        eyebrow: 'iPhone',
        title: 'Importez directement depuis votre iPhone.',
        text: 'Photosort lit son dossier DCIM, vous choisissez de trier toutes vos photos, seulement les nouvelles photos ou les photos sur une période donnée. Bien entendu, Photosort reprend là où il s’était arrêté si le câble est débranché.',
        bullets: ['Toutes, nouvelles ou période choisie', 'Pause et reprise de l’import', 'Sans iCloud, sans Wi-Fi'],
        alt: 'Le panneau d’import affiche le dossier DCIM d’un iPhone, le nombre de photos à importer et les choix Tout, Nouvelles ou Période.',
        caption: 'Import depuis le dossier DCIM',
      },
      {
        id: 'nas',
        shot: 'nas',
        eyebrow: 'NAS',
        title: 'Envoyez le tri vers votre NAS.',
        text: 'Les photos sont triées comme d’habitude puis envoyées avec rsync, par le serveur rsync du NAS ou par SSH. Rien n’est copié en local, et ce qui est déjà sur le NAS n’est jamais écrasé.',
        bullets: ['Test de connexion et simulation avant l’envoi', 'Aucune suppression sur le NAS', 'Vérification d’intégrité en option'],
        alt: 'La fenêtre « Destination distante (NAS) » avec une adresse fictive nas.local, un utilisateur et un dossier partagé Photos.',
        caption: 'Destination distante (NAS)',
      },
      {
        id: 'cartes',
        shot: 'cartes',
        eyebrow: 'Souvenirs',
        title: 'Des cartes de voyage et des pages de scrapbook.',
        text: 'Photosort repère vos voyages et trace le trajet sur une carte à imprimer en PDF. Il prépare aussi des pages de semainier à découper et à coller dans votre carnet.',
        bullets: ['Carte de voyage et carte de l’année', 'Semainier en PDF, A4 ou Letter', 'Fonds de carte au choix'],
        alt: 'Un aperçu de carte de voyage : le trajet entre deux villes, avec une photo à chaque étape.',
        caption: 'Une carte de voyage prête à imprimer',
      },
      {
        id: 'doublons',
        shot: 'doublons',
        eyebrow: 'Ménage',
        title: 'Doublons, photos similaires et floues, mis de côté.',
        text: 'Les copies exactes ne sont pas recopiées. Les photos très proches et les photos floues sont rangées dans des dossiers à part, pour que vous décidiez quoi garder.',
        bullets: ['Doublons exacts ignorés', 'Quasi-doublons et photos floues à part', 'Rien n’est supprimé à votre place'],
        alt: 'La liste des signalements : des paires de miniatures identiques avec le nom du fichier concerné.',
        caption: 'Les doublons signalés',
      },
      {
        id: 'confidentialite',
        shot: 'hero',
        eyebrow: 'Vie privée',
        title: 'Tout se passe sur votre ordinateur.',
        text: 'Le classement, la recherche de lieux et la détection des doublons fonctionnent hors ligne. Aucun compte, aucun serveur, aucune statistique d’usage. Les photos sont copiées vers une nouvelle bibliothèque : vos originaux restent intacts.',
        bullets: ['Hors ligne, sans compte', 'Originaux jamais modifiés ni déplacés', 'Aucune fonction ne s’exécute dans le cloud'],
        alt: 'Bilan d’un tri terminé : 18 photos classées, avec les doublons ignorés signalés.',
        caption: 'Un tri terminé',
      },
    ],
    features: {
      eyebrow: 'Et aussi',
      title: 'Les petits détails qui font la différence.',
      items: [
        { name: 'Repli de date', text: 'Pas d’EXIF ? La date du nom de fichier, puis celle du fichier, prennent le relais.' },
        { name: 'Captures d’écran à part', text: 'Elles ne se mélangent pas à vos souvenirs : un dossier dédié.' },
        { name: 'Géocodage hors ligne', text: 'Villes et monuments viennent d’une base embarquée, sans requête réseau.' },
        { name: 'Aucun écrasement', text: 'Un fichier déjà présent n’est jamais remplacé ni supprimé.' },
        { name: 'Tags par IA', text: 'En option, des tags enregistrés dans des fichiers .xmp à côté des photos.' },
        { name: 'Reprise après coupure', text: 'L’import et l’envoi reprennent là où ils s’étaient arrêtés.' },
        { name: 'Français, anglais, danois', text: 'L’application et ce site existent dans les trois langues.' },
        { name: 'Huit thèmes', text: 'Choisissez l’ambiance de l’application, claire ou sombre.' },
      ],
    },
    free: {
      eyebrow: 'Gratuit',
      title: 'Gratuit, sans publicité, sans suivi.',
      text: 'Photosort est un logiciel libre sous licence MIT. Pas d’abonnement, pas de version payante, pas de compte à créer.',
      items: ['Sans publicité', 'Sans suivi ni statistiques', 'Code source ouvert'],
    },
    download: {
      eyebrow: 'Télécharger',
      title: 'Prêt à ranger vos photos ?',
      lead: 'Choisissez le paquet de votre système dans la dernière version publiée.',
      generic: 'Télécharger Photosort',
      forOs: { mac: 'Télécharger pour macOS', windows: 'Télécharger pour Windows', linux: 'Télécharger pour Linux' },
      soon: 'Bientôt disponible',
      soonNote: 'Aucune version n’est encore publiée. Le bouton mène à la page des versions.',
      others: 'Autres plateformes',
      platforms: [
        { key: 'mac', os: 'macOS', detail: 'image disque .dmg' },
        { key: 'windows', os: 'Windows', detail: '.exe portable' },
        { key: 'linux', os: 'Linux', detail: 'AppImage ou .deb' },
      ],
      source: 'Voir le code source',
      pendingRepo: 'Le dépôt n’est pas encore public. Les liens de téléchargement apparaîtront ici dès qu’ils seront disponibles.',
      unsigned: 'Les paquets ne sont pas encore signés : macOS demande une confirmation à la première ouverture, et Windows peut afficher un avertissement SmartScreen.',
    },
    more: {
      title: 'Envie d’en savoir plus ?',
      text: 'Le guide complet, le semainier, les questions fréquentes et les limites actuelles sont sur la présentation détaillée.',
      cta: 'Voir la présentation détaillée',
    },
  },
  en: {
    meta: {
      title: 'Photosort – Forget the tedious chore of sorting your photos',
      description:
        'Photosort sorts your photos by date and place, on a disk or a NAS, with no cloud and no subscription. Your originals are never modified. Free and open source.',
    },
    nav: { features: 'Features', download: 'Download', details: 'Detailed overview' },
    hero: {
      eyebrow: 'Open source · local · no account',
      title: 'Forget the tedious chore of sorting your photos.',
      subtitle: 'By date, by place, on a disk or a NAS. No cloud, no subscription, and your originals are never modified.',
      note: 'macOS · Windows · Linux',
      alt: 'The Photosort window after a sort: photos are filed into year and month folders, with the place name.',
      caption: 'Photosort after a sort',
    },
    sections: [
      {
        id: 'tri',
        shot: 'tri',
        eyebrow: 'Sorting',
        title: 'Sorted by date and place, on their own.',
        text: 'Drop your folders: Photosort reads the shooting date and files each photo into YYYY / YYYY-MM. If it is geotagged, it adds the city, even the landmark, from a built-in place database.',
        bullets: ['Year / month tree, place optional', 'Full preview before sorting', 'Your originals stay where they are'],
        alt: 'Photosort shows the photos to sort on the left and the planned tree by year, month and place on the right.',
        caption: 'The preview before sorting',
      },
      {
        id: 'iphone',
        shot: 'iphone',
        eyebrow: 'iPhone',
        title: 'Import straight from your iPhone.',
        text: 'Photosort reads its DCIM folder, you choose to sort all your photos, only the new photos, or the photos from a given period. Of course, Photosort picks up where it left off if the cable is unplugged.',
        bullets: ['All, new or a chosen period', 'Pause and resume the import', 'No iCloud, no Wi-Fi'],
        alt: 'The import panel shows an iPhone DCIM folder, the number of photos to import and the All, New or Period choices.',
        caption: 'Import from the DCIM folder',
      },
      {
        id: 'nas',
        shot: 'nas',
        eyebrow: 'NAS',
        title: 'Send the result to your NAS.',
        text: 'Photos are sorted as usual, then sent with rsync, through the NAS rsync server or over SSH. Nothing is copied locally, and what is already on the NAS is never overwritten.',
        bullets: ['Connection test and dry run before sending', 'Nothing is ever deleted on the NAS', 'Optional integrity check'],
        alt: 'The “Remote destination (NAS)” window with a fictitious nas.local address, a user and a Photos share.',
        caption: 'Remote destination (NAS)',
      },
      {
        id: 'cartes',
        shot: 'cartes',
        eyebrow: 'Keepsakes',
        title: 'Travel maps and scrapbook pages.',
        text: 'Photosort spots your trips and draws the route on a map you can print as a PDF. It also prepares planner pages to cut out and paste into your notebook.',
        bullets: ['Travel map and year map', 'Planner PDF, A4 or Letter', 'Choice of map backgrounds'],
        alt: 'A travel map preview: the route between two cities, with a photo at each stop.',
        caption: 'A travel map ready to print',
      },
      {
        id: 'doublons',
        shot: 'doublons',
        eyebrow: 'Tidying',
        title: 'Duplicates, similar and blurry photos set aside.',
        text: 'Exact copies are not copied again. Very similar photos and blurry ones go into separate folders, so you decide what to keep.',
        bullets: ['Exact duplicates skipped', 'Near-duplicates and blurry photos kept apart', 'Nothing is deleted for you'],
        alt: 'The reports list: pairs of identical thumbnails with the name of the file concerned.',
        caption: 'Duplicates flagged',
      },
      {
        id: 'confidentialite',
        shot: 'hero',
        eyebrow: 'Privacy',
        title: 'Everything happens on your computer.',
        text: 'Sorting, place lookup and duplicate detection work offline. No account, no server, no usage statistics. Photos are copied into a new library: your originals stay untouched.',
        bullets: ['Offline, no account', 'Originals never modified or moved', 'Nothing runs in the cloud'],
        alt: 'Summary of a finished sort: 18 photos sorted, with the skipped duplicates flagged.',
        caption: 'A finished sort',
      },
    ],
    features: {
      eyebrow: 'Also',
      title: 'The little details that make the difference.',
      items: [
        { name: 'Date fallback', text: 'No EXIF? The date in the file name, then the file date, take over.' },
        { name: 'Screenshots kept apart', text: 'They do not mix with your memories: a dedicated folder.' },
        { name: 'Offline geocoding', text: 'Cities and landmarks come from a built-in database, with no network request.' },
        { name: 'Never overwrites', text: 'A file that is already there is never replaced or deleted.' },
        { name: 'AI tags', text: 'Optional tags saved in .xmp files next to the photos.' },
        { name: 'Resume after a break', text: 'Import and upload pick up where they stopped.' },
        { name: 'French, English, Danish', text: 'The app and this site exist in all three languages.' },
        { name: 'Eight themes', text: 'Choose the look of the app, light or dark.' },
      ],
    },
    free: {
      eyebrow: 'Free',
      title: 'Free, ad-free, no tracking.',
      text: 'Photosort is free software under the MIT licence. No subscription, no paid version, no account to create.',
      items: ['No ads', 'No tracking or statistics', 'Open source code'],
    },
    download: {
      eyebrow: 'Download',
      title: 'Ready to sort your photos?',
      lead: 'Pick the package for your system in the latest published release.',
      generic: 'Download Photosort',
      forOs: { mac: 'Download for macOS', windows: 'Download for Windows', linux: 'Download for Linux' },
      soon: 'Coming soon',
      soonNote: 'No release has been published yet. The button leads to the releases page.',
      others: 'Other platforms',
      platforms: [
        { key: 'mac', os: 'macOS', detail: '.dmg disk image' },
        { key: 'windows', os: 'Windows', detail: 'portable .exe' },
        { key: 'linux', os: 'Linux', detail: 'AppImage or .deb' },
      ],
      source: 'View the source code',
      pendingRepo: 'The repository is not public yet. Download links will appear here as soon as they are available.',
      unsigned: 'Packages are not signed yet: macOS will ask for confirmation the first time you open the app, and Windows may show a SmartScreen warning.',
    },
    more: {
      title: 'Want to know more?',
      text: 'The full guide, the planner, the frequently asked questions and the current limits are on the detailed overview.',
      cta: 'See the detailed overview',
    },
  },
  da: {
    meta: {
      title: 'Photosort – Glem det kedelige arbejde med at sortere dine billeder',
      description:
        'Photosort sorterer dine billeder efter dato og sted, på disk eller NAS, uden sky og uden abonnement. Dine originaler ændres aldrig. Gratis og open source.',
    },
    nav: { features: 'Funktioner', download: 'Download', details: 'Detaljeret præsentation' },
    hero: {
      eyebrow: 'Open source · lokalt · ingen konto',
      title: 'Glem det kedelige arbejde med at sortere dine billeder.',
      subtitle: 'Efter dato, efter sted, på disk eller NAS. Ingen sky, intet abonnement, og dine originaler ændres aldrig.',
      note: 'macOS · Windows · Linux',
      alt: 'Photosort-vinduet efter en sortering: billederne ligger i år- og månedsmapper med stednavn.',
      caption: 'Photosort efter en sortering',
    },
    sections: [
      {
        id: 'tri',
        shot: 'tri',
        eyebrow: 'Sortering',
        title: 'Sorteret efter dato og sted, helt af sig selv.',
        text: 'Slip dine mapper: Photosort læser optagelsesdatoen og lægger hvert billede i ÅÅÅÅ / ÅÅÅÅ-MM. Er billedet geotagget, tilføjer det byen og endda seværdigheden fra en indbygget stedsdatabase.',
        bullets: ['Træ med år / måned, sted som valgfrit', 'Fuld forhåndsvisning før sortering', 'Dine originaler bliver, hvor de er'],
        alt: 'Photosort viser billederne, der skal sorteres, til venstre og det planlagte træ efter år, måned og sted til højre.',
        caption: 'Forhåndsvisningen før sortering',
      },
      {
        id: 'iphone',
        shot: 'iphone',
        eyebrow: 'iPhone',
        title: 'Importér direkte fra din iPhone.',
        text: 'Photosort læser dens DCIM-mappe, du vælger at sortere alle dine billeder, kun de nye billeder eller billederne fra en given periode. Photosort genoptager naturligvis, hvor det slap, hvis kablet trækkes ud.',
        bullets: ['Alle, nye eller en valgt periode', 'Pause og genoptagelse af importen', 'Uden iCloud, uden Wi-Fi'],
        alt: 'Importpanelet viser en iPhones DCIM-mappe, antallet af billeder og valgene Alle, Nye eller Periode.',
        caption: 'Import fra DCIM-mappen',
      },
      {
        id: 'nas',
        shot: 'nas',
        eyebrow: 'NAS',
        title: 'Send sorteringen til din NAS.',
        text: 'Billederne sorteres som sædvanlig og sendes derefter med rsync, via NAS’ens rsync-server eller over SSH. Intet kopieres lokalt, og det, der allerede ligger på NAS’en, overskrives aldrig.',
        bullets: ['Forbindelsestest og prøvekørsel før afsendelse', 'Intet slettes nogensinde på NAS’en', 'Valgfri integritetskontrol'],
        alt: 'Vinduet “Fjerndestination (NAS)” med en opdigtet adresse nas.local, en bruger og en delt mappe Photos.',
        caption: 'Fjerndestination (NAS)',
      },
      {
        id: 'cartes',
        shot: 'cartes',
        eyebrow: 'Minder',
        title: 'Rejsekort og scrapbogssider.',
        text: 'Photosort finder dine rejser og tegner ruten på et kort, du kan printe som PDF. Det forbereder også ugekalendersider til at klippe ud og klistre ind i din bog.',
        bullets: ['Rejsekort og årskort', 'Ugekalender-PDF, A4 eller Letter', 'Valg af kortbaggrund'],
        alt: 'Forhåndsvisning af et rejsekort: ruten mellem to byer med et billede ved hvert stop.',
        caption: 'Et rejsekort klar til print',
      },
      {
        id: 'doublons',
        shot: 'doublons',
        eyebrow: 'Oprydning',
        title: 'Dubletter, ensartede og slørede billeder lægges til side.',
        text: 'Nøjagtige kopier kopieres ikke igen. Meget ensartede og slørede billeder lægges i separate mapper, så du selv vælger, hvad du beholder.',
        bullets: ['Nøjagtige dubletter springes over', 'Næsten-dubletter og slørede billeder for sig', 'Intet slettes for dig'],
        alt: 'Listen over meddelelser: par af identiske miniaturer med navnet på den pågældende fil.',
        caption: 'Markerede dubletter',
      },
      {
        id: 'confidentialite',
        shot: 'hero',
        eyebrow: 'Privatliv',
        title: 'Alt foregår på din computer.',
        text: 'Sortering, stedsopslag og dubletsøgning kører offline. Ingen konto, ingen server, ingen brugsstatistik. Billederne kopieres til et nyt bibliotek: dine originaler forbliver urørte.',
        bullets: ['Offline, uden konto', 'Originaler aldrig ændret eller flyttet', 'Intet kører i skyen'],
        alt: 'Oversigt over en afsluttet sortering: 18 billeder sorteret, med de oversprungne dubletter markeret.',
        caption: 'En afsluttet sortering',
      },
    ],
    features: {
      eyebrow: 'Og desuden',
      title: 'De små detaljer, der gør forskellen.',
      items: [
        { name: 'Datoreserve', text: 'Ingen EXIF? Datoen i filnavnet og derefter filens dato tager over.' },
        { name: 'Skærmbilleder for sig', text: 'De blandes ikke med dine minder: en dedikeret mappe.' },
        { name: 'Geokodning offline', text: 'Byer og seværdigheder kommer fra en indbygget database, uden netværk.' },
        { name: 'Overskriver aldrig', text: 'En fil, der allerede findes, erstattes eller slettes aldrig.' },
        { name: 'AI-tags', text: 'Valgfrie tags gemt i .xmp-filer ved siden af billederne.' },
        { name: 'Genoptag efter afbrydelse', text: 'Import og afsendelse fortsætter, hvor de slap.' },
        { name: 'Fransk, engelsk, dansk', text: 'Appen og dette site findes på alle tre sprog.' },
        { name: 'Otte temaer', text: 'Vælg appens udseende, lyst eller mørkt.' },
      ],
    },
    free: {
      eyebrow: 'Gratis',
      title: 'Gratis, uden reklamer, uden sporing.',
      text: 'Photosort er fri software under MIT-licensen. Intet abonnement, ingen betalt version, ingen konto at oprette.',
      items: ['Uden reklamer', 'Uden sporing eller statistik', 'Åben kildekode'],
    },
    download: {
      eyebrow: 'Download',
      title: 'Klar til at sortere dine billeder?',
      lead: 'Vælg pakken til dit system i den seneste udgivne version.',
      generic: 'Download Photosort',
      forOs: { mac: 'Download til macOS', windows: 'Download til Windows', linux: 'Download til Linux' },
      soon: 'Kommer snart',
      soonNote: 'Der er endnu ikke udgivet nogen version. Knappen fører til siden med udgivelser.',
      others: 'Andre platforme',
      platforms: [
        { key: 'mac', os: 'macOS', detail: '.dmg-diskbillede' },
        { key: 'windows', os: 'Windows', detail: 'bærbar .exe' },
        { key: 'linux', os: 'Linux', detail: 'AppImage eller .deb' },
      ],
      source: 'Se kildekoden',
      pendingRepo: 'Repositoriet er ikke offentligt endnu. Download-links dukker op her, så snart de er klar.',
      unsigned: 'Pakkerne er endnu ikke signeret: macOS beder om bekræftelse første gang, du åbner appen, og Windows kan vise en SmartScreen-advarsel.',
    },
    more: {
      title: 'Vil du vide mere?',
      text: 'Den fulde guide, ugekalenderen, de ofte stillede spørgsmål og de nuværende begrænsninger findes i den detaljerede præsentation.',
      cta: 'Se den detaljerede præsentation',
    },
  },
};
