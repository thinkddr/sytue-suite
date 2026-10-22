import type { AppCatalog } from '@sytue/i18n';
import type { en } from './en.js';
import { nucleo } from './it/nucleo.js';
import { editor } from './it/editor.js';
import { tienda } from './it/tienda.js';
import { ia } from './it/ia.js';

export const it: AppCatalog<typeof en> = {
  ...nucleo,
  ...editor,
  ...tienda,
  ...ia,
};
