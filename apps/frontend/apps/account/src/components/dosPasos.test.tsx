/*
 * SPDX-License-Identifier: Apache-2.0
 * SPDX-FileCopyrightText: (c) 2026 Daniel Delgado Ramos
 *
 * ACTIVAR LA VERIFICACIÓN EN DOS PASOS, CON EL SERVIDOR SIMULADO.
 *
 * El alta es un asistente (qué necesitas → escanear → código → códigos → hecho).
 * Aquí se vigila:
 *
 * - que el foco va al encabezado de cada paso;
 * - que el QR y la clave (con «Copiar») salen tras `/api/mfa/enroll`, y que volver
 *   atrás y seguir NO pide otro secreto (dejaría inservible el QR ya escaneado);
 * - que un código rechazado se dice en su paso, sin borrar lo escrito;
 * - que tras activar no se puede cancelar ni volver, y que no se sigue sin marcar
 *   «Los he guardado»;
 * - que «Descargar .txt» guarda un fichero con la cuenta y la fecha;
 * - que cancelar antes de confirmar no manda nada más que el alta pendiente;
 * - que desactivar pide confirmación en un diálogo y el fallo se queda en él.
 *
 * Vistos en ROJO con la tarjeta de antes: no había asistente (ni «Empezar», ni
 * encabezados por paso), ni «Copiar todos» / «Descargar .txt», y el formulario de
 * desactivar estaba siempre a la vista y salía al primer clic.
 */
import { GlobalRegistrator } from '@happy-dom/global-registrator';
import { afterAll, afterEach, beforeEach, describe, expect, test } from 'bun:test';
import { render, type ComponentChild } from 'preact';
import { act } from 'preact/test-utils';
import { useT as traductor } from '../i18n/index.js';
import { en } from '../i18n/messages/en.js';
import { olvidarLecturas } from '../lib/primeraLectura.js';
import { nombreFicheroCodigos, textoCodigos } from '../lib/codigosRecuperacion.js';

const registrado = typeof document === 'undefined';
if (registrado) GlobalRegistrator.register({ url: 'https://id.sytue.test/account/' });
const fetchOriginal = globalThis.fetch;

const { SeguridadCard } = await import('./SeguridadCard.js');

const t = traductor('en');

type Respuestas = Record<string, [number, unknown]>;
let respuestas: Respuestas = {};
let pedidas: { clave: string; cuerpo: string }[] = [];

