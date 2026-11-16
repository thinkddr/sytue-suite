/*
 * SPDX-License-Identifier: Apache-2.0
 * SPDX-FileCopyrightText: (c) 2026 Daniel Delgado Ramos
 *
 * Roles y PIM en SettingsShell con asistente: crear un rol o una elegibilidad ya no es
 * un modal de casillas, sino pasos con repaso antes de llamar a Sytue ID. Comprueba el
 * recorrido entero y lo que llega al BFF. Ejecutar: `bun test`.
 */
import { GlobalRegistrator } from '@happy-dom/global-registrator';
import { afterEach, expect, test } from 'bun:test';
import { render } from 'preact';
import { act } from 'preact/test-utils';

if (typeof document === 'undefined') GlobalRegistrator.register();

const { default: RolesPanel } = await import('./RolesPanel.js');
const { default: PrivilegedAccessPanel } = await import('./PrivilegedAccessPanel.js');
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

let host: HTMLElement | null = null;
afterEach(async () => {
  if (host) await act(async () => render(null, host!));
  host = null;
  document.body.innerHTML = '';
});
async function montar(vnode: preact.VNode) {
  host = document.createElement('div');
  document.body.appendChild(host);
  await act(async () => render(vnode, host!));
}
const tick = () => new Promise((r) => setTimeout(r, 0));
const boton = (texto: string) =>
  [...document.querySelectorAll('button')].find((b) => b.textContent?.trim() === texto);
// El cromo del asistente sale del catálogo común según el idioma del documento.
const sigue = () => (boton('Continuar') ? 'Continuar' : 'Continue');
async function pulsar(texto: string) {
  const b = boton(texto);
  if (!b) throw new Error(`no hay botón «${texto}»`);
  if (b.disabled) throw new Error(`el botón «${texto}» está deshabilitado`);
  await act(async () => {
    b.click();
    await tick();
  });
}
async function escribir(id: string, valor: string) {
  const i = document.getElementById(id) as HTMLInputElement;
  await act(async () => {
    i.value = valor;
    i.dispatchEvent(new Event('input', { bubbles: true }));
  });
}
async function marcar(texto: string) {
  const label = [...document.querySelectorAll('label')].find((l) => l.textContent?.includes(texto));
  const input = (label?.querySelector('input') ??
    document.getElementById(label?.htmlFor ?? '')) as HTMLInputElement | null;
  if (!input) throw new Error(`no hay casilla «${texto}»`);
  await act(async () => {
    input.click();
    await tick();
  });
}
const texto = () => document.body.textContent ?? '';

test('Roles: el alta es un asistente con repaso y acaba ofreciendo asignar', async () => {
  await montar(<RolesPanel locale="es" roles={[]} users={[]} live />);
  // La página es un SettingsShell con su sección de lista, no una cabecera suelta.
  expect(texto()).toContain(t('roles.listTitle'));
  await pulsar(t('roles.create'));
  expect(texto()).toContain(t('roles.wizard.name.heading'));
  expect(document.querySelector('[role="dialog"]')).toBeNull();
  // Sin nombre no se avanza.
  expect(boton(sigue())?.disabled).toBe(true);
  await escribir('new-role-name', '  Soporte  ');
  await pulsar(sigue());
  expect(texto()).toContain(t('roles.wizard.permissions.heading'));
  // Sin permisos avisa (pero deja seguir).
  expect(texto()).toContain(t('roles.wizard.permissions.noneTitle'));
  await marcar(t('perm.auditView'));
  await marcar(t('perm.usersRead'));
  expect(texto()).not.toContain(t('roles.wizard.permissions.noneTitle'));
  await pulsar(sigue());
  expect(texto()).toContain(t('roles.wizard.review.heading'));
  expect(texto()).toContain(`${t('perm.usersRead')} · ${t('perm.auditView')}`);

  respuestas(json(201, { id: 'r1', name: 'Soporte', permissions: ['users.read', 'audit.view'] }));
  await pulsar(t('roles.create'));
  expect(llamadas).toEqual([
    {
      url: '/api/admin/roles',
      method: 'POST',
      // Nombre recortado y permisos en el orden del catálogo, no en el de los clics.
      body: { name: 'Soporte', permissions: ['users.read', 'audit.view'] },
    },
  ]);
  expect(texto()).toContain(t('roles.wizard.done.heading'));

  // «Asignar a personas» vuelve a la lista con la ficha del rol nuevo abierta.
  respuestas(json(200, { members: [] }));
  await pulsar(t('roles.wizard.done.assign'));
  expect(llamadas[0]?.url).toBe('/api/admin/roles/r1');
  expect(document.querySelector('[role="dialog"]')).not.toBeNull();
  expect(texto()).toContain(t('roles.assignTo'));
});

