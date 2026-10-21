/**
 * El `target_kind` se VALIDA en vez de reenviarse. Un valor inventado no
 * produce un error claro en el servicio: devuelve una lista vacía, que en la
 * interfaz se lee como «esta nota no tiene retroenlaces». Es el peor resultado
 * posible, porque parece una respuesta correcta.
 */
import { join } from 'node:path';
import { expect, test } from 'bun:test';
import type { NoteHit } from '@sytue/api-clients';
import { deNotes } from '../../lib/elegibles.js';

// Mismo motivo que en export.test.ts: relativas al fichero, no al cwd.
const FRONTEND = join(import.meta.dir, '../../../../..');

// Los mismos destinos que declara el SDK. Si allí se añade uno y aquí no, el
// endpoint lo rechazaría en silencio; este test es lo que lo haría notar.
const DESTINOS = [
  'calendar_event',
  'room_channel',
  'room_message',
  'docs_document',
  'drive_file',
  'notebook',
  'note',
];

test('los destinos válidos son los del SDK', async () => {
  const fuente = await Bun.file(join(FRONTEND, 'packages/api-clients/src/notes/index.ts')).text();
  const bloque = fuente.slice(fuente.indexOf('export type NoteLinkTarget'));
  // `flatMap` y no `map`: un grupo de captura es `string | undefined` para el
  // compilador, y arrastrar ese `undefined` hasta el comparador obliga a
  // ensancharlo. Se estrecha en el origen, que es donde se sabe que hay valor.
  const declarados = [...bloque.slice(0, bloque.indexOf(';')).matchAll(/'([a-z_]+)'/g)].flatMap(
    (m) => (m[1] ? [m[1]] : []),
  );
  // Con comparador explícito: el orden por defecto de `sort` es por UTF-16 y
  // depende del entorno, así que un fallo aquí sería intermitente.
  const alfabetico = (a: string, b: string): number => a.localeCompare(b);
  expect(declarados.sort(alfabetico)).toEqual([...DESTINOS].sort(alfabetico));
});

test('el endpoint acepta los siete y rechaza lo demás', async () => {
  const fuente = await Bun.file(join(import.meta.dir, 'backlinks.ts')).text();
  for (const d of DESTINOS) {
    expect(fuente).toContain(`'${d}'`);
  }
  // Y no se cuela un `as never` que apague la comprobación de tipos.
  expect(fuente).not.toContain('as never');
});

/**
 * La referencia entre notas es un LAZO: el selector la crea con un `target_kind`
 * y este endpoint la busca por ese mismo valor. Si los dos lados dejan de
 * coincidir no hay error en ningún sitio — la nota se crea, el enlace se guarda y
 * «qué apunta aquí» sale vacío para siempre.
 */
test('todo origen del selector enlaza con un destino que este endpoint acepta', async () => {
  const { DESTINO_POR_ORIGEN } = await import('../../client/notesApi.js');
  for (const destino of Object.values(DESTINO_POR_ORIGEN)) {
    expect(DESTINOS).toContain(destino);
  }
  // Y el origen `notes` apunta a `note` y no a `notebook`: son dos filas
  // distintas de `note_links` y confundirlas deja el retroenlace en el cuaderno
  // en vez de en la nota.
  expect(DESTINO_POR_ORIGEN.notes).toBe('note');
});

test('el enlace a otra nota usa los parámetros que la app sabe leer', async () => {
  const hit = { id: 'n1', notebook_id: 'k1', notebook: 'Casa', title: 'Lista' } as NoteHit;
  const url = new URL(deNotes(hit).url ?? '');
  // `index.astro` lee exactamente estos dos. Un enlace con otros nombres abre la
  // app en su cuaderno por defecto, que se lee como que la referencia se perdió.
  const inicio = await Bun.file(join(import.meta.dir, '../index.astro')).text();
  expect(url.searchParams.get('notebook')).toBe('k1');
  expect(url.searchParams.get('note')).toBe('n1');
  for (const param of ['notebook', 'note']) {
    expect(inicio).toContain(`searchParams.get('${param}')`);
  }
});
