/*
 * SPDX-License-Identifier: Apache-2.0
 * SPDX-FileCopyrightText: (c) 2026 Daniel Delgado Ramos
 *
 * Puente entre el HTML del documento (lo que lleva la secuencia RGA, un elemento por
 * carácter del HTML) y el texto que ve el usuario (`textContent` del folio, en el que
 * están medidos los offsets de cursor). Sirve para anclar el cursor a la identidad de un
 * carácter (cursors.ts, rga.ts) en vez de a un número que cualquier edición ajena
 * desplaza.
 *
 * Es un lector de HTML serializado por el navegador (`innerHTML`), no un parser general:
 * etiquetas `<…>` (con comillas en los atributos), comentarios y referencias `&…;`.
 * Cuando no coincide con el DOM, quien lo usa lo detecta (`textoVisible(html) !== textContent`)
 * y vuelve a los offsets: nunca se ancla a ciegas.
 */

/** Cuántas unidades UTF-16 de texto aporta cada code point del HTML (0 = marcado). */
function unidades(html: string): { cps: string[]; texto: number[] } {
  const cps = Array.from(html);
  const texto = new Array<number>(cps.length).fill(0);
  let i = 0;
  while (i < cps.length) {
    const c = cps[i]!;
    if (c === '<') {
      // comentario o etiqueta: hasta el '>' que cierra, sin contar los de dentro de comillas
      if (cps[i + 1] === '!' && cps[i + 2] === '-' && cps[i + 3] === '-') {
        let j = i + 4;
        while (j < cps.length && !(cps[j] === '>' && cps[j - 1] === '-' && cps[j - 2] === '-')) j++;
        i = j + 1;
        continue;
      }
      let j = i + 1;
      let comilla = '';
      while (j < cps.length) {
        const d = cps[j]!;
        if (comilla !== '') {
          if (d === comilla) comilla = '';
        } else if (d === '"' || d === "'") comilla = d;
        else if (d === '>') break;
        j++;
      }
      i = j + 1;
      continue;
    }
    if (c === '&') {
      let j = i + 1;
      while (j < cps.length && j - i <= 12 && /[#a-zA-Z0-9]/.test(cps[j]!)) j++;
      if (cps[j] === ';' && j > i + 1) {
        const ref = cps.slice(i + 1, j).join('');
        const n = /^#x([0-9a-f]+)$/i.exec(ref) ?? /^#(\d+)$/.exec(ref);
        const cp = n ? Number.parseInt(n[1]!, ref[1] === 'x' || ref[1] === 'X' ? 16 : 10) : 0;
        texto[j] = cp > 0xffff ? 2 : 1; // el texto se cuenta en el último carácter de la referencia
        i = j + 1;
        continue;
      }
    }
    texto[i] = c.length; // un code point son 1 o 2 unidades UTF-16
    i++;
  }
  return { cps, texto };
}

/** El texto visible de un HTML serializado: lo que sería su `textContent`, sin decodificar
 *  las referencias (solo se usa para comparar longitudes y contenido con el DOM). */
export function longitudVisible(html: string): number {
  return unidades(html).texto.reduce((a, b) => a + b, 0);
}

/** Tras cuántos code points del HTML se han consumido `offTexto` unidades de texto
 *  (la posición justo después del último carácter visible, antes del marcado que siga). */
export function indiceHtmlDeTexto(html: string, offTexto: number): number | null {
  const { texto } = unidades(html);
  if (offTexto <= 0) return 0;
  let visto = 0;
  for (let i = 0; i < texto.length; i++) {
    visto += texto[i]!;
    if (visto === offTexto) return i + 1;
    if (visto > offTexto) return null; // a mitad de un par suplente
  }
  return null; // más allá del final
}

/** Cuántas unidades de texto hay en los primeros `indice` code points del HTML. */
export function offsetDeIndiceHtml(html: string, indice: number): number {
  const { texto } = unidades(html);
  let visto = 0;
  for (let i = 0; i < Math.min(indice, texto.length); i++) visto += texto[i]!;
  return visto;
}
