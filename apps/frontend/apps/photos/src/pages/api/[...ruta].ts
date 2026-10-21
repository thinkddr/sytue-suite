/**
 * Proxy del BFF hacia el servicio Go de Sytue Photos usando el proxy compartido.
 */
import type { APIRoute } from 'astro';
import { crearProxyDeServicio, serverEnv } from '@sytue/auth/server';
import { bff } from '../../lib/bff.js';

export const prerender = false;

const env = serverEnv(import.meta.env);
const destino = env.SYTUE_PHOTOS_API_URL ?? 'http://syphotos:8097';

const proxy: APIRoute = crearProxyDeServicio({
  destino,
  bff,
});

export const GET = proxy;
export const POST = proxy;
export const PUT = proxy;
export const DELETE = proxy;
