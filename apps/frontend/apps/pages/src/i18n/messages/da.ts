import type { AppCatalog } from '@sytue/i18n';
import type { en } from './en.js';
import { nucleo } from './da/nucleo.js';
import { editor } from './da/editor.js';
import { tienda } from './da/tienda.js';
import { ia } from './da/ia.js';

export const da: AppCatalog<typeof en> = {
  ...nucleo,
  ...editor,
  ...tienda,
  ...ia,
};
