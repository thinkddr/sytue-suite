/*
 * SPDX-License-Identifier: Apache-2.0
 * SPDX-FileCopyrightText: (c) 2026 Daniel Delgado Ramos
 *
 * EL ESTUDIO DE UN PDF DE LA BÓVEDA (ADR 0086): `/pdf?path=` con el Drive en la bóveda
 * abre el fichero de la bóveda descifrado EN MEMORIA, para verlo y firmarlo en el
 * dispositivo, con el repositorio de verdad (`PrivateDriveRepository`) sobre el almacén
 * sintético de sus pruebas. Lo que se fija: que la bóveda no se abre sola; que lo que
 * necesita al servicio sale apagado y explicado; que el PDF se ve desde un `blob:`
 * local; que al firmar con sello al BFF solo le llega la huella (32 bytes) y nunca el
 * PDF; y que al almacén solo le llega texto cifrado.
 */
import { GlobalRegistrator } from '@happy-dom/global-registrator';
import { afterEach, beforeEach, expect, test } from 'bun:test';
import { render } from 'preact';
import { act } from 'preact/test-utils';
import type { PrivateDriveRepository } from '@sytue/offline/matrix';
import { privateDriveWriteFixture } from '../../../offline/src/matrix/private-drive-fixture.js';
import { docsT } from '../i18n';
import {
  cadenaOctetos,
  conjunto,
  contexto,
  enteroDER,
  nulo,
  oidDER,
  secuencia,
  tlv,
} from './dispositivo/der';
import { OID_SELLO } from './dispositivo/sello';
import { verificarFirmas } from './dispositivo/pades';
import { EstudioBoveda, EstudioPdf } from './EstudioBoveda';

if (typeof document === 'undefined') GlobalRegistrator.register();

const t = docsT('es');
const CORPUS = new URL('../../../../../nucleo/pdf/testdata/pdf/', import.meta.url);
const P12 = new URL('../../../../../../platform/pkg/pkcs12/testdata/', import.meta.url);
const leer = async (base: URL, nombre: string) =>
  new Uint8Array(await Bun.file(new URL(nombre, base)).arrayBuffer());
const latin1 = (b: Uint8Array) => Array.from(b, (c) => String.fromCharCode(c)).join('');
const tick = (ms = 0): Promise<void> => new Promise((r) => setTimeout(r, ms));
async function hasta(cond: () => boolean, ms = 4000): Promise<void> {
  const fin = Date.now() + ms;
  while (!cond() && Date.now() < fin) await act(() => tick(5));
}

/** Un token RFC 3161 que sella `huella` (la autoridad de prueba del BFF). */
function token(huella: Uint8Array): Uint8Array {
  const info = secuencia(
    enteroDER(1),
    oidDER('1.2.3.4.1'),
    secuencia(secuencia(oidDER(OID_SELLO.sha256), nulo()), cadenaOctetos(huella)),
    enteroDER(9),
    tlv(
      0x18,
      Uint8Array.from('20261123100030Z', (c) => c.charCodeAt(0)),
    ),
  );
  return secuencia(
    oidDER(OID_SELLO.firmado),
    tlv(
      contexto(0),
      secuencia(
        enteroDER(3),
        conjunto(),
        secuencia(oidDER(OID_SELLO.tstInfo), tlv(contexto(0), cadenaOctetos(info))),
        conjunto(),
      ),
    ),
  );
}

const fetchOriginal = globalThis.fetch;
// happy-dom no carga páginas en un iframe y lo dice por consola: el visor del navegador
// no es lo que se prueba aquí (se mira su `src`).
const errorOriginal = console.error;
/** Todo lo que el navegador mandó por la red, para comprobar que no viaja el PDF. */
let enviado: { url: string; cuerpo: string }[] = [];
let selloConfigurado = true;
let estadoDrive: Record<string, unknown> | null = null;
let host: HTMLElement;

