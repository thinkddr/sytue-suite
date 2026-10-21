/**
 * Proxy del BFF hacia el servicio Go de Sytue Cast usando el proxy compartido.
 */
import type { APIRoute } from 'astro';
import { crearProxyDeServicio } from '@sytue/auth/server';
import { bff } from '../../lib/bff.js';
import { DESTINO } from '../../lib/servicio.js';

export const prerender = false;

const proxy: APIRoute = crearProxyDeServicio({
  destino: DESTINO,
  bff,
  rutasPublicas: ['/api/v1/public/'],
  // La denuncia de quien no ha entrado: un vídeo (o algo de él) y un canal. El
  // servicio exige la prueba de trabajo y limita por IP.
  escriturasPublicas: [
    /^\/api\/v1\/public\/videos\/[0-9a-f-]{36}\/reports$/i,
    /^\/api\/v1\/public\/channels\/[a-z0-9-]{1,63}\/reports$/,
  ],
});

export const GET = proxy;
export const POST = proxy;
export const PUT = proxy;
export const PATCH = proxy;
export const DELETE = proxy;
