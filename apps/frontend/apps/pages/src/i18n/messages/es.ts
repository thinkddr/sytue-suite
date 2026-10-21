import type { AppCatalog } from '@sytue/i18n';
import type { en } from './en.js';
import { nucleo } from './es/nucleo.js';
import { editor } from './es/editor.js';
import { tienda } from './es/tienda.js';
import { ia } from './es/ia.js';

export const es: AppCatalog<typeof en> = {
  ...nucleo,
  ...editor,
  ...tienda,
  ...ia,
};
