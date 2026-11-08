/* SPDX-License-Identifier: Apache-2.0 */
/**
 * El «De» del redactor, como el de Apple Mail (fase B, punto 6): las direcciones de
 * Sytue —la principal, los alias y los buzones compartidos que dejan enviar— y
 * después cada cuenta externa que puede enviar. Elegir una externa manda el correo
 * por su proveedor (`POST /external-mail/{id}/send`, con `expected_address`) en vez
 * de por Sytue.
 *
 * Es puro y lo comparte Fyne (`remitentes` en apps/fyne/apps/mail/remitentes.go),
 * que pasa los mismos casos (apps/fyne/apps/mail/testdata/remitentes.json).
 */
import {
  HttpError,
  isICloudMail,
  type ExternalMailAccount,
  type ExternalMailAddress,
  type ExternalMailAttachment,
  type ExternalMailClient,
  type ExternalMailOutgoing,
} from '@sytue/api-clients';
import { errorKey, type ExternalMailErrorKey } from './externalMailErrors.js';

/** Lo que va delante de la instancia en el valor de una externa. */
const PREFIJO_EXTERNA = 'externa:';

/**
 * Los adjuntos que caben por una cuenta externa: con la sobrecarga de MIME y base64
 * entran en los 25 MiB por mensaje del proveedor. Es `limiteAdjuntosExternos` en Fyne.
 */
export const LIMITE_ADJUNTOS_EXTERNOS = 18 * 1024 * 1024;

/** Una opción del «De». */
export interface Remitente {
  /** Lo que guarda el desplegable: la dirección de Sytue, o `externa:<instancia>`. */
  valor: string;
  /** Lo que se lee en el desplegable. */
  direccion: string;
  /** La instancia de la cuenta externa; vacía en las de Sytue. */
  cuenta: string;
}

/** El valor del «De» que escribe desde esa cuenta externa. */
export function valorExterno(instancia: string): string {
  return PREFIJO_EXTERNA + instancia;
}

/**
 * Las opciones del «De»: las de Sytue, ya sin repetir, y detrás las externas que
 * envían, en el orden de la lista (la primera vez que sale una instancia es la que
 * cuenta). Una externa sin dirección, o con la de una opción de Sytue, no se ofrece:
 * dos «ana@ejemplo.eu» iguales no dirían por dónde sale el correo.
 */
export function remitentes(
  sytue: readonly string[],
  externas: readonly Pick<ExternalMailAccount, 'instance_id' | 'address' | 'capabilities'>[],
): Remitente[] {
  const out: Remitente[] = sytue.map((d) => ({ valor: d, direccion: d, cuenta: '' }));
  const vistas = new Set(sytue.map((d) => d.toLowerCase()));
  const cuentas = new Set<string>();
  for (const c of externas) {
    if (cuentas.has(c.instance_id)) continue;
    cuentas.add(c.instance_id);
    if (!c.capabilities.send || !c.address || vistas.has(c.address.toLowerCase())) continue;
    vistas.add(c.address.toLowerCase());
    out.push({ valor: valorExterno(c.instance_id), direccion: c.address, cuenta: c.instance_id });
  }
  return out;
}

/**
 * La opción de ese valor. La que ya no está (la cuenta se desconectó, o el borrador
 * del dispositivo es de antes) se queda en la primera, que es lo que enseña el
 * desplegable: nunca se envía desde algo que no se ve.
 */
export function remitenteElegido(
  valor: string,
  lista: readonly Remitente[],
): Remitente | undefined {
  return lista.find((r) => r.valor === valor) ?? lista[0];
}

/**
 * El HTML sin las imágenes metidas en el texto (`<img src="cid:…">`): el envío de una
 * cuenta externa no lleva Content-ID, así que esas imágenes salen como adjuntos.
 */
export function sinImagenesCid(html: string): string {
  return html.replace(/<img\b[^>]*?\bsrc\s*=\s*["']?cid:[^>]*>/gi, '');
}

/** Lo que se escribió en el redactor, ya listo para salir. */
export interface Escrito {
  to: ExternalMailAddress[];
  cc: ExternalMailAddress[];
  bcc: ExternalMailAddress[];
  subject: string;
  text: string;
  html: string;
  files: readonly { name: string; type: string; size: number; contentBase64: string }[];
}

/** El correo tal y como lo envía el proveedor de esa cuenta. */
export function salidaExterna(
  cuenta: Pick<ExternalMailAccount, 'address'>,
  e: Escrito,
): ExternalMailOutgoing {
  const html = sinImagenesCid(e.html);
  return {
    // El servidor comprueba que la cuenta sigue siendo esa dirección.
    expected_address: cuenta.address,
    to: e.to,
    cc: e.cc,
    bcc: e.bcc,
    subject: e.subject,
    text: e.text,
    ...(html.trim() ? { html } : {}),
    attachments: e.files.map((f): ExternalMailAttachment => ({
      name: f.name,
      type: f.type,
      size: f.size,
      content_base64: f.contentBase64,
    })),
  };
}

/** Lo que pasó al enviar por una cuenta externa. */
export type EnvioExterno =
  | { tipo: 'enviado'; copiaGuardada: boolean }
  /** `externalMail.sendUnknown`: pudo salir. No se reintenta; se pide mirar Enviados. */
  | { tipo: 'error'; clave: ExternalMailErrorKey };

/**
 * Envía una sola vez por el proveedor de la cuenta. Lo que pudo aceptar sin decirlo
 * (un 5xx, o la respuesta que no llegó) no se da por fallido: es `sendUnknown`.
 */
export async function enviarPorCuenta(
  api: Pick<ExternalMailClient, 'send'>,
  cuenta: Pick<ExternalMailAccount, 'instance_id' | 'address' | 'provider' | 'configuration'>,
  e: Escrito,
): Promise<EnvioExterno> {
  const total = e.files.reduce((suma, f) => suma + f.size, 0);
  if (total > LIMITE_ADJUNTOS_EXTERNOS) return { tipo: 'error', clave: 'externalMail.tooLarge' };
  try {
    const res = await api.send(cuenta.instance_id, salidaExterna(cuenta, e));
    return { tipo: 'enviado', copiaGuardada: res.sent_copy_saved !== false };
  } catch (err) {
    if (!(err instanceof HttpError) || err.status >= 500)
      return { tipo: 'error', clave: 'externalMail.sendUnknown' };
    return { tipo: 'error', clave: errorKey(err, isICloudMail(cuenta)) };
  }
}
