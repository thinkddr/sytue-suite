/*
 * SPDX-License-Identifier: Apache-2.0
 * SPDX-FileCopyrightText: (c) 2026 Daniel Delgado Ramos
 *
 * LOS GLIFOS SIN EQUIVALENTE EN `Icon` SON DIBUJO DE LA CASA, Y UNO SOLO POR CONCEPTO.
 *
 * Carpeta, fichero, documento, estrella, clip, responder, responder a todos y
 * reenviar no están en el catálogo común, así que cada app los dibujaba en su
 * `glyphs.tsx` (o en línea). Varios eran trazados de Feather o Lucide copiados casi
 * a la letra —la carpeta y el clip de Mail, el fichero de Drive y del Hub, el
 * reenviar de Rooms— y la misma estrella salía con cinco geometrías en cinco apps.
 * Se decidió (D-DS-03) redibujarlos en limpio, en la rejilla de la casa: márgenes de
 * 3,5, esquinas de radio 1,5 y caja óptica centrada, como el resto de `Icon`.
 *
 * Este guardián mira dos cosas:
 *  1. Que ninguna huella de aquellos trazados vuelva, ni en la web ni en Fyne (que
 *     copia los de la web y los compara con su propio guardián).
 *  2. Que cada sitio que dibuja uno de estos conceptos use EL dibujo de la casa, el
 *     mismo en todas las apps: una estrella de favorito no puede tener otra forma en
 *     Contacts que en Drive.
 *
 * SI ESTE TEST TE PARA: no copies un icono de una librería; si el concepto ya está
 * aquí, usa su trazado tal cual; si es nuevo, dibújalo en la rejilla de la casa.
 *
 * VISTO EN ROJO (2026-11-12, en el frontal): con la carpeta de Feather devuelta a
 * `apps/mail/src/components/glyphs.tsx`, el primer test nombra el fichero y la
 * huella, y el segundo dice que Mail ya no lleva la carpeta de la casa.
 */
import { expect, test } from 'bun:test';

const RAIZ = new URL('../../../../', import.meta.url).pathname; // apps/frontend/
const FYNE = new URL('../fyne/', `file://${RAIZ}`).pathname; // apps/fyne/

const CARPETA =
  'M3.5 6.5A1.5 1.5 0 0 1 5 5h4.4l2 2.5H19a1.5 1.5 0 0 1 1.5 1.5v9A1.5 1.5 0 0 1 19 19.5H5a1.5 1.5 0 0 1-1.5-1.5ZM3.5 10.5h17';
const FICHERO =
  'M13.5 3.5H7A1.5 1.5 0 0 0 5.5 5v14A1.5 1.5 0 0 0 7 20.5h10a1.5 1.5 0 0 0 1.5-1.5V8.5ZM13.5 3.5V8.5h5';

/** Concepto → [trazado de la casa, ficheros (relativos a apps/) que lo dibujan]. */
const DE_LA_CASA: Record<string, [string, string[]]> = {
  // Pestaña a la izquierda y la línea de la solapa: es lo que la distingue de una
  // bandeja o de una tarjeta a 16 px.
  carpeta: [
    CARPETA,
    [
      'frontend/apps/mail/src/components/glyphs.tsx',
      'frontend/apps/drive/src/components/glyphs.tsx',
      'frontend/apps/rooms/src/components/glyphs.tsx',
      'frontend/packages/shell/src/SelectorDeFicheros.tsx',
      'fyne/apps/mail/glifos.go',
      'fyne/apps/drive/glifos.go',
      'fyne/apps/rooms/adjuntos.go',
      'fyne/iconos/iconos.go',
    ],
  ],
  // Mover a una carpeta: la carpeta de arriba con esta flecha dentro del cuerpo.
  mover: [
    'M9 15h6M12.5 12.5 15 15l-2.5 2.5',
    ['frontend/apps/drive/src/components/glyphs.tsx', 'fyne/apps/drive/glifos.go'],
  ],
  // La hoja de Notepads, que ya estaba dibujada en la rejilla de la casa.
  fichero: [
    FICHERO,
    [
      'frontend/apps/notes/src/components/glyphs.tsx',
      'frontend/apps/drive/src/components/glyphs.tsx',
      'frontend/apps/rooms/src/components/glyphs.tsx',
      'frontend/apps/portal/src/components/Launcher.tsx',
      'frontend/packages/shell/src/SelectorDeFicheros.tsx',
      // La hoja del PDF, con el sello dentro.
      'frontend/packages/office/src/components/DocList.tsx',
      'fyne/apps/hub/glifos.go',
      'fyne/apps/rooms/adjuntos.go',
      'fyne/apps/certipdf/portada.go',
      'fyne/iconos/iconos.go',
    ],
  ],
  documento: [
    `${FICHERO}M8.5 13h7M8.5 16.5h4.5`,
    [
      'frontend/packages/office/src/components/DocList.tsx',
      'frontend/apps/drive/src/components/glyphs.tsx',
      'fyne/editor/glifos.go',
      'fyne/iconos/iconos.go',
    ],
  ],
  // Estrella regular de radios 9 y 4,1 con centro en (12, 12,6): más llena que la de
  // las librerías, para que el relleno de «destacado» se lea a 14 px.
  estrella: [
    'M12 3.6L14.4 9.3L20.6 9.8L15.9 13.9L17.3 19.9L12 16.7L6.7 19.9L8.1 13.9L3.4 9.8L9.6 9.3Z',
    [
      'frontend/apps/mail/src/components/glyphs.tsx',
      'frontend/apps/drive/src/components/glyphs.tsx',
      'frontend/apps/rooms/src/components/glyphs.tsx',
      'frontend/apps/contacts/src/components/ContactsApp.tsx',
      'frontend/apps/contacts/src/components/ContactDetail.tsx',
      'frontend/packages/office/src/components/DocList.tsx',
      'fyne/apps/mail/glifos.go',
      'fyne/apps/rooms/glifos.go',
      'fyne/contactos/glifos.go',
      'fyne/editor/glifos.go',
      'fyne/iconos/iconos.go',
    ],
  ],
  // Clip vertical de tres patas separadas 3,5: a 16 px quedan huecos entre trazos,
  // que en el de Feather (2 de separación en diagonal) se cerraban.
  clip: [
    'M6.75 10v5a5.25 5.25 0 0 0 10.5 0V7a3.5 3.5 0 0 0-7 0v8.5a1.75 1.75 0 0 0 3.5 0V9',
    [
      'frontend/apps/mail/src/components/glyphs.tsx',
      'frontend/apps/rooms/src/components/glyphs.tsx',
      'fyne/apps/mail/glifos.go',
      'fyne/apps/rooms/glifos.go',
    ],
  ],
  responder: [
    'M9.5 6 4.5 11l5 5M4.5 11H14a5.5 5.5 0 0 1 5.5 5.5V18',
    ['frontend/apps/mail/src/components/glyphs.tsx', 'fyne/apps/mail/glifos.go'],
  ],
  responderATodos: [
    'M8 6 3 11l5 5M12.5 6l-5 5 5 5M7.5 11H15a5 5 0 0 1 5 5v2',
    ['frontend/apps/mail/src/components/glyphs.tsx', 'fyne/apps/mail/glifos.go'],
  ],
  reenviar: [
    'M14.5 6l5 5-5 5M19.5 11H10a5.5 5.5 0 0 0-5.5 5.5V18',
    [
      'frontend/apps/mail/src/components/glyphs.tsx',
      'frontend/apps/rooms/src/components/glyphs.tsx',
      'fyne/apps/mail/glifos.go',
      'fyne/apps/rooms/glifos.go',
    ],
  ],
};

