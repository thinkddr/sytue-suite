/**
 * Mis dispositivos.
 *
 * SIN la cookie de dispositivo de Sytue ID: esta app comparte origen con el IdP y
 * no lee ni reenvía sus cookies (docs/security.md#riesgo-aceptado-mi-cuenta-en-el-origen-del-idp).
 * La lista sale igual; lo que no se ilumina es la marca de «este dispositivo».
 */
import type { APIRoute } from 'astro';
import { proxyId } from '../../lib/bff.js';

export const prerender = false;

export const GET: APIRoute = async (ctx) => proxyId(ctx, '/api/v1/me/devices');
