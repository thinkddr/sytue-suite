/*
 * LOS BLOQUES DEL EDITOR SON LOS MISMOS EN LA WEB Y EN EL SERVICIO.
 *
 * Lo que la web deja pegar (`BLOQUES_DE_NOTAS` en @sytue/ui/browser/portapapeles) y lo
 * que el servicio deja guardar (`apps/nucleo/notes/model/html.go`, que también lee Fyne)
 * tienen que ser la misma lista. Si la web admite un valor que el servicio tira, el
 * bloque se ve al escribir y desaparece al guardar; al revés, un pegado pierde lo que
 * la nota sí sabe guardar. Se lee el fuente de Go, como hace Docs con su saneador.
 */
import { expect, test } from 'bun:test';
import { readFileSync } from 'node:fs';
import { BLOQUES_DE_NOTAS } from '@sytue/ui/browser/portapapeles';
import { idDeAdjunto } from '@sytue/ui/browser/bloques-de-notas';

const GO = readFileSync(
  new URL('../../../../../nucleo/notes/model/html.go', import.meta.url),
  'utf8',
);

/** Los valores de `nombre = []string{…}` en el fuente de Go. */
function listaGo(nombre: string): string[] {
  const m = new RegExp(`${nombre}\\s*=\\s*\\[\\]string\\{([^}]*)\\}`).exec(GO);
  if (!m) throw new Error(`no encuentro ${nombre} en html.go`);
  return [...m[1]!.matchAll(/"([^"]+)"/g)].map((x) => x[1]!);
}

/** Un bloque `nombre = map[…]{ … }` del fuente de Go, hasta su cierre. */
function bloqueGo(nombre: string): string {
  const i = GO.indexOf(`${nombre} = map[`);
  if (i < 0) throw new Error(`no encuentro ${nombre} en html.go`);
  return GO.slice(i, GO.indexOf('\n\t}', i));
}

/** `"etiqueta": {…}` de un mapa de Go: atributo → lista literal o variable. */
function porEtiqueta(bloque: string): Record<string, Record<string, string[]>> {
  const out: Record<string, Record<string, string[]>> = {};
  for (const [, etiqueta = '', cuerpo = ''] of bloque.matchAll(/^\t\t"(\w+)":\s*\{(.*)\},?$/gm)) {
    out[etiqueta] = {};
    for (const [, attr = '', valor = ''] of cuerpo.matchAll(/"([\w-]+)":\s*(\{[^}]*\}|\w+)/g))
      out[etiqueta][attr] = valor.startsWith('{')
        ? [...valor.matchAll(/"([^"]+)"/g)].map((x) => x[1]!)
        : listaGo(valor);
  }
  return out;
}

test('los atributos con valores de lista son los mismos, con los mismos valores', () => {
  const go = porEtiqueta(bloqueGo('bloquesEnum'));
  const web = Object.fromEntries(
    Object.entries(BLOQUES_DE_NOTAS.enumerados).map(([e, attrs]) => [
      e,
      Object.fromEntries(Object.entries(attrs).map(([a, v]) => [a, [...v]])),
    ]),
  );
  expect(Object.keys(go).length).toBeGreaterThan(4); // el patrón sigue leyendo el fuente
  expect(go).toEqual(web);
});

test('los atributos sin valor son los mismos', () => {
  const go: Record<string, string[]> = {};
  for (const [, e = '', cuerpo = ''] of bloqueGo('bloquesSinValor').matchAll(
    /^\t\t"(\w+)":\s*\{([^}]*)\},?$/gm,
  ))
    go[e] = [...cuerpo.matchAll(/"([^"]+)"/g)].map((x) => x[1]!);
  expect(go).toEqual(
    Object.fromEntries(Object.entries(BLOQUES_DE_NOTAS.sinValor).map(([e, v]) => [e, [...v]])),
  );
});

test('los atributos numéricos (celdas combinadas) son los mismos, con los mismos topes', () => {
  const go: Record<string, Record<string, number>> = {};
  for (const [, e = '', cuerpo = ''] of bloqueGo('bloquesNumericos').matchAll(
    /^\t\t"(\w+)":\s*\{([^}]*)\},?$/gm,
  ))
    go[e] = Object.fromEntries(
      [...cuerpo.matchAll(/"([\w-]+)":\s*(\d+)/g)].map((x) => [x[1]!, Number(x[2])]),
    );
  expect(Object.keys(go).length).toBeGreaterThan(1);
  expect(go).toEqual(
    Object.fromEntries(
      Object.entries(BLOQUES_DE_NOTAS.numericos).map(([e, v]) => [e, { ...v }]),
    ) as Record<string, Record<string, number>>,
  );
});

test('el id de un adjunto tiene la misma forma en la web y en el servicio', () => {
  expect(GO).toContain('regexp.MustCompile(`^adjunto:[A-Za-z0-9_-]{1,64}$`)');
  expect(idDeAdjunto('adjunto:1732540000000-ab12cd')).toBe('1732540000000-ab12cd');
  for (const malo of [
    'adjunto:../x',
    ' adjunto:a',
    'adjunto:',
    'https://x',
    `adjunto:${'a'.repeat(65)}`,
  ])
    expect(idDeAdjunto(malo)).toBeNull();
});

test('las etiquetas de los bloques están en la lista del servicio', () => {
  for (const etiqueta of BLOQUES_DE_NOTAS.etiquetas) expect(GO).toContain(`"${etiqueta}": true`);
});
