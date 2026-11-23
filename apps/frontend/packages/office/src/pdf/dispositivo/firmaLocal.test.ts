/*
 * SPDX-License-Identifier: Apache-2.0
 * SPDX-FileCopyrightText: (c) 2026 Daniel Delgado Ramos
 *
 * «Firmar en el dispositivo» (ADR 0086): el `.p12` se abre aquí, el PDF se firma
 * anexando en PAdES y el firmado vuelve cifrado a la bóveda de Drive.
 *
 * Los PDF son los del corpus del verificador (`apps/nucleo/pdf/testdata/pdf`: uno de
 * LibreOffice con tabla clásica, otro con tabla y objetos en flujos, y otros ya
 * firmados por pyHanko) y los `.p12` los de `platform/pkg/pkcs12/testdata` —el clásico
 * en 3DES y el moderno en AES—, con su contraseña de prueba. El repositorio es el de
 * verdad (`PrivateDriveRepository`) sobre el almacén sintético de sus pruebas.
 *
 * Fuera de aquí se comprueba con verificadores AJENOS: `firmado-en-el-dispositivo.pdf`
 * del corpus de Go es `firma-valida-flujos.pdf` firmado con este código y el
 * `clasico.p12` (fecha 2026-11-23T10:00:00Z, motivo «Conforme», lugar «Bilbao»), y
 * `pdfverificar_corpus_test.go` lo da por válido; Poppler (`pdfsig`) también.
 */
import { expect, test } from 'bun:test';
import type { PrivateDriveRepository } from '@sytue/offline/matrix';
import { collectPrivateDriveFile } from '@sytue/offline/vault/drive';
import { privateDriveWriteFixture } from '../../../../offline/src/matrix/private-drive-fixture.js';
import { cifrarBloqueDES, descifrar3DESCBC } from './des';
import { ErrorPkcs12, leerPkcs12 } from './pkcs12';
import { DocumentoPdf, ErrorPdf } from './pdfObjetos';
import { firmarPdf, verificarFirmas } from './pades';
import { firmarEnBoveda, motivoFirmaLocal, nombreFirmado, pdfsDeBoveda } from './firmarEnBoveda';

const CORPUS = new URL('../../../../../../nucleo/pdf/testdata/pdf/', import.meta.url);
const P12 = new URL('../../../../../../../platform/pkg/pkcs12/testdata/', import.meta.url);
const CLAVE_P12 = 'secreta';

const leer = async (base: URL, nombre: string) =>
  new Uint8Array(await Bun.file(new URL(nombre, base)).arrayBuffer());

const hex = (s: string) => Uint8Array.from(s.match(/../g)!.map((b) => parseInt(b, 16)));
const latin1 = (b: Uint8Array) => Array.from(b, (c) => String.fromCharCode(c)).join('');
/** Cómo va un nombre de campo en el anexo: UTF-16BE con marca, en hexadecimal. */
const campoHex = (s: string) =>
  `/T <FEFF${Array.from(s, (c) => c.charCodeAt(0).toString(16).toUpperCase().padStart(4, '0')).join('')}>`;

test('DES da el vector conocido de FIPS 81, y 3DES-CBC con una clave repetida es DES-CBC', () => {
  // «Now is t» con 0123456789ABCDEF → 3FA40E8A984D4815 (FIPS 81, apéndice B).
  expect(cifrarBloqueDES(hex('0123456789ABCDEF'), hex('4E6F772069732074'))).toEqual(
    hex('3FA40E8A984D4815'),
  );
  // Con k1 = k2 = k3, EDE es DES: el bloque de arriba más un relleno completo.
  const k = hex('0123456789ABCDEF'.repeat(3));
  const iv = new Uint8Array(8);
  const relleno = cifrarBloqueDES(
    hex('0123456789ABCDEF'),
    hex('3FA40E8A984D4815').map((b) => b ^ 8),
  );
  expect(descifrar3DESCBC(k, iv, new Uint8Array([...hex('3FA40E8A984D4815'), ...relleno]))).toEqual(
    hex('4E6F772069732074'),
  );
  // Un relleno que no cuadra (la contraseña equivocada) no devuelve basura.
  expect(descifrar3DESCBC(k, iv, hex('3FA40E8A984D4815'))).toBeNull();
});

