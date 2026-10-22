import type { AppCatalog } from '@sytue/i18n';
import type { en } from './en.js';
import { nucleo } from './hr/nucleo.js';
import { editor } from './hr/editor.js';
import { tienda } from './hr/tienda.js';
import { ia } from './hr/ia.js';

export const hr: AppCatalog<typeof en> = {
  ...nucleo,
  ...editor,
  ...tienda,
  ...ia,
};
