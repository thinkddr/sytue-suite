/** GET /f/<token>/reto — el reto ALTCHA de un enlace de formulario (sin sesión). */
import type { APIRoute } from 'astro';
import { reenviar } from '../../../lib/formulario.js';

export const prerender = false;

export const GET: APIRoute = ({ params, request }) =>
  reenviar(params.token ?? '', '/altcha', request);