beforeEach(() => {
  console.error = (...x: unknown[]) => {
    if (!String(x[0]).includes('Iframe page loading is disabled')) errorOriginal(...x);
  };
  enviado = [];
  selloConfigurado = true;
  estadoDrive = null;
  globalThis.fetch = (async (input: RequestInfo | URL, init?: RequestInit) => {
    const url = typeof input === 'string' ? input : input.toString();
    enviado.push({ url, cuerpo: typeof init?.body === 'string' ? init.body : '' });
    if (url.includes('/api/drive-cifrado/') && url.includes('/migration'))
      return estadoDrive ? Response.json(estadoDrive) : new Response('', { status: 404 });
    if (url === '/api/pdf-sello' && init?.method === 'POST') {
      const { resumen } = JSON.parse(String(init.body)) as { resumen: string };
      const huella = Uint8Array.from(atob(resumen), (c) => c.charCodeAt(0));
      return Response.json({ token: btoa(String.fromCharCode(...token(huella))) });
    }
    if (url === '/api/pdf-sello')
      return selloConfigurado
        ? Response.json({ disponible: true, autoridad: 'tsa.example' })
        : Response.json({ disponible: false, autoridad: '' });
    // El estudio del servidor: que se note si se monta.
    return new Promise<Response>(() => undefined);
  }) as typeof fetch;
  host = document.createElement('div');
  document.body.appendChild(host);
});

afterEach(() => {
  act(() => render(null, host));
  host.remove();
  document.body.innerHTML = '';
  globalThis.fetch = fetchOriginal;
  console.error = errorOriginal;
});

const boton = (nombre: string) =>
  [...document.querySelectorAll('button')].find((b) => b.textContent?.trim() === nombre);

/** El control de un rótulo, por su `for`. */
function control<T extends HTMLElement>(rotulo: string): T {
  const label = [...document.querySelectorAll('label')].find((l) =>
    l.textContent?.startsWith(rotulo),
  )!;
  return document.getElementById(label.htmlFor) as T;
}

/** Una bóveda de verdad con `/Contratos/Acta.pdf`. */
async function bovedaConActa() {
  const f = await privateDriveWriteFixture();
  const repositorio: PrivateDriveRepository = f.open();
  await repositorio.initialize(f.state);
  const raiz = String((await repositorio.root()).id);
  const carpeta = String((await repositorio.createFolder(raiz, 'Contratos')).id);
  const pdf = await leer(CORPUS, 'sin-firmas.pdf');
  await repositorio.upload(
    carpeta,
    'Acta.pdf',
    'application/pdf',
    new Blob([pdf], { type: 'application/pdf' }),
  );
  const vida = new AbortController();
  let abierta = 0;
  const abrirDrive = async () => {
    abierta++;
    return { repository: repositorio, signal: vida.signal, close: async () => vida.abort() };
  };
  return { f, repositorio, pdf, abrirDrive, abiertas: () => abierta };
}

test('con el Drive en la bóveda, /pdf?path= es el estudio de la bóveda (y si no, el de siempre)', async () => {
  const { privateDriveWriteFixture: fixture } =
    await import('../../../offline/src/matrix/private-drive-fixture.js');
  const f = await fixture();
  try {
    estadoDrive = f.state as unknown as Record<string, unknown>;
    act(() => {
      render(
        <EstudioPdf
          path="/Contratos/Acta.pdf"
          locale="es"
          autor="Ana"
          actor="owner"
          tenant="acme"
        />,
        host,
      );
    });
    await hasta(() => host.querySelector('[data-estudio-boveda]') !== null);
    expect(host.textContent).toContain(t('pdf.vault.help'));
    // La bóveda no se abre sola.
    expect(boton(t('pdf.device.open'))).toBeDefined();
    // Lo que necesita al servicio, apagado y explicado.
    expect(host.textContent).toContain(t('pdf.vault.serverOnly'));
    const apagados = [...host.querySelectorAll('[data-solo-servidor] button')];
    expect(apagados.map((b) => b.textContent)).toContain(t('pdf.menu.requestSignature'));
    expect(apagados.every((b) => (b as HTMLButtonElement).disabled)).toBe(true);
    // Nada pidió el PDF al servidor.
    expect(enviado.some((e) => e.url.startsWith('/api/pdf') && e.url !== '/api/pdf-sello')).toBe(
      false,
    );
  } finally {
    await f.close();
  }

  act(() => render(null, host));
  estadoDrive = null;
  act(() => {
    render(
      <EstudioPdf path="/PDF/Contrato.pdf" locale="es" autor="Ana" actor="owner" tenant="acme" />,
      host,
    );
  });
  await hasta(() => enviado.some((e) => e.url.includes('/api/pdf') && !e.url.includes('sello')));
  expect(host.querySelector('[data-estudio-boveda]')).toBeNull();
});

