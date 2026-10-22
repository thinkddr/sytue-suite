/**
 * BFF de «Compartir un álbum con una persona»: el común de la suite
 * (`rutasDeCompartir`, @sytue/auth/server) sobre `/api/v1/albums/{id}/people` del
 * servicio. El correo se traduce aquí a su `sub` con Sytue ID; el navegador nunca
 * elige a quién se le da el permiso por su identificador.
 *
 *   GET    /api/people?album=<id>
 *   POST   /api/people?album=<id>  {email}
 *   DELETE /api/people?album=<id>&sub=<sub>
 */
import { crearDirectorio, rutasDeCompartir, serverEnv } from '@sytue/auth/server';
import { bff, getApiSession } from '../../lib/bff.js';

export const prerender = false;

const env = serverEnv(import.meta.env);
const destino = (env.SYTUE_PHOTOS_API_URL ?? 'http://syphotos:8097').replace(/\/$/, '');

export const { GET, POST, DELETE } = rutasDeCompartir({
  sesion: getApiSession,
  servicio: () => `${destino}/api/v1`,
  base: (q) => {
    const id = q.get('album');
    return id ? `/albums/${encodeURIComponent(id)}/people` : null;
  },
  directorio: crearDirectorio(bff, import.meta.env),
});
