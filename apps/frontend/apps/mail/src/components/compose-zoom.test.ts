/**
 * QUE EL RESTABLECIMIENTO DE ESCALA LLEGUE A EJECUTARSE.
 *
 * `ComposeModal` NO SE DESMONTA: `MailApp` lo pinta siempre y le pasa `open`.
 * Una limpieza de efecto con dependencias `[]` solo correría al desmontar Mail
 * entero —es decir, nunca—, así que el arreglo quedaría escrito y sin efecto:
 * ni error, ni aviso, ni nada que se vea en una revisión rápida. Se escribió así
 * la primera vez y por eso existe esta comprobación.
 *
 * Se mira la FUENTE porque lo que hay que fijar es la dependencia, no el
 * comportamiento del navegador: el efecto del zoom solo se ve en un iPhone.
 */
import { join } from 'node:path';
import { expect, test } from 'bun:test';

// Ruta desde el FICHERO y no desde el directorio de trabajo: con `bun test`
// lanzado en la raíz de apps/frontend —en vez de `bun run test`, que entra en
// cada paquete— la lectura daba ENOENT y el fichero moría entero, antes de
// registrar ni un test.
const AQUI = import.meta.dir;

const fuente = await Bun.file(join(AQUI, 'ComposeModal.tsx')).text();
const marco = await Bun.file(join(AQUI, 'MailApp.tsx')).text();

test('el restablecimiento de escala cuelga de `open`, no de `[]`', () => {
  // La ÚLTIMA aparición, que es la del efecto: la primera es el `import`.
  const i = fuente.lastIndexOf('restaurarZoomDelViewport');
  expect(i).toBeGreaterThan(0);
  // El `useEffect` que lo usa, desde su apertura hasta su lista de dependencias.
  const desde = fuente.lastIndexOf('useEffect(', i);
  const hasta = fuente.indexOf('}, [', i);
  expect(desde).toBeGreaterThan(0);
  expect(hasta).toBeGreaterThan(i);
  const deps = fuente.slice(hasta, fuente.indexOf(')', hasta));
  expect(deps).toContain('open');
});

test('el redactor sigue recibiendo `open` en vez de montarse y desmontarse', () => {
  // Si algún día se pinta con `{compose.open && <ComposeModal …>}`, el efecto
  // podría volver a `[]` sin problema — pero entonces ESTE test sobra y hay que
  // borrarlo a conciencia, no dejarlo pasando por casualidad.
  expect(marco).toContain('open={compose.open}');
});

test('en móvil el redactor se presenta como hoja inferior', () => {
  // Desde que TODO diálogo es una hoja en el teléfono (Modal.test.tsx lo vigila), el
  // redactor lo es por ser un `Modal`: si algún día se pinta con su propia capa,
  // reaparece la franja entre sus acciones y el teclado.
  expect(fuente).toContain('<Modal');
});
