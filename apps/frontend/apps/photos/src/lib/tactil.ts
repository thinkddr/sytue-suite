/*
 * SPDX-License-Identifier: Apache-2.0
 * SPDX-FileCopyrightText: (c) 2026 Daniel Delgado Ramos
 *
 * Con el dedo, un campo mide 44 de alto como los botones de al lado: `Input` se
 * queda en sus 36, que con un ratón está bien y con la yema no. Es la misma
 * medida que `Button` e `IconButton` ya traen para el puntero grueso.
 *
 * Vive aparte y no en PhotosApp.tsx para que «Álbumes» no arrastre la galería
 * entera a su página por una clase.
 */
export const CAMPO_TACTIL = 'pointer-coarse:min-h-[max(44px,2.75rem)]';
