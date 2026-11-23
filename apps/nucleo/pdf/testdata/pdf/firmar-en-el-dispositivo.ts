/*
 * SPDX-License-Identifier: Apache-2.0
 * SPDX-FileCopyrightText: (c) 2026 Daniel Delgado Ramos
 *
 * Los fixtures del firmador del NAVEGADOR (ADR 0086, «Firmar en el dispositivo»),
 * firmados con ESE código TypeScript (`packages/office/src/pdf/dispositivo`) y no con
 * el de Go: así el verificador de Go (`pdfverificar_corpus_test.go`) y Poppler
 * (`pdfsig`) contrastan una firma que no han escrito ellos.
 *
 *   firmado-en-el-dispositivo.pdf
 *     `firma-valida-flujos.pdf` (pyHanko) + una firma invisible B-B.
 *   firmado-en-el-dispositivo-visible-con-sello.pdf
 *     `sin-firmas.pdf` (LibreOffice) + una firma VISIBLE con rúbrica en la página 2 y
 *     SELLO DE TIEMPO (B-T) de una autoridad RFC 3161 de OpenSSL (`openssl ts -reply`):
 *     a la autoridad le llega solo la huella del valor de la firma, como en la app.
 *
 * Con el `.p12` de prueba `platform/pkg/pkcs12/testdata/clasico.p12` (3DES).
 *
 * USO (lo llama `generar-dispositivo.sh`):
 *   bun firmar-en-el-dispositivo.ts <tsa.cnf> <directorio de trabajo>
 */
import { leerPkcs12 } from '../../../../frontend/packages/office/src/pdf/dispositivo/pkcs12';
import {
  firmarPdf,
  firmarPdfConDetalle,
} from '../../../../frontend/packages/office/src/pdf/dispositivo/pades';
import { DocumentoPdf } from '../../../../frontend/packages/office/src/pdf/dispositivo/pdfObjetos';
import { rectFirma } from '../../../../frontend/packages/office/src/pdf/firmaPosicion';

const [tsaCnf, trabajo] = process.argv.slice(2);
if (!tsaCnf || !trabajo) throw new Error('uso: bun firmar-en-el-dispositivo.ts <tsa.cnf> <dir>');

const AQUI = new URL('./', import.meta.url);
const leer = async (u: URL) => new Uint8Array(await Bun.file(u).arrayBuffer());
const hex = (b: Uint8Array) => Array.from(b, (x) => x.toString(16).padStart(2, '0')).join('');

/** La autoridad: `openssl ts` con la huella (solo la huella) del valor de la firma. */
async function sellar(huella: Uint8Array): Promise<Uint8Array<ArrayBuffer>> {
  const tsq = `${trabajo}/peticion.tsq`;
  const tsr = `${trabajo}/token.der`;
  const pedir = Bun.spawnSync([
    'openssl',
    'ts',
    '-query',
    '-digest',
    hex(huella),
    '-sha256',
    '-cert',
    '-out',
    tsq,
  ]);
  if (pedir.exitCode !== 0) throw new Error(pedir.stderr.toString());
  const responder = Bun.spawnSync([
    'openssl',
    'ts',
    '-reply',
    '-config',
    tsaCnf!,
    '-queryfile',
    tsq,
    '-token_out',
    '-out',
    tsr,
  ]);
  if (responder.exitCode !== 0) throw new Error(responder.stderr.toString());
  return new Uint8Array(await Bun.file(tsr).arrayBuffer());
}

const identidad = await leerPkcs12(
  await leer(new URL('../../../../../platform/pkg/pkcs12/testdata/clasico.p12', AQUI)),
  'secreta',
);
const fecha = new Date('2026-11-23T10:00:00Z');

// 1. La invisible sin sello, sobre una firma de pyHanko.
const flujos = await leer(new URL('firma-valida-flujos.pdf', AQUI));
await Bun.write(
  new URL('firmado-en-el-dispositivo.pdf', AQUI),
  await firmarPdf(flujos, identidad, {
    motivo: 'Conforme',
    lugar: 'Bilbao',
    fecha,
  }),
);

// 2. La visible con rúbrica y sello, en la página 2 abajo a la derecha.
const base = await leer(new URL('sin-firmas.pdf', AQUI));
const paginas = await (await DocumentoPdf.abrir(base)).paginas();
const rect = rectFirma('abajoDerecha', paginas[1]!.caja);
const [x0, y0, x1, y1] = rect;
const w = x1 - x0;
const h = y1 - y0;
// Una rúbrica de dos trazos, en coordenadas de la página (como las manda el lienzo).
const trazos: [number, number][][] = [
  Array.from({ length: 24 }, (_, i): [number, number] => [
    x0 + w * (0.1 + (0.6 * i) / 23),
    y0 + h * (0.6 + 0.2 * Math.sin(i / 2)),
  ]),
  [
    [x0 + w * 0.15, y0 + h * 0.45],
    [x0 + w * 0.75, y0 + h * 0.5],
  ],
];
const r = await firmarPdfConDetalle(base, identidad, {
  motivo: 'Conforme',
  lugar: 'Bilbao',
  fecha,
  visible: { pagina: 2, rect, trazos },
  sello: sellar,
});
if (!r.sello) throw new Error('la firma salió sin sello');
await Bun.write(new URL('firmado-en-el-dispositivo-visible-con-sello.pdf', AQUI), r.datos);
console.log(`sellado el ${r.sello.toISOString()}`);
