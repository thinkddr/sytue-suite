/** POST /f/<token>/enviar — un envío del formulario público (sin sesión; ALTCHA + cupo). */
import type { APIRoute } from 'astro';
import { reenviar } from '../../../lib/formulario.js';

export const prerender = false;

export const POST: APIRoute = ({ params, request }) => reenviar(params.token ?? '', '', request);
