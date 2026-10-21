import type { AppCatalog } from '@sytue/i18n';
import type { en } from './en.js';
import { nucleo } from './pt/nucleo.js';
import { editor } from './pt/editor.js';
import { tienda } from './pt/tienda.js';
import { ia } from './pt/ia.js';

export const pt: AppCatalog<typeof en> = {
  ...nucleo,
  ...editor,
  ...tienda,
  ...ia,
};
