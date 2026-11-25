/*
 * SPDX-License-Identifier: Apache-2.0
 * SPDX-FileCopyrightText: (c) 2026 Daniel Delgado Ramos
 *
 * FIRMA PAdES EN EL DISPOSITIVO, para quien tiene su Drive en la bóveda (ADR 0086):
 * el PDF se descifra en memoria, se firma aquí con el certificado de la persona y el
 * resultado vuelve cifrado a la bóveda. Ni el PDF ni el certificado llegan a ningún
 * servidor.
 *
 * Es el MISMO modelo que firma en el servidor y en nativo (`apps/nucleo/pdf`,
 * `FirmarIncrementalPDF` y `construirCMS`), escrito con WebCrypto:
 *
 * - se ANEXA (§7.5.6) sin tocar un byte de lo que había, así que las firmas
 *   anteriores siguen verificando: dos partes pueden firmar el mismo contrato;
 * - /SubFilter /ETSI.CAdES.detached y los tres atributos firmados de la línea base
 *   B-B (content-type, message-digest y signing-certificate-v2), sin signing-time
 *   —PAdES lo prohíbe: la hora declarada va en /M—, con SHA-256 y RSA PKCS#1 v1.5 o
 *   ECDSA;
 * - el mismo hueco de 24 KiB, el mismo nombre de campo libre («Sytue.FirmaN») y la
 *   misma apariencia vacía de la firma invisible (PDF/A exige una);
 * - con SELLO DE TIEMPO si el despliegue tiene autoridad (B-T, `sello.ts`): se manda
 *   a sellar SOLO el SHA-256 del valor de la firma y el token va como atributo no
 *   firmado. Si la autoridad no responde se firma igual sin sello (B-B), como en el
 *   servidor, y quien firma lo sabe;
 * - firma VISIBLE opcional, con la MISMA apariencia que dibuja el servidor
 *   (`objetoAparienciaFirma`): el recuadro, la rúbrica dibujada si la hay, «Firmado
 *   por …» y la fecha, en Helvetica estándar (sin incrustar fuentes);
 * - CERTIFICA si se pide («bloquear el documento después de firmar»): /DocMDP con
 *   /P 1 en la firma y /Perms en el catálogo, como `dictDeFirma` y `anexarFirma` en
 *   Go, y solo en la PRIMERA firma (`certificarFirmado` si ya hay otra).
 *
 * LO QUE ESTE CAMINO NO HACE, y por qué (ADR 0086, «Firmar en el dispositivo»):
 *
 * - No firma un PDF protegido con contraseña ni uno con la tabla de referencias rota:
 *   habría que reescribir el fichero entero. Con contraseña, el estudio de la bóveda
 *   lo reescribe ANTES sin ella con el motor del dispositivo (como el servidor, que
 *   firma reescribiendo) y entonces lo firma aquí (`FirmarEnDispositivo.tsx`).
 */
import {
  ETIQUETA,
  type Nodo,
  cadenaOctetos,
  comparar,
  concatenar,
  conjunto,
  contexto,
  enteroDER,
  enteroSinSigno,
  hijo,
  hijos,
  leerNodo,
  nulo,
  oid,
  oidDER,
  secuencia,
  tlv,
} from './der';
import { type Certificado, type Identidad, leerCertificado } from './pkcs12';
import { OID_SELLO, type PedirSello, comprobarSello, leerSello } from './sello';
import {
  type Dict,
  type Obj,
  DocumentoPdf,
  ErrorPdf,
  Lector,
  aHex,
  buscar,
  entero,
  esCadena,
  esDict,
  esNombre,
  esRef,
  leerTextoPDF,
  nombre,
  num,
  ref,
  serializar,
  textoPDF,
} from './pdfObjetos';

/** El hueco del CMS: el mismo que en el servidor (`tamanoHuecoFirma`). */
export const TAMANO_HUECO = 24576;
export const SUBFILTRO = 'ETSI.CAdES.detached';
const MARCA_RANGO = '9999999999';

const OID = {
  datos: '1.2.840.113549.1.7.1',
  firmado: '1.2.840.113549.1.7.2',
  tipoContenido: '1.2.840.113549.1.9.3',
  resumen: '1.2.840.113549.1.9.4',
  certificadoFirmante: '1.2.840.113549.1.9.16.2.47',
  sha256: '2.16.840.1.101.3.4.2.1',
  rsa: '1.2.840.113549.1.1.1',
  rsaSHA256: '1.2.840.113549.1.1.11',
  ecdsaSHA256: '1.2.840.10045.4.3.2',
} as const;

