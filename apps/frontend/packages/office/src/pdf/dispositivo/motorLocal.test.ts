/*
 * SPDX-License-Identifier: Apache-2.0
 * SPDX-FileCopyrightText: (c) 2026 Daniel Delgado Ramos
 *
 * EL MOTOR DE PDF EN EL DISPOSITIVO (ADR 0086, tercera entrega): lo que el estudio de la
 * bóveda le pide al módulo `pdf` (`cmd/pdf-wasm`) y cómo traduce el «aplicar» del
 * estudio a sus operaciones. El motor en sí (Go) tiene sus pruebas en
 * `apps/nucleo/pdf/device_session_test.go`; aquí, el contrato de mensajes y la tabla.
 */
import { expect, test } from 'bun:test';
import { clientCoreModuleBounds, isClientCoreModule } from '@sytue/config/client-core';
import { ErrorPDF, invalidaFirmas } from '../api';
import { mensajeDeError } from '../errores';
import { docsT } from '../../i18n';
import { motorSobre, type EjecutorModulo, type MotorPdf } from './motorPdf';
import { aplicarEnDispositivo, limpiarSufijo, seHaceEnElDispositivo } from './aplicarLocal';

const t = docsT('es');
const bytes = (s: string) => new TextEncoder().encode(s);
const texto = (b: unknown) => new TextDecoder().decode(b as Uint8Array);

/** Un ejecutor que apunta lo que recibe y contesta lo que se le diga. */
function ejecutor(responder: (r: Record<string, unknown>) => unknown) {
  const pedido: Record<string, unknown>[] = [];
  let cerrado = false;
  const e: EjecutorModulo = {
    invoke: <T>(r: Record<string, unknown>) => {
      pedido.push(r);
      return Promise.resolve(responder(r) as T);
    },
    close: () => {
      cerrado = true;
    },
  };
  return { e, pedido, cerrado: () => cerrado };
}

test('el módulo `pdf` es del núcleo de cliente, con su tope', () => {
  expect(isClientCoreModule('pdf')).toBe(true);
  expect(clientCoreModuleBounds.pdf).toBe(32 << 20);
});

test('el motor manda bytes (no cadenas) y una COPIA del PDF; los errores son ErrorPDF', async () => {
  const contenido = { total: 2, paginas: [] };
  const { e, pedido, cerrado } = ejecutor((r) => {
    switch (r.operation) {
      case 'pdf.abrir':
        return { doc: 'd1', contenido: bytes(JSON.stringify(contenido)) };
      case 'pdf.render':
        return { body: bytes(JSON.stringify({ paginas: { '1': '<svg/>', '2': '<svg/>' } })) };
      case 'pdf.paginas':
        return {
          error: {
            codigo: 'pdf_invalidates_signatures',
            mensaje: 'Saving this change would invalidate the signatures',
          },
        };
      default:
        return { body: bytes('%PDF-x'), content_type: 'application/pdf', pdfa: 'perdido' };
    }
  });
  const motor = motorSobre(e);
  const pdf = bytes('%PDF-1.7 original');
  const abierto = await motor.abrir(pdf);
  expect(abierto).toEqual({ doc: 'd1', contenido: contenido as never });
  // El worker borra lo que recibe: lo que se manda no es el array de quien llama.
  expect(pedido[0]!.pdf).not.toBe(pdf);
  expect(pedido[0]!.pdf).toEqual(pdf);

  expect(await motor.render('d1', '1-2', 1)).toEqual({ 1: '<svg/>', 2: '<svg/>' });
  expect(pedido[1]!.campos).toBeInstanceOf(Uint8Array);
  expect(JSON.parse(texto(pedido[1]!.campos))).toEqual({ paginas: '1-2', escala: '1' });

  const anotado = await motor.anotar('d1', { op: 'poner', anotaciones: '[]' }, 'Ana');
  expect(anotado).toMatchObject({ tipo: 'application/pdf', pdfa: 'perdido' });
  expect(texto(anotado.datos)).toBe('%PDF-x');
  expect(pedido[2]).toMatchObject({ operation: 'pdf.anotar', doc: 'd1', autor: 'Ana' });

  const fallo = await motor.paginas(['d1'], { op: 'rotar' }).catch((x: unknown) => x);
  expect(fallo).toBeInstanceOf(ErrorPDF);
  expect(invalidaFirmas(fallo)).toBe(true);
  expect(pedido[3]!.docs).toEqual(['d1']);

  motor.apagar();
  expect(cerrado()).toBe(true);
});

