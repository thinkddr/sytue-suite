/*
 * SPDX-License-Identifier: Apache-2.0
 * SPDX-FileCopyrightText: (c) 2026 Daniel Delgado Ramos
 *
 * La galería de «Crear nuevo»: una tarjeta por plantilla con su miniatura, su
 * nombre y su descripción (los da el servicio, ya traducidos), y elegir una pasa
 * la plantilla entera a quien crea. Que cada plantilla de Sytue tenga su esquema
 * lo vigila el lado Go (plantillas_test.go), que es quien conoce la lista.
 */
import { GlobalRegistrator } from '@happy-dom/global-registrator';
import { expect, test } from 'bun:test';
import { render } from 'preact';
import type { DocTemplate } from '@sytue/api-clients';
import { docsT } from '../i18n';
import { GaleriaPlantillas, Miniatura } from './GaleriaPlantillas.js';

if (typeof document === 'undefined') GlobalRegistrator.register();

const t = docsT('es');

const PLANTILLAS: DocTemplate[] = [
  { key: 'carta', kind: 'document', name: 'Carta formal', description: 'Remitente y asunto.' },
  { key: 'presupuesto', kind: 'spreadsheet', name: 'Presupuesto', description: 'Previsto y real.' },
  { key: 'org-1', kind: 'presentation', name: 'Plantilla de la casa', description: '' },
];

function montar(nodo: preact.VNode): HTMLElement {
  const host = document.createElement('div');
  document.body.appendChild(host);
  render(nodo, host);
  return host;
}

test('una tarjeta por plantilla, con nombre y descripción, y elegir pasa la plantilla', () => {
  const elegidas: DocTemplate[] = [];
  const host = montar(
    <GaleriaPlantillas
      t={t}
      plantillas={PLANTILLAS}
      creando={null}
      deshabilitada={false}
      onElegir={(p) => elegidas.push(p)}
    />,
  );
  expect(host.querySelector('h3')?.textContent).toBe('Plantillas');
  const botones = [...host.querySelectorAll('button')];
  expect(botones.map((b) => b.textContent)).toEqual([
    'Carta formalRemitente y asunto.',
    'PresupuestoPrevisto y real.',
    'Plantilla de la casa',
  ]);
  botones[1]!.click();
  expect(elegidas).toEqual([PLANTILLAS[1]!]);
});

test('mientras se crea, la elegida dice «Creando…» y ninguna se puede pulsar', () => {
  const host = montar(
    <GaleriaPlantillas
      t={t}
      plantillas={PLANTILLAS}
      creando="carta"
      deshabilitada
      onElegir={() => undefined}
    />,
  );
  const botones = [...host.querySelectorAll('button')];
  expect(botones.every((b) => b.disabled)).toBe(true);
  expect(botones[0]!.getAttribute('aria-busy')).toBe('true');
  expect(botones[0]!.textContent).toContain('Creando');
});

test('sin plantillas no se pinta nada', () => {
  const host = montar(
    <GaleriaPlantillas
      t={t}
      plantillas={[]}
      creando={null}
      deshabilitada={false}
      onElegir={() => undefined}
    />,
  );
  expect(host.innerHTML).toBe('');
});

test('cada plantilla de Sytue tiene su esquema; una sin esquema lleva el dibujo de su tipo', () => {
  const huella = (clave: string, kind: DocTemplate['kind']): string =>
    montar(<Miniatura clave={clave} kind={kind} />).innerHTML;
  const claves = [
    'carta',
    'informe',
    'acta',
    'orden',
    'presupuesto',
    'gastos',
    'planificacion',
    'factura',
    'proyecto',
    'trimestral',
    'propuesta',
  ];
  const esquemas = claves.map((c) => huella(c, 'document'));
  expect(new Set(esquemas).size).toBe(claves.length);
  expect(huella('desconocida', 'spreadsheet')).toContain('M4 5h16v14H4');
});

const DE_LA_CASA: DocTemplate = {
  key: 'org:11111111-1111-4111-8111-111111111111',
  kind: 'document',
  name: 'Carta de la casa',
  description: 'Con el membrete.',
  versions: [
    { no: 2, id: 'v2', author_sub: 'ada', created_at: '2026-10-02T10:00:00Z' },
    { no: 1, id: 'v1', author_sub: 'ada', created_at: '2026-10-01T10:00:00Z' },
  ],
};

