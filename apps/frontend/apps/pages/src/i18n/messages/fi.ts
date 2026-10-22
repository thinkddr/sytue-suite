import type { AppCatalog } from '@sytue/i18n';
import type { en } from './en.js';
import { nucleo } from './fi/nucleo.js';
import { editor } from './fi/editor.js';
import { tienda } from './fi/tienda.js';
import { ia } from './fi/ia.js';

export const fi: AppCatalog<typeof en> = {
  ...nucleo,
  ...editor,
  ...tienda,
  ...ia,
};
