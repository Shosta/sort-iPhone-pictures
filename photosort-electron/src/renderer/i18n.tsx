import { createContext, useContext } from 'react'

export type Language = 'fr' | 'en-GB' | 'da'

export const LANGUAGES: { code: Language; label: string }[] = [
  { code: 'fr', label: 'Français' },
  { code: 'en-GB', label: 'English' },
  { code: 'da', label: 'Dansk' }
]

const fr = {
  'drop.hover': 'Relâchez pour ajouter',
  'drop.title': 'Déposez vos dossiers de photos',
  'drop.browse': 'Parcourir…',
  'drop.pick': 'Dossiers sources',
  'step.sources': 'Sources',
  'step.analysis': 'Analyse',
  'step.sort': 'Tri',
  'pending': 'À trier',
  'sorted': 'Classées',
  'more': '+ {n} autres',
  'pending.empty': 'Ajoutez des dossiers puis lancez l’analyse.',
  'sorted.empty': 'Les photos classées apparaîtront ici.',
  'destination': 'Destination',
  'destination.placeholder': '/chemin/vers/classement',
  'destination.pick': 'Dossier de destination',
  'choose': 'Choisir',
  'byLocation': 'Classer par lieu (ville, si la photo est géolocalisée)',
  'landmarks': 'Nommer les monuments (musée, château, stade…)',
  'skipDuplicates': 'Ignorer les vrais doublons (photos identiques)',
  'duplicates': '{n} doublon(s) ignoré(s)',
  'status.analyzing': 'Analyse des EXIF…',
  'status.copying': 'Copie {done}/{total}',
  'status.done': '✅ {n} photos classées',
  'status.ready': '{n} photos prêtes',
  'status.idle': 'En attente',
  'issues': '{n} signalement(s)',
  'issues.title': 'Signalements',
  'openFolder': 'Ouvrir le dossier',
  'cancel': 'Annuler',
  'clean': 'Nettoyer',
  'clean.hint': 'Vide les dossiers déposés et les résultats (les fichiers déjà copiés ne sont pas supprimés)',
  'analyze': 'Analyser',
  'sort': 'Trier',
  'close': 'Fermer',
  'prefs': 'Préférences',
  'prefs.language': 'Langue',
  'prefs.theme': 'Thème de couleur',
  'theme.aurora': 'Aurore',
  'theme.monochrome': 'Monochrome',
  'theme.analogous': 'Analogue',
  'theme.complementary': 'Complémentaire',
  'theme.triadic': 'Triadique',
  'theme.stonewash': 'Stonewash Côtier',
  'theme.brass': 'Drame Laiton Minuit',
  'theme.linen': 'Retraite en Lin Chaud',
  'prefs.output': 'Dossier de sortie',
  'prefs.location': 'Géolocalisation',
  'prefs.saved': 'Les préférences sont enregistrées automatiquement.'
}

export type Key = keyof typeof fr

const enGB: Record<Key, string> = {
  'drop.hover': 'Release to add',
  'drop.title': 'Drop your photo folders here',
  'drop.browse': 'Browse…',
  'drop.pick': 'Source folders',
  'step.sources': 'Sources',
  'step.analysis': 'Analysis',
  'step.sort': 'Sort',
  'pending': 'To sort',
  'sorted': 'Sorted',
  'more': '+ {n} more',
  'pending.empty': 'Add folders, then start the analysis.',
  'sorted.empty': 'Sorted photos will appear here.',
  'destination': 'Destination',
  'destination.placeholder': '/path/to/library',
  'destination.pick': 'Destination folder',
  'choose': 'Choose',
  'byLocation': 'Sort by place (city, when the photo is geotagged)',
  'landmarks': 'Name landmarks (museum, castle, stadium…)',
  'skipDuplicates': 'Skip true duplicates (identical photos)',
  'duplicates': '{n} duplicate(s) skipped',
  'status.analyzing': 'Reading EXIF data…',
  'status.copying': 'Copying {done}/{total}',
  'status.done': '✅ {n} photos sorted',
  'status.ready': '{n} photos ready',
  'status.idle': 'Waiting',
  'issues': '{n} warning(s)',
  'issues.title': 'Warnings',
  'openFolder': 'Open folder',
  'cancel': 'Cancel',
  'clean': 'Clear',
  'clean.hint': 'Clears the dropped folders and the results (files already copied are not deleted)',
  'analyze': 'Analyse',
  'sort': 'Sort',
  'close': 'Close',
  'prefs': 'Preferences',
  'prefs.language': 'Language',
  'prefs.theme': 'Colour theme',
  'theme.aurora': 'Aurora',
  'theme.monochrome': 'Monochrome',
  'theme.analogous': 'Analogous',
  'theme.complementary': 'Complementary',
  'theme.triadic': 'Triadic',
  'theme.stonewash': 'Coastal Stonewash',
  'theme.brass': 'Midnight Brass Drama',
  'theme.linen': 'Warm Linen Retreat',
  'prefs.output': 'Output folder',
  'prefs.location': 'Geolocation',
  'prefs.saved': 'Preferences are saved automatically.'
}

