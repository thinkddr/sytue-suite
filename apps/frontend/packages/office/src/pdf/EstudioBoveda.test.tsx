/*
 * SPDX-License-Identifier: Apache-2.0
 * SPDX-FileCopyrightText: (c) 2026 Daniel Delgado Ramos
 *
 * EL ESTUDIO DE UN PDF DE LA BÓVEDA (ADR 0086): `/pdf?path=` con el Drive en la bóveda
 * abre el fichero de la bóveda descifrado EN MEMORIA, para verlo, editarlo y firmarlo en
 * el dispositivo, con el repositorio de verdad (`PrivateDriveRepository`) sobre el
 * almacén sintético de sus pruebas. Lo que se fija: que la bóveda no se abre sola ni el
 * motor se carga antes de abrir; que lo que necesita al servicio sale apagado y
 * explicado; que las páginas las dibuja el motor del dispositivo en el lienzo del
 * estudio; que anotar, rellenar y organizar dejan versión nueva o fichero nuevo CIFRADOS
 * en la bóveda; que ningún cuerpo de ninguna petición lleva el PDF (ni al firmar con
 * sello, donde al BFF solo le llega la huella de 32 bytes); que al almacén solo le llega
 * texto cifrado; y que sin motor se cae al visor del navegador.
 *
 * El motor aquí es uno FALSO (`motorFalso`): el de verdad es Go compilado a wasm y tiene
 * sus pruebas en `apps/nucleo/pdf/device_session_test.go`; lo que se prueba aquí es el
 * estudio que lo usa.
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
import { DocumentoPdf } from './dispositivo/pdfObjetos';
import type { EdicionLocal, MotorPdf } from './dispositivo/motorPdf';
import type { Contenido } from './api';
import { EstudioBoveda, EstudioPdf, type CrearMotor } from './EstudioBoveda';

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

/** El cuerpo de una petición, sea del tipo que sea, en latin1 (para buscar `%PDF`). */
async function cuerpoDe(body: BodyInit | null | undefined): Promise<string> {
  if (body === undefined || body === null) return '';
  if (typeof body === 'string') return body;
  return latin1(new Uint8Array(await new Response(body).arrayBuffer()));
}

/**
 * Un motor FALSO con el contrato de `MotorPdf`: lee las páginas con el lector del
 * firmador (`DocumentoPdf`), dibuja un SVG por página y «edita» añadiendo al final un
 * comentario con la operación y sus campos, para poder comprobar qué llegó a la bóveda.
 */
