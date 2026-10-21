/*
 * SPDX-License-Identifier: Apache-2.0
 * SPDX-FileCopyrightText: (c) 2026 Daniel Delgado Ramos
 *
 * Llevarse un evento fuera: el .ics tal cual lo guarda el servidor. Sirve para
 * mandárselo a alguien que no está en la organización, para archivarlo o para
 * abrirlo en otro calendario, y no necesita al servicio: el recurso CalDAV ya
 * viaja con el evento.
 */

/** Nombre de fichero: el asunto saneado, o «evento» si no hay. */
export function nombreIcs(summary: string): string {
  const base = (summary || '')
    .trim()
    .replace(/[\\/:*?"<>|]+/g, '_')
    .slice(0, 120);
  return `${base || 'evento'}.ics`;
}

/**
 * El ICS listo para guardar. Si lo que trae el evento es solo el VEVENT —algunos
 * servidores lo entregan así—, se envuelve en un VCALENDAR mínimo: sin envoltura
 * ningún calendario lo abre.
 */
export function icsCompleto(ical: string): string {
  const texto = ical.trim();
  if (/^BEGIN:VCALENDAR/i.test(texto)) return texto;
  return [
    'BEGIN:VCALENDAR',
    'VERSION:2.0',
    'PRODID:-//Sytue One//Sytue Calendar//ES',
    'CALSCALE:GREGORIAN',
    texto,
    'END:VCALENDAR',
  ].join('\r\n');
}

/** Guarda el evento como .ics en el equipo. */
export function descargarIcs(summary: string, ical: string): void {
  const blob = new Blob([icsCompleto(ical)], { type: 'text/calendar;charset=utf-8' });
  const url = URL.createObjectURL(blob);
  const a = document.createElement('a');
  a.href = url;
  a.download = nombreIcs(summary);
  document.body.appendChild(a);
  a.click();
  a.remove();
  URL.revokeObjectURL(url);
}
