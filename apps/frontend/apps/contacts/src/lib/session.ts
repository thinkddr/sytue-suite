/**
 * Guard de sesión del lado servidor para Sytue Contacts. El access token solo se
 * usa en el servidor para autorizar CardDAV contra el servicio Go Sytue Contacts
 * (Bearer); nunca se envía al navegador para los datos de contactos.
 *
 * ponytail: el guard vive en `@sytue/auth/server` y se monta en `./bff.js` con el
 * BFF de esta app; aquí solo queda el NOMBRE. Este fichero sigue existiendo
 * porque de él importan más de cien módulos de la suite, y renombrar el import en
 * todos ellos sería un diff enorme para no ganar nada. Si algún día hay que
 * tocarlos por otro motivo, que se aproveche y desaparezca.
 */
export { requirePageSession, getApiSession } from './bff.js';
export type { ServerSession } from '@sytue/auth/server';