test('abre el .p12 clásico (3DES) y el moderno (AES); la contraseña mala se dice como tal', async () => {
  for (const f of ['clasico.p12', 'moderno.p12']) {
    const datos = await leer(P12, f);
    const id = await leerPkcs12(datos.slice(), CLAVE_P12);
    expect(id.cadena[0]!.nombre).toBe('EMPRESA DE PRUEBAS SL');
    expect(id.cadena[0]!.tipo).toBe('rsa');
    // La clave queda en WebCrypto y NO se puede sacar.
    expect(id.clave.extractable).toBe(false);
    expect(id.clave.type).toBe('private');
    const mala = await leerPkcs12(datos.slice(), 'otra').catch((e: unknown) => e);
    expect(mala).toBeInstanceOf(ErrorPkcs12);
    expect((mala as ErrorPkcs12).motivo).toBe('contrasena');
    expect(motivoFirmaLocal(mala)).toBe('contrasena');
  }
  const basura = await leerPkcs12(new TextEncoder().encode('no es un p12'), 'x').catch((e) => e);
  expect((basura as ErrorPkcs12).motivo).toBe('formato');
  expect(motivoFirmaLocal(basura)).toBe('certificado');
});

test('firma anexando sobre tabla clásica y sobre flujos; la segunda firma no rompe la primera', async () => {
  const id = await leerPkcs12(await leer(P12, 'moderno.p12'), CLAVE_P12);
  for (const f of ['sin-firmas.pdf', 'sin-firmas-flujos.pdf']) {
    const original = await leer(CORPUS, f);
    const firmado = await firmarPdf(original, id, {
      motivo: 'Conforme',
      lugar: 'Bilbao',
      fecha: new Date('2026-11-23T10:00:00Z'),
    });
    // NI UN BYTE de lo que había cambia: se anexa detrás.
    expect(firmado.subarray(0, original.length)).toEqual(original);
    const texto = latin1(firmado.subarray(original.length));
    expect(texto).toContain('/SubFilter /ETSI.CAdES.detached');
    expect(texto).toContain(campoHex('Sytue.Firma1'));
    // Tabla del mismo tipo que la del original.
    expect(texto.includes('/Type /XRef')).toBe(f.includes('flujos'));
    expect(await verificarFirmas(firmado)).toEqual([
      { firmante: 'EMPRESA DE PRUEBAS SL', integra: true, cubreTodo: true },
    ]);
    // El anexo se vuelve a leer como PDF y se puede firmar otra vez, con otro campo.
    await DocumentoPdf.abrir(firmado);
    const dos = await firmarPdf(firmado, id);
    expect(latin1(dos.subarray(firmado.length))).toContain(campoHex('Sytue.Firma2'));
    expect(await verificarFirmas(dos)).toEqual([
      { firmante: 'EMPRESA DE PRUEBAS SL', integra: true, cubreTodo: false },
      { firmante: 'EMPRESA DE PRUEBAS SL', integra: true, cubreTodo: true },
    ]);
  }
});

test('sobre un PDF ya firmado por otra herramienta, la firma anterior sigue verificando', async () => {
  const id = await leerPkcs12(await leer(P12, 'clasico.p12'), CLAVE_P12);
  const ajeno = await leer(CORPUS, 'firma-valida-rsa.pdf');
  const firmado = await firmarPdf(ajeno, id);
  expect(await verificarFirmas(firmado)).toEqual([
    { firmante: 'Ana Pérez', integra: true, cubreTodo: false },
    { firmante: 'EMPRESA DE PRUEBAS SL', integra: true, cubreTodo: true },
  ]);
  // Un byte cambiado en lo firmado se nota.
  const tocado = firmado.slice();
  const i = latin1(tocado).indexOf('/Producer');
  tocado[i + 2] = tocado[i + 2]! ^ 1;
  expect((await verificarFirmas(tocado)).map((f) => f.integra)).toEqual([false, false]);
});

test('lo que no se puede firmar anexando se rechaza con su motivo, sin escribir nada', async () => {
  const id = await leerPkcs12(await leer(P12, 'moderno.p12'), CLAVE_P12);
  // Certificado con /DocMDP P=1: no admite ningún cambio.
  const certificado = await firmarPdf(await leer(CORPUS, 'certificacion-p1.pdf'), id).catch(
    (e: unknown) => e,
  );
  expect(certificado).toBeInstanceOf(ErrorPdf);
  expect(motivoFirmaLocal(certificado)).toBe('bloqueado');
  // Protegido con contraseña.
  const cuerpo = '%PDF-1.7\n1 0 obj\n<</Type /Catalog>>\nendobj\n';
  const cifrado = new TextEncoder().encode(
    `${cuerpo}xref\n0 2\n0000000000 65535 f \n0000000009 00000 n \ntrailer\n<</Size 2 /Root 1 0 R /Encrypt <</Filter /Standard>>>>\nstartxref\n${cuerpo.length}\n%%EOF\n`,
  );
  expect(motivoFirmaLocal(await firmarPdf(cifrado, id).catch((e: unknown) => e))).toBe('cifrado');
  expect(
    motivoFirmaLocal(
      await firmarPdf(new TextEncoder().encode('no es un pdf'), id).catch((e: unknown) => e),
    ),
  ).toBe('ilegible');
});

