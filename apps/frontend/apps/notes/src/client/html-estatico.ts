/*
 * SPDX-License-Identifier: Apache-2.0
 * SPDX-FileCopyrightText: (c) 2026 Daniel Delgado Ramos
 *
 * parse5 y el saneador del pegado de forma ESTÁTICA, para quien pinta notas desde el
 * primer render: la página pública (se pinta en el servidor con las notas dentro y se
 * hidrata con ellas) y las pruebas. La app privada NO lo importa: baja los dos con
 * `cargarHtml()` (ver `html.ts`).
 */
import { parseFragment } from 'parse5';
import { htmlATexto, pegadoConFormato, textoPegado } from '@sytue/ui/browser/portapapeles';
import { registrarHtml } from './html.js';

registrarHtml({
  parse5: { parseFragment },
  portapapeles: { htmlATexto, pegadoConFormato, textoPegado },
});
