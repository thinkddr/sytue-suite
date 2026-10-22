import type { AppCatalog } from '@sytue/i18n';
import type { en } from './en.js';
import { nucleo } from './ro/nucleo.js';
import { editor } from './ro/editor.js';
import { tienda } from './ro/tienda.js';
import { ia } from './ro/ia.js';

export const ro: AppCatalog<typeof en> = {
  ...nucleo,
  ...editor,
  ...tienda,
  ...ia,
};
