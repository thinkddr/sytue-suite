/**
 * BFF de Sytue Web (solo servidor). Client OIDC confidencial `sytue-pages`.
 * El secreto del client se resuelve del entorno; nunca llega al navegador.
 * docs/CONVENTIONS.md §8; PAGES-PLAN §14.
 */
import { crearBffDeApp } from '@sytue/auth/server';

export const { bff, requirePageSession, getApiSession } = crearBffDeApp(import.meta.env);
export { toBffContext } from '@sytue/auth/server';