/** Un motor que apunta qué se le pidió, sin wasm. */
function motorQueApunta() {
  const llamadas: { que: string; docs: string[]; campos: Record<string, string> }[] = [];
  const abiertos: string[] = [];
  const cerrados: string[] = [];
  const edicion = { datos: bytes('%PDF-nuevo'), tipo: 'application/pdf' };
  const motor: MotorPdf = {
    abrir: () => {
      const doc = `o${abiertos.length + 1}`;
      abiertos.push(doc);
      return Promise.resolve({ doc, contenido: {} as never });
    },
    render: () => Promise.resolve({}),
    anotar: (doc, campos) => (
      llamadas.push({ que: 'anotar', docs: [doc], campos }),
      Promise.resolve(edicion)
    ),
    formulario: (doc, campos) => (
      llamadas.push({ que: 'formulario', docs: [doc], campos }),
      Promise.resolve(edicion)
    ),
    paginas: (docs, campos) => (
      llamadas.push({ que: 'paginas', docs, campos }),
      Promise.resolve(edicion)
    ),
    herramienta: (doc, campos) => (
      llamadas.push({ que: 'herramienta', docs: [doc], campos }),
      Promise.resolve(edicion)
    ),
    cerrar: (doc) => (cerrados.push(doc), Promise.resolve()),
    apagar: () => undefined,
  };
  return { motor, llamadas, abiertos, cerrados };
}

test('aplicar en el dispositivo es la tabla del BFF: operación, campo de datos y copia', async () => {
  const { motor, llamadas, cerrados } = motorQueApunta();
  const base = { motor, doc: 'd1', nombre: 'Acta.pdf', autor: 'Ana' };
  const abrirOtro = (ruta: string) => motor.abrir(bytes(ruta)).then((a) => a.doc);

  const anotado = await aplicarEnDispositivo({
    ...base,
    abrirOtro,
    cuerpo: { op: 'anotar', datos: [{ pagina: 0, tipo: 'nota' }] },
  });
  expect(anotado.copia).toBe(false);
  expect(llamadas[0]).toEqual({
    que: 'anotar',
    docs: ['d1'],
    campos: { op: 'poner', anotaciones: '[{"pagina":0,"tipo":"nota"}]' },
  });

  const relleno = await aplicarEnDispositivo({
    ...base,
    abrirOtro,
    cuerpo: { op: 'rellenar', datos: { nombre: 'Ana' } },
  });
  expect(relleno.copia).toBe(false);
  expect(llamadas[1]!.campos).toEqual({ op: 'rellenar', valores: '{"nombre":"Ana"}' });

  // Organizar deja SIEMPRE copia, con la palabra acotada como en el BFF.
  const girado = await aplicarEnDispositivo({
    ...base,
    abrirOtro,
    cuerpo: {
      op: 'paginas',
      campos: { op: 'rotar', paginas: '1', grados: 90 },
      sufijo: 'gi/ra..do',
    },
  });
  expect(girado).toMatchObject({ copia: true, sufijo: 'girado' });
  expect(llamadas[2]).toEqual({
    que: 'paginas',
    docs: ['d1'],
    campos: { op: 'rotar', paginas: '1', grados: '90' },
  });

  // Combinar abre los otros de la bóveda, en su orden, y los cierra al acabar.
  await aplicarEnDispositivo({
    ...base,
    abrirOtro,
    cuerpo: { op: 'paginas', campos: { op: 'combinar' }, rutas: ['/A.pdf', ' ', '/B.pdf'] },
  });
  expect(llamadas[3]!.docs).toEqual(['d1', 'o1', 'o2']);
  expect(cerrados).toEqual(['o1', 'o2']);

  await aplicarEnDispositivo({
    ...base,
    abrirOtro,
    cuerpo: { op: 'comprimir', campos: { nivel: 'fuerte', invalidar_firmas: '1' } },
  });
  expect(llamadas[4]!.campos).toEqual({ op: 'comprimir', nivel: 'fuerte', invalidar_firmas: '1' });

  expect(limpiarSufijo('  a b/../c\n')).toBe('a bc');
});

test('lo que no se hace en el dispositivo se dice, sin llamar al motor', async () => {
  const { motor, llamadas } = motorQueApunta();
  for (const op of [
    'proteger',
    'redactar',
    'reconocer-texto',
    'firmar-certificado',
    'capas',
    'x',
  ]) {
    expect(seHaceEnElDispositivo(op)).toBe(false);
    const e = await aplicarEnDispositivo({
      motor,
      doc: 'd1',
      nombre: 'Acta.pdf',
      autor: 'Ana',
      abrirOtro: () => Promise.reject(new Error('no')),
      cuerpo: { op },
    }).catch((x: unknown) => x);
    expect(e).toBeInstanceOf(ErrorPDF);
    expect(mensajeDeError(e, t, 'pdf.error.apply')).toBe(t('pdf.vault.unavailable'));
  }
  expect(llamadas).toHaveLength(0);
});

test('el motor se carga solo en el estudio de la bóveda, y solo al abrir', async () => {
  const fuente = (f: string) => Bun.file(new URL(f, import.meta.url)).text();
  const motorPdf = await fuente('./motorPdf.ts');
  // El runtime (worker y wasm) entra por `import()` dinámico, no al cargar la página.
  expect(motorPdf).toContain("await import('@sytue/offline/pqc/runtime')");
  expect(motorPdf).not.toMatch(/^import .*@sytue\/offline\/pqc\/runtime/m);
  const estudio = await fuente('../EstudioBoveda.tsx');
  expect(estudio).not.toContain('@sytue/offline/pqc/runtime');
  // El estudio del servidor ni lo conoce.
  for (const f of ['../PdfApp.tsx', '../Paneles.tsx', '../Barra.tsx'])
    expect(await fuente(f)).not.toContain('motorPdf');
});
