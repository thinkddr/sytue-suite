/*
 * SPDX-License-Identifier: Apache-2.0
 * SPDX-FileCopyrightText: (c) 2026 Daniel Delgado Ramos
 *
 * El asistente de Sytue Contactos — helpers PUROS (sin DOM ni Preact).
 *
 * EL CASO REAL ES PEGAR UNA FIRMA DE CORREO. «Ana Ruiz — Directora Comercial,
 * Acme S.L. · ana.ruiz@acme.es · +34 600 11 22 33» son cinco campos que hoy se
 * teclean uno a uno mirando otra ventana. Eso es lo que sobra.
 *
 * COMO EN CALENDAR, ESTO PRERRELLENA Y NO GUARDA: la ficha aparece rellena y la
 * persona la revisa antes de pulsar. Un teléfono mal copiado no se ve hasta que
 * alguien llama y no contesta nadie.
 */
import { cadena, lista, objetoDe } from '@sytue/ai';

/** Lo que el asistente puede proponer. Todo opcional salvo el nombre. */
// SIN PUESTO: el formulario no tiene ese campo (la vCard sí, pero aquí no se
// edita), y extraer un dato que no cabe en ningún sitio es tirarlo con pasos
// de más.
export interface FichaPropuesta {
  nombre: string;
  organizacion?: string;
  correos: string[];
  telefonos: string[];
  nota?: string;
}

/** Instrucción para el modelo. Pide JSON plano y prohíbe rellenar huecos. */
export function instruccionFicha(): string {
  return (
    'Extrae los datos de contacto del texto (una firma de correo, una tarjeta o unas notas). ' +
    'Devuelve SOLO un objeto JSON con las claves ' +
    '{nombre, organizacion, correos, telefonos, nota}, donde correos y telefonos son listas de cadenas. ' +
    'Copia los valores TAL CUAL aparecen; no inventes correos ni teléfonos ni completes los que estén a medias. ' +
    'Omite las claves que el texto no diga.'
  );
}

/** Descarta lo que no parece un correo: el modelo a veces cuela la web. */
const CORREO = /^[^\s@]+@[^\s@]+\.[^\s@]{2,}$/;

/**
 * Un teléfono utilizable: 6 dígitos o más, y nada raro fuera de los separadores
 * habituales. NO SE NORMALIZA el formato —quitar el prefijo o los espacios es
 * decidir por quien luego marca—, solo se descarta lo que no es un número.
 */
const TELEFONO = /^\+?[\d\s().-]{6,}$/;

/** Valida lo que respondió el modelo. `null` si no hay ni nombre. */
export function fichaDe(bruto: string): FichaPropuesta | null {
  const obj = objetoDe(bruto);
  if (obj === null) return null;
  const nombre = cadena(obj, 'nombre', 200);
  if (nombre === undefined) return null;

  const ficha: FichaPropuesta = {
    nombre,
    correos: lista(obj, 'correos', 320).filter((c) => CORREO.test(c)),
    telefonos: lista(obj, 'telefonos', 40).filter(
      (n) => TELEFONO.test(n) && (n.match(/\d/g) ?? []).length >= 6,
    ),
  };
  const organizacion = cadena(obj, 'organizacion', 200);
  const nota = cadena(obj, 'nota', 2000);
  if (organizacion !== undefined) ficha.organizacion = organizacion;
  if (nota !== undefined) ficha.nota = nota;
  return ficha;
}
