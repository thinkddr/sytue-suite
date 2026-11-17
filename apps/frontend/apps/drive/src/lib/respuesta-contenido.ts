/*
 * SPDX-License-Identifier: Apache-2.0
 * SPDX-FileCopyrightText: (c) 2026 Daniel Delgado Ramos
 *
 * Política de cabeceras con que el BFF reenvía el contenido de un fichero.
 *
 * Vive fuera de la ruta de Astro para poder probarla sin levantar el servidor: es
 * donde se decide qué se puede pintar inline en el origen de la app (seguridad) y
 * cómo viaja una respuesta parcial 206 (reproducir vídeo y audio).
 */

/** ¿Se puede servir inline este content-type (ya normalizado) sin riesgo? */
export function inlineSeguro(type: string): boolean {
  return (
    type.startsWith('image/') ||
    type === 'application/pdf' ||
    type.startsWith('text/plain') ||
    // Vídeo y audio no ejecutan nada: el navegador los entrega a su reproductor.
    type.startsWith('video/') ||
    type.startsWith('audio/')
  );
}

/**
 * Estado y cabeceras de la respuesta del BFF a partir de la del servicio Go.
 *
 * El 206 se respeta con su `content-range`: sin él, `<video>` no puede saltar a otro
 * minuto y Safari ni arranca. `accept-ranges` se propaga para que el navegador sepa
 * que puede pedir tramos desde la primera respuesta.
 */
export function respuestaDeContenido(
  upstream: { status: number; headers: Headers },
  opts: { name: string; inline: boolean },
): { status: number; headers: Record<string, string> } {
  let type = upstream.headers.get('content-type') || 'application/octet-stream';
  const headers: Record<string, string> = {
    'cache-control': 'no-store',
    'x-content-type-options': 'nosniff',
  };
  if (opts.inline) {
    // Todo texto baja como text/plain: servir text/html en el origen de la app con
    // cookies sería XSS almacenado.
    if (type.startsWith('text/') || type === 'application/json') type = 'text/plain; charset=utf-8';
    const safe = inlineSeguro(type);
    headers['content-disposition'] = safe
      ? `inline; filename="${opts.name}"`
      : `attachment; filename="${opts.name}"`;
    // Cinturón para navegación directa (p. ej. SVG con <script>): documento sandbox
    // sin scripts ni origen; no afecta a <img>/<video>/<pre>.
    if (safe) headers['content-security-policy'] = 'sandbox';
  } else {
    headers['content-disposition'] = `attachment; filename="${opts.name}"`;
  }
  headers['content-type'] = type;
  // El service worker nativo exige tamaño explícito para aplicar el límite offline;
  // en un 206 es el del tramo, que es lo que el servicio Go ya pone.
  for (const h of ['content-length', 'content-range', 'accept-ranges', 'etag']) {
    const v = upstream.headers.get(h);
    if (v) headers[h] = v;
  }
  return { status: upstream.status === 206 ? 206 : 200, headers };
}
