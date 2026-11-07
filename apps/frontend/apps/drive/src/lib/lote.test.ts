// Deshacer un lote y vaciar la papelera, elemento a elemento. Ejecutar: `bun test`.
import { expect, test } from 'bun:test';
import { alRestaurar, avisarRestaurados, conId, enTandas, recargarLaVista } from './lote.js';

test('lo que falla se cuenta aparte y no corta el resto', async () => {
  const r = await enTandas(['a', 'b', 'c', 'd'], async (x) => {
    if (x === 'b') throw new Error('403');
  });
  expect(r.hechos.sort()).toEqual(['a', 'c', 'd']);
  expect(r.fallidos).toEqual(['b']);
});

test('nunca van más llamadas a la vez de las pedidas', async () => {
  // Vaciar una papelera de cien elementos no puede ser cien peticiones de golpe.
  let enVuelo = 0;
  let pico = 0;
  const r = await enTandas(
    Array.from({ length: 20 }, (_, i) => i),
    async () => {
      enVuelo++;
      pico = Math.max(pico, enVuelo);
      await new Promise((ok) => setTimeout(ok, 1));
      enVuelo--;
    },
    3,
  );
  expect(r.hechos.length).toBe(20);
  expect(pico).toBe(3);
});

test('una lista vacía no llama a nada', async () => {
  let llamadas = 0;
  const r = await enTandas([], async () => {
    llamadas++;
  });
  expect(llamadas).toBe(0);
  expect(r).toEqual({ hechos: [], fallidos: [] });
});

test('solo se devuelve lo que tiene id, con su clase', () => {
  expect(
    conId([
      { id: 'f1', isDirectory: false, name: 'acta.odt' },
      { id: 'c1', isDirectory: true, name: 'Contratos' },
      { isDirectory: false, name: 'sin-id.txt' },
    ]),
  ).toEqual([
    { id: 'f1', kind: 'file', name: 'acta.odt' },
    { id: 'c1', kind: 'folder', name: 'Contratos' },
  ]);
});

test('el «Deshacer» recarga la carpeta que se ve ahora, no la de cuando se tiró', async () => {
  const cargadas: string[] = [];
  const cargar = async (ruta: string) => void cargadas.push(ruta);
  const vista = { current: { montada: true, ruta: '/A' } };
  // Se tira en /A y, antes de los 10 s, se entra en /B.
  vista.current.ruta = '/B';
  await recargarLaVista(vista, cargar);
  expect(cargadas).toEqual(['/B']);
  // Y si el explorador ya no está (se ha ido a la Papelera), no pinta nada.
  vista.current.montada = false;
  await recargarLaVista(vista, cargar);
  expect(cargadas).toEqual(['/B']);
});

test('la papelera se entera de lo que se restaura desde otro sitio, y deja de hacerlo al irse', () => {
  let recargas = 0;
  const baja = alRestaurar(() => recargas++);
  avisarRestaurados();
  expect(recargas).toBe(1);
  baja();
  avisarRestaurados();
  expect(recargas).toBe(1);
});