function motorFalso(registro: { abierto: number; llamadas: string[] }): CrearMotor {
  return async () => {
    const docs = new Map<string, Uint8Array>();
    let n = 0;
    const editar = (doc: string, que: string, campos: Record<string, string>): EdicionLocal => {
      registro.llamadas.push(`${que} ${JSON.stringify(campos)}`);
      const marca = new TextEncoder().encode(`\n%${que} ${JSON.stringify(campos)}\n`);
      const antes = docs.get(doc)!;
      const datos = new Uint8Array(antes.length + marca.length);
      datos.set(antes);
      datos.set(marca, antes.length);
      return { datos, tipo: 'application/pdf' };
    };
    const motor: MotorPdf = {
      async abrir(pdf) {
        registro.abierto++;
        const doc = `f${++n}`;
        docs.set(doc, pdf.slice());
        const paginas = await (await DocumentoPdf.abrir(pdf)).paginas();
        const relleno = /%formulario \{[^\n]*"valores":"(\{[^\n]*\})"/.exec(latin1(pdf));
        const valor = relleno
          ? ((JSON.parse(JSON.parse(`"${relleno[1]}"`) as string) as Record<string, string>)
              .nombre ?? '')
          : '';
        const contenido = {
          paginas: paginas.map((p, i) => ({
            numero: i + 1,
            ancho: p.caja[2] - p.caja[0],
            alto: p.caja[3] - p.caja[1],
            rotacion: 0,
            caja: p.caja,
          })),
          total: paginas.length,
          indice: [],
          enlaces: [],
          adjuntos: [],
          anotaciones: [],
          campos: [
            {
              nombre: 'nombre',
              tipo: 'texto',
              valor,
              pagina: 0,
              rect: [10, 10, 100, 30],
              obligatorio: false,
              solo_lectura: false,
            },
          ],
          firmas: [],
          cifrado: false,
          permisos: {
            Imprimir: true,
            Modificar: true,
            Copiar: true,
            Anotar: true,
            RellenarCampos: true,
            Accesibilidad: true,
            Ensamblar: true,
            ImprimirEnAlta: true,
            SoloConClaveDeDueno: false,
          },
          huella: {},
        } as unknown as Contenido;
        return { doc, contenido };
      },
      async render(_doc, paginas) {
        const [a, b] = paginas.split('-').map(Number);
        const out: Record<number, string> = {};
        for (let i = a!; i <= (b ?? a)!; i++)
          out[i] =
            `<svg xmlns="http://www.w3.org/2000/svg" data-pagina-motor="${i}"><text>página ${i}</text></svg>`;
        return out;
      },
      anotar: async (doc, campos) => editar(doc, 'anotar', campos),
      formulario: async (doc, campos) => editar(doc, 'formulario', campos),
      paginas: async (ds, campos) => editar(ds[0]!, `paginas-${ds.length}`, campos),
      herramienta: async (doc, campos) => editar(doc, 'herramienta', campos),
      cerrar: async (doc) => {
        docs.delete(doc);
      },
      apagar: () => undefined,
    };
    return motor;
  };
}
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
    enviado.push({ url, cuerpo: await cuerpoDe(init?.body) });
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

test('abre el PDF de la bóveda en memoria, lo dibuja el motor del dispositivo, lo firma con sello y nada en claro sale', async () => {
  const { f, repositorio, pdf, abrirDrive, abiertas } = await bovedaConActa();
  const motor = { abierto: 0, llamadas: [] as string[] };
  let motores = 0;
  const crear = motorFalso(motor);
  try {
    act(() => {
      render(
        <EstudioBoveda
          path="/Contratos/Acta.pdf"
          locale="es"
          actor="owner"
          tenant="acme"
          abrirDrive={abrirDrive}
          crearMotor={(signal) => {
            motores++;
            return crear(signal);
          }}
        />,
        host,
      );
    });
    await act(() => tick(10));
    // Ni la bóveda ni el motor se abren solos.
    expect(abiertas()).toBe(0);
    expect(motores).toBe(0);
    act(() => boton(t('pdf.device.open'))!.click());
    // Las dos páginas, dibujadas por el motor en el lienzo del estudio (no un iframe).
    await hasta(() => host.querySelectorAll('[data-pagina-motor]').length === 2);
    expect(motores).toBe(1);
    expect(host.querySelector('[data-lienzo-boveda]')).not.toBeNull();
    expect(host.querySelector('[data-visor-boveda]')).toBeNull();
    expect(host.querySelector('[data-pagina="2"] [data-pagina-motor="2"]')).not.toBeNull();
    // Lo que aquí no se hace, apagado y con su motivo en la cinta.
    const pestana = (texto: string) =>
      [...host.querySelectorAll<HTMLButtonElement>('[role="tab"]')].find(
        (b) => b.textContent?.trim() === texto,
      )!;
    act(() => pestana(t('pdf.menu.sign')).click());
    const pedir = boton(t('pdf.menu.requestSignature'))!;
    expect(pedir.disabled).toBe(true);
    expect(pedir.getAttribute('title')).toBe(t('pdf.vault.unavailable'));
    act(() => pestana(t('pdf.menu.organize')).click());
    const descarga = [...host.querySelectorAll('a')].find(
      (a) => a.textContent === t('pdf.menu.download'),
    )!;
    expect(descarga.getAttribute('download')).toBe('Acta.pdf');
    expect(descarga.getAttribute('href')).toStartWith('blob:');

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

test('sin motor, el visor del navegador (verlo, bajarlo y firmarlo); y sin autoridad de sellado, lo dice antes de firmar', async () => {
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
          crearMotor={() => Promise.reject(new Error('crypto_runtime_unavailable'))}
        />,
        host,
      );
    });
    act(() => boton(t('pdf.device.open'))!.click());
    await hasta(() => host.querySelector('[data-visor-boveda]') !== null);
    const visor = host.querySelector<HTMLIFrameElement>('[data-visor-boveda]')!;
    expect(visor.getAttribute('src')).toStartWith('blob:');
    expect(visor.getAttribute('title')).toBe('Acta.pdf');
    expect(host.textContent).toContain(t('pdf.vault.engineUnavailable'));
    expect(host.querySelector('[data-lienzo-boveda]')).toBeNull();
    // Sin motor no se organiza: apagado y explicado, como antes.
    const apagados = [...host.querySelectorAll<HTMLButtonElement>('[data-solo-servidor] button')];
    expect(apagados.map((b) => b.textContent)).toContain(t('pdf.menu.organize'));
    expect(apagados.every((b) => b.disabled)).toBe(true);
    const descarga = [...host.querySelectorAll('a')].find(
      (a) => a.textContent === t('pdf.menu.download'),
    )!;
    expect(descarga.getAttribute('download')).toBe('Acta.pdf');
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

test('anota, rellena y organiza en el dispositivo: versión nueva o fichero nuevo, cifrados, y el PDF no sale', async () => {
  const { f, repositorio, pdf, abrirDrive } = await bovedaConActa();
  const motor = { abierto: 0, llamadas: [] as string[] };
  try {
    act(() => {
      render(
        <EstudioBoveda
          path="/Contratos/Acta.pdf"
          locale="es"
          actor="owner"
          tenant="acme"
          autor="Ana"
          abrirDrive={abrirDrive}
          crearMotor={motorFalso(motor)}
        />,
        host,
      );
    });
    act(() => boton(t('pdf.device.open'))!.click());
    await hasta(() => host.querySelectorAll('[data-pagina-motor]').length === 2);
    const carpeta = String(
      (await repositorio.children(String((await repositorio.root()).id))).folders[0]!.id,
    );
    const acta = async () =>
      (await repositorio.children(carpeta)).files.find((x) => x.name === 'Acta.pdf')!;
    const { collectPrivateDriveFile } = await import('@sytue/offline/vault/drive');
    const leerDeBoveda = async (fila: Record<string, unknown>) =>
      latin1(
        new Uint8Array(
          await (
            await collectPrivateDriveFile({
              size: fila.size_bytes,
              maximum: 1 << 24,
              type: 'application/pdf',
              run: (sink) => repositorio.download(String(fila.id), sink),
            })
          ).arrayBuffer(),
        ),
      );
    const id = String((await acta()).id);

    // RELLENAR: el panel de campos del estudio; versión nueva del MISMO fichero.
    const campo = host.querySelector<HTMLInputElement>('#campo-nombre')!;
    act(() => {
      campo.value = 'Ana Perez';
      campo.dispatchEvent(new Event('input', { bubbles: true }));
    });
    act(() => boton(t('pdf.fields.save'))!.click());
    await hasta(() => (host.textContent ?? '').includes(t('pdf.saved.filled')));
    expect(String((await acta()).id)).toBe(id);
    expect(await leerDeBoveda(await acta())).toContain(
      '%formulario {"op":"rellenar","valores":"{\\"nombre\\":\\"Ana Perez\\"}"}',
    );
    expect((await repositorio.versions(id)).versions.length).toBeGreaterThanOrEqual(2);
    // Y el estudio vuelve a abrir lo guardado: el campo ya trae el valor.
    await hasta(() => host.querySelector<HTMLInputElement>('#campo-nombre')?.value === 'Ana Perez');
    expect(host.querySelector<HTMLInputElement>('#campo-nombre')?.value).toBe('Ana Perez');

    // ANOTAR: un rectángulo con la herramienta de la cinta, sobre la página 1.
    const pestana = (texto: string) =>
      [...host.querySelectorAll<HTMLButtonElement>('[role="tab"]')].find(
        (b) => b.textContent?.trim() === texto,
      )!;
    act(() => pestana(t('pdf.ribbon.tab.annotate')).click());
    act(() =>
      host.querySelector<HTMLButtonElement>(`[aria-label="${t('pdf.tool.rect')}"]`)!.click(),
    );
    const capa = host.querySelector<SVGSVGElement>('[data-pagina="1"] svg[aria-hidden="true"]')!;
    const puntero = (tipo: string, x: number, y: number) =>
      new PointerEvent(tipo, {
        bubbles: true,
        clientX: x,
        clientY: y,
        pointerId: 7,
        isPrimary: true,
        button: 0,
        pointerType: 'mouse',
      });
    act(() => {
      capa.dispatchEvent(puntero('pointerdown', 10, 10));
    });
    await act(async () => {
      capa.dispatchEvent(puntero('pointermove', 80, 60));
      await tick(40);
    });
    act(() => {
      capa.dispatchEvent(puntero('pointerup', 80, 60));
    });
    await hasta(() => motor.llamadas.some((l) => l.startsWith('anotar')));
    const anotado = motor.llamadas.find((l) => l.startsWith('anotar'))!;
    expect(anotado).toContain('"op":"poner"');
    expect(anotado).toContain('\\"tipo\\":\\"rectangulo\\"');
    expect(anotado).toContain('\\"autor\\":\\"Ana\\"');
    await hasta(() => (host.textContent ?? '').includes(t('pdf.saved.annotated')));
    expect(await leerDeBoveda(await acta())).toContain('%anotar {"op":"poner"');

    // ORGANIZAR: el diálogo del estudio; girar deja un fichero NUEVO a su lado.
    act(() => pestana(t('pdf.menu.organize')).click());
    const organizar = [...host.querySelectorAll('button')].filter(
      (b) => b.textContent?.trim() === t('pdf.menu.organize') && b.getAttribute('role') !== 'tab',
    );
    act(() => organizar[0]!.click());
    await hasta(() => document.querySelector('#pdf-herramienta') !== null);
    const girar = [...document.querySelectorAll('label')].find(
      (l) => l.textContent?.trim() === t('pdf.op.rotar'),
    )!;
    act(() => girar.querySelector('input')!.click());
    const paginas = document.querySelector<HTMLInputElement>(
      '#pdf-herramienta input[placeholder="1,3,5-8"]',
    )!;
    act(() => {
      paginas.value = '1';
      paginas.dispatchEvent(new Event('input', { bubbles: true }));
    });
    await act(async () => {
      document.querySelector<HTMLFormElement>('#pdf-herramienta')!.requestSubmit();
      await tick(10);
    });
    await hasta(() =>
      (host.textContent ?? '').includes(t('pdf.saved.copy', { name: 'Acta (girado).pdf' })),
    );
    const girado = (await repositorio.children(carpeta)).files.find(
      (x) => x.name === 'Acta (girado).pdf',
    )!;
    expect(await leerDeBoveda(girado)).toContain(
      '%paginas-1 {"op":"rotar","paginas":"1","grados":"90"}',
    );
    const abrirCopia = [...host.querySelectorAll('a')].find(
      (a) => a.textContent === t('pdf.saved.open'),
    )!;
    expect(decodeURIComponent(abrirCopia.getAttribute('href') ?? '')).toContain(
      '/pdf?path=/Contratos/Acta (girado).pdf',
    );
    // El original sigue con su nombre: girar no lo tocó.
    expect(await leerDeBoveda(await acta())).not.toContain('%paginas');

    // NADA de esto fue por la red: ni el PDF ni lo que se le hizo, y ni una llamada al
    // estudio del servidor.
    expect(enviado.some((e) => /\/api\/pdf(-aplicar)?$/.test(e.url))).toBe(false);
    for (const e of enviado) {
      expect(e.cuerpo).not.toContain('%PDF');
      expect(e.cuerpo).not.toContain(latin1(pdf.subarray(0, 64)));
      expect(e.cuerpo).not.toContain('Ana Perez');
    }
    // Y al almacén, solo cifrado.
    const enClaro = ['%PDF-', '%formulario', '%anotar', 'Ana Perez', 'Acta (girado)', 'rectangulo'];
    for (const cifrado of f.blobs.values())
      for (const x of enClaro) expect(latin1(cifrado)).not.toContain(x);
    for (const registro of f.records.values())
      for (const x of enClaro) expect(JSON.stringify(registro)).not.toContain(x);
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
