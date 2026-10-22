/*
 * SPDX-License-Identifier: Apache-2.0
 * SPDX-FileCopyrightText: (c) 2026 Daniel Delgado Ramos
 *
 * BUSCAR SIN ACENTOS NI MAYÚSCULAS, EN TODAS LAS APPS. Un filtro escrito a mano
 * con `x.toLowerCase().includes(q)` encuentra «Café» buscando «café» y no
 * buscando «cafe». La regla es una —`contiene`, `plegar` y `rangos` de
 * '@sytue/i18n'— y es la misma que la de Fyne y la del servidor.
 *
 * Se mira el CÓDIGO FUENTE porque el fallo se comete al escribir: el filtro nuevo
 * pasa todos los tests de su app. Las excepciones se comparan por IGUALDAD con lo
 * encontrado: una que sobra también falla.
 */
import { expect, test } from 'bun:test';

const RAIZ = new URL('../../../', import.meta.url);

const REGLAS: { nombre: string; re: RegExp; usa: string; excepciones: Record<string, string> }[] = [
  {
    nombre: 'filtro en minúsculas',
    // Contra algo que NO es un literal: `v.toLowerCase().includes('javascript:')`
    // comprueba un valor, no busca lo que alguien ha escrito.
    re: /\.to(Locale)?LowerCase\(\)\.includes\((?!['"`])/,
    usa: '`contiene(texto, q)`',
    excepciones: {
      'apps/calendar/src/components/CalendarApp.tsx':
        'saber si una línea ATTENDEE es la de quien mira (su correo), no un buscador',
      'packages/office/src/editor/sheets/format.ts':
        'formato condicional «el texto contiene» de la hoja: la semántica de Excel (y de Calcus en Fyne), que distingue acentos',
    },
  },
  {
    nombre: 'normalizador hecho a mano',
    re: /normalize\(['"]NFD['"]\)/,
    usa: '`plegar(s)`',
    excepciones: {
      'packages/i18n/src/plegar.ts': 'es la regla',
      'packages/api-clients/src/contacts/index.ts': 'slug ASCII de una libreta, no un buscador',
      'apps/portal/src/components/Onboarding.tsx': 'slug ASCII de la organización, no un buscador',
      'apps/calendar/src/pages/api/calendars.ts': 'segmento CalDAV ASCII, no un buscador',
      'apps/notes/src/client/enlaces.ts':
        'ancla `h=` de un título: cambiarla rompería los enlaces ya guardados',
      'apps/rooms/src/lib/hookVariants.ts': 'alias de correo ASCII, no un buscador',
      'apps/cloud-dashboard/src/components/TemplateWizard.tsx': 'subdominio ASCII, no un buscador',
      'apps/console/src/components/ConnectIdpWizard.tsx':
        'slug ASCII del proveedor, no un buscador',
    },
  },
];

function seRevisa(rel: string): boolean {
  if (rel.includes('/i18n/messages/') || rel.includes('.test.')) return false;
  return /\.(ts|tsx|astro)$/.test(rel);
}

async function fuentes(): Promise<Map<string, string>> {
  const out = new Map<string, string>();
  for (const patron of ['apps/*/src/**/*', 'packages/*/src/**/*']) {
    for await (const rel of new Bun.Glob(patron).scan({ cwd: RAIZ.pathname })) {
      if (seRevisa(rel)) out.set(rel, await Bun.file(new URL(rel, RAIZ)).text());
    }
  }
  return out;
}

test('los buscadores pasan por plegar/contiene', async () => {
  const todo = await fuentes();
  const malos: string[] = [];
  const sobran: string[] = [];
  for (const regla of REGLAS) {
    const con = new Set<string>();
    for (const [rel, src] of todo) {
      src.split('\n').forEach((linea, i) => {
        if (!regla.re.test(linea)) return;
        con.add(rel);
        if (!(rel in regla.excepciones))
          malos.push(`${rel}:${i + 1}  ${linea.trim()} — usa ${regla.usa}`);
      });
    }
    for (const rel of Object.keys(regla.excepciones)) {
      if (!con.has(rel)) sobran.push(`${rel} ya no lleva «${regla.nombre}»: quita la excepción`);
    }
  }
  expect(malos).toEqual([]);
  expect(sobran).toEqual([]);
});
