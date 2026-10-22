/*
 * SPDX-License-Identifier: Apache-2.0
 * SPDX-FileCopyrightText: (c) 2026 Daniel Delgado Ramos
 *
 * De código del servidor a frase para quien está delante.
 *
 * Vive aparte de `PdfApp.tsx` para poder comprobarse sin montar el visor: es la
 * pieza que decide lo ÚNICO que se ve cuando algo va mal, y hasta ahora solo se
 * podía probar abriendo un PDF roto a mano.
 */
import type { DocsKey, DocsT } from '../i18n';
import { ErrorPDF } from './api';

/**
 * Códigos del puente y del motor, traducidos a lo que le pasa a quien está
 * delante.
 *
 * Son CÓDIGOS, no frases: sin este mapa la pantalla enseñaba literalmente
 * «download_failed» a alguien que solo quería abrir un contrato. Y cuando el
 * motor sí manda frase, la manda EN INGLÉS —está escrita en el servicio Go—,
 * así que un código que falte aquí sale sin traducir en las otras cuatro
 * lenguas.
 */
export const ERRORES: Record<string, DocsKey> = {
  download_failed: 'pdf.error.download',
  upload_failed: 'pdf.error.upload',
  too_large: 'pdf.error.tooLarge',
  docs_unreachable: 'pdf.error.offline',
  unauthenticated: 'pdf.error.session',
  // Los cuatro del reconocimiento de texto (ADR-0034). Van los cuatro y no solo
  // el del perfil piloto —donde Sytue OCR no se despliega y todo acaba en
  // `ocr_not_configured`—: los otros tres llegan por el mismo camino y se veían
  // igual de crudos.
  ocr_not_configured: 'pdf.error.ocr.unavailable',
  ocr_busy: 'pdf.error.ocr.busy',
  ocr_language_unavailable: 'pdf.error.ocr.language',
  ocr_failed: 'pdf.error.ocr.failed',
  // «Pedir firma» (ADR 0051 §7).
  pdf_signature_recipient_not_found: 'pdf.error.signatureRequest.recipient',
  pdf_signature_request_self: 'pdf.error.signatureRequest.self',
  pdf_signature_request_exists: 'pdf.error.signatureRequest.exists',
  pdf_signature_request_not_signed: 'pdf.error.signatureRequest.notSigned',
  pdf_signature_request_not_found: 'pdf.error.signatureRequest.gone',
  pdf_signature_requests_unavailable: 'pdf.error.signatureRequest.unavailable',
  pdf_signature_request_too_many: 'pdf.error.signatureRequest.tooMany',
  // A alguien de otra organización (ADR 0057).
  pdf_signature_share_blocked: 'pdf.error.signatureRequest.shareBlocked',
  pdf_signature_request_link_gone: 'pdf.error.signatureRequest.linkGone',
  pdf_signature_request_recipient_busy: 'pdf.error.signatureRequest.recipientBusy',
  pdf_signature_request_changed: 'pdf.error.signatureRequest.changed',
  pdf_signature_request_busy: 'pdf.error.signatureRequest.busy',
  pdf_signature_org_closed: 'pdf.error.signatureRequest.orgClosed',
  pdf_signature_blocks_full: 'pdf.error.signatureRequest.blocksFull',
  // Editar un texto cuya fuente no dice qué letras usa.
  pdf_unknown_letters: 'pdf.error.unknownLetters',
  // Lo que el motor no sabe abrir. Antes llegaba todo como `bad_request` con una frase
  // en inglés, y por el sobre de `httpx.Fail` ni eso: la pantalla decía «pdf_failed».
  pdf_not_pdf: 'pdf.error.notPdf',
  pdf_unreadable: 'pdf.error.unreadable',
  pdf_encryption_unsupported: 'pdf.error.encryption',
  pdf_unsupported: 'pdf.error.unsupported',
  payload_too_large: 'pdf.error.tooLarge',
  // Reconocer, firmar y archivar: el motor los rechaza con su código propio.
  pdf_already_has_text: 'pdf.error.ocr.alreadyText',
  pdf_ocr_empty: 'pdf.error.ocr.empty',
  pdf_bad_certificate: 'pdf.error.certificate',
  pdf_certify_signed: 'pdf.error.certifySigned',
  pdf_signature_too_large: 'pdf.error.signatureTooLarge',
  pdf_archive_not_possible: 'pdf.error.archive',
};

export function mensajeDeError(e: unknown, t: DocsT, generico: DocsKey): string {
  // Lo que no llega a ser una respuesta es la red, no el documento: decir «no se
  // pudo abrir» de un fichero que está perfectamente manda a buscar donde no es.
  if (!(e instanceof ErrorPDF)) return t('pdf.error.offline');
  const clave = ERRORES[e.codigo ?? ''];
  if (clave) return t(clave);
  // Un código sin traducir NO se enseña tal cual: ni el código («pdf_failed», que es lo
  // que salía) ni la frase del motor, que está en inglés. Se dice lo que se intentaba.
  return t(generico);
}
