import type { AppCatalog } from '@sytue/i18n';
import type { en } from './en.js';
import { nucleo } from './hu/nucleo.js';
import { editor } from './hu/editor.js';
import { tienda } from './hu/tienda.js';
import { ia } from './hu/ia.js';

export const hu: AppCatalog<typeof en> = {
  ...nucleo,
  ...editor,
  ...tienda,
  ...ia,
};
