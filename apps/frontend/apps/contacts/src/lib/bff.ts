/**
 * BFF de Sytue Contacts (solo servidor). Client OIDC confidencial `sytue-contacts`
 * del realm `sytue`. El secreto del client se resuelve del entorno; nunca llega al
 * navegador. docs/CONVENTIONS.md §8.
 *
 * El montaje —config OIDC, secreto, `basePath`, cookies— y los dos guards están
 * en `crearBffDeApp` (`@sytue/auth/server`): eran el MISMO fichero en siete apps.
 */
import { crearBffDeApp } from '@sytue/auth/server';

export const { bff, requirePageSession, getApiSession } = crearBffDeApp(import.meta.env);
export { toBffContext } from '@sytue/auth/server';