export interface OpcionesFirmaLocal {
  /** Quién firma; sin él, el nombre común del certificado. */
  nombre?: string;
  motivo?: string;
  lugar?: string;
  /** La hora DECLARADA (/M). Sin sello, es solo lo que dice quien firma. */
  fecha?: Date;
  /** El recuadro visible; sin él, la firma es invisible (la opción por defecto). */
  visible?: FirmaVisible | undefined;
  /** Cómo pedir el sello de tiempo; sin él, la firma sale sin sello (B-B). */
  sello?: PedirSello | undefined;
  /**
   * CERTIFICA el documento (/DocMDP, /P 1): cualquier cambio posterior invalida la
   * firma. Solo en la primera firma, como en Go (`ErrCertificarFirmado`).
   */
  certificar?: boolean | undefined;
  signal?: AbortSignal | undefined;
}

/** Dónde y cómo se ve la firma, en coordenadas del PDF (origen abajo a la izquierda). */
export interface FirmaVisible {
  /** La página, desde 1. */
  pagina: number;
  rect: [number, number, number, number];
  /** La rúbrica dibujada, en coordenadas de la PÁGINA (como `Trazos` en Go). */
  trazos?: [number, number][][] | undefined;
}

/** Lo que dice el anexo de la firma: quién, por qué, dónde, cuándo y cómo se ve. */
interface DatosAnexo {
  nombre: string;
  motivo: string;
  lugar: string;
  fecha: Date;
  visible: FirmaVisible | undefined;
  certificar: boolean;
}

const ascii = (s: string) => Uint8Array.from(s, (c) => c.charCodeAt(0));

async function sha256(...partes: Uint8Array[]): Promise<Uint8Array<ArrayBuffer>> {
  return new Uint8Array(await crypto.subtle.digest('SHA-256', concatenar(...partes)));
}

/** `D:20261123101500Z`, como `fechaPDF` en Go. */
export function fechaPDF(f: Date): string {
  const d = (n: number) => String(n).padStart(2, '0');
  return `D:${f.getUTCFullYear()}${d(f.getUTCMonth() + 1)}${d(f.getUTCDate())}${d(f.getUTCHours())}${d(f.getUTCMinutes())}${d(f.getUTCSeconds())}Z`;
}

/* -------------------------------------------------------------------- CMS */

const algSHA256 = () => secuencia(oidDER(OID.sha256), nulo());

/** signing-certificate-v2 (RFC 5035): la huella del certificado y su IssuerSerial. */
async function certificadoFirmante(c: Certificado): Promise<Uint8Array<ArrayBuffer>> {
  const nombres = secuencia(tlv(contexto(4), c.emisor));
  const id = secuencia(cadenaOctetos(await sha256(c.der)), secuencia(nombres, c.serie));
  return secuencia(secuencia(id));
}

/** Los tres atributos firmados de B-B como SET OF: lo que de verdad se firma. */
async function atributosFirmados(
  resumen: Uint8Array,
  firmante: Certificado,
): Promise<Uint8Array<ArrayBuffer>> {
  const atributo = (tipo: string, valor: Uint8Array) => secuencia(oidDER(tipo), conjunto(valor));
  return conjunto(
    atributo(OID.tipoContenido, oidDER(OID.datos)),
    atributo(OID.resumen, cadenaOctetos(resumen)),
    atributo(OID.certificadoFirmante, await certificadoFirmante(firmante)),
  );
}

/** La firma de WebCrypto en el formato de CMS: ECDSA sale como r‖s y va en DER. */
async function firmar(identidad: Identidad, datos: Uint8Array<ArrayBuffer>): Promise<Uint8Array> {
  const c = identidad.cadena[0]!;
  if (c.tipo === 'rsa')
    return new Uint8Array(await crypto.subtle.sign('RSASSA-PKCS1-v1_5', identidad.clave, datos));
  const crudo = new Uint8Array(
    await crypto.subtle.sign({ name: 'ECDSA', hash: 'SHA-256' }, identidad.clave, datos),
  );
  const mitad = crudo.length / 2;
  return secuencia(enteroSinSigno(crudo.subarray(0, mitad)), enteroSinSigno(crudo.subarray(mitad)));
}

/** El CMS y lo que pasó con el sello. */
export interface CMSFirmado {
  cms: Uint8Array<ArrayBuffer>;
  /** La hora que atestigua la autoridad, o `null` si va sin sello. */
  sello: Date | null;
  /** Se pidió sello y la autoridad no lo dio (o dio uno que no era de esta firma). */
  selloFallido: boolean;
}

/**
 * El CMS desprendido (SignedData) que va en el hueco, como `montarCMS` en Go. Con
 * `sello`, pide el token del SHA-256 del valor de la firma y lo cuelga como atributo
 * no firmado; si no llega o no sella esta firma, sale sin él (`selloFallido`).
 */