test('abre el PDF de la bóveda en memoria, lo firma con sello y nada en claro sale', async () => {
  const { f, repositorio, pdf, abrirDrive, abiertas } = await bovedaConActa();
  try {
    act(() => {
      render(
        <EstudioBoveda
          path="/Contratos/Acta.pdf"
          locale="es"
          actor="owner"
          tenant="acme"
          abrirDrive={abrirDrive}
        />,
        host,
      );
    });
    await act(() => tick(10));
    expect(abiertas()).toBe(0);
    act(() => boton(t('pdf.device.open'))!.click());
    await hasta(() => host.querySelector('[data-visor-boveda]') !== null);
    const visor = host.querySelector<HTMLIFrameElement>('[data-visor-boveda]')!;
    expect(visor.getAttribute('src')).toStartWith('blob:');
    expect(visor.getAttribute('title')).toBe('Acta.pdf');
    const descarga = [...host.querySelectorAll('a')].find(
      (a) => a.textContent === t('pdf.menu.download'),
    )!;
    expect(descarga.getAttribute('download')).toBe('Acta.pdf');

    // Firmar: el diálogo dice ANTES que habrá sello y de quién.
    act(() => boton(t('pdf.menu.sign'))!.click());
    await hasta(() => document.querySelector('[data-sello="si"]') !== null);
    expect(document.body.textContent).toContain(
      t('pdf.device.stamp.on', { authority: 'tsa.example' }),
    );
    // El `.p12`, su contraseña y un recuadro visible en la página 2.
    const fichero = control<HTMLInputElement>(t('pdf.dialog.firmar.p12'));
    const p12 = await leer(P12, 'moderno.p12');
    Object.defineProperty(fichero, 'files', {
      value: [new File([p12], 'yo.p12', { type: 'application/x-pkcs12' })],
    });
    await act(async () => {
      fichero.dispatchEvent(new Event('change', { bubbles: true }));
      await tick(20);
    });
    const clave = control<HTMLInputElement>(t('pdf.dialog.firmar.p12Clave'));
    act(() => {
      clave.value = 'secreta';
      clave.dispatchEvent(new Event('input', { bubbles: true }));
    });
    act(() => boton(t('pdf.device.appearance.visible'))!.click());
    expect(document.querySelector('[data-firma-visible]')).not.toBeNull();
    const pagina = control<HTMLInputElement>(t('pdf.dialog.firmar.page'));
    expect(pagina.getAttribute('max')).toBe('2');
    act(() => {
      pagina.value = '2';
      pagina.dispatchEvent(new Event('input', { bubbles: true }));
    });
    const firmar = [...document.querySelectorAll('button')].filter(
      (b) => b.textContent?.trim() === t('pdf.device.sign'),
    );
    act(() => firmar[firmar.length - 1]!.click());
    await hasta(() => (document.body.textContent ?? '').includes(t('pdf.vault.openSigned')));
    expect(document.body.textContent).toContain(
      t('pdf.device.signedStamped', { name: 'Acta (firmado).pdf' }),
    );
    const abrirFirmado = [...host.querySelectorAll('a')].find(
      (a) => a.textContent === t('pdf.vault.openSigned'),
    )!;
    expect(decodeURIComponent(abrirFirmado.getAttribute('href') ?? '')).toContain(
      '/pdf?path=/Contratos/Acta (firmado).pdf',
    );

    // Al BFF del sello solo le llegó {resumen} de 32 bytes; a nadie, el PDF.
    const sellos = enviado.filter((e) => e.url === '/api/pdf-sello' && e.cuerpo);
    expect(sellos).toHaveLength(1);
    const cuerpo = JSON.parse(sellos[0]!.cuerpo) as Record<string, string>;
    expect(Object.keys(cuerpo)).toEqual(['resumen']);
    expect(atob(cuerpo.resumen!)).toHaveLength(32);
    for (const e of enviado) expect(e.cuerpo).not.toContain('%PDF');

    // El firmado está en la bóveda, verifica, es visible y lleva el sello.
    const { files } = await repositorio.children(
      String((await repositorio.children(String((await repositorio.root()).id))).folders[0]!.id),
    );
    const firmado = files.find((x) => x.name === 'Acta (firmado).pdf')!;
    const { collectPrivateDriveFile } = await import('@sytue/offline/vault/drive');
    const blob = await collectPrivateDriveFile({
      size: firmado.size_bytes,
      maximum: 1 << 24,
      type: 'application/pdf',
      run: (sink) => repositorio.download(String(firmado.id), sink),
    });
    const bytes = new Uint8Array(await blob.arrayBuffer());
    expect(bytes.subarray(0, pdf.length)).toEqual(pdf);
    const [v] = await verificarFirmas(bytes);
    expect(v).toMatchObject({ integra: true, cubreTodo: true });
    expect(v!.sello?.cubreFirma).toBe(true);
    expect(latin1(bytes)).toContain('(Firmado por EMPRESA DE PRUEBAS SL) Tj');

    // Al almacén (el servidor) solo le llega texto cifrado.
    const enClaro = ['%PDF-', 'ETSI.CAdES', 'EMPRESA DE PRUEBAS', 'Acta (firmado)', 'Firmado por'];
    expect(f.blobs.size).toBeGreaterThan(0);
    for (const cifrado of f.blobs.values())
      for (const x of enClaro) expect(latin1(cifrado)).not.toContain(x);
    for (const registro of f.records.values())
      for (const x of enClaro) expect(JSON.stringify(registro)).not.toContain(x);
  } finally {
    await repositorio.close();
    await f.close();
  }
});