test('el nombre del firmado no pisa nada', () => {
  expect(nombreFirmado('Acta.pdf', 'firmado')).toBe('Acta (firmado).pdf');
  expect(nombreFirmado('Acta.PDF', 'signed')).toBe('Acta (signed).pdf');
  expect(nombreFirmado('sin extensión', 'firmado')).toBe('sin extensión (firmado).pdf');
});

test('en la bóveda: lista los PDF, firma en el dispositivo y deja el firmado cifrado al lado', async () => {
  const id = await leerPkcs12(await leer(P12, 'moderno.p12'), CLAVE_P12);
  const pdf = await leer(CORPUS, 'sin-firmas.pdf');
  const f = await privateDriveWriteFixture(),
    repositorio: PrivateDriveRepository = f.open();
  try {
    await repositorio.initialize(f.state);
    const raiz = String((await repositorio.root()).id);
    const carpeta = String((await repositorio.createFolder(raiz, 'Contratos')).id);
    const fila = await repositorio.upload(
      carpeta,
      'Acta.pdf',
      'application/pdf',
      new Blob([pdf], { type: 'application/pdf' }),
    );
    await repositorio.upload(carpeta, 'notas.txt', 'text/plain', new Blob(['hola']));
    const lista = await pdfsDeBoveda(repositorio);
    expect(lista.map((p) => p.nombre)).toEqual(['Acta.pdf']);
    expect(lista[0]!.ruta).toContain('Contratos');

    const r = await firmarEnBoveda({
      repositorio,
      id: String(fila.id),
      identidad: id,
      opciones: { motivo: 'Conforme' },
      sufijo: 'firmado',
    });
    expect(r.nombre).toBe('Acta (firmado).pdf');
    // Otra vez el mismo original: otro fichero, sin pisar el primero.
    const otra = await firmarEnBoveda({
      repositorio,
      id: String(fila.id),
      identidad: id,
      sufijo: 'firmado',
    });
    expect(otra.nombre).toBe('Acta (firmado) (2).pdf');

    const bajar = async (fid: string) => {
      const meta = await repositorio.file(fid);
      const blob = await collectPrivateDriveFile({
        size: meta.size_bytes,
        maximum: 1 << 24,
        type: 'application/pdf',
        run: (sink) => repositorio.download(fid, sink),
      });
      return new Uint8Array(await blob.arrayBuffer());
    };
    // El original sigue igual y el firmado verifica.
    expect(await bajar(String(fila.id))).toEqual(pdf);
    const firmado = await bajar(r.id);
    expect(firmado.subarray(0, pdf.length)).toEqual(pdf);
    expect(await verificarFirmas(firmado)).toEqual([
      { firmante: 'EMPRESA DE PRUEBAS SL', integra: true, cubreTodo: true },
    ]);

    // Al almacén (el servidor) solo le llega texto cifrado: ni el PDF, ni la firma,
    // ni el nombre de quien firma.
    const enClaro = ['%PDF-', 'ETSI.CAdES', 'EMPRESA DE PRUEBAS', 'Acta (firmado)'];
    expect(f.blobs.size).toBeGreaterThan(0);
    for (const cifrado of f.blobs.values()) {
      const t = latin1(cifrado);
      for (const x of enClaro) expect(t).not.toContain(x);
    }
    for (const registro of f.records.values()) {
      const t = JSON.stringify(registro);
      for (const x of enClaro) expect(t).not.toContain(x);
    }
  } finally {
    await repositorio.close();
    await f.close();
  }
});

test('un fallo de la bóveda se cuenta como tal', async () => {
  const { PqVaultError } = await import('@sytue/offline/vault');
  const { PrivateDriveError } = await import('@sytue/offline/matrix');
  expect(motivoFirmaLocal(new PqVaultError('signin'))).toBe('entrar');
  expect(motivoFirmaLocal(new PrivateDriveError('not_ready'))).toBe('pendiente');
  expect(motivoFirmaLocal(new Error('red'))).toBe('fallo');
});