export async function construirCMS(
  resumen: Uint8Array,
  identidad: Identidad,
  sello?: PedirSello,
  signal?: AbortSignal,
): Promise<CMSFirmado> {
  const firmante = identidad.cadena[0]!;
  const atributos = await atributosFirmados(resumen, firmante);
  const firma = await firmar(identidad, atributos);
  let token: Uint8Array<ArrayBuffer> | null = null;
  let fechaSello: Date | null = null;
  let selloFallido = false;
  if (sello) {
    try {
      // El messageImprint es el SHA-256 de los octetos de `signature` (lo mismo
      // que `huellaDeFirma` en Go): ni el PDF ni su resumen salen de aquí.
      const huella = await sha256(firma);
      const t = await sello(huella, signal);
      fechaSello = comprobarSello(t, huella);
      token = t;
    } catch {
      // Cortar es cortar; cualquier otro fallo deja la firma sin sello, como en Go.
      signal?.throwIfAborted();
      selloFallido = true;
    }
  }
  // Dentro del SignerInfo los atributos van como [0] IMPLICIT: misma codificación
  // que el SET OF firmado salvo la etiqueta.
  const implicitos = atributos.slice();
  implicitos[0] = contexto(0);
  const algFirma =
    firmante.tipo === 'rsa'
      ? secuencia(oidDER(OID.rsa), nulo())
      : secuencia(oidDER(OID.ecdsaSHA256));
  const infoFirmante = secuencia(
    enteroDER(1),
    secuencia(firmante.emisor, firmante.serie),
    algSHA256(),
    implicitos,
    algFirma,
    cadenaOctetos(firma),
    // unsignedAttrs [1] IMPLICIT: el sello no existe hasta que existe la firma.
    ...(token ? [tlv(contexto(1), secuencia(oidDER(OID_SELLO.atributo), conjunto(token)))] : []),
  );
  const firmado = secuencia(
    enteroDER(1),
    conjunto(algSHA256()),
    secuencia(oidDER(OID.datos)),
    tlv(contexto(0), ...identidad.cadena.map((c) => c.der)),
    conjunto(infoFirmante),
  );
  return {
    cms: secuencia(oidDER(OID.firmado), tlv(contexto(0), firmado)),
    sello: fechaSello,
    selloFallido,
  };
}

/* ------------------------------------------------------------ Apariencia */

/** Como `numPDF` en Go: tres decimales como mucho, sin ceros de cola. */
function numPDF(v: number): string {
  if (!Number.isFinite(v)) return '0';
  let s = v.toFixed(3).replace(/0+$/, '').replace(/\.$/, '');
  if (s === '' || s === '-' || s === '-0') s = '0';
  return s;
}

/**
 * Como `escaparCadenaContenido` en Go: WinAnsi (lo que entiende la Helvetica
 * estándar) con lo de fuera marcado como «?», y sin los caracteres que rompen una
 * cadena literal.
 */
function cadenaContenido(s: string): string {
  let out = '';
  for (const c of s) {
    const v = c.codePointAt(0)!;
    if (v > 0xff) out += '?';
    else if (v < 0x20) out += ' ';
    else out += c === '(' || c === ')' || c === '\\' ? `\\${c}` : c;
  }
  return out;
}

/** «2026-11-23 10:00 UTC», el mismo formato que la apariencia del servidor. */
function fechaVisible(f: Date): string {
  const iso = f.toISOString();
  return `${iso.slice(0, 10)} ${iso.slice(11, 16)} UTC`;
}

/**
 * El XObject de la firma VISIBLE, el mismo dibujo que `objetoAparienciaFirma` en Go:
 * fondo claro, borde dorado, la rúbrica (si la hay) en tinta oscura y debajo
 * «Firmado por …» y la fecha. Devuelve el objeto entero (diccionario y flujo).
 */
