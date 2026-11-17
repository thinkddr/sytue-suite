/**
 * Tests de Kbd — la tecla dibujada. Lo que se comprueba es el MARCADO, que es donde
 * vive la receta: dos tonos, siempre escondida al lector y con cifras tabulares. Y el
 * guardián de trinquete: ninguna `<kbd>` escrita a mano fuera de `Kbd.tsx`.
 */
import { GlobalRegistrator } from '@happy-dom/global-registrator';
import { expect, test } from 'bun:test';
import { readdirSync, readFileSync, statSync } from 'node:fs';
import { join } from 'node:path';
import { fileURLToPath } from 'node:url';
import { render, type ComponentChild } from 'preact';
import { Kbd } from './Kbd.js';
import { ShortcutsModal } from './ShortcutsModal.js';

// Idempotente: otro fichero de test puede haber registrado ya happy-dom.
if (typeof document === 'undefined') GlobalRegistrator.register();

function pinta(vnode: ComponentChild): HTMLElement {
  const host = document.createElement('div');
  render(vnode, host);
  return host.querySelector('kbd') as HTMLElement;
}

const clases = (el: HTMLElement): string[] => el.className.split(/\s+/);

test('la tecla es una <kbd> escondida al lector, con cifras tabulares', () => {
  const k = pinta(<Kbd>Ctrl+1</Kbd>);
  expect(k.tagName).toBe('KBD');
  expect(k.textContent).toBe('Ctrl+1');
  expect(k.getAttribute('aria-hidden')).toBe('true');
  expect(clases(k)).toContain('tabular-nums');
  // Una letra suelta sale cuadrada: la caja mínima es la misma en alto y en ancho.
  expect(clases(k)).toEqual(expect.arrayContaining(['h-5', 'min-w-5', 'px-1.5', 'text-xs']));
});

test('no se puede volver audible: aria-hidden manda sobre lo que se le pase', () => {
  // El tipo ya lo impide; esto cubre a quien lo fuerce con un `as`.
  const props = { 'aria-hidden': 'false' } as Record<string, string>;
  expect(pinta(<Kbd {...props}>K</Kbd>).getAttribute('aria-hidden')).toBe('true');
});

test('el tono del papel lleva fondo; el del cromo deja ver lo de debajo', () => {
  const papel = clases(pinta(<Kbd>Esc</Kbd>));
  expect(papel).toEqual(
    expect.arrayContaining(['border-border', 'bg-bg-subtle', 'text-text-muted']),
  );

  const cromo = clases(pinta(<Kbd tone="chrome">⌘K</Kbd>));
  expect(cromo).toEqual(
    expect.arrayContaining(['border-chrome-border', 'text-on-chrome-muted', 'bg-transparent']),
  );
  expect(cromo).not.toContain('bg-bg-subtle');
});

test('las dos comparten medida: el tono solo cambia los colores', () => {
  const sinColor = (c: string[]): string[] =>
    c.filter((x) => !/^(border|bg|text)-(?!xs$)/.test(x)).sort();
  expect(sinColor(clases(pinta(<Kbd tone="chrome">K</Kbd>)))).toEqual(
    sinColor(clases(pinta(<Kbd>K</Kbd>))),
  );
});

test('`class` se suma para colocarla', () => {
  expect(clases(pinta(<Kbd class="ml-1">K</Kbd>))).toContain('ml-1');
});

test('la ventana de atajos sigue nombrando las teclas al lector', () => {
  const host = document.createElement('div');
  document.body.appendChild(host);
  render(
    <ShortcutsModal
      open
      title="Atajos"
      groups={[{ titulo: 'Lista', atajos: [{ teclas: ['j', 'k'], descripcion: 'Moverse' }] }]}
      onClose={() => {}}
      container={host}
    />,
    host,
  );
  const dt = document.querySelector('dt') as HTMLElement;
  expect([...dt.querySelectorAll('kbd')].map((k) => k.textContent)).toEqual(['j', 'k']);
  // Las teclas dibujadas están escondidas: sin el texto para el lector, el `<dt>` quedaría
  // mudo y la fila diría «Moverse» sin decir con qué.
  const audible = dt.querySelector('.sr-only');
  expect(audible?.textContent).toBe('j k');
  render(null, host);
  host.remove();
});

