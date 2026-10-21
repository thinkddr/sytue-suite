/*
 * SPDX-License-Identifier: Apache-2.0
 * SPDX-FileCopyrightText: (c) 2026 Daniel Delgado Ramos
 *
 * GET /.well-known/assetlinks.json — App Links de Android (ver lib/asociacionApp.ts).
 * Público y sin sesión: Android lo pide al instalar la app. El Ingress `-bff` lo lleva
 * desde la raíz del host a este prefijo.
 */
import type { APIRoute } from 'astro';
import { asociacionDelEntorno } from '../../lib/asociacionApp.js';

export const prerender = false;

export const GET: APIRoute = () =>
  new Response(JSON.stringify(asociacionDelEntorno().android), {
    headers: { 'content-type': 'application/json', 'cache-control': 'public, max-age=3600' },
  });
