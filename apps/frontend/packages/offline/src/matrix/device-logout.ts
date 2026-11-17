/* SPDX-License-Identifier: Apache-2.0 */
/**
 * Cierra en el homeserver el dispositivo de Matrix de ESTA sesión, con su propio token
 * (`POST /_matrix/client/v3/logout`): la misma operación que hace el logout del
 * dispositivo de Rooms en las nativas (apps/nucleo/matrix/cierre).
 *
 * Olvidar un dispositivo borrando solo su sesión y sus claves locales lo dejaba vivo en
 * el homeserver: uno muerto por cada entrada en Mail, Hub, ERP, Rooms…, que además
 * recibía por to-device cada clave de sala y seguía figurando como de confianza.
 *
 * Nunca bloquea ni hace fallar la salida: plazo corto, sin excepciones, y lo que pase
 * después (borrar lo local) ocurre igual aunque el homeserver no conteste.
 */

/** Lo que se espera al homeserver antes de seguir borrando lo local. */
export const MATRIX_DEVICE_LOGOUT_TIMEOUT_MS = 3000;

export interface MatrixDeviceLogoutOptions {
  fetch?: typeof fetch;
  timeoutMs?: number;
}

/**
 * `true` si el homeserver confirma que el dispositivo ya no existe: 2xx, o 401
 * M_UNKNOWN_TOKEN sin `soft_logout` (un token caducado no prueba que se cerrara).
 */
export async function logoutMatrixDevice(
  homeserver: string,
  accessToken: string,
  options: MatrixDeviceLogoutOptions = {},
): Promise<boolean> {
  const timeout = options.timeoutMs ?? MATRIX_DEVICE_LOGOUT_TIMEOUT_MS;
  const controller = new AbortController();
  let timer: ReturnType<typeof setTimeout> | undefined;
  // La carrera además de la señal: un fetch que no atienda la cancelación tampoco espera.
  const expired = new Promise<false>((resolve) => {
    timer = setTimeout(() => {
      controller.abort();
      resolve(false);
    }, timeout);
  });
  const attempt = (async (): Promise<boolean> => {
    try {
      const response = await (options.fetch ?? fetch)(
        `${homeserver.replace(/\/+$/, '')}/_matrix/client/v3/logout`,
        {
          method: 'POST',
          headers: {
            authorization: `Bearer ${accessToken}`,
            'content-type': 'application/json',
          },
          body: '{}',
          // El token va al homeserver de su sesión y a ningún otro sitio.
          credentials: 'omit',
          redirect: 'error',
          referrerPolicy: 'no-referrer',
          cache: 'no-store',
          signal: controller.signal,
        },
      );
      if (response.ok) return true;
      if (response.status !== 401) return false;
      const body = (await response.json().catch(() => null)) as {
        errcode?: unknown;
        soft_logout?: unknown;
      } | null;
      return (
        body?.errcode === 'M_UNKNOWN_TOKEN' &&
        (!('soft_logout' in body) || body.soft_logout === false)
      );
    } catch {
      return false;
    }
  })();
  try {
    return await Promise.race([attempt, expired]);
  } finally {
    clearTimeout(timer);
  }
}