export function aparienciaVisible(datos: {
  nombre: string;
  fecha: Date;
  visible: FirmaVisible;
}): string {
  const [x0, y0, x1, y1] = datos.visible.rect;
  const ancho = x1 - x0;
  const alto = y1 - y0;
  let b = `q 0.98 0.98 0.97 rg 0 0 ${numPDF(ancho)} ${numPDF(alto)} re f\n`;
  b += `0.72 0.56 0.24 RG 1 w 0.5 0.5 ${numPDF(ancho - 1)} ${numPDF(alto - 1)} re S\n`;
  b += '0.06 0.09 0.16 RG 1.6 w 1 J 1 j\n';
  for (const tr of datos.visible.trazos ?? []) {
    if (tr.length === 0) continue;
    // Los trazos llegan en coordenadas de la página; la apariencia dibuja en su caja.
    const pts = tr.map(([x, y]) => `${numPDF(x - x0)} ${numPDF(y - y0)}`);
    b += `${pts[0]} m${pts
      .slice(1)
      .map((p) => ` ${p} l`)
      .join('')}\nS\n`;
  }
  const tam = 7;
  b += `BT /SytueFirmaF ${numPDF(tam)} Tf 0.2 0.22 0.25 rg\n`;
  b += `4 ${numPDF(tam * 1.4)} Td (${cadenaContenido(`Firmado por ${datos.nombre}`)}) Tj\n`;
  b += `0 ${numPDF(-tam * 1.15)} Td (${cadenaContenido(fechaVisible(datos.fecha))}) Tj\n`;
  b += 'ET Q\n';
  const fuente = '<</Type /Font /Subtype /Type1 /BaseFont /Helvetica /Encoding /WinAnsiEncoding>>';
  return (
    `<</Type /XObject /Subtype /Form /FormType 1 /BBox [0 0 ${numPDF(ancho)} ${numPDF(alto)}]` +
    ` /Resources <</Font <</SytueFirmaF ${fuente}>>>> /Length ${b.length}>>\nstream\n${b}\nendstream`
  );
}

/** ¿El recuadro es de verdad un recuadro dentro de la página? */
function recuadroValido(r: readonly number[], caja: readonly number[]): boolean {
  if (r.length !== 4 || !r.every(Number.isFinite)) return false;
  const [x0, y0, x1, y1] = r as [number, number, number, number];
  const [cx0, cy0, cx1, cy1] = caja as [number, number, number, number];
  return (
    x1 - x0 > 1 &&
    y1 - y0 > 1 &&
    x0 >= cx0 - 0.5 &&
    y0 >= cy0 - 0.5 &&
    x1 <= cx1 + 0.5 &&
    y1 <= cy1 + 0.5
  );
}

/* ----------------------------------------------------------------- Anexo */

/** Los nombres (/T) de los campos que ya tiene el formulario: no se repite ninguno. */
async function nombresDeCampos(doc: DocumentoPdf, acro: Dict | null): Promise<Set<string>> {
  const usados = new Set<string>();
  const campos = acro ? await doc.resolver(acro.get('Fields')) : null;
  if (!Array.isArray(campos)) return usados;
  for (const c of campos.slice(0, 5000)) {
    const d = await doc.resolver(c);
    if (esDict(d)) usados.add(leerTextoPDF(d.get('T')));
  }
  return usados;
}

/**
 * ¿El formulario lleva ya alguna firma (un campo /Sig con valor)? Certificar solo
 * cabe en la primera: certifica el documento ENTERO, y con otra firma delante no se
 * puede sin invalidarla (`camposDeFirma` en Go).
 */
async function hayFirmas(doc: DocumentoPdf, acro: Dict | null): Promise<boolean> {
  const campos = acro ? await doc.resolver(acro.get('Fields')) : null;
  const pendientes: Obj[] = Array.isArray(campos) ? campos.slice(0, 5000) : [];
  for (let i = 0; i < pendientes.length && i < 5000; i++) {
    const d = await doc.resolver(pendientes[i]);
    if (!esDict(d)) continue;
    if (esNombre(d.get('FT'), 'Sig') && d.has('V')) return true;
    const hijos = await doc.resolver(d.get('Kids'));
    if (Array.isArray(hijos)) pendientes.push(...hijos);
  }
  return false;
}

/** ¿El documento está certificado sin admitir cambios (/DocMDP con /P 1)? */
async function bloqueado(doc: DocumentoPdf, catalogo: Dict): Promise<boolean> {
  const perms = await doc.resolver(catalogo.get('Perms'));
  if (!esDict(perms) || !perms.has('DocMDP')) return false;
  const sig = await doc.resolver(perms.get('DocMDP'));
  const refs = esDict(sig) ? await doc.resolver(sig.get('Reference')) : null;
  for (const r of Array.isArray(refs) ? refs : []) {
    const rd = await doc.resolver(r);
    const tp = esDict(rd) ? await doc.resolver(rd.get('TransformParams')) : null;
    // Sin /P, el valor por defecto de DocMDP es 2; solo /P 1 lo cierra del todo.
    if (esDict(tp) && entero(tp.get('P')) === 1) return true;
  }
  return false;
}

/** Copia del primer nivel: lo de debajo sigue apuntando a los objetos originales. */
const copia = (d: Dict): Dict => new Map(d);

/** El PDF con la firma anexada y el hueco localizado, listo para resumir. */
interface ConHueco {
  datos: Uint8Array<ArrayBuffer>;
  inicio: number;
  fin: number;
}

