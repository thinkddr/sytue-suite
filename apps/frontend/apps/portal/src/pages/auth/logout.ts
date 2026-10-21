/** POST|GET /auth/logout — sesión local + SSO. El porqué del GET, en `rutas.ts`. */
export const prerender = false;
export { logout as GET, logout as POST } from './_rutas.js';
