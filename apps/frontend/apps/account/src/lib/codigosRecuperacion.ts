/*
 * SPDX-License-Identifier: Apache-2.0
 * SPDX-FileCopyrightText: (c) 2026 Daniel Delgado Ramos
 *
 * Los códigos de recuperación, llevados a un fichero.
 *
 * El servicio solo guarda su hash: la pantalla que los enseña es la única ocasión
 * de quedárselos. Copiarlos a mano, de ocho en ocho caracteres, es donde se cuela
 * una errata que se descubre el día que hacen falta; por eso se ofrece un `.txt`.
 * Lleva la cuenta y la fecha DENTRO y en el nombre: quien tenga dos cuentas, o
 * haya pedido códigos nuevos, tiene que poder saber cuál es el bueno sin abrirlo.
 */

/** Fecha local `AAAA-MM-DD`: la del día en que se generan, no la de Greenwich. */
export function fechaCorta(fecha: Date): string {
  const dos = (n: number) => String(n).padStart(2, '0');
  return `${fecha.getFullYear()}-${dos(fecha.getMonth() + 1)}-${dos(fecha.getDate())}`;
}

/** `AAAA-MM-DD HH:MM`, también en hora local. */
export function fechaLarga(fecha: Date): string {
  const dos = (n: number) => String(n).padStart(2, '0');
  return `${fechaCorta(fecha)} ${dos(fecha.getHours())}:${dos(fecha.getMinutes())}`;
}

/**
 * Nombre del fichero: `sytue-recovery-codes-<cuenta>-<fecha>.txt`. La cuenta se
 * sanea (sin barras ni caracteres que Windows rechaza) y, si no se conoce, no se
 * inventa: queda solo la fecha.
 */
export function nombreFicheroCodigos(cuenta: string, fecha: Date): string {
  const limpia = cuenta
    .trim()
    .replace(/[\\/:*?"<>|\s]+/g, '_')
    .slice(0, 80);
  return ['sytue-recovery-codes', limpia, fechaCorta(fecha)].filter(Boolean).join('-') + '.txt';
}

/** Los rótulos del fichero, ya traducidos: este módulo no conoce el catálogo. */
export interface RotulosFichero {
  titulo: string;
  /** Recibe la cuenta y devuelve la línea entera («Cuenta: ana@…»). */
  cuenta: (cuenta: string) => string;
  /** Recibe la fecha ya formateada. */
  fecha: (fecha: string) => string;
  nota: string;
}

/**
 * El contenido del `.txt`. Fin de línea CRLF: el Bloc de notas de Windows antiguo
 * pinta un `\n` solo como una única línea con todos los códigos pegados.
 */
export function textoCodigos(
  codigos: readonly string[],
  cuenta: string,
  fecha: Date,
  r: RotulosFichero,
): string {
  const lineas = [
    r.titulo,
    ...(cuenta ? [r.cuenta(cuenta)] : []),
    r.fecha(fechaLarga(fecha)),
    '',
    ...codigos,
    '',
    r.nota,
    '',
  ];
  return lineas.join('\r\n');
}

/** Guarda un texto como fichero en el equipo. */
export function descargarTexto(nombre: string, texto: string): void {
  const blob = new Blob([texto], { type: 'text/plain;charset=utf-8' });
  const url = URL.createObjectURL(blob);
  const a = document.createElement('a');
  a.href = url;
  a.download = nombre;
  document.body.appendChild(a);
  a.click();
  a.remove();
  URL.revokeObjectURL(url);
}

/**
 * IMPRIMIR LOS CÓDIGOS, Y SOLO LOS CÓDIGOS.
 *
 * Ctrl+P en Mi cuenta imprime la página entera: el perfil, las sesiones, los
 * dispositivos… y los códigos en mitad de una hoja que nadie quiere archivar. El
 * botón marca el documento (`data-imprimir="codigos"`) mientras dura el diálogo de
 * impresión, y la hoja de esta app (`styles/app.css`) esconde todo lo que no es el
 * bloque `ID_IMPRIMIBLE`. Un `<iframe>` con su propio HTML habría sido lo obvio,
 * pero la CSP de esta app no deja estilos en línea y un folio sin estilos sale con
 * los códigos en una sola columna de letra de sistema.
 */
export const ID_IMPRIMIBLE = 'codigos-recuperacion-imprimibles';
export const MARCA_IMPRESION = 'codigos';

/** `imprimir` se inyecta en las pruebas; en el navegador es `window.print`. */
export function imprimirCodigos(imprimir: () => void = () => window.print()): void {
  const raiz = document.documentElement;
  raiz.dataset.imprimir = MARCA_IMPRESION;
  // `print()` bloquea hasta que se cierra el diálogo en los navegadores de
  // escritorio; en los que no (Safari de iOS), `afterprint` llega después. Se quita
  // la marca en los dos casos: si se queda, el siguiente Ctrl+P saldría recortado.
  const quitar = (): void => {
    delete raiz.dataset.imprimir;
    window.removeEventListener('afterprint', quitar);
  };
  window.addEventListener('afterprint', quitar);
  try {
    imprimir();
  } finally {
    if (!('onafterprint' in window)) quitar();
  }
}
