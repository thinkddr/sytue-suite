/*
 * SPDX-License-Identifier: Apache-2.0
 * SPDX-FileCopyrightText: (c) 2026 Daniel Delgado Ramos
 *
 * La carpeta de una cuenta externa en el panel principal y «Todos los buzones de
 * entrada». La mezcla recorre los vectores COMPARTIDOS con Fyne
 * (apps/fyne/apps/mail/testdata/mezcla-de-entradas.json, que recorre
 * `TestLaMezclaDeEntradasEsLaDeLaWeb`); los casos y su porqué viven en el JSON.
 * Ejecutar: `bun test`.
 *
 * VISTO EN ROJO (2026-11-06): sin la parada de la fuente vacía con páginas, la
 * mezcla sacaba la Entrada de Sytue entera antes de pedir la segunda página de
 * Gmail; comparando la fecha como texto, 10:00+02:00 salía antes que 09:00Z; y
 * eligiendo con `>=`, el empate lo ganaba la cuenta y no Sytue. Sin
 * `encodeURIComponent`, un id de IMAP con «:» se partía en otra carpeta. Deshecho.
 *
 * VISTO EN ROJO (2026-11-06): devolviendo `antes` entero, los de Gmail ya archivados
 * reaparecían; sin mirar las capacidades, un mensaje POP3 mezclado se podía triar; y
 * sin mirar el tipo de vista, «Todos los buzones de entrada» se soltaba al quedarse
 * sin cuentas. Deshecho.
 */
import { expect, test } from 'bun:test';
import { readFileSync } from 'node:fs';
import {
  abreRedactor,
  devolverQuitados,
  esMezclado,
  fuentesDeUnificado,
  idExterno,
  ROLES_MEZCLADOS,
  mezclarPorFecha,
  origenExterno,
  porPedir,
  quedanEnMezcla,
  sinAccionesExternas,
  sobreExterno,
  vistaSinCuenta,
  type FuenteMezcla,
  type RolMezclado,
  type VistaExterna,
} from './vistaExterna.js';
import type { BuzonUnificado } from './buzones.js';

interface Caso {
  nombre: string;
  fuentes: { clave: string; cursor: string; pendientes: { id: string; fecha: string }[] }[];
  salida: string[];
  porPedir: string[];
  quedan: boolean;
}

const { casos } = JSON.parse(
  readFileSync(
    new URL('../../../../../fyne/apps/mail/testdata/mezcla-de-entradas.json', import.meta.url),
    'utf8',
  ),
) as { casos: Caso[] };

test('hay vectores de mezcla que recorrer', () => {
  expect(casos.length).toBeGreaterThanOrEqual(6);
});

for (const c of casos) {
  test(`mezcla: ${c.nombre}`, () => {
    const fuentes: FuenteMezcla<{ id: string; receivedAt: string }>[] = c.fuentes.map((f) => ({
      clave: f.clave,
      cursor: f.cursor,
      pendientes: f.pendientes.map((p) => ({ id: p.id, receivedAt: p.fecha })),
    }));
    const r = mezclarPorFecha(fuentes);
    expect(r.salida.map((x) => x.id)).toEqual(c.salida);
    expect(porPedir(r.fuentes)).toEqual(c.porPedir);
    expect(quedanEnMezcla(r.fuentes)).toBe(c.quedan);
    // No toca lo que recibe: MailApp guarda las fuentes de antes hasta que llega la página.
    expect(fuentes.map((f) => f.pendientes.length)).toEqual(
      c.fuentes.map((f) => f.pendientes.length),
    );
  });
}

test('el id de un mensaje externo vuelve a su cuenta, su carpeta y su id', () => {
  const origen = { cuenta: 'inst-1', carpeta: 'INBOX/Facturas', id: '42:7' };
  const id = idExterno(origen);
  expect(id.startsWith('ext:')).toBe(true);
  expect(origenExterno(id)).toEqual(origen);
});

test('un id de Sytue no es externo', () => {
  expect(origenExterno('0b8a3b1e-6f0e-4d33-9a4e-5b0f0b7c1d2e')).toBeNull();
  expect(origenExterno('ext:solo-dos:partes')).toBeNull();
  expect(origenExterno('ext:%E0%A4%A:x:y')).toBeNull();
});

test('el sobre de un mensaje externo es el de una conversación de uno, con su fecha en ISO', () => {
  const sobre = sobreExterno('inst-1', 'INBOX', {
    id: '7',
    subject: 'Factura',
    from: [{ name: 'Ana', email: 'ana@example.com' }],
    to: [{ email: 'yo@example.com' }],
    date: '2026-11-06T10:00:00+02:00',
    snippet: 'Adjunto la factura',
    seen: false,
    flagged: true,
    attachments: [{ name: 'f.pdf', type: 'application/pdf' }],
  });
  expect(sobre).toEqual({
    id: idExterno({ cuenta: 'inst-1', carpeta: 'INBOX', id: '7' }),
    threadId: '',
    subject: 'Factura',
    from: [{ name: 'Ana', email: 'ana@example.com' }],
    to: [{ email: 'yo@example.com' }],
    cc: [],
    preview: 'Adjunto la factura',
    receivedAt: '2026-11-06T08:00:00.000Z',
    keywords: { $seen: false, $flagged: true },
    hasAttachment: true,
    mailboxId: 'INBOX',
  });
});

