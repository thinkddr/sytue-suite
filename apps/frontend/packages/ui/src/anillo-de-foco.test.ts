/**
 * El anillo de foco está declarado en DOS sitios —el global de `theme.css` y las
 * variantes de `styles.css`— y tienen que medir lo mismo.
 *
 * Divergieron, y así es como el foco de un campo acabó midiendo 6px: `Form.tsx`
 * se escribía el suyo a 3px de latón más 2px de contorno mientras el sistema iba
 * a 2+2. Nadie lo vio porque cada definición, por separado, parecía razonable.
 *
 * También se comprueba que ningún anillo se queda sin el contorno neutro: el
 * latón solo mide 2,81:1 sobre marfil, por debajo del 3:1 de WCAG 1.4.11, y en
 * tema oscuro sí pasa — que es lo que hace que el fallo sobreviva a mirarlo.
 * Ejecutar: `bun test`.
 */
import { describe, expect, test } from 'bun:test';

const theme = await Bun.file(new URL('../../config/src/theme.css', import.meta.url)).text();
const estilos = await Bun.file(new URL('./styles.css', import.meta.url)).text();

/** Los `Npx` de la primera declaración `box-shadow` que siga al selector dado. */
function anillo(css: string, selector: string): string[] {
  const desde = css.indexOf(selector);
  expect(desde, `no se encuentra ${selector}`).toBeGreaterThan(-1);
  const bloque = css.slice(desde, css.indexOf('}', desde));
  const sombra = bloque.slice(bloque.indexOf('box-shadow'));
  return [...sombra.matchAll(/(\d+)px/g)].map((m) => m[1] ?? '');
}

describe('anillo de foco', () => {
  test('el global y el canónico miden lo mismo', () => {
    expect(anillo(estilos, '.sytue-focus-ring:focus-visible')).toEqual(
      anillo(theme, ':where(:focus-visible)'),
    );
  });

  test('son 2px de anillo, ni uno más', () => {
    // 2px = el área que pide WCAG 2.4.13, y el 3:1 de WCAG 1.4.11 lo cumple ya el
    // propio token. Un tercer píxel de contorno es lo que hacía que un campo
    // enfocado se leyera como un marco negro.
    expect(anillo(theme, ':where(:focus-visible)')).toEqual(['2']);
    expect(anillo(estilos, '.sytue-focus-ring-inner:focus-visible')).toEqual(['2']);
    // La variante para rellenos sólidos abre antes 1px de hueco: 1 → 3.
    expect(anillo(estilos, '.sytue-focus-ring-inset:focus-visible')).toEqual(['1', '3']);
  });

  test('el global no fija la esquina: manda la del control', () => {
    // Sin capa, un `border-radius` aquí gana a cualquier `rounded-*` (que vive en
    // `@layer utilities`): un botón `rounded-md` o una pastilla pasaban a 4px al
    // enfocarlos con el teclado. La esquina de reserva va en `@layer base`.
    const global = theme.slice(theme.indexOf(':where(:focus-visible)'));
    expect(global.slice(0, global.indexOf('}'))).not.toContain('border-radius');
    const base = theme.slice(theme.indexOf('@layer base {'));
    expect(base.slice(0, base.indexOf('}'))).toMatch(/:focus-visible\s*\{\s*border-radius/);
  });

  test('la casilla y el radio: el aro va en el envoltorio y el input no pinta el suyo', () => {
    expect(anillo(estilos, '.sytue-focus-ring-choice:has(> :focus-visible)')).toEqual(['2']);
    const bloque = estilos.slice(estilos.indexOf('.sytue-focus-ring-choice > :focus-visible'));
    expect(bloque.slice(0, bloque.indexOf('}'))).toContain('box-shadow: none');
  });

  test('ninguna variante resucita el contorno neutro', () => {
    // Se retiró a propósito: era la línea negra que rodeaba cada campo. Si alguien
    // lo vuelve a meter, el aro engorda otra vez en las cinco apps a la vez.
    for (const v of ['', '-inner', '-inset']) {
      const bloque = estilos.slice(estilos.indexOf(`.sytue-focus-ring${v}:focus-visible`));
      expect(bloque.slice(0, bloque.indexOf('}'))).not.toContain('--sytue-focus-contrast');
    }
  });
});

describe('las variantes nuevas', () => {
  test('-within pinta en la etiqueta cuando el foco está DENTRO, con los mismos grosores', () => {
    expect(anillo(estilos, '.sytue-focus-ring-within:has(:focus-visible)')).toEqual(['2']);
    expect(anillo(estilos, '.sytue-focus-ring-inset-within:has(:focus-visible)')).toEqual([
      '1',
      '3',
    ]);
  });

  test('-chrome cambia el COLOR del aro, no pinta una sombra propia', () => {
    const bloque = estilos.slice(estilos.indexOf('.sytue-focus-ring-chrome {'));
    expect(bloque.slice(0, bloque.indexOf('}'))).toMatch(
      /--sytue-focus-ring:\s*var\(--sytue-chrome-focus\)/,
    );
  });
});

/*
 * NINGÚN `focus-visible:shadow-[…]` NI `focus-visible:ring-…` EN LA SUITE: no pintan nada.
 *
 * La utilidad vive en `@layer utilities` y el aro global de theme.css va sin capa, así
 * que gana siempre el global. Había 33 y ninguno se veía: el blanco del cromo salía
 * azul (3,4:1 sobre el carril de Calendar, medido) y los `inset` salían por fuera.
 * `ring-*` es la misma sombra con otro nombre: el `ring-primary/40` del editor de Notes
 * y el `ring-inset` de las versiones de Drive pintaban el global, 2 px por fuera.
 * SI ESTE TEST TE PARA: una variante de `styles.css` (`-inner`, `-inset`, `-within`)
 * o, para cambiar el color, la variable (`sytue-focus-ring-chrome`).
 * `peer-`/`group-focus-visible:` sí valen: pintan en OTRO nodo, que no tiene el foco.
 */
test('nadie pinta el aro con una utilidad de sombra ni de anillo', async () => {
  const raiz = new URL('../../../', import.meta.url).pathname; // apps/frontend/
  const muerta = /(?<![\w-])(?:focus-visible|has-\[:focus-visible\]):(?:shadow-\[|ring-)/g;
  const con: string[] = [];
  const glob = new Bun.Glob('{apps,packages}/*/src/**/*.{ts,tsx,astro}');
  for await (const rel of glob.scan({ cwd: raiz })) {
    if (rel.includes('.test.')) continue;
    if (muerta.test(await Bun.file(raiz + rel).text())) con.push(rel);
    muerta.lastIndex = 0;
  }
  expect(con).toEqual([]);
});