/*
 * UNA COLUMNA DE TECLAS PARA TODA LA HOJA. Con una fila `flex` por atajo, cada
 * descripción empezaba donde acababan SUS teclas y la hoja se leía en zigzag (en el
 * muestrario, «Ver los atajos» tras «?» y «Buscar» tras «Ctrl K» empezaban a unos 40 px
 * una de otra). La rejilla es de la hoja y todo lo de dentro la hereda con `subgrid`:
 * si un nivel deja de heredarla, sus filas vuelven a tener columnas propias.
 */
test('la hoja de atajos comparte una sola columna de teclas entre grupos y filas', () => {
  const host = document.createElement('div');
  document.body.appendChild(host);
  render(
    <ShortcutsModal
      open
      title="Atajos"
      groups={[
        { titulo: 'General', atajos: [{ teclas: ['?'], descripcion: 'Ver los atajos' }] },
        {
          titulo: 'Edición',
          atajos: [
            { teclas: ['Ctrl', 'Mayús', 'Z'], descripcion: 'Rehacer' },
            { teclas: ['Ctrl', 'Alt', 'Mayús', 'V'], descripcion: 'Pegar sin formato' },
          ],
        },
      ]}
      onClose={() => {}}
      container={host}
    />,
    host,
  );
  const secciones = [...document.querySelectorAll('[data-sytue-dialog-body] section')];
  expect(secciones).toHaveLength(2);
  const hoja = secciones[0]!.parentElement as HTMLElement;
  expect(clases(hoja)).toContain('grid');
  // `fit-content(50%)`: lo que la combinación más ancha, sin pasar de media hoja.
  expect(clases(hoja)).toContain('grid-cols-[fit-content(50%)_minmax(0,1fr)]');
  const heredan = [
    ...secciones,
    ...document.querySelectorAll('[data-sytue-dialog-body] dl'),
    ...[...document.querySelectorAll('[data-sytue-dialog-body] dt')].map((dt) => dt.parentElement!),
  ] as HTMLElement[];
  expect(heredan).toHaveLength(2 + 2 + 3);
  for (const el of heredan) {
    expect(clases(el)).toContain('grid-cols-subgrid');
    expect(clases(el)).toContain('col-span-full');
  }
  // Una combinación que no cabe en media hoja baja de línea en vez de empujar el texto.
  for (const dt of document.querySelectorAll('[data-sytue-dialog-body] dt')) {
    expect(clases(dt as HTMLElement)).toContain('flex-wrap');
  }
  render(null, host);
  host.remove();
});

/*
 * GUARDIÁN DE TRINQUETE. Había una receta de `<kbd>` por sitio y ninguna se parecía a
 * la de al lado; la cuenta permitida es CERO y no sube: una tecla nueva se pinta con
 * `Kbd` (y si su aspecto no encaja, se añade un tono allí, no una receta aquí).
 */
const FRONTEND = fileURLToPath(new URL('../../../../', import.meta.url));
const PROPIO = join(FRONTEND, 'packages/ui/src/components/Kbd.tsx');

function fuentes(dir: string, out: string[] = []): string[] {
  for (const e of readdirSync(dir)) {
    if (e === 'node_modules' || e === 'dist' || e === '.astro' || e.startsWith('.')) continue;
    const p = join(dir, e);
    if (statSync(p).isDirectory()) fuentes(p, out);
    else if (/\.(tsx|astro)$/.test(e) && !e.includes('.test.')) out.push(p);
  }
  return out;
}

test('no queda ninguna <kbd> escrita a mano fuera de Kbd.tsx', () => {
  const malos: string[] = [];
  for (const raiz of ['apps', 'packages']) {
    for (const f of fuentes(join(FRONTEND, raiz))) {
      if (f === PROPIO) continue;
      const src = readFileSync(f, 'utf8');
      for (const m of src.matchAll(/<kbd\b/g)) {
        const linea = src.slice(0, m.index).split('\n').length;
        malos.push(`${f.slice(FRONTEND.length)}:${linea}`);
      }
    }
  }
  expect(malos).toEqual([]);
});
