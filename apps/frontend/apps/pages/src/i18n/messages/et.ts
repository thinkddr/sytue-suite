import type { AppCatalog } from '@sytue/i18n';
import type { en } from './en.js';
import { nucleo } from './et/nucleo.js';
import { editor } from './et/editor.js';
import { tienda } from './et/tienda.js';
import { ia } from './et/ia.js';

export const et: AppCatalog<typeof en> = {
  ...nucleo,
  ...editor,
  ...tienda,
  ...ia,
};
