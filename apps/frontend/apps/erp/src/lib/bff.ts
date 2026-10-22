/**
 * BFF de SyERP (solo servidor). Client OIDC confidencial `sytue-syerp` del realm
 * `sytue`. El secreto del client se resuelve del entorno; nunca llega al
 * navegador. docs/CONVENTIONS.md §8.
 */
import { crearBffDeApp, serverEnv } from '@sytue/auth/server';

export const { bff, requirePageSession, getApiSession } = crearBffDeApp(import.meta.env);
export { toBffContext } from '@sytue/auth/server';

const env = serverEnv(import.meta.env);

/**
 * URL interna del servicio Go. Por el Service INTERNO: esto corre en un pod y el
 * nombre público no resuelve desde dentro.
 */
export function apiUrl(): string {
  return (
    env.SYTUE_SYERP_INTERNAL_URL ??
    env.SYTUE_SYERP_BASE_URL ??
    'https://erp.sytue.eu'
  ).replace(/\/$/, '');
}
