// Sytue One — configuracion Prettier compartida.
/** @type {import('prettier').Config} */
export const sytuePrettierConfig = {
  semi: true,
  singleQuote: true,
  trailingComma: 'all',
  printWidth: 100,
  tabWidth: 2,
  plugins: ['prettier-plugin-astro', 'prettier-plugin-tailwindcss'],
  overrides: [{ files: '*.astro', options: { parser: 'astro' } }],
};

export default sytuePrettierConfig;
