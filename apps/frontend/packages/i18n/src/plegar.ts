/*
 * SPDX-License-Identifier: Apache-2.0
 * SPDX-FileCopyrightText: (c) 2026 Daniel Delgado Ramos
 *
 * La regla ÚNICA de «¿coincide lo que escribo con lo que hay?» de la suite:
 * «reunion» encuentra «reunión», «cafe» encuentra «Café», «Muller» encuentra
 * «Müller», y al revés. Sin mayúsculas ni acentos.
 *
 * Es el espejo de `buscar.Plegar` (apps/nucleo/buscar, servicios y Fyne) y de la
 * función SQL `plegar(text)` de cada base. Los tres leen los mismos casos,
 * `apps/nucleo/buscar/testdata/casos.json`: si uno pliega distinto, la web, Fyne
 * y el servidor dejan de encontrar lo mismo con la misma búsqueda.
 */

/** Letras que la NFD deja como están y que aun así se leen como otras. */
const SUELTAS: Record<string, string> = {
  ß: 'ss',
  æ: 'ae',
  œ: 'oe',
  ø: 'o',
  ł: 'l',
  đ: 'd',
  ð: 'd',
  þ: 'th',
  ı: 'i',
};

/** `s` en minúsculas y sin diacríticos. No recorta: eso es cosa de quien lee la caja. */
export function plegar(s: string): string {
  return s
    .toLowerCase()
    .replace(/[ßæœøłđðþı]/g, (c) => SUELTAS[c] ?? c)
    .normalize('NFD')
    .replace(/\p{Mn}/gu, '')
    .normalize('NFC');
}

/** ¿Aparece `aguja` en `texto` con la regla de `plegar`? Una aguja vacía está en todo. */
export function contiene(texto: string, aguja: string): boolean {
  return plegar(texto).includes(plegar(aguja));
}

/**
 * Dónde está `aguja` dentro de `texto` con la regla de `plegar`: rangos
 * `[inicio, fin)` sobre el texto ORIGINAL, sin solaparse. Para resaltar o
 * sustituir lo encontrado: «reunion» marca «reunión» entera, tilde incluida.
 */
export function rangos(texto: string, aguja: string): [number, number][] {
  const buscada = plegar(aguja);
  if (!buscada) return [];
  // De cada letra plegada, el índice del carácter original del que sale. Las marcas
  // sueltas (una tilde combinada) se pliegan a nada y quedan dentro del rango.
  let plano = '';
  const origen: number[] = [];
  let i = 0;
  for (const c of texto) {
    const p = plegar(c);
    for (let k = 0; k < p.length; k++) origen.push(i);
    plano += p;
    i += c.length;
  }
  origen.push(texto.length);
  const out: [number, number][] = [];
  for (let j = plano.indexOf(buscada); j !== -1; j = plano.indexOf(buscada, j + buscada.length)) {
    const inicio = origen[j] ?? 0;
    let fin = origen[j + buscada.length] ?? texto.length;
    // Media letra plegada («s» dentro de «ß»): se marca la letra entera.
    if (fin <= inicio) fin = inicio + (texto.codePointAt(inicio)! > 0xffff ? 2 : 1);
    // La otra mitad de esa misma letra ya está marcada.
    const previo = out[out.length - 1];
    if (!previo || inicio >= previo[1]) out.push([inicio, fin]);
  }
  return out;
}

/**
 * Las palabras de una consulta de la caja de búsqueda, sin la sintaxis que el
 * servidor entiende y que no se resalta (comillas, el «-» de «sin esta palabra» y
 * el «or»). Es `buscar.Terminos`.
 */
export function terminos(consulta: string): string[] {
  return consulta
    .split(/\s+/)
    .filter((p) => p && !p.startsWith('-') && p.toLowerCase() !== 'or')
    .map((p) => {
      let inicio = 0;
      let fin = p.length;
      while (p.charCodeAt(inicio) === 34) inicio++;
      while (fin > inicio && p.charCodeAt(fin - 1) === 34) fin--;
      return p.slice(inicio, fin);
    })
    .filter(Boolean);
}

/**
 * Los rangos de TODOS los términos de la consulta, en orden y fundidos donde se
 * tocan: lo que se pinta resaltado en un resultado. Es `buscar.RangosDeConsulta`.
 */
export function rangosDeConsulta(texto: string, consulta: string): [number, number][] {
  const todos = terminos(consulta)
    .flatMap((t) => rangos(texto, t))
    .sort((a, b) => a[0] - b[0]);
  const out: [number, number][] = [];
  for (const r of todos) {
    const ultimo = out[out.length - 1];
    if (ultimo && r[0] <= ultimo[1]) ultimo[1] = Math.max(ultimo[1], r[1]);
    else out.push([r[0], r[1]]);
  }
  return out;
}
