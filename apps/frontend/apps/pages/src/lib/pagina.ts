/**
 * Ayudantes de página de Sytue Web.
 * Guarda con motivo: 'solo_organizaciones' (PAGES-PLAN §14).
 */
import type { APIContext } from 'astro';
import { requireSession, serverEnv, toBffContext, type GuardResult } from '@sytue/auth/server';
import { bff } from './bff.js';

export async function protegerPagina(ctx: APIContext): Promise<GuardResult | Response> {
  return requireSession(bff, toBffContext(ctx), {
    orgKinds: ['org'],
    motivo: 'solo_organizaciones',
  });
}

export function hubBaseUrl(): string {
  return serverEnv(import.meta.env).SYTUE_HUB_BASE_URL || 'https://hub.sytue.localhost';
}
