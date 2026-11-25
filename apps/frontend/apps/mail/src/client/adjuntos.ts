/*
 * SPDX-License-Identifier: Apache-2.0
 * SPDX-FileCopyrightText: (c) 2026 Daniel Delgado Ramos
 *
 * «Te adjunto el informe» sin informe: el correo que más veces se manda dos
 * veces. Antes de enviar se mira si el texto PROPIO del mensaje —sin lo citado
 * de la respuesta— habla de un adjunto en cualquiera de los 26 idiomas de la
 * suite, y si no lleva ninguno se pregunta. Falsos positivos («sin adjuntos»)
 * cuestan un clic; el falso negativo cuesta un segundo correo pidiendo perdón.
 *
 * Hasta el 2026-11-25 solo miraba cinco idiomas: quien escribía en polaco o en
 * sueco se quedaba sin aviso. Las raíces viven en `adjuntos.vectores.json`, que
 * también lee la prueba de Fyne para que las dos digan lo mismo.
 *
 * El límite de palabra NO es `\b`: en JavaScript (y en RE2) solo entiende ASCII,
 * así que «συνημμένο» o «załącznik» no tendrían borde. Se mira a mano que antes y
 * después no haya una letra ni un dígito de cualquier alfabeto.
 */
import VECTORES from './adjuntos.vectores.json';

const PATRON = new RegExp(
  `(?<![\\p{L}\\p{N}])(?:${VECTORES.raices.join('|')})(?![\\p{L}\\p{N}])`,
  'iu',
);

/** ¿El texto habla de un adjunto? */
export function mencionaAdjunto(texto: string): boolean {
  return PATRON.test(texto);
}

/** El HTML del mensaje sin las citas de la respuesta (`<blockquote>`), que son de otro. */
export function sinCitas(html: string): string {
  return html.replace(/<blockquote[\s\S]*?<\/blockquote>/gi, ' ');
}