test('las de la organización van en su fila y la versión por defecto es la vigente', () => {
  const elegidas: [string, string | undefined][] = [];
  const host = montar(
    <GaleriaPlantillas
      t={t}
      plantillas={PLANTILLAS}
      deOrganizacion={[DE_LA_CASA]}
      creando={null}
      deshabilitada={false}
      onElegir={(p, v) => elegidas.push([p.key, v])}
    />,
  );
  expect([...host.querySelectorAll('h3')].map((h) => h.textContent)).toEqual([
    'Tu organización',
    'Plantillas',
  ]);
  const select = host.querySelector('select')!;
  // Dos versiones: la vigente primero y marcada; sin tocar nada no se manda versión.
  expect([...select.options].map((o) => o.textContent)).toEqual([
    'Versión 2 (vigente)',
    'Versión 1',
  ]);
  host.querySelector<HTMLButtonElement>('ul button')!.click();
  expect(elegidas).toEqual([[DE_LA_CASA.key, undefined]]);

  // Elegir la 1 la manda; los selectores solo existen en las de la organización.
  select.value = 'v1';
  select.dispatchEvent(new Event('change', { bubbles: true }));
  return Promise.resolve().then(() => {
    host.querySelector<HTMLButtonElement>('ul button')!.click();
    expect(elegidas.at(-1)).toEqual([DE_LA_CASA.key, 'v1']);
    expect(host.querySelectorAll('select').length).toBe(1);
  });
});

test('con solo plantillas de la organización también se pinta la galería', () => {
  const host = montar(
    <GaleriaPlantillas
      t={t}
      plantillas={[]}
      deOrganizacion={[{ ...DE_LA_CASA, versions: DE_LA_CASA.versions!.slice(0, 1) }]}
      creando={null}
      deshabilitada={false}
      onElegir={() => undefined}
    />,
  );
  expect(host.querySelectorAll('li').length).toBe(1);
  expect(host.querySelector('select')).toBeNull(); // una sola versión: nada que elegir
});

test('el selector de tema solo aparece si hay temas y avisa del elegido', () => {
  const elegidos: string[] = [];
  const temas = [
    { id: 'marca', label: 'Marca: Acme (v2)' },
    { id: 'carbon', label: 'Carbón' },
  ];
  const host = montar(
    <GaleriaPlantillas
      t={t}
      plantillas={PLANTILLAS}
      creando={null}
      deshabilitada={false}
      onElegir={() => undefined}
      temas={temas}
      tema=""
      onTema={(id) => elegidos.push(id)}
    />,
  );
  const select = host.querySelector('label select') as HTMLSelectElement;
  expect([...select.options].map((o) => o.textContent)).toEqual([
    'Como en la plantilla',
    'Marca: Acme (v2)',
    'Carbón',
  ]);
  select.value = 'marca';
  select.dispatchEvent(new Event('change', { bubbles: true }));
  expect(elegidos).toEqual(['marca']);
  // Sin temas, no hay selector.
  const sin = montar(
    <GaleriaPlantillas
      t={t}
      plantillas={PLANTILLAS}
      creando={null}
      deshabilitada={false}
      onElegir={() => undefined}
    />,
  );
  expect(sin.querySelector('label select')).toBeNull();
});

test('cada fila declara su desplazamiento lateral con data-scroll-x', () => {
  // Es lo que distingue un carrusel de un contenido que se sale: sin el atributo,
  // el barrido de desbordamiento ponía la portada de Docs en rojo en cada pantalla.
  const host = montar(
    <GaleriaPlantillas
      t={t}
      plantillas={PLANTILLAS}
      creando={null}
      deshabilitada={false}
      onElegir={() => undefined}
    />,
  );
  const filas = [...host.querySelectorAll('ul')];
  expect(filas.length).toBeGreaterThan(0);
  for (const fila of filas) expect(fila.hasAttribute('data-scroll-x')).toBe(true);
});
