/** Publicación autenticada del sitio; el secreto nunca llega al navegador. */
import { serverEnv } from '@sytue/auth/server';
import { getApiSession } from '../../../lib/bff.js';
import { crearPublicar } from '../../../lib/publicar-bff.js';
import { DESTINO } from '../../../lib/servicio.js';

export const prerender = false;
export const POST = crearPublicar({
  destino: DESTINO,
  sesion: getApiSession,
  secreto: () => serverEnv(import.meta.env).PAGES_PUBLICADOR_SECRET,
});