/** Huella → de qué trazado de librería es. Trozos que no salen en ningún dibujo propio. */
const HUELLAS: Record<string, string> = {
  'h5l2 3h9': 'la carpeta de Feather',
  '7.93 3H4': 'la carpeta de Lucide',
  'M14 2H6a2 2 0 0 0-2 2v16': 'el fichero de Feather (file-text)',
  'M14 3H7a2 2 0 0 0-2 2v14': 'el documento calcado de Lucide (file-text)',
  '21.44 11.05': 'el clip de Feather',
  '2.9 5.9': 'la estrella de Feather',
  '2.6 5.27': 'la estrella de Feather, reescalada',
  '15 17 20 12 15 7': 'el reenviar de Lucide',
  '4-4h13': 'la cola del reenviar de Lucide',
  'M9 17 4 12l5-5': 'el responder de Lucide',
  'M7 17 2 12l5-5': 'el responder a todos de Lucide',
  'm15 17 5-5-5-5': 'el reenviar de Lucide',
};

/*
 * Las fuentes se leen UNA vez al cargar el fichero (son más de mil) y sin
 * comentarios, que pueden citar una huella para explicar de dónde se vino.
 */
const FUENTES = new Map<string, string>();
const leer = async (base: string, prefijo: string, patron: string): Promise<void> => {
  for await (const rel of new Bun.Glob(patron).scan({ cwd: base })) {
    if (rel.includes('.test.') || rel.endsWith('_test.go') || rel.includes('node_modules/')) {
      continue;
    }
    const texto = await Bun.file(base + rel).text();
    FUENTES.set(prefijo + rel, texto.replace(/\/\*[\s\S]*?\*\/|^\s*\/\/.*$/gm, ''));
  }
};
await leer(RAIZ, 'frontend/', '{apps,packages}/*/src/**/*.{ts,tsx,astro}');
await leer(FYNE, 'fyne/', '**/*.go');

test('ningún glifo vuelve a un trazado de librería', () => {
  const encontradas: string[] = [];
  for (const [rel, codigo] of FUENTES) {
    for (const [huella, deQue] of Object.entries(HUELLAS)) {
      if (codigo.includes(huella)) encontradas.push(`${rel}: ${deQue} («${huella}»)`);
    }
  }
  expect(encontradas).toEqual([]);
});

test('cada concepto se dibuja con el trazado de la casa en todas las apps', () => {
  const sinElDeLaCasa: string[] = [];
  for (const [concepto, [trazado, ficheros]] of Object.entries(DE_LA_CASA)) {
    for (const rel of ficheros) {
      const codigo = FUENTES.get(rel);
      if (codigo === undefined) sinElDeLaCasa.push(`${rel}: ya no existe (${concepto})`);
      else if (!codigo.includes(trazado)) sinElDeLaCasa.push(`${rel}: ${concepto}`);
    }
  }
  expect(sinElDeLaCasa).toEqual([]);
});
