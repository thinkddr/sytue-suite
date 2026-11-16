/*
 * SPDX-License-Identifier: Apache-2.0
 * SPDX-FileCopyrightText: (c) 2026 Daniel Delgado Ramos
 *
 * LO IRREVERSIBLE DE MI CUENTA, CON EL SERVIDOR SIMULADO.
 *
 * Cerrar sesiones, olvidar un dispositivo, revocar una contraseña de aplicación,
 * quitar una passkey y pedir códigos nuevos no tienen vuelta atrás. Aquí se vigila:
 *
 * - que cada una PREGUNTA antes, diciendo qué pasa y con el nombre de lo que toca, y
 *   que «Cancelar» no manda nada;
 * - que si el servidor contesta 500 la tarjeta lo dice (antes se releía la lista y
 *   no se decía nada);
 * - que la carga pinta filas fantasma y el fallo de carga trae «Reintentar»;
 * - que cada aspa tiene su propio nombre accesible y el anillo de foco de la suite.
 *
 * Vistos en ROJO con las tarjetas de antes: sin diálogo el DELETE salía al primer
 * clic, el 500 no dejaba rastro y las aspas se llamaban todas igual.
 */
import { GlobalRegistrator } from '@happy-dom/global-registrator';
import { afterAll, afterEach, beforeEach, describe, expect, test } from 'bun:test';
import { render, type ComponentChild } from 'preact';
import { act } from 'preact/test-utils';
import { useT as traductor } from '../i18n/index.js';
import { en } from '../i18n/messages/en.js';
import { olvidarLecturas } from '../lib/primeraLectura.js';

const registrado = typeof document === 'undefined';
if (registrado) GlobalRegistrator.register({ url: 'https://id.sytue.test/account/' });
const fetchOriginal = globalThis.fetch;

const { SesionesCard } = await import('./SesionesCard.js');
const { DispositivosCard } = await import('./DispositivosCard.js');
const { AppPasswordsCard } = await import('./AppPasswordsCard.js');
const { SeguridadCard } = await import('./SeguridadCard.js');
const { RubricaCard } = await import('./RubricaCard.js');

const t = traductor('en');

/** Lo que contesta el servidor simulado: `"MÉTODO /ruta"` → [estado, cuerpo]. */
type Respuestas = Record<string, [number, unknown]>;
let respuestas: Respuestas = {};
let pedidas: string[] = [];

beforeEach(() => {
  pedidas = [];
  globalThis.fetch = (async (input: RequestInfo | URL, init?: RequestInit) => {
    const clave = `${init?.method ?? 'GET'} ${String(input)}`;
    pedidas.push(clave);
    const [estado, cuerpo] = respuestas[clave] ?? [404, { error: 'not_found' }];
    return new Response(JSON.stringify(cuerpo), {
      status: estado,
      headers: { 'content-type': 'application/json' },
    });
  }) as unknown as typeof fetch;
});

afterEach(() => {
  act(() => render(null, document.body));
  document.body.innerHTML = '';
  globalThis.fetch = fetchOriginal;
  respuestas = {};
  olvidarLecturas();
});

afterAll(async () => {
  if (registrado) await GlobalRegistrator.unregister();
});

async function esperar(cond: () => boolean): Promise<void> {
  for (let i = 0; i < 50 && !cond(); i++) {
    await act(async () => {
      await new Promise((r) => setTimeout(r, 5));
    });
  }
  expect(cond()).toBe(true);
}

async function montar(arbol: ComponentChild): Promise<void> {
  await act(async () => {
    render(arbol, document.body);
  });
}

const botonPorNombre = (nombre: string): HTMLButtonElement | undefined =>
  [...document.querySelectorAll<HTMLButtonElement>('button')].find(
    (b) => b.getAttribute('aria-label') === nombre || b.textContent?.trim() === nombre,
  );

const dialogo = (): HTMLElement | null => document.querySelector('[role="dialog"]');

