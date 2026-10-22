/*
 * SPDX-License-Identifier: Apache-2.0
 * SPDX-FileCopyrightText: (c) 2026 Daniel Delgado Ramos
 *
 * Los nombres de los temas del catálogo común. Aparte de la cinta porque también los
 * pinta la portada (el selector de tema de la galería) y no debe arrastrar el editor.
 */
import type { DocsKey } from '../i18n';

/** Nombre de cada tema del catálogo: los de Diapos ya están traducidos a los 26 idiomas. */
export const NOMBRE_DE_TEMA: Record<string, DocsKey> = {
  papel: 'slides.theme.paper',
  carbon: 'slides.theme.charcoal',
  pizarra: 'slides.theme.slate',
  arena: 'slides.theme.sand',
  menta: 'slides.theme.mint',
  cielo: 'slides.theme.sky',
};
