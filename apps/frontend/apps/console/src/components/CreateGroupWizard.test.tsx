/*
 * SPDX-License-Identifier: Apache-2.0
 * SPDX-FileCopyrightText: (c) 2026 Daniel Delgado Ramos
 *
 * Crear un grupo es un asistente: tipo, nombre, miembros o regla, quién entra,
 * repaso y hecho. Comprueba con `fetch` simulado que en un dinámico la
 * previsualización se pide ANTES de crear (y otra vez si la regla cambia), que
 * cancelar no crea nada, que un manual añade a las personas elegidas y que cada
 * error de Sytue ID vuelve al paso que lo corrige. Ejecutar: `bun test`.
 */
import { GlobalRegistrator } from '@happy-dom/global-registrator';
import { afterEach, expect, test } from 'bun:test';
import { render } from 'preact';
import { act } from 'preact/test-utils';
import type { AdminGroup, DirectoryUser } from '../lib/types.js';

if (typeof document === 'undefined') GlobalRegistrator.register();

const {
  default: CreateGroupWizard,
  pasoDelError,
  reglaCompleta,
} = await import('./CreateGroupWizard.js');
const { useT: traductor } = await import('../i18n/index.js');
const t = traductor('es');

let llamadas: { url: string; method: string; body: Record<string, unknown> }[] = [];
function respuestas(...rs: Response[]) {
  llamadas = [];
  globalThis.fetch = (async (url: string, init?: RequestInit) => {
    llamadas.push({
      url,
      method: init?.method ?? 'GET',
      body: JSON.parse(String(init?.body ?? '{}')),
    });
    return rs.shift() ?? new Response(null, { status: 500 });
  }) as unknown as typeof fetch;
}
const json = (status: number, body: unknown) =>
  new Response(JSON.stringify(body), { status, headers: { 'content-type': 'application/json' } });

const persona = (id: string, fullName: string): DirectoryUser =>
  ({ id, fullName, email: `${id}@acme.es` }) as DirectoryUser;
const USUARIOS = [
  persona('ana', 'Ana Ruiz'),
  persona('luis', 'Luis Gil'),
  persona('eva', 'Eva Sanz'),
];

let host: HTMLElement | null = null;
afterEach(async () => {
  if (host) await act(async () => render(null, host!));
  host = null;
  document.body.innerHTML = '';
});
let creados: AdminGroup[] = [];
let abierto: AdminGroup | null = null;
let cerrado = false;
async function montar() {
  creados = [];
  abierto = null;
  cerrado = false;
  respuestas();
  host = document.createElement('div');
  document.body.appendChild(host);
  await act(async () =>
    render(
      <CreateGroupWizard
        t={t}
        locale="es"
        users={USUARIOS}
        onCreated={(g) => creados.push(g)}
        onOpenGroup={(g) => (abierto = g)}
        onClose={() => (cerrado = true)}
      />,
      host!,
    ),
  );
}
const tick = () => new Promise((r) => setTimeout(r, 0));
const boton = (texto: string) =>
  [...document.querySelectorAll('button')].find((b) => b.textContent?.trim() === texto);
const sigue = () => (boton('Continuar') ? 'Continuar' : 'Continue');
const cancela = () => (boton('Cancelar') ? 'Cancelar' : 'Cancel');
async function pulsar(texto: string) {
  const b = boton(texto);
  if (!b) throw new Error(`no hay botón «${texto}»`);
  if (b.disabled) throw new Error(`el botón «${texto}» está deshabilitado`);
  await act(async () => {
    b.click();
    await tick();
  });
}
async function pulsarAria(aria: string) {
  const b = document.querySelector<HTMLButtonElement>(`button[aria-label="${aria}"]`);
  if (!b) throw new Error(`no hay botón «${aria}»`);
  await act(async () => {
    b.click();
    await tick();
  });
}
async function escribir(el: HTMLInputElement, valor: string) {
  await act(async () => {
    el.value = valor;
    el.dispatchEvent(new Event('input', { bubbles: true }));
  });
}
const campo = (id: string) => document.getElementById(id) as HTMLInputElement;
const valorRegla = () =>
  document.querySelector<HTMLInputElement>(`input[aria-label="${t('groups.rule.value')}"]`)!;
async function elegir(titulo: string) {
  const label = [...document.querySelectorAll('label')].find((l) =>
    l.textContent?.includes(titulo),
  );
  const input = label?.querySelector('input');
  if (!input) throw new Error(`no hay opción «${titulo}»`);
  await act(async () => {
    input.click();
    await tick();
  });
}
const texto = () => document.body.textContent ?? '';
const enFoco = () => document.activeElement?.textContent ?? '';

const PREVIEW = {
  count: 14,
  members: [
    { userId: 'ana', name: 'Ana Ruiz', email: 'ana@acme.es' },
    { userId: 'luis', name: 'Luis Gil', email: 'luis@acme.es' },
  ],
};