async function anexarHueco(pdf: Uint8Array, opts: DatosAnexo): Promise<ConHueco> {
  const doc = await DocumentoPdf.abrir(pdf);
  const raizRef = doc.trailer.get('Root');
  if (!esRef(raizRef)) throw new ErrorPdf('ilegible', '/Root');
  const catalogo = await doc.resolver(raizRef);
  if (!esDict(catalogo)) throw new ErrorPdf('ilegible', 'catálogo');
  if (await bloqueado(doc, catalogo)) throw new ErrorPdf('bloqueado');
  let pagina: number;
  if (opts.visible) {
    const paginas = await doc.paginas();
    const p = paginas[opts.visible.pagina - 1];
    if (!p || !recuadroValido(opts.visible.rect, p.caja))
      throw new ErrorPdf('pagina', 'el recuadro de la firma no cae en la página');
    pagina = p.n;
  } else pagina = await doc.primeraPagina();
  // El anexo reescribe la página y el catálogo con la generación 0: si alguno es de
  // otra, sus referencias «N 1 R» quedarían colgando (lo mismo que en Go).
  if (raizRef.g !== 0 || doc.generacion(raizRef.n) !== 0 || doc.generacion(pagina) !== 0)
    throw new ErrorPdf('ilegible', 'generación distinta de 0');
  const nodoPagina = await doc.objeto(pagina);
  if (!esDict(nodoPagina)) throw new ErrorPdf('ilegible', 'página');

  const acroAntes = await doc.resolver(catalogo.get('AcroForm'));
  const acro: Dict = esDict(acroAntes) ? copia(acroAntes) : new Map();
  if (opts.certificar && (await hayFirmas(doc, esDict(acroAntes) ? acroAntes : null)))
    throw new ErrorPdf('certificarFirmado');
  const usados = await nombresDeCampos(doc, esDict(acroAntes) ? acroAntes : null);
  let k = 1;
  while (usados.has(`Sytue.Firma${k}`)) k++;

  let siguiente = doc.maxObjeto() + 1;
  const numSig = siguiente++;
  const numCampo = siguiente++;
  const numAp = siguiente++;

  const objetos = new Map<number, string>();
  // La firma: el /ByteRange y el /Contents se rellenan después, sin cambiar su largo.
  const sig: string[] = [
    '<</Type /Sig /Filter /Adobe.PPKLite',
    `/SubFilter /${SUBFILTRO}`,
    `/ByteRange [0 ${MARCA_RANGO} ${MARCA_RANGO} ${MARCA_RANGO}]`,
    `/Contents <${'0'.repeat(TAMANO_HUECO * 2)}>`,
    `/M ${serializar(textoPDF(fechaPDF(opts.fecha)))}`,
    `/Name ${serializar(textoPDF(opts.nombre))}`,
  ];
  if (opts.motivo) sig.push(`/Reason ${serializar(textoPDF(opts.motivo))}`);
  if (opts.lugar) sig.push(`/Location ${serializar(textoPDF(opts.lugar))}`);
  // /P 1: no se admite NINGÚN cambio posterior. /V 1.2 es la versión del método de
  // transformación, no la del PDF (`dictDeFirma` en Go).
  if (opts.certificar)
    sig.push(
      '/Reference [<</Type /SigRef /TransformMethod /DocMDP /TransformParams <</Type /TransformParams /P 1 /V /1.2>>>>]',
    );
  objetos.set(numSig, `${sig.join('\n')}>>`);

  const campo: Dict = new Map<string, Obj>([
    ['Type', nombre('Annot')],
    ['Subtype', nombre('Widget')],
    ['FT', nombre('Sig')],
    ['T', textoPDF(`Sytue.Firma${k}`)],
    ['V', ref(numSig)],
    ['F', num(4)],
    ['P', ref(pagina)],
    ['Rect', (opts.visible?.rect ?? [0, 0, 0, 0]).map((v): Obj => ({ t: 'num', v: numPDF(v) }))],
    ['AP', new Map<string, Obj>([['N', ref(numAp)]])],
  ]);
  objetos.set(numCampo, serializar(campo));
  objetos.set(
    numAp,
    opts.visible
      ? aparienciaVisible({ nombre: opts.nombre, fecha: opts.fecha, visible: opts.visible })
      : // La apariencia de la firma invisible: no dibuja nada, pero PDF/A la exige.
        `<</Type /XObject /Subtype /Form /BBox [0 0 0 0] /Length 0>>\nstream\n\nendstream`,
  );

  const nuevaPagina = copia(nodoPagina);
  const annots = await doc.resolver(nodoPagina.get('Annots'));
  nuevaPagina.set('Annots', [...(Array.isArray(annots) ? annots : []), ref(numCampo)]);
  objetos.set(pagina, serializar(nuevaPagina));

  const campos = await doc.resolver(acro.get('Fields'));
  acro.set('Fields', [...(Array.isArray(campos) ? campos : []), ref(numCampo)]);
  // /SigFlags 3: hay firmas y no se puede guardar de forma que las invalide.
  acro.set('SigFlags', num(3));
  const nuevoCatalogo = copia(catalogo);
  nuevoCatalogo.set('AcroForm', acro);
  // Sin /Perms en el catálogo el /DocMDP de la firma no lo mira ningún lector: el
  // documento saldría «certificado» solo en el nombre de la casilla.
  if (opts.certificar) nuevoCatalogo.set('Perms', new Map<string, Obj>([['DocMDP', ref(numSig)]]));
  objetos.set(raizRef.n, serializar(nuevoCatalogo));

  // --- Serializar el anexo detrás del original, sin tocar un byte de lo que había.
  const base = pdf.length;
  let texto = pdf[pdf.length - 1] === 10 ? '' : '\n';
  const offsets = new Map<number, number>();
  const nums = [...objetos.keys()];
  const numXref = doc.clasica ? 0 : siguiente++;
  if (numXref) nums.push(numXref);
  nums.sort((a, b) => a - b);
  for (const n of nums) {
    if (n === numXref) continue;
    offsets.set(n, base + texto.length);
    texto += `${n} 0 obj\n${objetos.get(n)!}\nendobj\n`;
  }
  const inicioXref = base + texto.length;
  const trailer: Dict = new Map<string, Obj>([
    ['Size', num(siguiente)],
    ['Root', raizRef],
    ['Prev', num(doc.ultimoXref)],
  ]);
  for (const k2 of ['ID', 'Info']) {
    const v = doc.trailer.get(k2);
    if (v !== undefined) trailer.set(k2, v);
  }
  const grupos: [number, number[]][] = [];
  for (const n of nums) {
    const ultimo = grupos[grupos.length - 1];
    if (ultimo && ultimo[0] + ultimo[1].length === n) ultimo[1].push(n);
    else grupos.push([n, [n]]);
  }
  if (!numXref) {
    texto += 'xref\n';
    for (const [ini, ns] of grupos) {
      texto += `${ini} ${ns.length}\n`;
      for (const n of ns) texto += `${String(offsets.get(n)!).padStart(10, '0')} 00000 n \n`;
    }
    texto += `trailer\n${serializar(trailer)}\n`;
  } else {
    // El original usa flujos de referencias: el anexo también (§7.5.8). Sin comprimir.
    offsets.set(numXref, inicioXref);
    const filas: number[] = [];
    for (const [, ns] of grupos)
      for (const n of ns) {
        const off = offsets.get(n)!;
        filas.push(1, (off >>> 24) & 255, (off >>> 16) & 255, (off >>> 8) & 255, off & 255, 0, 0);
      }
    const dict: Dict = new Map<string, Obj>([
      ['Type', nombre('XRef')],
      ['W', [num(1), num(4), num(2)]],
      ['Index', grupos.flatMap(([ini, ns]) => [num(ini), num(ns.length)])],
      ['Length', num(filas.length)],
    ]);
    for (const [k2, v] of trailer) dict.set(k2, v);
    texto += `${numXref} 0 obj\n${serializar(dict)}\nstream\n`;
    const cola = `\nendstream\nendobj\nstartxref\n${inicioXref}\n%%EOF\n`;
    const anexo = concatenar(ascii(texto), Uint8Array.from(filas), ascii(cola));
    return localizar(concatenar(pdf, anexo), base);
  }
  texto += `startxref\n${inicioXref}\n%%EOF\n`;
  return localizar(concatenar(pdf, ascii(texto)), base);
}

