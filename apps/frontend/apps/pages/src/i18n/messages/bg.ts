import type { AppCatalog } from '@sytue/i18n';
import type { en } from './en.js';
import { nucleo } from './bg/nucleo.js';
import { editor } from './bg/editor.js';
import { tienda } from './bg/tienda.js';
import { ia } from './bg/ia.js';

export const bg: AppCatalog<typeof en> = {
  ...nucleo,
  ...editor,
  ...tienda,
  ...ia,
};