beforeEach(() => {
  pedidas = [];
  globalThis.fetch = (async (input: RequestInfo | URL, init?: RequestInit) => {
    const clave = `${init?.method ?? 'GET'} ${String(input)}`;
    pedidas.push({ clave, cuerpo: typeof init?.body === 'string' ? init.body : '' });
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
  for (let i = 0; i < 60 && !cond(); i++) {
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

const boton = (nombre: string, raiz: ParentNode = document): HTMLButtonElement | undefined =>
  [...raiz.querySelectorAll<HTMLButtonElement>('button')].find(
    (b) => b.getAttribute('aria-label') === nombre || b.textContent?.trim() === nombre,
  );

async function pulsar(nombre: string, raiz: ParentNode = document): Promise<void> {
  const b = boton(nombre, raiz);
  expect(b).toBeDefined();
  expect(b!.disabled).toBe(false);
  await act(async () => {
    b!.click();
  });
}

async function escribir(id: string, valor: string): Promise<void> {
  const el = document.getElementById(id) as HTMLInputElement;
  expect(el).not.toBeNull();
  await act(async () => {
    el.value = valor;
    el.dispatchEvent(new Event('input', { bubbles: true }));
  });
}

const enfocado = (): string => document.activeElement?.textContent?.trim() ?? '';
const llamadas = (clave: string): number => pedidas.filter((p) => p.clave === clave).length;
const dialogo = (): HTMLElement | null => document.querySelector('[role="dialog"]');

const ALTA = {
  otpauth_uri: 'otpauth://totp/Sytue:ana@example.eu?secret=JBSWY3DPEHPK3PXP',
  secret: 'JBSWY3DPEHPK3PXP',
  qr_png: 'data:image/png;base64,iVBORw0KGgo=',
};
const CODIGOS = ['aaaa-1111', 'bbbb-2222', 'cccc-3333'];

describe('activar la verificación en dos pasos', () => {
  test('recorre el asistente con el foco en cada encabezado y no sigue sin guardar los códigos', async () => {
    respuestas['GET /api/me'] = [200, { mfa_enabled: false, email: 'ana@example.eu' }];
    respuestas['GET /api/passkeys'] = [200, { credentials: [] }];
    respuestas['POST /api/mfa/enroll'] = [200, ALTA];
    await montar(<SeguridadCard t={t} />);
    await esperar(() => !!boton(en['security.mfa.enable']));

    await pulsar(en['security.mfa.enable']);
    expect(document.body.textContent).toContain(en['security.mfa.step.need.heading']);
    expect(llamadas('POST /api/mfa/enroll')).toBe(0);

    // 1 → 2: pide el alta y enseña el QR y la clave con su botón de copiar.
    await pulsar(en['security.mfa.step.need.next']);
    await esperar(() => enfocado() === en['security.mfa.step.scan.heading']);
    expect(llamadas('POST /api/mfa/enroll')).toBe(1);
    expect(document.querySelector('img')?.getAttribute('src')).toBe(ALTA.qr_png);
    expect(document.body.textContent).toContain(ALTA.secret);
    expect(
      [...document.querySelectorAll('button')].find((b) =>
        b.getAttribute('aria-label')?.includes(en['security.mfa.key_label']),
      ),
    ).toBeDefined();

    // Atrás y otra vez adelante: el mismo secreto, sin otro alta.
    await pulsar('Back');
    await esperar(() => enfocado() === en['security.mfa.step.need.heading']);
    await pulsar(en['security.mfa.step.need.next']);
    await esperar(() => enfocado() === en['security.mfa.step.scan.heading']);
    expect(llamadas('POST /api/mfa/enroll')).toBe(1);

    // 2 → 3: el código. Sin seis cifras no se puede activar.
    await pulsar('Continue');
    await esperar(() => enfocado() === en['security.mfa.step.code.heading']);
    const campo = document.getElementById('mfa-codigo') as HTMLInputElement;
    expect(campo.getAttribute('autocomplete')).toBe('one-time-code');
    expect(boton(en['security.mfa.confirm'])?.disabled).toBe(true);

    // Un código rechazado se dice en el paso, junto al campo, sin borrar lo escrito.
    respuestas['POST /api/mfa/confirm'] = [400, { message: 'That code is not valid.' }];
    await escribir('mfa-codigo', '111111');
    await pulsar(en['security.mfa.confirm']);
    await esperar(() => document.body.textContent?.includes('That code is not valid.') ?? false);
    expect(document.body.textContent).toContain(en['security.mfa.step.code.heading']);
    expect((document.getElementById('mfa-codigo') as HTMLInputElement).value).toBe('111111');

    // 3 → 4: activada. Ya no hay «Cancelar» ni «Atrás», y no se sigue sin marcar.
    // El código partido como lo enseña la app («123 456») vale y viaja sin blancos.
    respuestas['POST /api/mfa/confirm'] = [200, { recovery_codes: CODIGOS }];
    respuestas['GET /api/me'] = [200, { mfa_enabled: true, email: 'ana@example.eu' }];
    await escribir('mfa-codigo', '123 456');
    await pulsar(en['security.mfa.confirm']);
    await esperar(() => enfocado() === en['security.mfa.step.codes.heading']);
    expect(pedidas.filter((p) => p.clave === 'POST /api/mfa/confirm').map((p) => p.cuerpo)).toEqual(
      ['{"code":"111111"}', '{"code":"123456"}'],
    );
    for (const c of CODIGOS) expect(document.body.textContent).toContain(c);
    expect(boton('Cancel')).toBeUndefined();
    expect(boton('Back')).toBeUndefined();
    expect(boton('Continue')?.disabled).toBe(true);

    const casilla = [...document.querySelectorAll<HTMLInputElement>('input[type="checkbox"]')].find(
      (c) => c.closest('label')?.textContent?.includes(en['security.codes.saved']),
    );
    expect(casilla).toBeDefined();
    await act(async () => {
      casilla!.click();
    });
    expect(boton('Continue')?.disabled).toBe(false);

    // 4 → 5: hecho, y al cerrar la tarjeta ya dice que está activa.
    await pulsar('Continue');
    await esperar(() => enfocado() === en['security.mfa.step.done.heading']);
    await pulsar(en['security.mfa.close']);
    await esperar(() => document.body.textContent?.includes(en['security.mfa.on']) ?? false);
  });

  test('cancelar antes de confirmar cierra sin activar nada', async () => {
    respuestas['GET /api/me'] = [200, { mfa_enabled: false }];
    respuestas['GET /api/passkeys'] = [200, { credentials: [] }];
    respuestas['POST /api/mfa/enroll'] = [200, ALTA];
    await montar(<SeguridadCard t={t} />);
    await esperar(() => !!boton(en['security.mfa.enable']));

    await pulsar(en['security.mfa.enable']);
    await pulsar(en['security.mfa.step.need.next']);
    await esperar(() => document.body.textContent?.includes(ALTA.secret) ?? false);
    await pulsar('Cancel');

    expect(document.body.textContent).not.toContain(ALTA.secret);
    expect(document.body.textContent).toContain(en['security.mfa.off']);
    expect(llamadas('POST /api/mfa/confirm')).toBe(0);

    // Empezar de nuevo pide un alta nueva: la anterior, sin confirmar, no vale.
    await pulsar(en['security.mfa.enable']);
    await pulsar(en['security.mfa.step.need.next']);
    await esperar(() => llamadas('POST /api/mfa/enroll') === 2);
  });

  test('un fallo al empezar se queda en el primer paso', async () => {
    respuestas['GET /api/me'] = [200, { mfa_enabled: false }];
    respuestas['GET /api/passkeys'] = [200, { credentials: [] }];
    respuestas['POST /api/mfa/enroll'] = [409, { message: 'Already on.' }];
    await montar(<SeguridadCard t={t} />);
    await esperar(() => !!boton(en['security.mfa.enable']));

    await pulsar(en['security.mfa.enable']);
    await pulsar(en['security.mfa.step.need.next']);
    await esperar(() =>
      [...document.querySelectorAll('[role="alert"]')].some((a) =>
        a.textContent?.includes('Already on.'),
      ),
    );
    expect(document.body.textContent).toContain(en['security.mfa.step.need.heading']);
  });
});

describe('los códigos de recuperación', () => {
  test('«Descargar .txt» guarda un fichero con la cuenta y la fecha', async () => {
    respuestas['GET /api/me'] = [200, { mfa_enabled: true, email: 'ana@example.eu' }];
    respuestas['GET /api/passkeys'] = [200, { credentials: [] }];
    respuestas['POST /api/mfa/recovery-codes'] = [200, { recovery_codes: CODIGOS }];

    let blob: Blob | null = null;
    let nombre = '';
    const crear = URL.createObjectURL;
    const revocar = URL.revokeObjectURL;
    const clic = HTMLAnchorElement.prototype.click;
    URL.createObjectURL = ((b: Blob) => {
      blob = b;
      return 'blob:codigos';
    }) as typeof URL.createObjectURL;
    URL.revokeObjectURL = () => undefined;
    HTMLAnchorElement.prototype.click = function (this: HTMLAnchorElement) {
      nombre = this.download;
    };
    try {
      await montar(<SeguridadCard t={t} />);
      await esperar(() => !!boton(en['security.mfa.new_codes']));
      await pulsar(en['security.mfa.new_codes']);
      await esperar(() => dialogo() !== null);
      await pulsar(en['security.mfa.new_codes'], dialogo()!);
      await esperar(() => !!boton(en['security.codes.download']));

      // «Cerrar» no se deja pulsar hasta marcar que se han guardado.
      expect(boton(en['security.mfa.close'])?.disabled).toBe(true);

      await pulsar(en['security.codes.download']);
      const hoy = new Date();
      const dia = `${hoy.getFullYear()}-${String(hoy.getMonth() + 1).padStart(2, '0')}-${String(hoy.getDate()).padStart(2, '0')}`;
      expect(nombre).toBe(`sytue-recovery-codes-ana@example.eu-${dia}.txt`);
      expect(blob).not.toBeNull();
      const texto = await blob!.text();
      expect(texto).toContain('Account: ana@example.eu');
      expect(texto).toContain(`Created: ${dia}`);
      for (const c of CODIGOS) expect(texto).toContain(c);
    } finally {
      URL.createObjectURL = crear;
      URL.revokeObjectURL = revocar;
      HTMLAnchorElement.prototype.click = clic;
    }
  });

  test('nombre y contenido del fichero', () => {
    const fecha = new Date(2026, 10, 3, 9, 5);
    expect(nombreFicheroCodigos('ana@example.eu', fecha)).toBe(
      'sytue-recovery-codes-ana@example.eu-2026-11-03.txt',
    );
    // Lo que Windows no acepta en un nombre se cambia; sin cuenta, solo la fecha.
    expect(nombreFicheroCodigos('a/b:c', fecha)).toBe('sytue-recovery-codes-a_b_c-2026-11-03.txt');
    expect(nombreFicheroCodigos('', fecha)).toBe('sytue-recovery-codes-2026-11-03.txt');

    const texto = textoCodigos(['x1', 'x2'], 'ana@example.eu', fecha, {
      titulo: 'T',
      cuenta: (c) => `C ${c}`,
      fecha: (f) => `F ${f}`,
      nota: 'N',
    });
    expect(texto.split('\r\n')).toEqual([
      'T',
      'C ana@example.eu',
      'F 2026-11-03 09:05',
      '',
      'x1',
      'x2',
      '',
      'N',
      '',
    ]);
  });
});

describe('desactivar', () => {
  test('pide confirmación en un diálogo y el fallo se queda dentro', async () => {
    respuestas['GET /api/me'] = [200, { mfa_enabled: true }];
    respuestas['GET /api/passkeys'] = [200, { credentials: [] }];
    respuestas['POST /api/mfa/disable'] = [403, { message: 'Wrong code.' }];
    await montar(<SeguridadCard t={t} />);
    await esperar(() => !!boton(en['security.mfa.disable']));

    // El formulario ya no está a la vista: solo el botón.
    expect(document.getElementById('mfa-prueba')).toBeNull();
    await pulsar(en['security.mfa.disable']);
    await esperar(() => dialogo() !== null);
    expect(dialogo()!.textContent).toContain(en['security.mfa.disable_warning']);
    // Sin código ni contraseña no se puede confirmar.
    expect(boton(en['security.mfa.disable'], dialogo()!)?.disabled).toBe(true);

    await escribir('mfa-prueba', '654321');
    await pulsar(en['security.mfa.disable'], dialogo()!);
    await esperar(() => dialogo()?.textContent?.includes('Wrong code.') ?? false);
    expect(llamadas('POST /api/mfa/disable')).toBe(1);
    expect((document.getElementById('mfa-prueba') as HTMLInputElement).value).toBe('654321');

    respuestas['POST /api/mfa/disable'] = [204, {}];
    respuestas['GET /api/me'] = [200, { mfa_enabled: false }];
    await pulsar(en['security.mfa.disable'], dialogo()!);
    await esperar(() => document.body.textContent?.includes(en['security.mfa.off']) ?? false);
  });
});
