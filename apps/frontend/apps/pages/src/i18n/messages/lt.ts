import type { AppCatalog } from '@sytue/i18n';
import type { en } from './en.js';
import { nucleo } from './lt/nucleo.js';
import { editor } from './lt/editor.js';
import { tienda } from './lt/tienda.js';
import { ia } from './lt/ia.js';

export const lt: AppCatalog<typeof en> = {
  ...nucleo,
  ...editor,
  ...tienda,
  ...ia,
};
