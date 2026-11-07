/**
 * «Marcar como leído» desde el punto azul: es un BOTÓN con nombre, marca ese aviso y
 * nada más —el clic no sube a la fila, que navegaría—.
 * Ejecutar: `bun test`.
 */
import { GlobalRegistrator } from '@happy-dom/global-registrator';
import { afterAll, beforeAll, expect, test } from 'bun:test';
import { render } from 'preact';
import { useState } from 'preact/hooks';
import { MarcarLeido } from './MarcarLeido.js';

// Idempotente: otro fichero de test puede haber registrado ya happy-dom. Y se retira al
// acabar si lo puso este, como en viewportVisual.test.ts: los ficheros corren en el mismo
// proceso, y con happy-dom puesto `localStorage` es de solo lectura y
// useAppsDesplegadas.test.ts no puede sustituirlo («Attempted to assign to readonly
// property», CI de main del 2026-11-06).
let registradoAqui = false;
beforeAll(() => {
  if (typeof document === 'undefined') {
    GlobalRegistrator.register();
    registradoAqui = true;
  }
});
afterAll(async () => {
  if (registradoAqui) await GlobalRegistrator.unregister();
});

test('el punto es un botón con nombre que marca sin que el clic suba a la fila', () => {
  const host = document.createElement('div');
  document.body.appendChild(host);
  const marcados: string[] = [];
  // La «fila» es el contenedor: un clic que suba hasta él navegaría.
  let fila = 0;
  host.addEventListener('click', () => (fila += 1));
  render(<MarcarLeido rotulo="Marcar como leído" alMarcar={() => marcados.push('a')} />, host);
  const boton = host.querySelector('button');
  expect(boton).not.toBeNull();
  expect(boton!.getAttribute('type')).toBe('button');
  expect(boton!.getAttribute('aria-label')).toBe('Marcar como leído');
  // El punto en sí no se anuncia: el nombre es la acción.
  expect(boton!.querySelector('span')!.getAttribute('aria-hidden')).toBe('true');
  boton!.click();
  expect(marcados).toEqual(['a']);
  expect(fila).toBe(0);
  host.remove();
});

// Visto en ROJO a mano: sin el `fila.focus()`, el foco se queda en un botón que la lista
// quita al marcar y vuelve a <body>; el test lo dice.
test('al marcar, el foco pasa a la fila: el botón se va y Tab no vuelve arriba', () => {
  const host = document.createElement('div');
  document.body.appendChild(host);
  render(
    <div>
      <a href="/aviso">Aviso</a>
      <MarcarLeido rotulo="Marcar como leído" alMarcar={() => undefined} />
    </div>,
    host,
  );
  const boton = host.querySelector('button')!;
  boton.focus();
  boton.click();
  expect(document.activeElement).toBe(host.querySelector('a'));
  host.remove();
});

// Con «Sin leer», el aviso marcado SALE de la lista con su fila: el foco va a la fila del
// siguiente, o a la del anterior si era el último. Visto en ROJO a mano: sin el
// `setTimeout` que mira si la fila sigue, el foco acaba en <body>.
function SoloSinLeer({ ids }: { ids: string[] }) {
  const [quedan, setQuedan] = useState(ids);
  return (
    <ul>
      {quedan.map((id) => (
        <li key={id}>
          <a href={`/${id}`}>{id}</a>
          <MarcarLeido
            rotulo="Marcar como leído"
            alMarcar={() => setQuedan((q) => q.filter((x) => x !== id))}
          />
        </li>
      ))}
    </ul>
  );
}

const repintado = () => new Promise((r) => setTimeout(r, 5));

test('con «Sin leer», el foco pasa al aviso siguiente, o al anterior si era el último', async () => {
  const host = document.createElement('div');
  document.body.appendChild(host);
  render(<SoloSinLeer ids={['a', 'b', 'c']} />, host);
  const botonDe = (id: string) =>
    host.querySelector(`a[href="/${id}"]`)!.nextElementSibling as HTMLButtonElement;

  botonDe('a').focus();
  botonDe('a').click();
  await repintado();
  expect(host.querySelector('a[href="/a"]')).toBeNull();
  expect(document.activeElement).toBe(host.querySelector('a[href="/b"]'));

  botonDe('c').focus();
  botonDe('c').click();
  await repintado();
  expect(document.activeElement).toBe(host.querySelector('a[href="/b"]'));
  render(null, host);
  host.remove();
});
