import type { AppCatalog } from '@sytue/i18n';
import type { en } from './en.js';
import { nucleo } from './pl/nucleo.js';
import { editor } from './pl/editor.js';
import { tienda } from './pl/tienda.js';
import { ia } from './pl/ia.js';

export const pl: AppCatalog<typeof en> = {
  ...nucleo,
  ...editor,
  ...tienda,
  ...ia,
};
