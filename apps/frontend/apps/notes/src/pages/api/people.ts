/**
 * BFF de «Compartir con una persona» de Sytue Notepads: el común de la suite
 * (`rutasDeCompartir`, @sytue/auth/server) sobre `/notebooks/{id}/people` y
 * `/notes/{id}/people` del servicio. El correo se traduce aquí a su `sub` con Sytue
 * ID; el navegador nunca elige a quién se le da el permiso por su identificador.
 *
 *   GET    /api/people?notebook=<id> | ?note=<id>
 *   POST   /api/people?…  {email, permission: read|write}
 *   DELETE /api/people?…&sub=<sub>
 */
import { crearDirectorio, rutasDeCompartir } from '@sytue/auth/server';
import { bff, getApiSession } from '../../lib/bff.js';
import { notesApiUrl } from '../../lib/notes.js';

export const prerender = false;

export const { GET, POST, DELETE } = rutasDeCompartir({
  sesion: getApiSession,
  servicio: () => `${notesApiUrl()}/api/v1`,
  base: (q) => {
    const cuaderno = q.get('notebook');
    if (cuaderno) return `/notebooks/${encodeURIComponent(cuaderno)}/people`;
    const nota = q.get('note');
    return nota ? `/notes/${encodeURIComponent(nota)}/people` : null;
  },
  directorio: crearDirectorio(bff, import.meta.env),
});
