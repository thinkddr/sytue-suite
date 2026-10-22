import type { AppCatalog } from '@sytue/i18n';
import type { en } from './en.js';
import { nucleo } from './nb/nucleo.js';
import { editor } from './nb/editor.js';
import { tienda } from './nb/tienda.js';
import { ia } from './nb/ia.js';

export const nb: AppCatalog<typeof en> = {
  ...nucleo,
  ...editor,
  ...tienda,
  ...ia,
};
