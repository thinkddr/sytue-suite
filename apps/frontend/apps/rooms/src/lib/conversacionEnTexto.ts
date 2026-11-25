/*
 * SPDX-License-Identifier: Apache-2.0
 * SPDX-FileCopyrightText: (c) 2026 Daniel Delgado Ramos
 *
 * «Descargar la conversación»: lo que se está viendo de un canal, en un `.txt`.
 *
 * Para guardar un acuerdo, adjuntarlo a un expediente o leerlo sin conexión. Se arma
 * en el navegador con los mensajes ya cargados —y, en un canal cifrado, ya
 * descifrados en este dispositivo—: no pide nada al servicio ni le manda nada. Lo que
 * no se ha cargado (más arriba del historial) no entra, y por eso el fichero dice
 * desde cuándo va.
 *
 * Es la MISMA forma que `ConversacionEnTexto` de Fyne (apps/fyne/apps/rooms):
 * «[AAAA-MM-DD HH:MM] Nombre: texto», una línea por mensaje, en hora local.
 */

export interface MensajeParaTexto {
  author: { name: string };
  body: string;
  createdAt: string;
}

/** `AAAA-MM-DD HH:MM` en hora local. */
export function fechaHoraLocal(d: Date): string {
  const dos = (n: number) => String(n).padStart(2, '0');
  return `${d.getFullYear()}-${dos(d.getMonth() + 1)}-${dos(d.getDate())} ${dos(d.getHours())}:${dos(d.getMinutes())}`;
}

/** El texto del fichero: la cabecera con el canal y cada mensaje en su línea. */
export function conversacionEnTexto(canal: string, mensajes: readonly MensajeParaTexto[]): string {
  const lineas = [`# ${canal}`, ''];
  for (const m of mensajes) {
    const d = new Date(m.createdAt);
    const cuando = Number.isNaN(d.getTime()) ? '' : `[${fechaHoraLocal(d)}] `;
    // Un mensaje de varias líneas sigue sangrado bajo su autor: así cada línea que
    // empieza por «[» es un mensaje nuevo.
    const cuerpo = m.body.trim().replace(/\r?\n/g, '\n    ');
    lineas.push(`${cuando}${m.author.name}: ${cuerpo}`);
  }
  return lineas.join('\n') + '\n';
}

/** Nombre del fichero: el del canal sin lo que un sistema de ficheros rechaza, y la fecha. */
export function nombreDeConversacion(canal: string, hoy: Date): string {
  const limpio =
    canal
      // eslint-disable-next-line no-control-regex -- los de control tampoco valen en un nombre
      .replace(/[\\/:*?"<>|\u0000-\u001f]+/g, ' ')
      .replace(/\s+/g, ' ')
      .trim()
      .slice(0, 60) || 'rooms';
  return `${limpio} ${fechaHoraLocal(hoy).slice(0, 10)}.txt`;
}

/** Guarda el texto como fichero en el equipo. */
export function descargarTexto(nombre: string, texto: string): void {
  const url = URL.createObjectURL(new Blob([texto], { type: 'text/plain;charset=utf-8' }));
  const a = document.createElement('a');
  a.href = url;
  a.download = nombre;
  document.body.appendChild(a);
  a.click();
  a.remove();
  URL.revokeObjectURL(url);
}
