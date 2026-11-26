/*
 * SPDX-License-Identifier: Apache-2.0
 * SPDX-FileCopyrightText: (c) 2026 Daniel Delgado Ramos
 *
 * /api/import — «Traer mis datos» de Sytue Drive: importar los archivos y carpetas desde Google
 * Workspace o Microsoft 365.
 *
 *   GET  /api/import?domains=…  → { origins, jobs }  orígenes conectados + mi progreso
 *   POST /api/import { instanceId, source, domains } → lanza la importación
 *
 * Proxy BFF al Sytue Integration Gateway con el access token resuelto en el SERVIDOR: el
 * token nunca toca el navegador. El gateway ignora cualquier identidad que se le mande —
 * destino y origen son siempre quien llama—, así que esta puerta no puede tocar los datos
 * de otra persona.
 *
 * El endpoint entero vive en @sytue/api-clients (`importEndpoint`): las cuatro rutas eran
 * copias con la misma validación escrita cuatro veces, y aquí solo queda lo que es de
 * este producto —qué contenido trae y cómo se lee su sesión—.
 */
import type { APIRoute } from 'astro';
import { importEndpoint } from '@sytue/api-clients';
import type { ImportDomain } from '@sytue/api-clients';
import { resolveIntegrationsUrl, serverEnv } from '@sytue/auth/server';
import { bff, toBffContext } from '../../lib/auth';

export const prerender = false;

/** Contenido que ESTE producto sabe traer. */
const PROPIOS: ImportDomain[] = ['drive'];

/**
 * `resolveIntegrationsUrl` y no la URL pública: desde el pod, salir al ingress y volver
 * a entrar no funciona (ver `SYTUE_INTEGRATIONS_INTERNAL_URL` en el chart). Y
 * `serverEnv` porque en el SSR compilado `import.meta.env` va congelado del build.
 */
const handler: APIRoute = async (ctx) =>
  importEndpoint(ctx.request, {
    baseUrl: resolveIntegrationsUrl(serverEnv(import.meta.env)),
    accessToken: (await bff.getSession(toBffContext(ctx)))?.tokens.accessToken,
    domains: PROPIOS,
  });

export const GET = handler;
export const POST = handler;
