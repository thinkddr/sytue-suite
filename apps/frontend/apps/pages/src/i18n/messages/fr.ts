import type { AppCatalog } from '@sytue/i18n';
import type { en } from './en.js';
import { nucleo } from './fr/nucleo.js';
import { editor } from './fr/editor.js';
import { tienda } from './fr/tienda.js';
import { ia } from './fr/ia.js';

export const fr: AppCatalog<typeof en> = {
  ...nucleo,
  ...editor,
  ...tienda,
  ...ia,
};
