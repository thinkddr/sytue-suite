import type { AppCatalog } from '@sytue/i18n';
import type { en } from './en.js';
import { nucleo } from './sv/nucleo.js';
import { editor } from './sv/editor.js';
import { tienda } from './sv/tienda.js';
import { ia } from './sv/ia.js';

export const sv: AppCatalog<typeof en> = {
  ...nucleo,
  ...editor,
  ...tienda,
  ...ia,
};
