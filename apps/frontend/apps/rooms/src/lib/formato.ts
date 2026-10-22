// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos
//
// Atajos de formato del composer (Cmd/Ctrl+B, +I, +K).
//
// Quien no sabe markdown no tenía forma de poner negrita ni un enlace con texto:
// el vocabulario de Slack/Teams —seleccionar y pulsar Cmd+B, o pegar un enlace
// encima de la selección— simplemente no respondía.
//
// La lógica vive aquí y no en el componente porque es donde están los casos que
// se rompen (selección vacía, marca ya puesta, cursor final) y así se prueban sin
// montar un textarea.

/** Qué marca aplica cada atajo. `link` es el par `[texto](url)`. */
export type MarcaFormato = 'bold' | 'italic' | 'strike' | 'code' | 'link' | 'quote' | 'list';

export interface ResultadoFormato {
  /** Texto completo ya con la marca aplicada. */
  value: string;
  /** Dónde dejar el cursor/selección después (el componente lo aplica al textarea). */
  selStart: number;
  selEnd: number;
}

const MARCAS: Record<'bold' | 'italic' | 'strike' | 'code', string> = {
  bold: '**',
  italic: '_',
  strike: '~~',
  code: '`',
};

/** Marcas de LÍNEA: van al principio de cada línea que toca la selección. */
const PREFIJOS: Record<'quote' | 'list', string> = { quote: '> ', list: '- ' };

/**
 * Aplica una marca de formato sobre la selección `[start, end)` de `value`.
 *
 * - Con texto seleccionado: lo envuelve y deja la selección sobre el mismo texto.
 * - Sin selección: inserta las marcas y deja el cursor EN MEDIO, listo para escribir.
 * - Si la selección ya estaba envuelta con esa marca, la QUITA (alternar), que es lo
 *   que hace cualquier editor y evita acabar con `****palabra****`.
 */
export function aplicarFormato(
  value: string,
  start: number,
  end: number,
  marca: MarcaFormato,
): ResultadoFormato {
  const sel = value.slice(start, end);

  if (marca === 'link') {
    // `[texto](url)`: el cursor queda dentro del paréntesis, que es lo siguiente
    // que hay que teclear (o pegar). Sin selección, el hueco del texto va primero.
    if (sel === '') {
      return {
        value: `${value.slice(0, start)}[]()${value.slice(end)}`,
        selStart: start + 1,
        selEnd: start + 1,
      };
    }
    const nuevo = `${value.slice(0, start)}[${sel}]()${value.slice(end)}`;
    const dentroDelParentesis = start + sel.length + 3;
    return { value: nuevo, selStart: dentroDelParentesis, selEnd: dentroDelParentesis };
  }

  if (marca === 'quote' || marca === 'list')
    return alternarPrefijo(value, start, end, PREFIJOS[marca]);
  const m = MARCAS[marca];
  const yaEnvuelta = sel.length >= m.length * 2 && sel.startsWith(m) && sel.endsWith(m);
  if (yaEnvuelta) {
    const desnudo = sel.slice(m.length, sel.length - m.length);
    return {
      value: value.slice(0, start) + desnudo + value.slice(end),
      selStart: start,
      selEnd: start + desnudo.length,
    };
  }

  const nuevo = `${value.slice(0, start)}${m}${sel}${m}${value.slice(end)}`;
  return sel === ''
    ? { value: nuevo, selStart: start + m.length, selEnd: start + m.length }
    : { value: nuevo, selStart: start + m.length, selEnd: start + m.length + sel.length };
}

/**
 * Prefijo de línea («> », «- ») en todas las líneas que toca la selección. Si TODAS
 * lo llevan ya, se quita; si no, se pone en las que faltan. El cursor acompaña a la
 * primera línea y el final de la selección se mueve con lo añadido o quitado.
 */
function alternarPrefijo(
  value: string,
  start: number,
  end: number,
  prefijo: string,
): ResultadoFormato {
  const ini = value.lastIndexOf('\n', start - 1) + 1;
  const corte = value.indexOf('\n', end);
  const fin = corte === -1 ? value.length : corte;
  const lineas = value.slice(ini, fin).split('\n');
  const todas = lineas.every((l) => l.startsWith(prefijo));
  const nuevas = lineas.map((l) =>
    todas ? l.slice(prefijo.length) : l.startsWith(prefijo) ? l : prefijo + l,
  );
  const bloque = nuevas.join('\n');
  const delta = bloque.length - (fin - ini);
  const primera = todas ? -prefijo.length : lineas[0]!.startsWith(prefijo) ? 0 : prefijo.length;
  return {
    value: value.slice(0, ini) + bloque + value.slice(fin),
    selStart: Math.max(ini, start + primera),
    selEnd: Math.max(ini, end + delta),
  };
}

/** Traduce un atajo de teclado a su marca. `null` si la combinación no es un atajo. */
export function marcaDeAtajo(e: {
  key: string;
  metaKey: boolean;
  ctrlKey: boolean;
  altKey: boolean;
  shiftKey?: boolean;
}): MarcaFormato | null {
  // Alt+B en algunos teclados escribe un carácter: si está pulsado, no es el atajo.
  if (!(e.metaKey || e.ctrlKey) || e.altKey) return null;
  // Cmd/Ctrl+Mayús+X: tachado (Slack, Mattermost). Código y cita no llevan atajo:
  // Cmd+Mayús+C abre el inspector del navegador y Cmd+E es «buscar» en macOS.
  if (e.shiftKey && e.key.toLowerCase() === 'x') return 'strike';
  switch (e.key.toLowerCase()) {
    case 'b':
      return 'bold';
    case 'i':
      return 'italic';
    case 'k':
      return 'link';
    default:
      return null;
  }
}
