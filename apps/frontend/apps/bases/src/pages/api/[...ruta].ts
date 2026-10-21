/**
 * Proxy del BFF hacia el servicio Go de Sytue Bases usando el proxy compartido.
 */
import type { APIRoute } from 'astro';
import { crearProxyDeServicio } from '@sytue/auth/server';
import { bff } from '../../lib/bff.js';
import { DESTINO } from '../../lib/servicio.js';

export const prerender = false;

const proxy: APIRoute = crearProxyDeServicio({
  destino: DESTINO,
  bff,
  prefijo: '/api/v1/',
  denegar: (ruta) => ruta.includes('/internal') || ruta.includes('/public'),
});

export const GET = proxy;
export const POST = proxy;
export const PUT = proxy;
export const PATCH = proxy;
export const DELETE = proxy;