const da: Record<Key, string> = {
  'drop.hover': 'Slip for at tilføje',
  'drop.title': 'Slip dine fotomapper her',
  'drop.browse': 'Gennemse…',
  'drop.pick': 'Kildemapper',
  'step.sources': 'Kilder',
  'step.analysis': 'Analyse',
  'step.sort': 'Sortering',
  'pending': 'Skal sorteres',
  'sorted': 'Sorteret',
  'more': '+ {n} flere',
  'pending.empty': 'Tilføj mapper, og start analysen.',
  'sorted.empty': 'Sorterede billeder vises her.',
  'destination': 'Destination',
  'destination.placeholder': '/sti/til/bibliotek',
  'destination.pick': 'Destinationsmappe',
  'choose': 'Vælg',
  'byLocation': 'Sortér efter sted (by, hvis billedet er geotagget)',
  'landmarks': 'Navngiv seværdigheder (museum, slot, stadion…)',
  'skipDuplicates': 'Spring ægte dubletter over (identiske billeder)',
  'duplicates': '{n} dublet(ter) sprunget over',
  'status.analyzing': 'Læser EXIF-data…',
  'status.copying': 'Kopierer {done}/{total}',
  'status.done': '✅ {n} billeder sorteret',
  'status.ready': '{n} billeder klar',
  'status.idle': 'Venter',
  'issues': '{n} advarsel/advarsler',
  'issues.title': 'Advarsler',
  'openFolder': 'Åbn mappen',
  'cancel': 'Annullér',
  'clean': 'Ryd',
  'clean.hint': 'Rydder de slupne mapper og resultaterne (allerede kopierede filer slettes ikke)',
  'analyze': 'Analysér',
  'sort': 'Sortér',
  'close': 'Luk',
  'prefs': 'Indstillinger',
  'prefs.language': 'Sprog',
  'prefs.theme': 'Farvetema',
  'theme.aurora': 'Nordlys',
  'theme.monochrome': 'Monokrom',
  'theme.analogous': 'Analog',
  'theme.complementary': 'Komplementær',
  'theme.triadic': 'Triadisk',
  'theme.stonewash': 'Kyst-stonewash',
  'theme.brass': 'Midnatsdrama i messing',
  'theme.linen': 'Varm hørhygge',
  'prefs.output': 'Outputmappe',
  'prefs.location': 'Geolokalisering',
  'prefs.saved': 'Indstillinger gemmes automatisk.'
}

const dictionaries: Record<Language, Record<Key, string>> = { fr, 'en-GB': enGB, da }

export type Translate = (key: Key, vars?: Record<string, string | number>) => string

export function makeTranslate(language: Language): Translate {
  return (key, vars) =>
    dictionaries[language][key].replace(/\{(\w+)\}/g, (_, name: string) => String(vars?.[name] ?? ''))
}

export function detectLanguage(): Language {
  const locale = navigator.language.toLowerCase()
  if (locale.startsWith('fr')) return 'fr'
  if (locale.startsWith('da')) return 'da'
  return 'en-GB'
}

export const I18nContext = createContext<Translate>(makeTranslate('fr'))
export const useT = (): Translate => useContext(I18nContext)
