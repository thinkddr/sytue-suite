import type { AppCatalog } from '@sytue/i18n';
import type { en } from './en.js';
import { nucleo } from './ga/nucleo.js';
import { editor } from './ga/editor.js';
import { tienda } from './ga/tienda.js';
import { ia } from './ga/ia.js';

export const ga: AppCatalog<typeof en> = {
  ...nucleo,
  ...editor,
  ...tienda,
  ...ia,
};
