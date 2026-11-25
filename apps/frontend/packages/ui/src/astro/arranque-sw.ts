/*
 * SPDX-License-Identifier: Apache-2.0
 * SPDX-FileCopyrightText: (c) 2026 Daniel Delgado Ramos
 *
 * `/arranque.js` de las apps con Service Worker (Portal, Rooms, Cloud): lo mismo que
 * `arranque.ts` más el registro del worker con el prefijo de la app. Una ruta aparte
 * y no una opción leída en tiempo de ejecución: la elige `sytueArranque()` al
 * construir la app.
 */
import type { APIRoute } from 'astro';
import { respuestaDeArranque } from './arranque.js';

export const prerender = false;

export const GET: APIRoute = () => respuestaDeArranque(true);
