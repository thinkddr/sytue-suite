/**
 * Handlers de /auth/* de esta app: el BFF de la ofimática (`@sytue/office/auth`)
 * atado a las rutas comunes de @sytue/auth. Los ficheros de al lado solo reexportan de aquí y
 * tienen que seguir existiendo, porque Astro enruta por fichero. El guion bajo
 * inicial deja a este fuera del routing.
 */
import { rutasDeAuth } from '@sytue/auth/server';
import { bff, toBffContext } from '@sytue/office/auth';

export const { login, callback, logout, refresh, session, organizations, token, avatar } =
  rutasDeAuth(() => bff, toBffContext);