test('un mensaje de una cuenta POP3 no se tría, tampoco mezclado; uno de Sytue, siempre', () => {
  const acciones = new Map([
    ['pop', false],
    ['imap', true],
  ]);
  const de = (cuenta: string): boolean | undefined => acciones.get(cuenta);
  expect(sinAccionesExternas(idExterno({ cuenta: 'pop', carpeta: 'INBOX', id: '1' }), de)).toBe(
    true,
  );
  expect(sinAccionesExternas(idExterno({ cuenta: 'imap', carpeta: 'INBOX', id: '1' }), de)).toBe(
    false,
  );
  // Una cuenta que aún no está en el resumen no se da por POP3.
  expect(sinAccionesExternas(idExterno({ cuenta: 'nueva', carpeta: 'INBOX', id: '1' }), de)).toBe(
    false,
  );
  expect(sinAccionesExternas('0d6a2c4e-uuid-de-sytue', de)).toBe(false);
});

test('la carpeta de una cuenta desconectada se suelta; «Todos los buzones de entrada», no', () => {
  const cuentas = [{ instance_id: 'gmail' }];
  expect(vistaSinCuenta({ tipo: 'carpeta', cuenta: 'gmail', carpeta: 'INBOX' }, cuentas)).toBe(
    false,
  );
  expect(vistaSinCuenta({ tipo: 'carpeta', cuenta: 'ida', carpeta: 'INBOX' }, cuentas)).toBe(true);
  expect(vistaSinCuenta({ tipo: 'unificado', rol: 'inbox' }, [])).toBe(false);
  expect(vistaSinCuenta({ tipo: 'unificado', rol: 'sent' }, [])).toBe(false);
  expect(vistaSinCuenta(null, [])).toBe(false);
});

test('deshacer un quitado devuelve solo lo suyo, en su sitio, con el estado de ahora', () => {
  const antes = [
    { id: 's1', leido: false },
    { id: 'g1', leido: false },
    { id: 's2', leido: false },
    { id: 's3', leido: false },
  ];
  // s1 y s2 los quitó el lote de Sytue que falló; g1, el de Gmail, que sí se movió;
  // s3 se leyó entre tanto y n1 llegó después.
  const ahora = [
    { id: 's3', leido: true },
    { id: 'n1', leido: false },
  ];
  expect(devolverQuitados(antes, ahora, new Set(['s1', 's2']))).toEqual([
    { id: 's1', leido: false },
    { id: 's2', leido: false },
    { id: 's3', leido: true },
    { id: 'n1', leido: false },
  ]);
});

// Fase B, punto 5 completo: qué buzones unificados se abren mezclados, qué carpetas
// pide cada uno y qué abre una fila. Vectores COMPARTIDOS con Fyne
// (apps/fyne/apps/mail/testdata/buzones-mezclados.json, que recorre
// `TestLosBuzonesMezcladosSonLosDeLaWeb`).
//
// VISTO EN ROJO (2026-11-07): con `fuentesDeUnificado` buscando siempre `inbox`,
// «Todos los enviados» pedía las Entradas (y el sondeo de 30 s, también); con
// `abreRedactor` mirando solo el rol de Sytue, un borrador de Sytue en «Todos los
// borradores» se abría en el lector; y sin `origenExterno`, el de una cuenta abría
// el redactor de Sytue; y sin mirar la búsqueda, un recibido buscado desde Borradores
// o «Todos los borradores» abría el redactor. Deshecho.
const mezclados = JSON.parse(
  readFileSync(
    new URL('../../../../../fyne/apps/mail/testdata/buzones-mezclados.json', import.meta.url),
    'utf8',
  ),
) as {
  mezclados: string[];
  noMezclados: string[];
  unificados: BuzonUnificado[];
  fuentes: { rol: RolMezclado; carpetas: string[] }[];
  redactor: {
    nombre: string;
    vista: VistaExterna | null;
    rolSytue: string;
    externo: boolean;
    enBusqueda?: boolean;
    abre: boolean;
  }[];
};

test('se mezclan la Entrada, los Enviados y los Borradores; lo demás solo se despliega', () => {
  expect<string[]>([...ROLES_MEZCLADOS]).toEqual(mezclados.mezclados);
  for (const rol of mezclados.mezclados) expect(esMezclado(rol)).toBe(true);
  for (const rol of mezclados.noMezclados) expect(esMezclado(rol)).toBe(false);
});

for (const c of mezclados.fuentes) {
  test(`un buzón unificado pide solo las carpetas de su rol: ${c.rol}`, () => {
    const fuentes = fuentesDeUnificado(mezclados.unificados, c.rol);
    expect(fuentes.map((f) => `${f.cuenta}/${f.carpeta}`)).toEqual(c.carpetas);
    // Una copia: el sondeo no puede tocar el árbol del lateral.
    fuentes.pop();
    expect(fuentesDeUnificado(mezclados.unificados, c.rol)).toHaveLength(c.carpetas.length);
  });
}

test('sin la carpeta de ese rol en ninguna cuenta, no se pide nada', () => {
  expect(fuentesDeUnificado([], 'sent')).toEqual([]);
});

for (const c of mezclados.redactor) {
  test(`abrir una fila: ${c.nombre}`, () => {
    const id = c.externo
      ? idExterno({ cuenta: 'gmail', carpeta: 'DRAFT', id: 'r1' })
      : '0b8a3b1e-6f0e-4d33-9a4e-5b0f0b7c1d2e';
    expect(abreRedactor(c.vista, c.rolSytue || undefined, id, c.enBusqueda ?? false)).toBe(c.abre);
  });
}
