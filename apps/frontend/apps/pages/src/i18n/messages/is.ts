import type { AppCatalog } from '@sytue/i18n';
import type { en } from './en.js';
import { nucleo } from './is/nucleo.js';
import { editor } from './is/editor.js';
import { tienda } from './is/tienda.js';
import { ia } from './is/ia.js';

export const is: AppCatalog<typeof en> = {
  ...nucleo,
  ...editor,
  ...tienda,
  ...ia,
};