/** Tipo dinámico, nombre «Ventas» y la regla «departamento es Ventas». */
async function hastaLaRegla() {
  await montar();
  expect(texto()).toContain(t('groups.wizard.type.heading'));
  await elegir(t('groups.wizard.type.dynamic'));
  await pulsar(sigue());
  expect(enFoco()).toContain(t('groups.wizard.name.heading'));
  expect(boton(sigue())?.disabled).toBe(true);
  await escribir(campo('new-group-name'), '  Ventas ');
  await pulsar(sigue());
  expect(enFoco()).toContain(t('groups.wizard.rule.heading'));
  // Sin valor no se puede pedir la previsualización.
  expect(boton(t('groups.wizard.rule.previewNext'))?.disabled).toBe(true);
  await escribir(valorRegla(), 'Ventas');
}

test('dinámico: se ve quién entra ANTES de crear, con la regla en una frase', async () => {
  await hastaLaRegla();
  const frase = 'Entran las personas si su departamento es «Ventas».';
  expect(texto()).toContain(frase);

  respuestas(json(200, PREVIEW));
  await pulsar(t('groups.wizard.rule.previewNext'));
  expect(llamadas).toEqual([
    {
      url: '/api/admin/groups/preview',
      method: 'POST',
      body: {
        membership_rule: {
          match: 'all',
          conditions: [{ attribute: 'department', op: 'eq', value: 'Ventas' }],
        },
      },
    },
  ]);
  expect(enFoco()).toContain(t('groups.wizard.preview.heading'));
  expect(texto()).toContain('Entrarían 14 personas:');
  expect(texto()).toContain('Luis Gil');
  expect(texto()).toContain(t('groups.wizard.preview.more', { count: 12 }));
  expect(creados).toEqual([]);

  await pulsar(sigue());
  expect(enFoco()).toContain(t('groups.wizard.review.heading'));
  expect(texto()).toContain(frase);
  expect(texto()).toContain(t('groups.memberCount', { count: 14 }));

  respuestas(
    json(201, { id: 'g1', name: 'Ventas', description: '', memberCount: 14, dynamic: true }),
  );
  await pulsar(t('groups.create'));
  expect(llamadas.map((l) => `${l.method} ${l.url}`)).toEqual(['POST /api/admin/groups']);
  expect(llamadas[0]!.body).toEqual({
    name: 'Ventas',
    description: '',
    membership_rule: {
      match: 'all',
      conditions: [{ attribute: 'department', op: 'eq', value: 'Ventas' }],
    },
  });
  expect(creados.map((g) => [g.id, g.memberCount, g.dynamic])).toEqual([['g1', 14, true]]);
  expect(enFoco()).toContain(t('groups.wizard.done.heading'));
  await pulsar(t('groups.wizard.done.open'));
  expect(abierto?.id).toBe('g1');
});

test('cambiar la regla tras verla obliga a previsualizar otra vez', async () => {
  await hastaLaRegla();
  respuestas(json(200, PREVIEW));
  await pulsar(t('groups.wizard.rule.previewNext'));
  expect(enFoco()).toContain(t('groups.wizard.preview.heading'));

  await pulsar(boton('Atrás') ? 'Atrás' : 'Back');
  await escribir(valorRegla(), 'Marketing');
  respuestas(json(200, { count: 0, members: [] }));
  await pulsar(t('groups.wizard.rule.previewNext'));
  expect(llamadas.map((l) => l.url)).toEqual(['/api/admin/groups/preview']);
  // Nadie cumple: se avisa de que nacerá vacío, sin impedir crearlo.
  expect(texto()).toContain(t('groups.noRuleMatches'));
  expect(texto()).toContain(t('groups.wizard.preview.nobodyRuleBody'));
  expect(boton(sigue())?.disabled).toBe(false);
});

test('si Sytue ID rechaza la regla, el error se queda en la regla y no se avanza', async () => {
  await hastaLaRegla();
  respuestas(json(400, { error: 'bad_request' }));
  await pulsar(t('groups.wizard.rule.previewNext'));
  expect(texto()).toContain(t('groups.wizard.rule.heading'));
  expect(texto()).toContain(t('groups.toast.ruleInvalid'));
  expect(texto()).not.toContain(t('groups.wizard.preview.heading'));
});

test('cancelar en el repaso no crea nada', async () => {
  await hastaLaRegla();
  respuestas(json(200, PREVIEW));
  await pulsar(t('groups.wizard.rule.previewNext'));
  await pulsar(sigue());
  expect(enFoco()).toContain(t('groups.wizard.review.heading'));
  await pulsar(cancela());
  expect(cerrado).toBe(true);
  expect(creados).toEqual([]);
  expect(llamadas.some((l) => l.url === '/api/admin/groups')).toBe(false);
});

