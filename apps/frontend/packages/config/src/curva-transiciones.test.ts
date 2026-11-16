import { expect, test } from 'bun:test';
import { dirname, resolve } from 'node:path';
import { fileURLToPath } from 'node:url';
import { compile } from 'tailwindcss';

// §1.5 del sistema de diseño: hover, foco y pulsado van con la curva de la casa
// (`--ease-standard`), no con la que trae Tailwind para `transition-*`
// (`cubic-bezier(0.4, 0, 0.2, 1)`). Se compila de verdad, con el mismo
// `@import "tailwindcss"` + theme.css que usan las apps, para que lo que se vigile sea lo
// que acaba en el CSS servido y no un texto de la hoja. La nativa lo lee de aquí en
// `ui.TestLasTransicionesDeColorLlevanLaCurvaDeLaHoja` (apps/fyne).
const aqui = dirname(fileURLToPath(import.meta.url));

async function compilar(clases: string[]): Promise<string> {
  const entrada = `@import 'tailwindcss';\n@import './theme.css';\n`;
  const compilador = await compile(entrada, {
    base: aqui,
    async loadStylesheet(id: string, base: string) {
      const ruta =
        id === 'tailwindcss' ? require.resolve('tailwindcss/index.css') : resolve(base, id);
      return { path: ruta, base: dirname(ruta), content: await Bun.file(ruta).text() };
    },
  });
  return compilador.build(clases);
}

test('las transiciones de hover usan la curva estándar de la casa', async () => {
  const css = await compilar(['transition-colors', 'transition', 'transition-all']);
  expect(css).toContain('--default-transition-timing-function: var(--ease-standard);');
  expect(css).toContain('--ease-standard: cubic-bezier(0.2, 0, 0, 1);');
  expect(css).toMatch(
    /\.transition-colors\s*\{[^}]*transition-timing-function:\s*var\(--tw-ease,\s*var\(--default-transition-timing-function\)\)/,
  );
  // La de Tailwind no puede quedar como valor por defecto de ninguna transición.
  expect(css).not.toMatch(
    /--default-transition-timing-function:\s*cubic-bezier\(0\.4,\s*0,\s*0\.2,\s*1\)/,
  );
});
