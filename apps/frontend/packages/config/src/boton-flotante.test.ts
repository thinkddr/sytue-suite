/**
 * EL «+» FLOTANTE ES EL ÚNICO CONTROL DE CREAR (producto, 2026-11-25).
 *
 * «Crear evento», «Redactar», «Nuevo contacto», el «Nuevo» de Drive, el «Subir» de Cast
 * y «Añadir fotos» son un «+» redondo que flota abajo a la derecha en TODOS los anchos
 * (`BotonFlotante` de `@sytue/ui`). Hasta entonces había dos controles para lo mismo: el
 * botón primario del menú lateral en escritorio y el botón flotante solo en el teléfono.
 * Este guardián impide que vuelva el segundo: una app que pone otra vez su acción de
 * crear en la cabecera del menú lateral, o que se queda sin su «+», se ve aquí.
 *
 * Mira el fuente y no monta las apps: cada una necesita su BFF para pintarse. Cómo es el
 * botón —redondo, el «+», en todos los anchos, con el menú colgado de él— lo prueba
 * `packages/ui/src/components/BotonFlotante.test.tsx`; la pareja nativa,
 * `apps/fyne/ui/botonflotante_test.go`. Ejecutar: `bun test`.
 */
import { expect, test } from 'bun:test';
import { readFileSync } from 'node:fs';

/** Rutas relativas a `apps/frontend/`. */
const RAIZ = new URL('../../../', import.meta.url);
const leer = (rel: string): string => readFileSync(new URL(rel, RAIZ), 'utf8');

/** Cada app con acción de crear, el fichero que monta su «+» y lo que dice. */
const APPS: readonly { app: string; fichero: string; rotulo: string }[] = [
  { app: 'mail', fichero: 'apps/mail/src/components/MailApp.tsx', rotulo: 'nav.compose' },
  {
    app: 'calendar',
    fichero: 'apps/calendar/src/components/CalendarApp.tsx',
    rotulo: 'nav.createEvent',
  },
  {
    app: 'contacts',
    fichero: 'apps/contacts/src/components/ContactsApp.tsx',
    rotulo: 'nav.newContact',
  },
  // La vista de «Mi unidad» que se monta hoy (la de la bóveda, ADR 0086).
  {
    app: 'drive',
    fichero: 'apps/drive/src/components/PrivateFileBrowser.tsx',
    rotulo: 'files.new',
  },
  { app: 'cast', fichero: 'apps/cast/src/components/Marco.tsx', rotulo: 'nav.upload' },
  { app: 'photos', fichero: 'apps/photos/src/components/PhotosApp.tsx', rotulo: 'gallery.add' },
];

const escapar = (s: string): string => s.replace(/[.*+?^${}()|[\]\\]/g, '\\$&');

for (const { app, fichero, rotulo } of APPS) {
  test(`${app}: crear es UN «+» flotante («${rotulo}»), no un botón del menú lateral`, () => {
    const fuente = leer(fichero);
    expect(fuente.match(/<BotonFlotante\b/g)?.length ?? 0).toBe(1);
    expect(fuente).toMatch(new RegExp(`<BotonFlotante\\s+label=\\{t\\('${escapar(rotulo)}'\\)\\}`));
  });
}

/**
 * Los ficheros de un patrón. UN patrón por llamada: con llaves que abarcan varios tramos
 * de ruta (`{apps/*\/src/**,packages/…}`) `Bun.Glob` no encuentra NADA y el guardián
 * pasaría en verde sin mirar un fichero; por eso también se cuentan.
 */
async function ficheros(patrones: readonly string[]): Promise<string[]> {
  const out: string[] = [];
  for (const p of patrones) {
    for await (const rel of new Bun.Glob(p).scan({ cwd: RAIZ.pathname })) out.push(rel);
  }
  return out;
}

test('ninguna app pone un botón en la cabecera del menú lateral', async () => {
  // Era el sitio de «Crear evento», «Redactar» y «Nuevo contacto» en escritorio. Rooms
  // sigue usando la cabecera, pero para su presencia (un `<div>`), no para un botón.
  const mirados = (await ficheros(['apps/*/src/**/*.tsx'])).filter((r) => !r.includes('.test.'));
  expect(mirados.length).toBeGreaterThan(100);
  const culpables = mirados.filter((rel) => /\bheader=\{\s*<Button\b/.test(leer(rel)));
  expect(culpables).toEqual([]);
});

test('`MobileFab` no vuelve: el «+» se ve en todos los anchos y se llama BotonFlotante', async () => {
  const mirados = await ficheros([
    'apps/*/src/**/*.{ts,tsx}',
    'packages/*/src/**/*.{ts,tsx}',
    'packages/*/test/**/*.tsx',
  ]);
  expect(mirados.length).toBeGreaterThan(100);
  // Su uso (importarlo o pintarlo), no su nombre en la historia de un comentario. Este
  // fichero lleva el patrón y no cuenta.
  const culpables = mirados.filter(
    (rel) =>
      !rel.endsWith('boton-flotante.test.ts') &&
      /<MobileFab\b|\bMobileFab,|MobileFab\.js/.test(leer(rel)),
  );
  expect(culpables).toEqual([]);
});
