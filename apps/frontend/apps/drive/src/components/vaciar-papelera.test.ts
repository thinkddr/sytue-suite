// «Vaciar papelera» en TrashView. Ejecutar: `bun test`.
//
// Como `subir-flotante.test.ts`, se vigila el fuente (Drive no monta componentes en
// sus pruebas): el botón solo aparece con algo que vaciar, pasa por su confirmación
// —no purga al pulsarlo— y purga cada elemento por su id y su clase, en tandas, para
// que lo que el servicio niegue se cuente aparte (`lote.test.ts`). La pareja nativa es
// `TestVaciarLaPapeleraPreguntaYPurgaTodo`.
import { expect, test } from 'bun:test';
import { readFileSync } from 'node:fs';

const fuente = readFileSync(new URL('./FileBrowser.tsx', import.meta.url), 'utf8');
const papelera = fuente.slice(fuente.indexOf('export function TrashView'));

test('el botón solo sale con algo que vaciar y abre la confirmación', () => {
  expect(papelera).toMatch(
    /items && items\.length > 0 \? \(\s*<Button[^]*?onClick=\{\(\) => setEmptyAllOpen\(true\)\}/,
  );
});

test('confirmar purga cada elemento por su id y su clase, en tandas', () => {
  expect(papelera).toMatch(/enTandas\(conId\(items\), \(x\) => drive\.purge\(x\.id, x\.kind\)\)/);
  expect(papelera).toMatch(
    /<Modal\s+open=\{emptyAllOpen\}[^]*?onClick=\{\(\) => void onEmptyAll\(\)\}/,
  );
});