/**
 * Busca el hueco SOLO dentro del anexo —delante hay firmas anteriores con su propio
 * /ByteRange, y pisar «el primero del fichero» destrozaría la del primer firmante— y
 * escribe el /ByteRange real sin cambiar el largo (relleno con espacios).
 */
function localizar(datos: Uint8Array<ArrayBuffer>, base: number): ConHueco {
  const br = buscar(datos, `/ByteRange [0 ${MARCA_RANGO}`, base);
  const ct = buscar(datos, '/Contents <', br);
  if (br < 0 || ct < 0) throw new ErrorPdf('ilegible', 'hueco');
  const inicio = ct + '/Contents '.length;
  const fin = inicio + TAMANO_HUECO * 2 + 2;
  const hueco = br + '/ByteRange ['.length;
  const cierre = datos.indexOf(0x5d, hueco);
  const real = `0 ${inicio} ${fin} ${datos.length - fin}`;
  if (real.length > cierre - hueco) throw new ErrorPdf('ilegible', '/ByteRange');
  datos.set(ascii(real.padEnd(cierre - hueco, ' ')), hueco);
  return { datos, inicio, fin };
}

/** El PDF firmado y lo que pasó con el sello. */
export interface PdfFirmado {
  datos: Uint8Array<ArrayBuffer>;
  /** La hora atestiguada por la autoridad (B-T), o `null` (B-B). */
  sello: Date | null;
  /** Se pidió sello y no llegó: la firma es válida, pero sin él. */
  selloFallido: boolean;
}

