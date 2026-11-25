/*
 * SPDX-License-Identifier: Apache-2.0
 * SPDX-FileCopyrightText: (c) 2026 Daniel Delgado Ramos
 *
 * Campos de formulario rellenables — AYUDAS PURAS (sin DOM). El pegamento con el
 * folio (insertar los campos, el modo «Rellenar formulario» y su interacción)
 * vive en ribbon/tabs/Insertar.tsx y canvas/PageCanvas.tsx; aquí solo la lógica
 * no trivial y testeable: construir el HTML del campo (FUENTE ÚNICA del markup,
 * con escape anti-XSS) y validar/normalizar un valor. Extraer {nombre→valor} de un
 * HTML con campos va aparte (`formfields-respuestas.ts`, con parse5: se baja al
 * abrir «Ver respuestas»). El VALOR vive en el
 * propio HTML del campo (texto = contenido; casilla = data-checked; desplegable =
 * data-value), así persiste con el autosave. Mismo patrón que mailmerge.ts.
 * Ejecutar el self-test: cd apps/frontend && bun test packages/office/test/formfields.test.ts
 */
/** Los tres tipos de campo rellenable (elementos inline en el HTML del folio). */
export type FormFieldType = 'text' | 'checkbox' | 'select';

/** Especificación de un campo a insertar (la da el popover del ribbon). */
export interface FormFieldSpec {
  type: FormFieldType;
  /** Nombre del campo (clave en el mapa de respuestas). */
  name: string;
  /** Texto tenue cuando el campo está vacío (texto y desplegable). */
  placeholder?: string;
  /** Opciones del desplegable, en orden. */
  options?: string[];
  /** Casilla marcada por defecto. */
  checked?: boolean;
}

/** Respuesta extraída de un campo del folio (para revisar/exportar). */
export interface FormAnswer {
  name: string;
  type: FormFieldType;
  /** texto: lo tecleado · casilla: «Sí»/«No» · desplegable: opción elegida. */
  value: string;
}

const HTML_ESCAPES: Record<string, string> = {
  '&': '&amp;',
  '<': '&lt;',
  '>': '&gt;',
  '"': '&quot;',
  "'": '&#39;',
};

/** Escapa texto para inyectarlo como HTML (frontera de confianza, anti-XSS). */
export function escapeHtml(text: string): string {
  return text.replace(/[&<>"']/g, (ch) => HTML_ESCAPES[ch] ?? ch);
}

/** «a | b | c» → ['a','b','c'] (recorta, descarta vacíos y duplicados; «|» separa). */
export function parseOptions(raw: string): string[] {
  const out: string[] = [];
  for (const part of raw.split('|')) {
    const opt = part.trim();
    if (opt !== '' && !out.includes(opt)) out.push(opt);
  }
  return out;
}

/**
 * HTML de un campo rellenable (FUENTE ÚNICA del markup, para que insertar,
 * extraer y renderizar no diverjan). Fichas inline con atributos data-*; casilla
 * y desplegable son `contenteditable=false` (fichas atómicas), el texto se hace
 * editable en el modo rellenar. Todo dato de usuario va escapado (anti-XSS). El
 * aspecto (borde/fondo, casilla, caret, placeholder) lo da editor.css.
 */
export function buildFormFieldHtml(spec: FormFieldSpec): string {
  const name = escapeHtml(spec.name.trim());
  if (spec.type === 'checkbox') {
    return (
      `<span data-formfield="checkbox" contenteditable="false" ` +
      `data-name="${name}" data-checked="${spec.checked ? '1' : '0'}"></span>`
    );
  }
  if (spec.type === 'select') {
    const options = escapeHtml(parseOptions((spec.options ?? []).join('|')).join('|'));
    const placeholder = escapeHtml(spec.placeholder?.trim() || 'Elegir…');
    return (
      `<span data-formfield="select" contenteditable="false" data-name="${name}" ` +
      `data-options="${options}" data-value="" data-placeholder="${placeholder}"></span>`
    );
  }
  const placeholder = escapeHtml(spec.placeholder?.trim() || 'Texto');
  return `<span data-formfield="text" data-name="${name}" data-placeholder="${placeholder}"></span>`;
}

/**
 * Normaliza/valida un valor para un campo (aplicar una respuesta). Devuelve el
 * valor a almacenar en el HTML del campo, o `null` si NO es válido para ese campo:
 *  - texto: cualquier cadena (nunca null).
 *  - casilla: '1' si el valor es afirmativo, '0' si no (nunca null).
 *  - desplegable: la opción si está en la lista; '' para vaciar; null si no encaja.
 */
export function coerceFieldValue(
  type: FormFieldType,
  raw: string,
  options: string[] = [],
): string | null {
  if (type === 'checkbox') {
    return /^(1|true|s[íi]|x|on|yes)$/i.test(raw.trim()) ? '1' : '0';
  }
  if (type === 'select') {
    const value = raw.trim();
    if (value === '') return '';
    return options.includes(value) ? value : null;
  }
  return raw;
}

/** Respuestas → texto «Nombre: valor» por línea (para copiar/exportar). */
export function answersToText(answers: FormAnswer[]): string {
  return answers.map((a) => `${a.name || '(sin nombre)'}: ${a.value}`).join('\n');
}
