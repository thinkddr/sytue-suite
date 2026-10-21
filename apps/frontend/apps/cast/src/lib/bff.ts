/**
 * BFF de Sytue Cast (solo servidor). Client OIDC confidencial `sytue-cast`.
 * El secreto del client se resuelve del entorno; nunca llega al navegador.
 * docs/CONVENTIONS.md §8; CAST-ENCARGO §2.
 *
 * El montaje y los dos guards están en `crearBffDeApp` (`@sytue/auth/server`).
 */
import { crearBffDeApp } from '@sytue/auth/server';

export const { bff, requirePageSession, getApiSession } = crearBffDeApp(import.meta.env);
export { toBffContext } from '@sytue/auth/server';
