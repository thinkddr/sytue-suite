/**
 * Clic derecho → menú «⋯» del elemento (`menu-contextual.ts`). Ejecutar: `bun test`.
 *
 * Se monta un `Dropdown` de `@sytue/ui` de verdad dentro de una fila, como hace
 * `ActionsMenu`, y se dispara `contextmenu` sobre la fila: tiene que salir el menú de
 * ESA fila, con el foco dentro, y Escape devolverlo al «⋯». Además se vigila el fuente
 * para que las filas, las teselas y las filas planas de Drive lleven el manejador. La
 * pareja nativa son los `TestElClicDerecho*` de `apps/fyne`.
 */
import { GlobalRegistrator } from '@happy-dom/global-registrator';
import { afterEach, expect, test } from 'bun:test';
import { readFileSync } from 'node:fs';
import { render } from 'preact';
import { Dropdown, MenuItem } from '@sytue/ui';
import { abrirMenuDeFila } from './menu-contextual';

if (typeof document === 'undefined') GlobalRegistrator.register();

/** Los efectos de Preact corren «tras el pintado»; en happy-dom, con un setTimeout. */
const tick = (): Promise<void> => new Promise((r) => setTimeout(r, 150));

function Fila({ nombre, conMenu = true }: { nombre: string; conMenu?: boolean }) {
  return (
    <div role="button" tabIndex={0} data-fila={nombre} onContextMenu={abrirMenuDeFila}>
      <span>{nombre}</span>
      {conMenu ? (
        <Dropdown
          label={`Acciones de ${nombre}`}
          trigger={({ onClick, ...aria }) => (
            <button type="button" onClick={onClick} data-menu-trigger="" {...aria}>
              ⋯
            </button>
          )}
        >
          <MenuItem onClick={() => {}}>{`Descargar ${nombre}`}</MenuItem>
          <MenuItem onClick={() => {}}>{`Renombrar ${nombre}`}</MenuItem>
        </Dropdown>
      ) : null}
    </div>
  );
}

let host: HTMLElement | null = null;
afterEach(() => {
  if (host) render(null, host);
  host?.remove();
  host = null;
});

function montar(): HTMLElement {
  host = document.body.appendChild(document.createElement('div'));
  render(
    <>
      <Fila nombre="a.pdf" />
      <Fila nombre="b.txt" />
      <Fila nombre="c.md" conMenu={false} />
    </>,
    host,
  );
  return host;
}

function clicDerecho(el: Element): MouseEvent {
  const e = new MouseEvent('contextmenu', { bubbles: true, cancelable: true, button: 2 });
  el.dispatchEvent(e);
  return e;
}

test('el clic derecho sobre una fila abre SU menú, con el foco dentro', async () => {
  const h = montar();
  const e = clicDerecho(h.querySelector('[data-fila="b.txt"] span')!);
  await tick();
  expect(e.defaultPrevented).toBe(true);
  const menu = document.querySelector<HTMLElement>('[role="menu"]');
  expect(menu).not.toBeNull();
  expect(menu!.textContent).toContain('Descargar b.txt');
  expect(menu!.textContent).not.toContain('a.pdf');
  expect(menu!.contains(document.activeElement)).toBe(true);

  // Escape cierra y devuelve el foco al «⋯» de esa fila.
  document.dispatchEvent(new KeyboardEvent('keydown', { key: 'Escape', bubbles: true }));
  await tick();
  expect(document.querySelector('[role="menu"]')).toBeNull();
  expect(document.activeElement).toBe(h.querySelector('[data-fila="b.txt"] [data-menu-trigger]'));
});

test('el clic derecho sobre el «⋯» con su menú abierto no lo cierra', async () => {
  const h = montar();
  const mas = h.querySelector('[data-fila="a.pdf"] [data-menu-trigger]')!;
  clicDerecho(mas);
  await tick();
  expect(document.querySelector('[role="menu"]')).not.toBeNull();
  // Sobre el propio disparador el `mousedown` no cuenta como fuera del menú.
  mas.dispatchEvent(new MouseEvent('mousedown', { bubbles: true, button: 2 }));
  const e = clicDerecho(mas);
  await tick();
  expect(e.defaultPrevented).toBe(true);
  expect(document.querySelector('[role="menu"]')?.textContent).toContain('Descargar a.pdf');
});

test('mantener pulsada el asa de mover no abre el menú', async () => {
  const h = montar();
  const asa = h.querySelector('[data-fila="a.pdf"]')!.appendChild(document.createElement('button'));
  asa.setAttribute('data-drive-move', '/a.pdf');
  const e = clicDerecho(asa);
  await tick();
  expect(e.defaultPrevented).toBe(false);
  expect(document.querySelector('[role="menu"]')).toBeNull();
});

test('sin «⋯» en la fila no pasa nada y queda el menú del navegador', async () => {
  const h = montar();
  const e = clicDerecho(h.querySelector('[data-fila="c.md"]')!);
  await tick();
  expect(e.defaultPrevented).toBe(false);
  expect(document.querySelector('[role="menu"]')).toBeNull();
});

test('las filas, las teselas y las filas planas de Drive llevan el manejador', () => {
  const fuente = readFileSync(new URL('../components/FileBrowser.tsx', import.meta.url), 'utf8');
  const tramo = (desde: string, hasta: string): string =>
    fuente.slice(fuente.indexOf(desde), fuente.indexOf(hasta, fuente.indexOf(desde)));
  // El «⋯» de ActionsMenu es el disparador que se pulsa.
  expect(tramo('function ActionsMenu(', 'function MoveDialog(')).toMatch(
    /title=\{t\('action\.more'\)\}\s+data-menu-trigger=""/,
  );
  for (const [desde, hasta] of [
    ['function FileRow(', 'function FileGridSkeleton('],
    ['function FileGrid(', 'function trashAsResource('],
    ['export function ResourceListView(', 'function PermBadge('],
  ] as const) {
    expect(tramo(desde, hasta)).toContain('onContextMenu={abrirMenuDeFila}');
  }
});