/** Pulsa un botón del diálogo de confirmación por su texto. */
async function contestar(texto: string): Promise<void> {
  const b = [...(dialogo()?.querySelectorAll<HTMLButtonElement>('button') ?? [])].find(
    (x) => x.textContent?.trim() === texto,
  );
  expect(b).toBeDefined();
  await act(async () => {
    b!.click();
  });
}

async function pulsar(nombre: string): Promise<void> {
  const b = botonPorNombre(nombre);
  expect(b).toBeDefined();
  await act(async () => {
    b!.click();
  });
  await esperar(() => dialogo() !== null);
}

const alertas = (): string[] =>
  [...document.querySelectorAll('[role="alert"]')].map((a) => a.textContent ?? '');

const borrados = (): string[] => pedidas.filter((p) => p.startsWith('DELETE'));

describe('sesiones', () => {
  const hace = (min: number) => new Date(Date.now() - min * 60_000).toISOString();
  const SESIONES = {
    sessions: [
      {
        id: 's1',
        ip: '10.0.0.1',
        user_agent: 'UA1',
        device: 'Firefox on Linux',
        auth_time: hace(3),
        expires_at: '',
        current: true,
      },
      {
        id: 's2',
        ip: '10.0.0.2',
        user_agent: 'UA2',
        device: 'Safari on iPhone',
        auth_time: hace(90),
        expires_at: '',
        current: false,
      },
      {
        id: 's3',
        ip: '10.0.0.3',
        user_agent: 'UA3',
        device: 'Edge on Windows',
        auth_time: hace(30),
        expires_at: '',
        current: false,
      },
    ],
  };

  test('mientras carga hay filas fantasma, y luego cada aspa dice qué sesión cierra', async () => {
    let soltar!: () => void;
    const listo = new Promise<void>((r) => (soltar = r));
    const base = globalThis.fetch;
    globalThis.fetch = (async (i: RequestInfo | URL, o?: RequestInit) => {
      await listo;
      return base(i, o);
    }) as unknown as typeof fetch;
    respuestas['GET /api/sessions'] = [200, SESIONES];
    await montar(<SesionesCard t={t} locale="en" />);

    const cargando = document.querySelector('[aria-busy="true"]');
    expect(cargando?.getAttribute('aria-label')).toBe(en['common.loading']);
    expect(cargando?.querySelectorAll('[data-fila-cargando]').length).toBeGreaterThan(0);

    soltar();
    await esperar(() => document.body.textContent?.includes('Safari on iPhone') ?? false);
    expect(document.querySelector('[aria-busy="true"]')).toBeNull();

    const aspas = ['Safari on iPhone', 'Edge on Windows'].map((d) =>
      botonPorNombre(t('sessions.close_named', { device: d })),
    );
    expect(aspas.every(Boolean)).toBe(true);
    // El anillo de foco canónico, que el `<button class="p-1">` de antes no traía.
    expect(aspas.every((b) => b!.className.includes('sytue-focus-ring'))).toBe(true);
    // La actual no lleva aspa.
    expect(
      botonPorNombre(t('sessions.close_named', { device: 'Firefox on Linux' })),
    ).toBeUndefined();
    // Cuándo se inició cada sesión (`auth_time`).
    expect(document.body.textContent).toContain(t('sessions.signed_in', { time: '' }).trim());
  });

  test('cerrar una pregunta nombrando el dispositivo; cancelar no manda nada', async () => {
    respuestas['GET /api/sessions'] = [200, SESIONES];
    respuestas['DELETE /api/sessions/s2'] = [200, {}];
    await montar(<SesionesCard t={t} locale="en" />);
    await esperar(
      () => !!botonPorNombre(t('sessions.close_named', { device: 'Safari on iPhone' })),
    );

    await pulsar(t('sessions.close_named', { device: 'Safari on iPhone' }));
    expect(dialogo()?.textContent).toContain(
      t('sessions.close_one_confirm', { device: 'Safari on iPhone' }),
    );
    await contestar(en['common.cancel']);
    await esperar(() => dialogo() === null);
    expect(borrados()).toEqual([]);

    await pulsar(t('sessions.close_named', { device: 'Safari on iPhone' }));
    await contestar(en['sessions.close_one']);
    await esperar(() => borrados().length === 1);
    expect(borrados()).toEqual(['DELETE /api/sessions/s2']);
    expect(alertas()).toEqual([]);
  });

  test('si el servidor falla al cerrar, la tarjeta lo dice', async () => {
    respuestas['GET /api/sessions'] = [200, SESIONES];
    respuestas['DELETE /api/sessions/s3'] = [500, { error: 'internal' }];
    await montar(<SesionesCard t={t} locale="en" />);
    await esperar(() => !!botonPorNombre(t('sessions.close_named', { device: 'Edge on Windows' })));

    await pulsar(t('sessions.close_named', { device: 'Edge on Windows' }));
    await contestar(en['sessions.close_one']);
    await esperar(() => alertas().includes(en['security.failed']));
  });

  test('«Cerrar todas» avisa de que esta también se cierra', async () => {
    respuestas['GET /api/sessions'] = [200, SESIONES];
    respuestas['DELETE /api/sessions'] = [200, {}];
    await montar(<SesionesCard t={t} locale="en" />);
    await esperar(() => !!botonPorNombre(en['sessions.close_others']));

    await pulsar(en['sessions.close_others']);
    expect(dialogo()?.textContent).toContain(en['sessions.close_all_confirm']);
    await contestar(en['common.cancel']);
    expect(borrados()).toEqual([]);

    await pulsar(en['sessions.close_others']);
    await contestar(en['sessions.close_others']);
    await esperar(() => borrados().length === 1);
    expect(borrados()).toEqual(['DELETE /api/sessions']);
  });

  test('un fallo de carga trae «Reintentar», y reintentar carga la lista', async () => {
    respuestas['GET /api/sessions'] = [500, { error: 'internal' }];
    await montar(<SesionesCard t={t} locale="en" />);
    await esperar(() => alertas().some((a) => a.includes(en['sessions.failed'])));

    respuestas['GET /api/sessions'] = [200, SESIONES];
    await act(async () => {
      botonPorNombre(en['common.retry'])!.click();
    });
    await esperar(() => document.body.textContent?.includes('Safari on iPhone') ?? false);
    expect(alertas()).toEqual([]);
  });
});

