/*
 * SPDX-License-Identifier: Apache-2.0
 * SPDX-FileCopyrightText: (c) 2026 Daniel Delgado Ramos
 *
 * El asistente de Sytue Calendar — helpers PUROS (sin DOM ni Preact).
 *
 * AQUÍ EL ASISTENTE NO ESCRIBE TEXTO, PROPONE UN EVENTO. «Comida con Ana el
 * jueves a las dos en Casa Lucio» tiene dentro un título, una hora y un lugar, y
 * teclearlos en cuatro campos es justo el trabajo que nadie quiere hacer.
 *
 * LO QUE SALE DE AQUÍ PRERRELLENA EL FORMULARIO, no guarda nada. El asistente
 * propone y quien firma es quien pulsa «Guardar»: una alucinación en un párrafo
 * se ve, y en una agenda se convierte en una cita a la que no va nadie.
 */
import { cadena, fechaHora, objetoDe } from '@sytue/ai';

/** Lo que el asistente puede proponer. Todo opcional: lo que no sepa, se deja. */
export interface EventoPropuesto {
  titulo?: string;
  /** ISO 8601. Sin hora reconocible no se propone ninguna. */
  inicio?: string;
  fin?: string;
  lugar?: string;
  descripcion?: string;
}

/**
 * Instrucción para el modelo.
 *
 * LLEVA LA FECHA DE HOY DENTRO, y no es un detalle: sin ella «el jueves» no
 * tiene referencia y el modelo se inventa una semana. Con ella, resuelve la
 * fecha relativa él, que es donde un modelo de lenguaje sí es mejor que una
 * expresión regular.
 *
 * Y pide ISO 8601 explícitamente porque es lo único que se acepta al validar:
 * adivinar aquí si «12/03» es marzo o diciembre sería elegir por el usuario.
 */
export function instruccionEvento(ahora: Date, zona: string): string {
  return (
    'Extrae un evento de agenda del texto. Devuelve SOLO un objeto JSON con las claves ' +
    '{titulo, inicio, fin, lugar, descripcion}. Las fechas en ISO 8601 con zona horaria. ' +
    'Omite las claves que el texto no diga; no inventes ni la hora ni el lugar. ' +
    `Hoy es ${ahora.toISOString()} y la zona horaria es ${zona}.`
  );
}

/**
 * Valida lo que respondió el modelo. Devuelve `null` si no hay nada usable.
 *
 * SIN TÍTULO NO HAY PROPUESTA: un evento sin nombre en la agenda no le dice nada
 * a nadie, y prerrellenar solo la hora obliga a adivinar de qué era la cita.
 */
export function eventoDe(bruto: string): EventoPropuesto | null {
  const obj = objetoDe(bruto);
  if (obj === null) return null;
  const titulo = cadena(obj, 'titulo', 200);
  if (titulo === undefined) return null;

  const propuesta: EventoPropuesto = { titulo };
  const inicio = fechaHora(obj, 'inicio');
  const fin = fechaHora(obj, 'fin');
  const lugar = cadena(obj, 'lugar', 200);
  const descripcion = cadena(obj, 'descripcion', 2000);
  if (inicio !== undefined) propuesta.inicio = inicio;
  // Un fin ANTERIOR al inicio es basura, y aceptarlo deja un evento que el
  // calendario pinta al revés o rechaza al guardar. Se descarta solo el fin.
  if (fin !== undefined && (inicio === undefined || fin > inicio)) propuesta.fin = fin;
  if (lugar !== undefined) propuesta.lugar = lugar;
  if (descripcion !== undefined) propuesta.descripcion = descripcion;
  return propuesta;
}