/**
 * Firma `pdf` anexando con la identidad de un `.p12`, y dice si lleva sello. Lanza
 * `ErrorPdf` si el documento no se puede firmar así (ver arriba). El resultado se
 * comprueba antes de devolverlo: una firma que no verifica no sale de aquí.
 */
export async function firmarPdfConDetalle(
  pdf: Uint8Array,
  identidad: Identidad,
  opciones: OpcionesFirmaLocal = {},
): Promise<PdfFirmado> {
  const opts: DatosAnexo = {
    nombre: opciones.nombre?.trim() || identidad.cadena[0]!.nombre,
    motivo: opciones.motivo?.trim() ?? '',
    lugar: opciones.lugar?.trim() ?? '',
    fecha: opciones.fecha ?? new Date(),
    visible: opciones.visible,
    certificar: opciones.certificar === true,
  };
  const h = await anexarHueco(pdf, opts);
  const resumen = await sha256(h.datos.subarray(0, h.inicio), h.datos.subarray(h.fin));
  const r = await construirCMS(resumen, identidad, opciones.sello, opciones.signal);
  if (r.cms.length > TAMANO_HUECO) throw new ErrorPdf('ilegible', 'la firma no cabe');
  h.datos.set(ascii(`<${aHex(r.cms).padEnd(TAMANO_HUECO * 2, '0')}>`), h.inicio);
  const [ultima] = (await verificarFirmas(h.datos)).slice(-1);
  if (!ultima?.integra || !ultima.cubreTodo) throw new ErrorPdf('ilegible', 'la firma no verifica');
  if (r.sello && !ultima.sello?.cubreFirma)
    throw new ErrorPdf('ilegible', 'el sello no cubre la firma');
  return { datos: h.datos, sello: r.sello, selloFallido: r.selloFallido };
}

/** Firma `pdf` anexando (ver `firmarPdfConDetalle`) y devuelve solo los bytes. */
export async function firmarPdf(
  pdf: Uint8Array,
  identidad: Identidad,
  opciones: OpcionesFirmaLocal = {},
): Promise<Uint8Array<ArrayBuffer>> {
  return (await firmarPdfConDetalle(pdf, identidad, opciones)).datos;
}

/* ---------------------------------------------------------- Verificación */

export interface FirmaComprobada {
  /** El nombre común del certificado que firmó. */
  firmante: string;
  /** El resumen casa con lo cubierto y la firma, con la clave del certificado. */
  integra: boolean;
  /** El /ByteRange llega hasta el final del fichero: no hay nada añadido después. */
  cubreTodo: boolean;
  /**
   * El sello de tiempo, si lo lleva: su hora y si sella EL VALOR DE ESTA FIRMA. No
   * se juzga la confianza de la autoridad (eso es de `pdfverificar.go`).
   */
  sello?: SelloComprobado;
}

export interface SelloComprobado {
  fecha: Date;
  cubreFirma: boolean;
}

/** El atributo firmado `tipo` (su primer valor), o `undefined`. */
function atributo(atributos: Nodo, tipo: string): Nodo | undefined {
  for (const a of hijos(atributos))
    if (oid(hijo(a, 0)) === tipo) return hijos(hijo(a, 1, ETIQUETA.conjunto))[0];
  return undefined;
}

/** ECDSA en DER → r‖s, que es lo que verifica WebCrypto. */
function aP1363(der: Uint8Array<ArrayBuffer>, tam: number): Uint8Array<ArrayBuffer> {
  const sec = leerNodo(der);
  const out = new Uint8Array(tam * 2);
  hijos(sec).forEach((n, i) => {
    let v = n.valor;
    while (v.length > tam && v[0] === 0) v = v.subarray(1);
    out.set(v, i * tam + tam - v.length);
  });
  return out;
}

/** El sello del SignerInfo (atributo no firmado), si lo hay, contra su firma. */
async function selloDe(
  noFirmados: Nodo | undefined,
  firma: Nodo,
): Promise<SelloComprobado | undefined> {
  if (!noFirmados) return undefined;
  for (const a of hijos(noFirmados)) {
    if (oid(hijo(a, 0)) !== OID_SELLO.atributo) continue;
    const token = hijos(hijo(a, 1, ETIQUETA.conjunto))[0];
    if (!token) return undefined;
    try {
      const s = leerSello(token.bruto.slice());
      const huella = await sha256(firma.valor);
      return {
        fecha: s.fecha,
        cubreFirma: s.algoritmo === OID_SELLO.sha256 && comparar(s.huella, huella) === 0,
      };
    } catch {
      return { fecha: new Date(NaN), cubreFirma: false };
    }
  }
  return undefined;
}

