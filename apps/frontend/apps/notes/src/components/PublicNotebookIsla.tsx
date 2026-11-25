/*
 * SPDX-License-Identifier: Apache-2.0
 * SPDX-FileCopyrightText: (c) 2026 Daniel Delgado Ramos
 *
 * La isla de la página pública (`/p/<token>`). Se pinta en el servidor con las notas
 * dentro y se hidrata con ellas, así que lee su HTML desde el primer render: registra
 * parse5 de forma estática (`html-estatico.ts`). La app privada usa `PublicNotebook`
 * (sus `NotasSoloLectura`) sin esto y baja parse5 aparte (`html.ts`).
 */
import '../client/html-estatico.js';

export { PublicNotebook } from './PublicNotebook.js';
