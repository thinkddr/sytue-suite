import type { AppCatalog } from '@sytue/i18n';
import type { en } from './en.js';
import { nucleo } from './mt/nucleo.js';
import { editor } from './mt/editor.js';
import { tienda } from './mt/tienda.js';
import { ia } from './mt/ia.js';

export const mt: AppCatalog<typeof en> = {
  ...nucleo,
  ...editor,
  ...tienda,
  ...ia,
};