describe('dispositivos', () => {
  const EQUIPOS = {
    devices: [
      {
        id: 'd1',
        name: 'Work laptop',
        user_agent: 'UA',
        trusted: true,
        last_seen: '',
        last_ip: '',
        current: false,
      },
      {
        id: 'd2',
        name: 'Old phone',
        user_agent: 'UA',
        trusted: false,
        last_seen: '',
        last_ip: '',
        current: false,
      },
    ],
  };

  test('olvidar pregunta, cancelar no manda nada, y un 500 se dice', async () => {
    respuestas['GET /api/devices'] = [200, EQUIPOS];
    respuestas['DELETE /api/devices/d2'] = [500, { error: 'internal' }];
    await montar(<DispositivosCard t={t} locale="en" />);
    await esperar(() => !!botonPorNombre(t('devices.forget_named', { device: 'Old phone' })));
    // Nombres distintos para cada aspa.
    expect(botonPorNombre(t('devices.forget_named', { device: 'Work laptop' }))).toBeDefined();

    await pulsar(t('devices.forget_named', { device: 'Old phone' }));
    expect(dialogo()?.textContent).toContain(t('devices.forget_confirm', { device: 'Old phone' }));
    await contestar(en['common.cancel']);
    expect(borrados()).toEqual([]);

    await pulsar(t('devices.forget_named', { device: 'Old phone' }));
    await contestar(en['devices.forget']);
    await esperar(() => alertas().includes(en['security.failed']));
    expect(borrados()).toEqual(['DELETE /api/devices/d2']);
  });

  test('un fallo de carga trae «Reintentar»', async () => {
    respuestas['GET /api/devices'] = [503, {}];
    await montar(<DispositivosCard t={t} locale="en" />);
    await esperar(() => alertas().some((a) => a.includes(en['devices.failed'])));
    expect(botonPorNombre(en['common.retry'])).toBeDefined();
  });
});

