// @ts-check
// Sytue One — configuracion ESLint plana (flat config) compartida.
//
// ESLint quedo reducido a los ficheros *.astro: el resto del frontend (.ts, .tsx,
// .js, .mjs) lo lintea oxlint, que es ~40x mas rapido y cubre las mismas reglas
// (ver .oxlintrc.json en la raiz del workspace). oxlint todavia no parsea el
// formato .astro, asi que estas 83 plantillas siguen aqui hasta que lo haga.
import js from '@eslint/js';
import astro from 'eslint-plugin-astro';

/** @type {import('eslint').Linter.Config[]} */
export const sytueEslintConfig = [
  {
    ignores: ['**/dist/**', '**/.astro/**', '**/node_modules/**', '**/*.d.ts'],
  },
  ...astro.configs.recommended,
  {
    files: ['**/*.astro'],
    rules: {
      ...js.configs.recommended.rules,
      // `== null` / `!= null` (comprueba null y undefined a la vez) se permite;
      // el resto de comparaciones debe usar estricta.
      eqeqeq: ['error', 'always', { null: 'ignore' }],
      'no-console': ['warn', { allow: ['warn', 'error'] }],
      // no-useless-assignment (nueva en ESLint 10) marca el init de los patrones try/catch
      // defensivos (`let x = fallback; try { x = compute() } catch { return }`), legitimos
      // aqui. Off para no forzar quitar fallbacks reales (revisable en una pasada dedicada).
      'no-useless-assignment': 'off',
    },
  },
];

export default sytueEslintConfig;
