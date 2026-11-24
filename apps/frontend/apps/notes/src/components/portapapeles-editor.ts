/*
 * SPDX-License-Identifier: Apache-2.0
 * SPDX-FileCopyrightText: (c) 2026 Daniel Delgado Ramos
 *
 * Copiar, cortar y pegar desde la burbuja de la selección, con la API asíncrona del
 * portapapeles (`navigator.clipboard`).
 *
 * COPIAR lleva el HTML de lo seleccionado y su texto: pegado en otra nota conserva el
 * formato y los bloques; en cualquier otro sitio, el texto. Sin `write`, el texto con
 * `writeText`; sin ninguna de las dos (un navegador viejo, un contexto inseguro), la
 * orden de copiar del navegador, que vale dentro del gesto del clic.
 *
 * PEGAR lee el portapapeles y lo entrega como un `DataTransfer` de pega: lo que se
 * inserta pasa después por `queSePega`/`pegadoConFormato`, el MISMO camino que Ctrl+V,
 * así que nada del HTML ajeno llega sin sanear. Leer el portapapeles pide permiso, y el
 * navegador puede negarlo o no tener la API (Firefox sin ella, Safari fuera del gesto):
 * entonces devuelve `null` y quien llama avisa de que se pegue con el teclado, en vez de
 * fallar en silencio.
 */
import { executeEditingCommand } from '@sytue/ui/browser/editing';
import { normalizarSalida } from '../client/salida.js';

type Portapapeles = Partial<Pick<Clipboard, 'read' | 'readText' | 'write' | 'writeText'>>;

function portapapeles(): Portapapeles | null {
  try {
    return (navigator as Navigator & { clipboard?: Portapapeles }).clipboard ?? null;
  } catch {
    return null;
  }
}

/** Lo seleccionado como HTML (con los colores ya en hexadecimal) y como texto. */
export function contenidoDe(rango: Range): { html: string; texto: string } {
  const caja = document.createElement('div');
  caja.append(rango.cloneContents());
  return { html: normalizarSalida(caja.innerHTML), texto: rango.toString() };
}

/** Copia `rango` al portapapeles. `true` si alguna de las vías lo ha hecho. */
export async function copiarRango(rango: Range): Promise<boolean> {
  const { html, texto } = contenidoDe(rango);
  const cb = portapapeles();
  const Item = (globalThis as { ClipboardItem?: typeof ClipboardItem }).ClipboardItem;
  try {
    if (cb?.write && Item) {
      await cb.write([
        new Item({
          'text/html': new Blob([html], { type: 'text/html' }),
          'text/plain': new Blob([texto], { type: 'text/plain' }),
        }),
      ]);
      return true;
    }
    if (cb?.writeText) {
      await cb.writeText(texto);
      return true;
    }
  } catch {
    // Sin permiso o sin foco: la orden del navegador, que aún está en el gesto.
  }
  return executeEditingCommand('copy');
}

/** Lo que hay en el portapapeles, como lo entrega un `paste`; `null` si no se puede leer. */
export async function leerPortapapeles(): Promise<Pick<DataTransfer, 'getData'> | null> {
  const cb = portapapeles();
  const datos: Record<string, string> = {};
  try {
    if (cb?.read) {
      for (const item of await cb.read()) {
        for (const tipo of ['text/html', 'text/plain']) {
          if (datos[tipo] === undefined && item.types.includes(tipo))
            datos[tipo] = await (await item.getType(tipo)).text();
        }
      }
    } else if (cb?.readText) {
      datos['text/plain'] = await cb.readText();
    } else {
      return null;
    }
  } catch {
    // `read` puede faltar a medias (sin HTML) y `readText` valer: se intenta.
    try {
      if (!cb?.readText || datos['text/plain'] !== undefined) return null;
      datos['text/plain'] = await cb.readText();
    } catch {
      return null;
    }
  }
  return { getData: (tipo: string) => datos[tipo] ?? '' };
}
