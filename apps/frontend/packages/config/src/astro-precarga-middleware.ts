/*
 * SPDX-License-Identifier: Apache-2.0
 * SPDX-FileCopyrightText: (c) 2026 Daniel Delgado Ramos
 *
 * El middleware de `sytuePrecarga()` (ver `precarga.ts`): pone los
 * `<link rel="modulepreload">` delante de cada isla del HTML que sale del servidor,
 * sin esperar a la página entera.
 *
 * El manifiesto no existe cuando se compila este módulo: el bundle del servidor se
 * construye ANTES que el del cliente. La integración escribe el manifiesto en el
 * literal de abajo al terminar el build, como hace Astro con el suyo. Sin él (en
 * `astro dev`, o en el prerender, que corre antes) el middleware no toca nada.
 */
import type { MiddlewareHandler } from 'astro';
import { InyectorDePrecarga, type ManifiestoDePrecarga } from './precarga.js';

// Lo lee de `globalThis` primero solo para que el compilador no pueda dar por sabido el
// valor del literal y quitar el código que lo usa.
const crudo: string =
  (globalThis as { __sytuePrecarga?: string }).__sytuePrecarga ?? '__SYTUE_PRECARGA_MANIFIESTO__';

let manifiesto: { datos: ManifiestoDePrecarga; activos: string } | null | undefined;
function leer(): { datos: ManifiestoDePrecarga; activos: string } | null {
  if (manifiesto !== undefined) return manifiesto;
  try {
    manifiesto = crudo.startsWith('{')
      ? (JSON.parse(crudo) as { datos: ManifiestoDePrecarga; activos: string })
      : null;
  } catch {
    manifiesto = null;
  }
  return manifiesto;
}

export const onRequest: MiddlewareHandler = async (_context, next) => {
  const response = await next();
  const precarga = leer();
  if (
    !precarga ||
    !response.body ||
    !(response.headers.get('content-type') ?? '').toLowerCase().includes('text/html')
  )
    return response;
  const inyector = new InyectorDePrecarga(precarga.datos, precarga.activos);
  const decoder = new TextDecoder();
  const encoder = new TextEncoder();
  const body = response.body.pipeThrough(
    new TransformStream<Uint8Array, Uint8Array>({
      transform(trozo, salida) {
        const texto = inyector.push(decoder.decode(trozo, { stream: true }));
        if (texto) salida.enqueue(encoder.encode(texto));
      },
      flush(salida) {
        const texto = inyector.push(decoder.decode()) + inyector.end();
        if (texto) salida.enqueue(encoder.encode(texto));
      },
    }),
  );
  const headers = new Headers(response.headers);
  // El cuerpo crece: una longitud declarada cortaría la página.
  headers.delete('content-length');
  return new Response(body, { status: response.status, statusText: response.statusText, headers });
};
