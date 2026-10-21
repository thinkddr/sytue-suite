/*
 * Middleware de Sytue DevX: la organización en el path (ADR 0056) + el estándar
 * de la suite.
 *
 * `middlewareDeOrganizacion` atiende `cloud.sytue.eu/cloud/<org>/...`: una URL
 * con org se reescribe por dentro a la ruta real (las páginas siguen donde
 * estaban) y deja la org en `locals.org`; una URL sin org CON sesión salta a la
 * canónica; sin sesión pasa, como siempre. La org del path NO autoriza: qué
 * proyectos puede tocar quién lo sigue decidiendo el token y el control-plane.
 *
 * Sin org se dejan las rutas comunes (api, auth, assets…) más las de esta app:
 * las OCHO rutas de página, todas con nombre de slug válido (`audit`, `billing`,
 * `projects`, `settings`…), que sin excepción colisionan con el namespace de
 * organizaciones: `/cloud/projects/x` tratado como org se reescribiría a
 * `/cloud/x` —un 404—. El guard de cada página compone ahí la canónica con la
 * org de la sesión (`canonicaConOrg`, @sytue/config), el mismo salto local que
 * el middleware aplica a lo que no colisiona.
 *
 * El idioma y las cabeceras de la casa los pone `@sytue/config/astro-middleware`,
 * compartido por las doce superficies. Aquí solo queda la composición.
 */
import { sequence } from 'astro:middleware';
import {
  cabecerasCerradas,
  middlewareDeIdioma,
  middlewareDeOrganizacion,
} from '@sytue/config/astro-middleware';
import { serverEnv } from '@sytue/auth/server';
import { getBff, toBffContext } from './server/bff.js';

export const onRequest = sequence(
  middlewareDeOrganizacion({
    base: '/cloud',
    sesion: async (context) => getBff().getSession(toBffContext(context)),
    sinOrg: ['audit', 'billing', 'projects', 'settings', 'team', 'templates', 'tokens', 'usage'],
  }),
  // CSP cerrada (FE-03), medida en producción: su origen y el Hub.
  middlewareDeIdioma(undefined, () =>
    cabecerasCerradas(serverEnv(import.meta.env).SYTUE_HUB_BASE_URL),
  ),
);
