import { da } from './da';
import { en } from './en';
import { fr } from './fr';
import { languages, type Copy, type Lang } from './types';

export { languages };
export type { Copy, Lang };

export const copies: Record<Lang, Copy> = { fr, en, da };
export const defaultLang: Lang = 'fr';
