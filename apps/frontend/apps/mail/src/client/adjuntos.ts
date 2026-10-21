/*
 * SPDX-License-Identifier: Apache-2.0
 * SPDX-FileCopyrightText: (c) 2026 Daniel Delgado Ramos
 *
 * «Te adjunto el informe» sin informe: el correo que más veces se manda dos
 * veces. Antes de enviar se mira si el texto PROPIO del mensaje —sin lo citado
 * de la respuesta— habla de un adjunto en cualquiera de los cinco idiomas de la
 * suite, y si no lleva ninguno se pregunta. Falsos positivos («sin adjuntos»)
 * cuestan un clic; el falso negativo cuesta un segundo correo pidiendo perdón.
 */
const PATRON =
  /\b(adjunt[oa]s?|adjuntad[oa]s?|attach(?:ed|ment|ments|ing)?|pi[eè]ces? jointes?|ci-joint[es]?|anh[aä]ng(?:e|end)?|angeh[aä]ngt|beigef[uü]gt|anbei|allegat[oaie]|allego)\b/i;

/** ¿El texto habla de un adjunto? */
export function mencionaAdjunto(texto: string): boolean {
  return PATRON.test(texto);
}

/** El HTML del mensaje sin las citas de la respuesta (`<blockquote>`), que son de otro. */
export function sinCitas(html: string): string {
  return html.replace(/<blockquote[\s\S]*?<\/blockquote>/gi, ' ');
}
