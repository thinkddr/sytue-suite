/*
 * SPDX-License-Identifier: Apache-2.0
 * SPDX-FileCopyrightText: (c) 2026 Daniel Delgado Ramos
 */

/** La dirección canónica la resuelve el servicio con SitesDomain y el dominio principal. */
export function direccionDelRender(datos: unknown): string | null {
  const host = (datos as { site?: { canonical_host?: unknown } } | null)?.site?.canonical_host;
  if (typeof host !== 'string' || !host) return null;
  try {
    const url = new URL(`https://${host}/`);
    // Es un hostname, no una URL, credenciales o un destino con ruta aportado por el cliente.
    if (
      url.host !== host ||
      url.username ||
      url.password ||
      url.port ||
      url.pathname !== '/' ||
      url.search ||
      url.hash
    )
      return null;
    return url.href;
  } catch {
    return null;
  }
}
