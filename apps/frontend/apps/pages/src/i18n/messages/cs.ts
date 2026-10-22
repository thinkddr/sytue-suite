import type { AppCatalog } from '@sytue/i18n';
import type { en } from './en.js';
import { nucleo } from './cs/nucleo.js';
import { editor } from './cs/editor.js';
import { tienda } from './cs/tienda.js';
import { ia } from './cs/ia.js';

export const cs: AppCatalog<typeof en> = {
  ...nucleo,
  ...editor,
  ...tienda,
  ...ia,
};
