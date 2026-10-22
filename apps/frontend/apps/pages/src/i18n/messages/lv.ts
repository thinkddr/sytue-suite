import type { AppCatalog } from '@sytue/i18n';
import type { en } from './en.js';
import { nucleo } from './lv/nucleo.js';
import { editor } from './lv/editor.js';
import { tienda } from './lv/tienda.js';
import { ia } from './lv/ia.js';

export const lv: AppCatalog<typeof en> = {
  ...nucleo,
  ...editor,
  ...tienda,
  ...ia,
};
