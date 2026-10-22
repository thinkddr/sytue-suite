/*
 * SPDX-License-Identifier: Apache-2.0
 * SPDX-FileCopyrightText: (c) 2026 Daniel Delgado Ramos
 *
 * GET /.well-known/apple-app-site-association — Universal Links de iOS (ver
 * lib/asociacionApp.ts). Apple exige `application/json`, sin extensión y sin
 * redirecciones. Sin Team ID configurado, 404: no se reclama nada.
 */
import type { APIRoute } from 'astro';
import { asociacionDelEntorno } from '../../lib/asociacionApp.js';

export const prerender = false;

export const GET: APIRoute = () => {
  const a = asociacionDelEntorno().apple;
  if (!a) return new Response('not found', { status: 404 });
  return new Response(JSON.stringify(a), {
    headers: { 'content-type': 'application/json', 'cache-control': 'public, max-age=3600' },
  });
};