describe('contraseñas de aplicación', () => {
  const LISTA = {
    app_passwords: [{ id: 'a1', label: 'Thunderbird', created_at: '2026-10-01T00:00:00Z' }],
  };

  test('revocar pregunta con el nombre; cancelar no manda nada; ok revoca', async () => {
    respuestas['GET /api/app-passwords'] = [200, LISTA];
    respuestas['DELETE /api/app-passwords/a1'] = [200, {}];
    await montar(<AppPasswordsCard t={t} locale="en" />);
    const nombre = t('app_passwords.revoke_named', { label: 'Thunderbird' });
    await esperar(() => !!botonPorNombre(nombre));

    await pulsar(nombre);
    expect(dialogo()?.textContent).toContain(
      t('app_passwords.revoke_confirm', { label: 'Thunderbird' }),
    );
    await contestar(en['common.cancel']);
    expect(borrados()).toEqual([]);

    await pulsar(nombre);
    await contestar(en['app_passwords.revoke']);
    await esperar(() => borrados().length === 1);
    expect(alertas()).toEqual([]);
  });

  test('un 500 al revocar se dice, y no como un fallo de carga', async () => {
    respuestas['GET /api/app-passwords'] = [200, LISTA];
    respuestas['DELETE /api/app-passwords/a1'] = [500, {}];
    await montar(<AppPasswordsCard t={t} locale="en" />);
    const nombre = t('app_passwords.revoke_named', { label: 'Thunderbird' });
    await esperar(() => !!botonPorNombre(nombre));

    await pulsar(nombre);
    await contestar(en['app_passwords.revoke']);
    await esperar(() => alertas().includes(en['security.failed']));
    expect(alertas().some((a) => a.includes(en['app_passwords.failed']))).toBe(false);
  });
});

