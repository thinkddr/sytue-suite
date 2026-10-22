/*
 * SPDX-License-Identifier: Apache-2.0
 * SPDX-FileCopyrightText: (c) 2026 Daniel Delgado Ramos
 */

/** El tipo que exige QUERY: el mismo que manda `HttpClient.consulta`. */
const TIPO = 'application/x-www-form-urlencoded';

/** 64 KiB, el mismo tope que `platform/pkg/httpx`. */
const MAXIMO = 64 * 1024;

/**
 * Los parámetros de una consulta: del CUERPO si el método es QUERY, de la URL
 * si no. Es el lado Astro de `platform/pkg/httpx.Consulta`, y el que permite
 * que un mismo handler sirva `GET` y `QUERY` sin escribirlo dos veces.
 *
 * POR QUÉ QUERY Y NO GET para buscar: lo que alguien escribe en el buscador
 * deja de viajar en la URL, así que no queda en el log de accesos de cada
 * salto, ni en el `Referer`, ni en el historial del navegador. Y en Astro sale
 * gratis una segunda ventaja, medida en producción el 2026-09-11: su protección
 * contra CSRF trata QUERY con cuerpo de formulario como un envío de formulario
 * y exige un `Origin` del propio sitio —«Cross-site QUERY form submissions are
 * forbidden», 403—, mientras que el mismo GET desde otro sitio entra sin que
 * nadie mire el origen.
 *
 * CON UN CUERPO QUE NO CUADRA DEVUELVE VACÍO, sin inventarse un error propio:
 * el servicio de detrás ya contesta que falta el término de búsqueda, que es lo
 * mismo que habría que decir y ya está escrito. El cuerpo lo manda nuestro
 * cliente, así que el caso es un error de programación, no una entrada hostil.
 */
export async function consulta(request: Request, url: URL): Promise<URLSearchParams> {
  if (request.method !== 'QUERY') return url.searchParams;
  const tipo = (request.headers.get('content-type') ?? '').split(';')[0]?.trim();
  const largo = Number(request.headers.get('content-length') ?? '0');
  if (tipo !== TIPO || !Number.isFinite(largo) || largo > MAXIMO) return new URLSearchParams();
  // Y OTRA VEZ con lo leído, porque la cabecera puede no estar: una petición con
  // `Transfer-Encoding: chunked` no lleva `content-length`, así que `largo` vale 0
  // y el tope de arriba no mira nada. El segundo corte no es gratis —el cuerpo ya
  // está en memoria— pero acota lo que se PARSEA, que es lo caro. Se cuenta en
  // caracteres y no en bytes, así que con multibyte deja pasar algo más de 64 KiB;
  // el tope es una cordura para un formulario de búsqueda, no una cuota exacta.
  const texto = await request.text();
  if (texto.length > MAXIMO) return new URLSearchParams();
  return new URLSearchParams(texto);
}
