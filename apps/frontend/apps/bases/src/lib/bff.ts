/**
 * BFF de Sytue Bases (solo servidor). Client OIDC confidencial `sytue-bases`.
 * El secreto del client se resuelve del entorno; nunca llega al navegador.
 * docs/CONVENTIONS.md §8.
 */
import { crearBffDeApp } from '@sytue/auth/server';

export const { bff, requirePageSession, getApiSession } = crearBffDeApp(import.meta.env);
export { toBffContext } from '@sytue/auth/server';
