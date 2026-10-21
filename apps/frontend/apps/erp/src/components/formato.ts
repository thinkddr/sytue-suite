/*
 * SPDX-License-Identifier: Apache-2.0
 * SPDX-FileCopyrightText: (c) 2026 Daniel Delgado Ramos
 *
 * Presentación de valores: de lo que guarda el servicio a lo que se lee, y de
 * vuelta. Vive aparte de la interfaz porque es lo único aquí con reglas que se
 * pueden equivocar en silencio —el dinero— y así se puede probar solo.
 */
import type { Campo, Registro, ValorCampo } from './tipos.js';

/**
 * Los importes viajan en CÉNTIMOS enteros, igual que en el servicio y en la base
 * de datos. La división por 100 se hace SOLO para enseñarlos; nada que vuelva al
 * servidor pasa por un decimal flotante.
 */
export function formatearDinero(centimos: number, locale: string, moneda = 'EUR'): string {
  return new Intl.NumberFormat(locale, { style: 'currency', currency: moneda }).format(
    centimos / 100,
  );
}

/**
 * Base e IVA de un total con el IVA incluido, en céntimos. El IVA es lo que falta
 * hasta el total, no un redondeo aparte: la suma cuadra siempre al céntimo, que
 * es lo que comprueba el libro.
 */
export function desglosar(total: number, tipo: number): { base: number; iva: number } {
  const base = Math.round((total * 100) / (100 + tipo));
  return { base, iva: total - base };
}

/**
 * De lo que se escribe en un campo de importe a céntimos, SIN pasar por
 * `parseFloat`: 1234.56 * 100 da 123455.99999999999 en binario, y truncar eso
 * pierde un céntimo por factura.
 *
 * Admite coma o punto como separador decimal porque quien teclea usa el de su
 * teclado, no el de la configuración regional del servidor.
 */
export function aCentimos(texto: string): number | null {
  const limpio = texto.trim().replace(/\s/g, '').replace(',', '.');
  if (limpio === '') return null;
  if (!/^-?\d*(\.\d*)?$/.test(limpio) || limpio === '-' || limpio === '.') return null;
  const negativo = limpio.startsWith('-');
  const [entera = '0', decimal = ''] = limpio.replace('-', '').split('.');
  const dos = (decimal + '00').slice(0, 2);
  // El tercer decimal redondea al modo comercial, como cualquier hoja de cálculo.
  const arrastre = decimal.length > 2 && decimal.charCodeAt(2) >= 53 ? 1 : 0;
  const total = Number(entera || '0') * 100 + Number(dos) + arrastre;
  return negativo ? -total : total;
}

/** De céntimos al texto que se edita (unidades con dos decimales). */
export function deCentimos(centimos: number): string {
  const signo = centimos < 0 ? '-' : '';
  const abs = Math.abs(centimos);
  return `${signo}${Math.floor(abs / 100)}.${String(abs % 100).padStart(2, '0')}`;
}

/**
 * Un decimal canónico («1234.5») al formato de quien mira («1.234,5»).
 *
 * Se parte por el punto y se formatea la parte ENTERA con `Intl`, en vez de
 * pasar la cadena por `Number`: un `parseFloat` aquí volvería a meter coma
 * flotante en el único sitio del frontend que la había evitado a propósito, y
 * para nada — lo que hace falta es separar miles y cambiar el separador, no
 * calcular.
 *
 * Conserva los decimales TAL CUAL vinieron: si el dato dice «2.50», se enseña
 * «2,50» y no «2,5». En un inventario, los ceros de la derecha son la precisión
 * con la que se midió.
 */
export function formatearDecimal(canonico: string, locale: string): string {
  const negativo = canonico.startsWith('-');
  const sinSigno = negativo ? canonico.slice(1) : canonico;
  const [entera = '0', decimales] = sinSigno.split('.');
  const enteraNum = Number.parseInt(entera, 10);
  if (Number.isNaN(enteraNum)) return canonico;
  const partes = new Intl.NumberFormat(locale).formatToParts(enteraNum);
  const separador =
    new Intl.NumberFormat(locale).formatToParts(1.1).find((p) => p.type === 'decimal')?.value ??
    '.';
  const enteraFormateada = partes.map((p) => p.value).join('');
  const cuerpo = decimales ? `${enteraFormateada}${separador}${decimales}` : enteraFormateada;
  return negativo ? `-${cuerpo}` : cuerpo;
}

export function formatearFecha(iso: string, locale: string, conHora = false): string {
  const d = new Date(conHora ? iso : `${iso}T00:00:00`);
  if (Number.isNaN(d.getTime())) return iso;
  return new Intl.DateTimeFormat(locale, {
    dateStyle: 'medium',
    ...(conHora ? { timeStyle: 'short' } : {}),
  }).format(d);
}

/**
 * El valor de una celda, listo para pintar.
 *
 * `etiquetas` trae el nombre legible de las relaciones de esta página, que el
 * servicio resuelve en una sola consulta: sin él, una lista de cincuenta pedidos
 * enseñaría cincuenta identificadores.
 */
export function celda(
  campo: Campo,
  // `undefined` entra a propósito: `data[clave]` de un campo sin rellenar lo es,
  // y quien pinta no debería tener que comprobarlo antes de cada celda.
  valor: ValorCampo | undefined,
  locale: string,
  etiquetas: Record<string, string> = {},
): string {
  if (valor === undefined || valor === null || valor === '') return '—';
  switch (campo.tipo) {
    case 'dinero':
      return typeof valor === 'number' ? formatearDinero(valor, locale) : String(valor);
    case 'fecha':
      return formatearFecha(String(valor), locale);
    case 'fechahora':
      return formatearFecha(String(valor), locale, true);
    case 'booleano':
      return valor ? '✓' : '—';
    case 'entero':
      return typeof valor === 'number'
        ? new Intl.NumberFormat(locale).format(valor)
        : String(valor);
    case 'decimal':
      // Los decimales viajan como CADENA en forma canónica («2.5», punto) para no
      // perder precisión por el camino. Aquí se pintan con el separador de quien
      // mira: en castellano «2,5» y en inglés «2.5». Enseñar la forma canónica
      // tal cual es enseñar un dato interno.
      return formatearDecimal(String(valor), locale);
    case 'seleccion':
      return campo.opciones?.find((o) => o.valor === valor)?.etiqueta ?? String(valor);
    case 'relacion':
      // Si el nombre no vino resuelto, es que la fila apuntada está borrada: se
      // dice, en vez de enseñar un identificador que no significa nada.
      return etiquetas[String(valor)] ?? '—';
    default:
      return String(valor);
  }
}

/** Los campos de dinero se alinean a la derecha: es como se comparan cifras. */
export function alineadoDerecha(tipo: Campo['tipo']): boolean {
  return tipo === 'dinero' || tipo === 'entero' || tipo === 'decimal';
}

/** Nombre legible de una fila en las listas y en la paleta. */
export function nombreDe(r: Registro): string {
  return r.etiqueta || r.ref || r.id.slice(0, 8);
}
