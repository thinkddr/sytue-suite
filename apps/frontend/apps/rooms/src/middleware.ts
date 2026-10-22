/*
 * Middleware de Sytue Rooms: la organización en el path (ADR 0056) + el estándar
 * de la suite con sus permisos de medios.
 *
 * El idioma y las cabeceras de la casa los pone `@sytue/config/astro-middleware`,
 * compartido por las doce superficies. La organización como segmento de path
 * (`rooms.sytue.eu/rooms/<org>/channels/x`) la compone el middleware compartido
 * `middlewareDeOrganizacion` PRIMERO: reescribe por dentro las URL con org, manda
 * a la canónica las sin org cuando hay sesión, y deja la org en `locals.org`.
 * La org del path NO autoriza: eso sigue en el token y en el servicio.
 *
 * Sin org se dejan las rutas comunes (api, auth, assets...) más las de esta app:
 * los app-links de Android/iOS (`.well-known`), que el sistema pide sin sesión
 * al instalar y no pueden llevar ni org ni redirección, y las TRES rutas de
 * página cuyo nombre es también un slug válido (`channels`, `pip`, `share`):
 * si el middleware las tratara como org, el deep-link antiguo
 * `/rooms/channels/x` se reescribiría a `/rooms/x` —un 404— y `/rooms/share`
 * abriría el chat home. El guard de cada página compone ahí la canónica con la
 * org de la sesión (`canonicaConOrg`, @sytue/config), que es el mismo salto
 * local que el middleware aplica a las rutas que no colisionan.
 */
import { sequence } from 'astro:middleware';
import { suiteApex } from '@sytue/config';
import {
  cabecerasCerradas,
  middlewareDeIdioma,
  middlewareDeOrganizacion,
} from '@sytue/config/astro-middleware';
import { SYTUE_PERMISSIONS_POLICY_ROOMS } from '@sytue/config/astro-security';
import { serverEnv } from '@sytue/auth/server';
import { bff, toBffContext } from './lib/bff.js';

const sfuOrigen = (): string[] => {
  const url = serverEnv(import.meta.env).SYTUE_LIVEKIT_URL ?? `wss://rooms-sfu.${suiteApex}`;
  return URL.canParse(url) ? [new URL(url).origin] : [`wss://rooms-sfu.${suiteApex}`];
};

export const onRequest = sequence(
  middlewareDeOrganizacion({
    base: '/rooms',
    sesion: async (context) => bff.getSession(toBffContext(context)),
    sinOrg: ['.well-known', 'channels', 'pip', 'share', 'matrix'],
  }),
  // Rooms es la única superficie con cámara, micrófono y captura de pantalla:
  // su Permissions-Policy las concede a su propio origen y apaga el resto, y su
  // CSP cerrada añade la señalización del SFU (`wss://rooms-sfu.<dominio>`).
  middlewareDeIdioma(
    () => ({
      'permissions-policy': SYTUE_PERMISSIONS_POLICY_ROOMS,
      'cross-origin-opener-policy': 'same-origin-allow-popups',
    }),
    () =>
      cabecerasCerradas(serverEnv(import.meta.env).SYTUE_HUB_BASE_URL, {
        conectar: sfuOrigen(),
      }),
  ),
);
