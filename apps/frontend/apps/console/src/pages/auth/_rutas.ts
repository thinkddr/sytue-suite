/**
 * Handlers de /auth/* de esta app: el BFF de `src/server/bff.ts` atado a las rutas
 * comunes de @sytue/auth. Los ficheros de al lado solo reexportan de aquí y
 * tienen que seguir existiendo, porque Astro enruta por fichero. El guion bajo
 * inicial deja a este fuera del routing.
 *
 * `getBff` se pasa SIN LLAMAR: construye el BFF en la primera petición y lo
 * cachea. Invocarlo aquí lo construiría al cargar el módulo, y entonces un
 * `resolveOidcConfig` que lanza por una SYTUE_OIDC_* ausente dejaría de ser un
 * 500 en el endpoint para impedir que la app arranque.
 */
import { rutasDeAuth } from '@sytue/auth/server';
import { getBff, toBffContext } from '../../server/bff.js';

export const { login, callback, logout, refresh, session, organizations, avatar, token } =
  rutasDeAuth(getBff, toBffContext);
