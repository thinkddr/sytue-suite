import type { AppCatalog } from '@sytue/i18n';
import type { en } from './en.js';
import { nucleo } from './nl/nucleo.js';
import { editor } from './nl/editor.js';
import { tienda } from './nl/tienda.js';
import { ia } from './nl/ia.js';

export const nl: AppCatalog<typeof en> = {
  ...nucleo,
  ...editor,
  ...tienda,
  ...ia,
};