test('manual: crea el grupo, añade a las elegidas y dice quién falló', async () => {
  await montar();
  // Manual es la opción por defecto.
  await pulsar(sigue());
  await escribir(campo('new-group-name'), 'Comité');
  await escribir(campo('new-group-desc'), 'Seguridad e higiene');
  await pulsar(sigue());
  expect(enFoco()).toContain(t('groups.wizard.members.heading'));
  // Sin regla ni desplegables: se elige a las personas del directorio.
  expect(document.querySelector('select')).toBeNull();
  await pulsarAria(t('groups.wizard.members.addAria', { name: 'Ana Ruiz' }));
  await pulsarAria(t('groups.wizard.members.addAria', { name: 'Eva Sanz' }));
  await pulsarAria(t('groups.wizard.members.addAria', { name: 'Luis Gil' }));
  await pulsarAria(t('groups.wizard.members.removeAria', { name: 'Luis Gil' }));
  expect(texto()).toContain(t('groups.wizard.members.chosen', { count: 2 }));
  await pulsar(sigue());

  // «Quién entra» de un manual es la selección: no hace falta preguntar a nadie.
  expect(llamadas).toEqual([]);
  expect(texto()).toContain('Entrarían 2 personas:');
  await pulsar(sigue());
  expect(texto()).toContain('Ana Ruiz, Eva Sanz');
  expect(texto()).toContain(t('groups.wizard.review.manual'));

  respuestas(
    json(201, { id: 'g2', name: 'Comité', description: 'Seguridad e higiene', memberCount: 0 }),
    json(201, {}),
    json(500, {}),
  );
  await pulsar(t('groups.create'));
  expect(llamadas.map((l) => `${l.method} ${l.url} ${JSON.stringify(l.body)}`)).toEqual([
    'POST /api/admin/groups {"name":"Comité","description":"Seguridad e higiene"}',
    'POST /api/admin/groups/g2/members {"user_id":"ana"}',
    'POST /api/admin/groups/g2/members {"user_id":"eva"}',
  ]);
  expect(creados.map((g) => [g.id, g.memberCount, g.dynamic])).toEqual([['g2', 1, false]]);
  expect(texto()).toContain(t('groups.wizard.done.someFailed', { count: 1 }));
  expect(texto()).toContain('Eva Sanz');

  // «Crear otro» vuelve al principio con todo vacío.
  await pulsar(t('groups.wizard.done.another'));
  expect(texto()).toContain(t('groups.wizard.type.heading'));
  await pulsar(sigue());
  expect(campo('new-group-name').value).toBe('');
});

test('un nombre repetido vuelve al paso del nombre con el aviso en su campo', async () => {
  await montar();
  await pulsar(sigue());
  await escribir(campo('new-group-name'), 'Dirección');
  await pulsar(sigue());
  await pulsar(sigue());
  await pulsar(sigue());
  respuestas(json(409, { error: 'conflict' }));
  await pulsar(t('groups.create'));
  expect(enFoco()).toContain(t('groups.wizard.name.heading'));
  expect(campo('new-group-name').getAttribute('aria-describedby')).toBe('new-group-name-hint');
  expect(document.getElementById('new-group-name-hint')?.textContent).toBe(
    t('groups.toast.nameTaken'),
  );
  expect(creados).toEqual([]);
});

test('cada error vuelve a su paso y la regla incompleta no avanza', () => {
  expect(pasoDelError('409', 'dynamic')).toBe(1);
  expect(pasoDelError('400', 'dynamic')).toBe(2);
  expect(pasoDelError('400', 'manual')).toBe(1);
  expect(pasoDelError('502', 'manual')).toBe(4);
  expect(
    reglaCompleta({ match: 'all', conditions: [{ attribute: 'title', op: 'in', value: ' , ' }] }),
  ).toBe(false);
  expect(
    reglaCompleta({ match: 'any', conditions: [{ attribute: 'title', op: 'in', value: 'a, ' }] }),
  ).toBe(true);
});

test('con la previsualización en vuelo la regla no se puede tocar y el diálogo sabe que está ocupado', async () => {
  const ocupado: boolean[] = [];
  creados = [];
  host = document.createElement('div');
  document.body.appendChild(host);
  await act(async () =>
    render(
      <CreateGroupWizard
        t={t}
        locale="es"
        users={USUARIOS}
        onCreated={(g) => creados.push(g)}
        onOpenGroup={() => {}}
        onClose={() => {}}
        onBusyChange={(b) => ocupado.push(b)}
      />,
      host!,
    ),
  );
  await elegir(t('groups.wizard.type.dynamic'));
  await pulsar(sigue());
  await escribir(campo('new-group-name'), 'Ventas');
  await pulsar(sigue());
  await escribir(valorRegla(), 'Ventas');

  let soltar: (r: Response) => void = () => {};
  llamadas = [];
  globalThis.fetch = (() => new Promise<Response>((r) => (soltar = r))) as unknown as typeof fetch;
  await pulsar(t('groups.wizard.rule.previewNext'));
  // Una edición ahora dejaría ver la lista de la regla anterior como si fuera la nueva.
  expect(valorRegla().closest('fieldset')?.disabled).toBe(true);
  expect(ocupado.at(-1)).toBe(true);

  await act(async () => {
    soltar(json(200, PREVIEW));
    await tick();
  });
  expect(enFoco()).toContain(t('groups.wizard.preview.heading'));
  expect(ocupado.at(-1)).toBe(false);
});
