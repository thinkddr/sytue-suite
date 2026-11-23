/*
 * SPDX-License-Identifier: Apache-2.0
 * SPDX-FileCopyrightText: (c) 2026 Daniel Delgado Ramos
 *
 * EL SELLO DE TIEMPO (RFC 3161) DE UNA FIRMA HECHA EN EL DISPOSITIVO: PAdES B-T
 * (ADR 0086, «Firmar en el dispositivo»).
 *
 * Una firma sin sello prueba QUIÉN firmó, no CUÁNDO: la hora la pone quien firma.
 * Con sello la atestigua un tercero y la firma se sigue validando cuando el
 * certificado caduque. Es el mismo modelo que el firmador del servidor
 * (`apps/nucleo/pdf/pdftimestamp.go`): se sella el SHA-256 del VALOR DE LA FIRMA
 * —el messageImprint del SignerInfo, no el documento ni su resumen— y el token se
 * cuelga como atributo NO firmado `id-aa-signatureTimeStampToken`.
 *
 * La autoridad solo la alcanza el servidor (su URL, credenciales y anclas son del
 * despliegue), así que la petición va por el BFF (`/api/pdf-sello`) al servicio de
 * Docs, que la reenvía y comprueba el token (huella, nonce y, con anclas, quién lo
 * firma). Aquí se comprueba OTRA vez que sella esta huella: un token de otra cosa no
 * entra en el PDF aunque el camino de vuelta fallara.
 */
import { ETIQUETA, comparar, contexto, hijo, hijos, leerNodo, octetos, oid } from './der';

export const OID_SELLO = {
  /** id-aa-signatureTimeStampToken: el atributo no firmado que lleva el token. */
  atributo: '1.2.840.113549.1.9.16.2.14',
  /** id-ct-TSTInfo: el contenido de un token. */
  tstInfo: '1.2.840.113549.1.9.16.1.4',
  firmado: '1.2.840.113549.1.7.2',
  sha256: '2.16.840.1.101.3.4.2.1',
} as const;

const GENERALIZED_TIME = 0x18;

/** El token no es un sello de esta firma. */
export class ErrorSello extends Error {
  constructor(motivo: string) {
    super(`sello: ${motivo}`);
    this.name = 'ErrorSello';
  }
}

/** Lo que dice un token: qué huella sella, con qué algoritmo y a qué hora. */
export interface SelloLeido {
  algoritmo: string;
  huella: Uint8Array<ArrayBuffer>;
  fecha: Date;
}

/** «20261123100000Z» o «20261123100000.123Z» → la fecha. */
function fechaGeneralizada(texto: string): Date {
  const m = /^(\d{4})(\d{2})(\d{2})(\d{2})(\d{2})(\d{2})(?:\.(\d{1,9}))?Z$/.exec(texto);
  if (!m) throw new ErrorSello('genTime ilegible');
  const ms = m[7] ? Number(m[7].slice(0, 3).padEnd(3, '0')) : 0;
  return new Date(Date.UTC(+m[1]!, +m[2]! - 1, +m[3]!, +m[4]!, +m[5]!, +m[6]!, ms));
}

/**
 * Baja por ContentInfo → SignedData → eContent (OCTET STRING) → TSTInfo. NO
 * verifica la firma de la autoridad: eso es del servicio (con sus anclas) y de quien
 * valide el PDF; aquí se mira QUÉ se selló.
 */
export function leerSello(token: Uint8Array<ArrayBuffer>): SelloLeido {
  try {
    const ci = leerNodo(token);
    if (ci.fin !== token.length) throw new ErrorSello('bytes de más detrás del token');
    if (oid(hijo(ci, 0)) !== OID_SELLO.firmado) throw new ErrorSello('no es un SignedData');
    const sd = hijo(hijo(ci, 1, contexto(0)), 0, ETIQUETA.secuencia);
    const encap = hijo(sd, 2, ETIQUETA.secuencia);
    if (oid(hijo(encap, 0)) !== OID_SELLO.tstInfo) throw new ErrorSello('no es un TSTInfo');
    const info = leerNodo(octetos(hijo(hijo(encap, 1, contexto(0)), 0)).slice());
    const campos = hijos(info);
    const imprint = campos[2]!;
    const alg = oid(hijo(hijo(imprint, 0, ETIQUETA.secuencia), 0));
    const huella = octetos(hijo(imprint, 1)).slice();
    const hora = campos[4];
    if (!hora || hora.etiqueta !== GENERALIZED_TIME) throw new ErrorSello('sin genTime');
    const fecha = fechaGeneralizada(String.fromCharCode(...hora.valor));
    return { algoritmo: alg, huella, fecha };
  } catch (e) {
    if (e instanceof ErrorSello) throw e;
    throw new ErrorSello('token ilegible');
  }
}

/** Comprueba que `token` sella EXACTAMENTE `huella` en SHA-256 y devuelve su hora. */
export function comprobarSello(token: Uint8Array<ArrayBuffer>, huella: Uint8Array): Date {
  const s = leerSello(token);
  if (s.algoritmo !== OID_SELLO.sha256) throw new ErrorSello('otro algoritmo que SHA-256');
  if (comparar(s.huella, huella) !== 0) throw new ErrorSello('sella otra huella');
  return s.fecha;
}

/** Pide el token de una huella (el SHA-256 del valor de la firma). */
export type PedirSello = (
  huella: Uint8Array,
  signal?: AbortSignal,
) => Promise<Uint8Array<ArrayBuffer>>;

/** Si el despliegue tiene autoridad de sellado, y su nombre (host) para decirlo. */
export interface SelloDisponible {
  disponible: boolean;
  autoridad: string;
}

const RUTA = '/api/pdf-sello';

const aBase64 = (b: Uint8Array) => btoa(String.fromCharCode(...b));
const deBase64 = (s: string): Uint8Array<ArrayBuffer> =>
  Uint8Array.from(atob(s), (c) => c.charCodeAt(0));

/** ¿Hay sello? Ante cualquier duda, no: la firma sale B-B y la pantalla lo dice. */
export async function selloDisponible(
  signal?: AbortSignal,
  pedir: typeof fetch = fetch,
): Promise<SelloDisponible> {
  try {
    const r = await pedir(RUTA, { credentials: 'include', ...(signal ? { signal } : {}) });
    if (!r.ok) return { disponible: false, autoridad: '' };
    const d = (await r.json()) as Partial<SelloDisponible>;
    return {
      disponible: d.disponible === true,
      autoridad: typeof d.autoridad === 'string' ? d.autoridad : '',
    };
  } catch {
    return { disponible: false, autoridad: '' };
  }
}

/** Pide el sello por el BFF: viaja SOLO la huella de 32 bytes. */
export function pedirSelloAlServidor(pedir: typeof fetch = fetch): PedirSello {
  return async (huella, signal) => {
    if (huella.length !== 32) throw new ErrorSello('la huella tiene que ser SHA-256');
    const r = await pedir(RUTA, {
      method: 'POST',
      credentials: 'include',
      headers: { 'content-type': 'application/json' },
      body: JSON.stringify({ resumen: aBase64(huella) }),
      ...(signal ? { signal } : {}),
    });
    if (!r.ok) throw new ErrorSello(`la autoridad no dio el sello (${r.status})`);
    const d = (await r.json()) as { token?: unknown };
    if (typeof d.token !== 'string' || !d.token) throw new ErrorSello('respuesta sin token');
    return deBase64(d.token);
  };
}
