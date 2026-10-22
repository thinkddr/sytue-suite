import type { AppCatalog } from '@sytue/i18n';
import type { en } from './en.js';
import { nucleo } from './sl/nucleo.js';
import { editor } from './sl/editor.js';
import { tienda } from './sl/tienda.js';
import { ia } from './sl/ia.js';

export const sl: AppCatalog<typeof en> = {
  ...nucleo,
  ...editor,
  ...tienda,
  ...ia,
};
