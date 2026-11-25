/* SPDX-License-Identifier: Apache-2.0 */
/**
 * `createClient` de matrix-js-sdk tras un módulo propio, para el `import()` de
 * `auth.ts`: así el SDK sigue bajando solo después de limpiar la URL del retorno de
 * SSO, y el `import()` nunca apunta a un módulo que otro importa de forma estática
 * (`client.ts` importa el SDK entero; donde los dos acaban en el mismo trozo, como en
 * Rooms, el build avisaba INEFFECTIVE_DYNAMIC_IMPORT).
 */
export { createClient } from 'matrix-js-sdk';