describe('seguridad', () => {
  /** Los puntos de la lista de requisitos: clave → cumplido. */
  const requisitos = (): Record<string, boolean> =>
    Object.fromEntries(
      [...document.querySelectorAll<HTMLElement>('[data-requisito]')].map((li) => [
        li.dataset['requisito'],
        li.dataset['cumplido'] === 'si',
      ]),
    );

  async function escribir(id: string, valor: string): Promise<void> {
    const campo = document.getElementById(id) as HTMLInputElement;
    await act(async () => {
      campo.value = valor;
      campo.dispatchEvent(new Event('input', { bubbles: true }));
    });
  }

  test('enseña los requisitos de /me y los marca mientras se escribe', async () => {
    respuestas['GET /api/me'] = [
      200,
      { mfa_enabled: false, password_policy: { min_length: 14, require_complexity: true } },
    ];
    respuestas['GET /api/passkeys'] = [200, { credentials: [] }];
    await montar(<SeguridadCard t={t} />);
    await esperar(() => Object.keys(requisitos()).length > 0);

    // Visibles ANTES de escribir nada, con la cifra de la política y todo pendiente.
    const lista = document.getElementById('pwd-requisitos');
    expect(lista?.textContent).toContain(t('security.password.req_length', { count: 14 }));
    expect(lista?.textContent).toContain(en['security.password.req_complexity']);
    expect(requisitos()).toEqual({ length: false, complexity: false, different: false });
    // El campo la enlaza, para que un lector de pantalla la lea al entrar.
    expect(document.getElementById('pwd-nueva')?.getAttribute('aria-describedby')).toBe(
      'pwd-requisitos',
    );

    await escribir('pwd-actual', 'la-de-antes-123');
    await escribir('pwd-nueva', 'solo-letras-largas');
    expect(requisitos()).toEqual({ length: true, complexity: false, different: true });
    // Con algo pendiente no se manda: el servidor lo rechazaría igual.
    expect(botonPorNombre(en['security.password.submit'])?.disabled).toBe(true);

    await escribir('pwd-nueva', 'letras-y-numero-7');
    expect(requisitos()).toEqual({ length: true, complexity: true, different: true });
    expect(botonPorNombre(en['security.password.submit'])?.disabled).toBe(false);
  });

  test('sin política en /me no pinta lista ni bloquea el envío', async () => {
    respuestas['GET /api/me'] = [200, { mfa_enabled: false }];
    respuestas['GET /api/passkeys'] = [200, { credentials: [] }];
    await montar(<SeguridadCard t={t} />);
    // Esperar a que /me haya llegado: la tarjeta ya sabe que no hay dos pasos.
    await esperar(() => document.body.textContent?.includes(en['security.mfa.off']) ?? false);
    await escribir('pwd-actual', 'actual');
    await escribir('pwd-nueva', 'x');
    expect(document.getElementById('pwd-requisitos')).toBeNull();
    expect(document.getElementById('pwd-nueva')?.hasAttribute('aria-describedby')).toBe(false);
    expect(botonPorNombre(en['security.password.submit'])?.disabled).toBe(false);
  });

  test('quitar una passkey pregunta, nombrándola', async () => {
    respuestas['GET /api/me'] = [200, { mfa_enabled: false }];
    respuestas['GET /api/passkeys'] = [200, { credentials: [{ id: 'k1', name: 'YubiKey' }] }];
    respuestas['DELETE /api/passkeys/k1'] = [204, {}];
    await montar(<SeguridadCard t={t} />);
    const nombre = t('security.passkeys.remove_named', { name: 'YubiKey' });
    await esperar(() => !!botonPorNombre(nombre));

    await pulsar(nombre);
    expect(dialogo()?.textContent).toContain(
      t('security.passkeys.remove_confirm', { name: 'YubiKey' }),
    );
    await contestar(en['common.cancel']);
    expect(borrados()).toEqual([]);

    await pulsar(nombre);
    await contestar(en['security.passkeys.remove']);
    await esperar(() => borrados().length === 1);
    expect(borrados()).toEqual(['DELETE /api/passkeys/k1']);
  });

  test('quitar una passkey sin red lo dice, en vez de callar', async () => {
    respuestas['GET /api/me'] = [200, { mfa_enabled: false }];
    respuestas['GET /api/passkeys'] = [200, { credentials: [{ id: 'k1', name: 'YubiKey' }] }];
    const simulado = globalThis.fetch;
    globalThis.fetch = (async (input: RequestInfo | URL, init?: RequestInit) => {
      if (init?.method === 'DELETE') throw new TypeError('Failed to fetch');
      return simulado(input, init);
    }) as unknown as typeof fetch;
    await montar(<SeguridadCard t={t} />);
    const nombre = t('security.passkeys.remove_named', { name: 'YubiKey' });
    await esperar(() => !!botonPorNombre(nombre));

    await pulsar(nombre);
    await contestar(en['security.passkeys.remove']);
    await esperar(() => alertas().some((a) => a.includes(en['security.failed'])));
  });

  test('pedir códigos nuevos sin red lo dice y no deja los botones bloqueados', async () => {
    respuestas['GET /api/me'] = [200, { mfa_enabled: true }];
    respuestas['GET /api/passkeys'] = [200, { credentials: [] }];
    const simulado = globalThis.fetch;
    globalThis.fetch = (async (input: RequestInfo | URL, init?: RequestInit) => {
      if (init?.method === 'POST') throw new TypeError('Failed to fetch');
      return simulado(input, init);
    }) as unknown as typeof fetch;
    await montar(<SeguridadCard t={t} />);
    await esperar(() => !!botonPorNombre(en['security.mfa.new_codes']));

    await pulsar(en['security.mfa.new_codes']);
    await contestar(en['security.mfa.new_codes']);
    await esperar(() => alertas().some((a) => a.includes(en['security.failed'])));
    expect(botonPorNombre(en['security.mfa.new_codes'])?.disabled).toBe(false);
  });

  test('pedir códigos nuevos avisa de que los de ahora dejan de valer', async () => {
    respuestas['GET /api/me'] = [200, { mfa_enabled: true }];
    respuestas['GET /api/passkeys'] = [200, { credentials: [] }];
    respuestas['POST /api/mfa/recovery-codes'] = [200, { recovery_codes: ['aaaa-bbbb'] }];
    await montar(<SeguridadCard t={t} />);
    await esperar(() => !!botonPorNombre(en['security.mfa.new_codes']));
    const regenerados = () => pedidas.filter((p) => p === 'POST /api/mfa/recovery-codes');

    await pulsar(en['security.mfa.new_codes']);
    expect(dialogo()?.textContent).toContain(en['security.codes.regenerate_confirm']);
    await contestar(en['common.cancel']);
    expect(regenerados()).toEqual([]);

    await pulsar(en['security.mfa.new_codes']);
    // El botón del diálogo lleva el mismo rótulo que el que lo abrió.
    await contestar(en['security.mfa.new_codes']);
    await esperar(() => document.body.textContent?.includes('aaaa-bbbb') ?? false);
    expect(regenerados()).toHaveLength(1);
  });
});

