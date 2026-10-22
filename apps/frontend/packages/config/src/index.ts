/**
 * @sytue/config — configuracion compartida del frontend Sytue One.
 * Tokens tipados (seguros para browser/SSR), y (via package.json exports)
 * tsconfig base, ESLint plano y Prettier.
 *
 * NOTA: los helpers de Tailwind v4 (`./tailwind`) NO se re-exportan aqui a
 * proposito: usan `node:url` (Node-only) y arrastrarlos al barrel `.` los
 * introduciria en los bundles de browser/SSR de las apps, provocando el WARN
 * "Module 'node:url' has been externalized for browser compatibility".
 * Se consumen unicamente desde configuracion de build via el subpath
 * `@sytue/config/tailwind` (server-only).
 */
export * from './tokens.js';
export * from './organizacion.js';
export * from './office.js';
export * from './altcha.js';
export * from './viewport.js';
