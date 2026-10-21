import type { AppCatalog } from '@sytue/i18n';
import type { en } from './en.js';
import { nucleo } from './de/nucleo.js';
import { editor } from './de/editor.js';
import { tienda } from './de/tienda.js';
import { ia } from './de/ia.js';

export const de: AppCatalog<typeof en> = {
  ...nucleo,
  ...editor,
  ...tienda,
  ...ia,
};