describe('firma guardada', () => {
  const RUBRICA = {
    trazos: [
      [
        [5, 5],
        [60, 40],
      ],
    ],
    updated_at: '2026-11-01T10:00:00Z',
  };

  test('sin firma guardada lo dice y no ofrece borrar nada', async () => {
    await montar(<RubricaCard t={t} locale="en" />);
    await esperar(() => document.body.textContent?.includes(en['signature.empty']) ?? false);
    expect(botonPorNombre(en['signature.delete'])).toBeUndefined();
  });

  test('la enseña, pregunta antes de borrar y cancelar no manda nada', async () => {
    respuestas = { 'GET /api/rubrica': [200, RUBRICA] };
    await montar(<RubricaCard t={t} locale="en" />);
    await esperar(() => document.querySelector('[data-vista-firma] polyline') !== null);
    expect(document.querySelector('polyline')!.getAttribute('points')).toBe('5,5 60,40');

    await pulsar(en['signature.delete']);
    expect(dialogo()!.textContent).toContain(en['signature.delete_confirm']);
    await contestar(en['common.cancel']);
    expect(borrados()).toEqual([]);

    respuestas['DELETE /api/rubrica'] = [200, { ok: true }];
    await pulsar(en['signature.delete']);
    respuestas['GET /api/rubrica'] = [404, { error: 'not_found' }];
    await contestar(en['signature.delete']);
    await esperar(() => document.body.textContent?.includes(en['signature.empty']) ?? false);
    expect(borrados()).toEqual(['DELETE /api/rubrica']);
  });

  test('si el servidor no la borra, la tarjeta lo dice', async () => {
    respuestas = {
      'GET /api/rubrica': [200, RUBRICA],
      'DELETE /api/rubrica': [500, { error: 'x' }],
    };
    await montar(<RubricaCard t={t} locale="en" />);
    await esperar(() => document.querySelector('[data-vista-firma]') !== null);
    await pulsar(en['signature.delete']);
    await contestar(en['signature.delete']);
    await esperar(() => alertas().some((a) => a.includes(en['security.failed'])));
  });
});
