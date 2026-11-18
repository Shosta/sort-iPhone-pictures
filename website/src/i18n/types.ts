export const languages = ['fr', 'en', 'da'] as const;
export type Lang = (typeof languages)[number];

export interface Copy {
  htmlLang: string;
  ogLocale: string;
  languageName: string;
  meta: { title: string; description: string };
  skip: string;
  nav: { label: string; scrapbook: string; features: string; download: string; faq: string; languages: string };
  hero: {
    eyebrow: string;
    titleA: string;
    titleB: string;
    lead: string;
    ctaScrapbook: string;
    ctaDownload: string;
    note: string;
    sheetAlt: string;
    sheetCaption: string;
  };
  stamp: { top: string; main: string; bottom: string };
  scrapbook: {
    eyebrow: string;
    title: string;
    lead: string;
    steps: { title: string; text: string }[];
    facts: { label: string; value: string }[];
    note: string;
  };
  how: { eyebrow: string; title: string; items: { title: string; text: string }[]; note: string };
  features: {
    eyebrow: string;
    title: string;
    lead: string;
    specimen: string;
    items: { name: string; latin: string; text: string }[];
  };
  shots: {
    eyebrow: string;
    title: string;
    pending: string;
    items: { key: 'apercu' | 'lieux' | 'doublons' | 'scrapbook'; alt: string; caption: string }[];
  };
  download: {
    eyebrow: string;
    title: string;
    lead: string;
    platforms: { os: string; detail: string }[];
    button: string;
    repoButton: string;
    pendingRepo: string;
    unsigned: string;
  };
  limits: { eyebrow: string; title: string; items: string[] };
  faq: { eyebrow: string; title: string; items: { q: string; a: string }[] };
  footer: { tagline: string; independent: string; license: string };
}