test('Roles: un nombre repetido devuelve al paso del nombre', async () => {
  await montar(<RolesPanel locale="es" roles={[]} users={[]} live />);
  await pulsar(t('roles.create'));
  await escribir('new-role-name', 'Soporte');
  await pulsar(sigue());
  await pulsar(sigue());
  respuestas(new Response(null, { status: 409 }));
  await pulsar(t('roles.create'));
  expect(texto()).toContain(t('roles.wizard.name.heading'));
  expect(texto()).toContain(t('roles.toast.nameTaken'));
});

test('PIM: la elegibilidad se crea por pasos y avisa de los superusuarios', async () => {
  await montar(
    <PrivilegedAccessPanel
      locale="es"
      myEligibilities={[]}
      myActivations={[]}
      eligibilities={[]}
      activations={[]}
      users={[{ id: 'u1', email: 'ana@ejemplo.es', name: 'Ana' }]}
      roles={[{ id: 'r1', name: 'Soporte' }]}
      live
    />,
  );
  // La pestaña de administración tiene el botón de alta.
  const tab = [...document.querySelectorAll('[role="tab"]')].find(
    (b) => b.textContent?.trim() === t('pim.tab.admin'),
  ) as HTMLElement;
  await act(async () => {
    tab.click();
    await tick();
  });
  await pulsar(t('pim.admin.newEligibility'));
  expect(texto()).toContain(t('pim.wizard.who.heading'));
  const sel = document.getElementById('pim-user') as HTMLSelectElement;
  await act(async () => {
    sel.value = 'u1';
    sel.dispatchEvent(new Event('change', { bubbles: true }));
  });
  await pulsar(sigue());
  expect(texto()).toContain(t('pim.wizard.role.heading'));
  expect(texto()).not.toContain(t('pim.wizard.role.builtinWarnTitle'));
  const owner = [...document.querySelectorAll('input[name="sytue-pim-role"]')].find(
    (i) => (i as HTMLInputElement).value === 'builtin:owner',
  ) as HTMLInputElement;
  await act(async () => {
    owner.click();
    await tick();
  });
  expect(texto()).toContain(t('pim.wizard.role.builtinWarnTitle'));
  await pulsar(sigue());
  const aprob = [
    ...document.querySelectorAll('input[name="sytue-pim-approval"]'),
  ][1] as HTMLInputElement;
  await act(async () => {
    aprob.click();
    await tick();
  });
  await escribir('pim-max', '5000');
  expect(boton(sigue())?.disabled).toBe(true);
  await escribir('pim-max', '90');
  await pulsar(sigue());
  expect(texto()).toContain(t('pim.wizard.review.heading'));

  respuestas(
    json(201, {
      id: 'e1',
      userId: 'u1',
      userEmail: 'ana@ejemplo.es',
      userName: 'Ana',
      roleId: '',
      builtinRole: 'owner',
      roleName: 'owner',
      requiresApproval: true,
      maxDurationMinutes: 90,
    }),
  );
  await pulsar(t('pim.newElig.submit'));
  expect(llamadas[0]).toEqual({
    url: '/api/admin/pim/eligibilities',
    method: 'POST',
    body: {
      user_id: 'u1',
      builtin_role: 'owner',
      requires_approval: true,
      max_duration_minutes: 90,
    },
  });
  expect(texto()).toContain(t('pim.wizard.done.bodyApproval'));
  await pulsar(t('pim.wizard.done.back'));
  expect(texto()).toContain('ana@ejemplo.es');
});
