import type { AppCatalog } from '@sytue/i18n';
import type { en } from './en.js';
import { nucleo } from './sk/nucleo.js';
import { editor } from './sk/editor.js';
import { tienda } from './sk/tienda.js';
import { ia } from './sk/ia.js';

export const sk: AppCatalog<typeof en> = {
  ...nucleo,
  ...editor,
  ...tienda,
  ...ia,
};
