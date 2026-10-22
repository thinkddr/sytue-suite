import type { AppCatalog } from '@sytue/i18n';
import type { en } from './en.js';
import { nucleo } from './el/nucleo.js';
import { editor } from './el/editor.js';
import { tienda } from './el/tienda.js';
import { ia } from './el/ia.js';

export const el: AppCatalog<typeof en> = {
  ...nucleo,
  ...editor,
  ...tienda,
  ...ia,
};