test('sin autoridad de sellado, lo dice antes de firmar', async () => {
  selloConfigurado = false;
  const { f, repositorio, abrirDrive } = await bovedaConActa();
  try {
    act(() => {
      render(
        <EstudioBoveda
          path="/Contratos/Acta.pdf"
          locale="es"
          actor="owner"
          tenant="acme"
          abrirDrive={abrirDrive}
        />,
        host,
      );
    });
    act(() => boton(t('pdf.device.open'))!.click());
    await hasta(() => host.querySelector('[data-visor-boveda]') !== null);
    act(() => boton(t('pdf.menu.sign'))!.click());
    await hasta(() => document.querySelector('[data-sello="no"]') !== null);
    expect(document.body.textContent).toContain(t('pdf.device.stamp.off'));
    // La invisible sigue siendo la opción por defecto.
    expect(document.querySelector('[data-firma-visible]')).toBeNull();
  } finally {
    await repositorio.close();
    await f.close();
  }
});

test('una ruta que no está en la bóveda se dice con su estado vacío', async () => {
  const { f, repositorio, abrirDrive } = await bovedaConActa();
  try {
    act(() => {
      render(
        <EstudioBoveda
          path="/Contratos/Otro.pdf"
          locale="es"
          actor="owner"
          tenant="acme"
          abrirDrive={abrirDrive}
        />,
        host,
      );
    });
    act(() => boton(t('pdf.device.open'))!.click());
    await hasta(() => (host.textContent ?? '').includes(t('pdf.vault.notFound.title')));
    expect(host.textContent).toContain(t('pdf.vault.notFound.description'));
    expect(host.querySelector('[data-visor-boveda]')).toBeNull();
  } finally {
    await repositorio.close();
    await f.close();
  }
});