interface CMSComprobado {
  firmante: string;
  integra: boolean;
  sello?: SelloComprobado;
}

async function comprobarCMS(
  cms: Uint8Array<ArrayBuffer>,
  cubierto: Uint8Array[],
): Promise<CMSComprobado> {
  const ci = leerNodo(cms);
  if (oid(hijo(ci, 0)) !== OID.firmado) return { firmante: '', integra: false };
  const sd = hijo(hijo(ci, 1, contexto(0)), 0, ETIQUETA.secuencia);
  const partes = hijos(sd);
  const certsNodo = partes.find((n) => n.etiqueta === contexto(0));
  const certs = certsNodo ? hijos(certsNodo).map((n) => leerCertificado(n.bruto.slice())) : [];
  const conjuntoFirmantes = partes[partes.length - 1]!;
  const si = hijo(conjuntoFirmantes, 0, ETIQUETA.secuencia);
  const campos = hijos(si);
  const ias = campos[1]!;
  const emisor = hijo(ias, 0).bruto;
  const serie = hijo(ias, 1).bruto;
  const cert = certs.find(
    (c) => comparar(c.emisor, emisor) === 0 && comparar(c.serie, serie) === 0,
  );
  const attrs = campos.find((n) => n.etiqueta === contexto(0));
  // Detrás de la firma pueden ir los atributos no firmados ([1]): ahí va el sello.
  const ultimo = campos[campos.length - 1]!;
  const noFirmados = ultimo.etiqueta === contexto(1) ? ultimo : undefined;
  const firma = campos[campos.length - (noFirmados ? 2 : 1)]!;
  const sello = await selloDe(noFirmados, firma);
  if (!cert || !attrs) return { firmante: cert?.nombre ?? '', integra: false };
  const resumen = atributo(attrs, OID.resumen);
  const real = await sha256(...cubierto);
  if (!resumen || comparar(resumen.valor, real) !== 0)
    return { firmante: cert.nombre, integra: false };
  const firmados = attrs.bruto.slice();
  firmados[0] = ETIQUETA.conjunto;
  const alg =
    cert.tipo === 'rsa'
      ? { name: 'RSASSA-PKCS1-v1_5', hash: 'SHA-256' }
      : { name: 'ECDSA', namedCurve: cert.curva ?? 'P-256' };
  const publica = await crypto.subtle.importKey('spki', cert.spki, alg, false, ['verify']);
  const tam = cert.curva === 'P-521' ? 66 : cert.curva === 'P-384' ? 48 : 32;
  const ok = await crypto.subtle.verify(
    cert.tipo === 'rsa' ? 'RSASSA-PKCS1-v1_5' : { name: 'ECDSA', hash: 'SHA-256' },
    publica,
    cert.tipo === 'rsa' ? firma.valor.slice() : aP1363(firma.valor.slice(), tam),
    firmados,
  );
  return { firmante: cert.nombre, integra: ok, ...(sello ? { sello } : {}) };
}

/**
 * Comprueba las firmas PAdES/CMS que lleve el fichero (integridad y si cubren todo).
 * No juzga la CONFIANZA del certificado ni su vigencia: eso es del verificador
 * completo (`pdfverificar.go`). Sirve para no guardar nunca una firma rota.
 */
export async function verificarFirmas(pdf: Uint8Array): Promise<FirmaComprobada[]> {
  const out: FirmaComprobada[] = [];
  let p = 0;
  for (;;) {
    const i = buscar(pdf, '/ByteRange', p);
    if (i < 0) break;
    p = i + 1;
    const l = new Lector(pdf, i + '/ByteRange'.length);
    let rango: Obj;
    try {
      rango = l.objeto();
    } catch {
      continue;
    }
    const r = Array.isArray(rango) ? rango.map(entero) : [];
    const [a, b, c, d] = r;
    if (r.length !== 4 || a !== 0 || b === undefined || c === undefined || d === undefined)
      continue;
    if (c + d > pdf.length || b >= c) continue;
    const hex = new Lector(pdf, b).objeto();
    if (!esCadena(hex)) continue;
    const cubierto = [pdf.subarray(0, b), pdf.subarray(c, c + d)];
    let comprobada: CMSComprobado;
    try {
      // El hueco va relleno de ceros detrás del CMS: se lee solo el nodo.
      comprobada = await comprobarCMS(leerNodo(hex.v.slice()).bruto.slice(), cubierto);
    } catch {
      comprobada = { firmante: '', integra: false };
    }
    out.push({ ...comprobada, cubreTodo: c + d === pdf.length });
  }
  return out;
}
