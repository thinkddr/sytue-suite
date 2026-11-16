/*
 * SPDX-License-Identifier: Apache-2.0
 * SPDX-FileCopyrightText: (c) 2026 Daniel Delgado Ramos
 *
 * Enlaces de una celda: el destino de `=HYPERLINK(destino; [nombre])` y la URL escrita
 * tal cual en una celda. El motor solo devuelve el NOMBRE visible de HYPERLINK (el valor
 * de la celda); el destino lo saca este módulo para que la rejilla lo pinte como enlace y
 * lo abra con Ctrl/⌘+clic o con el chip de la celda activa.
 *
 * Solo se abren `http:`, `https:` y `mailto:`: un `javascript:` o un `data:` que llegue
 * de un libro importado o de un coeditor no se convierte nunca en algo clicable.
 * Helpers PUROS (sin DOM); los prueba test/sheets-enlaces.test.ts.
 */
import { parseFormula, type AstNode, type CellValue } from '@sytue/formula';
import { rawText } from './format';

const ESQUEMAS_PERMITIDOS = new Set(['http:', 'https:', 'mailto:']);

/**
 * El `href` que se puede abrir para un texto, o null si no es un enlace permitido. Exige el
 * esquema escrito (nada de adivinar «www.…»: un texto cualquiera con un punto no debe
 * volverse un enlace) y sin espacios dentro, que es lo que distingue una URL de una frase
 * que empieza por «http».
 */
export function hrefSeguro(texto: string): string | null {
  const t = texto.trim();
  if (t === '' || /\s/.test(t)) return null;
  // Esquema explícito antes de `new URL`: así un «a:b» no pasa por URL relativa ni rara.
  const esquema = /^([a-z][a-z0-9+.-]*):/i.exec(t)?.[1]?.toLowerCase();
  if (!esquema || !ESQUEMAS_PERMITIDOS.has(`${esquema}:`)) return null;
  let url: URL;
  try {
    url = new URL(t);
  } catch {
    return null;
  }
  if (!ESQUEMAS_PERMITIDOS.has(url.protocol)) return null;
  if (url.protocol === 'mailto:') return url.pathname === '' ? null : url.href;
  // «http://» sin máquina no lleva a ninguna parte.
  return url.hostname === '' ? null : url.href;
}

/** La llamada HYPERLINK de la raíz de una fórmula (con 1 o 2 argumentos), o null. */
function llamadaHyperlink(formula: string): Extract<AstNode, { kind: 'call' }> | null {
  let ast: AstNode;
  try {
    ast = parseFormula(formula);
  } catch {
    return null;
  }
  if (ast.kind !== 'call' || ast.name.toUpperCase() !== 'HYPERLINK') return null;
  if (ast.args.length < 1 || ast.args.length > 2) return null;
  return ast;
}

/**
 * Fórmula que evalúa SOLO el destino de un `=HYPERLINK(…)` de raíz, o null si la celda no
 * es eso. Se reescribe la llamada como `IF(TRUE, destino, nombre)` y no se recompone el
 * argumento desde el árbol: así el destino se evalúa con el mismo motor, las mismas
 * referencias y los mismos nombres que la celda, sin un impresor de fórmulas aparte.
 */
export function formulaDelDestino(formula: string): string | null {
  if (!llamadaHyperlink(formula)) return null;
  const cuerpo = formula.trim().replace(/^=\s*/, '');
  const m = /^HYPERLINK\s*\(/i.exec(cuerpo);
  if (!m) return null;
  return `=IF(TRUE,${cuerpo.slice(m[0].length)}`;
}

/**
 * El enlace que abre una celda: el destino de su `=HYPERLINK(…)` o su literal si es una
 * URL. `evaluar` calcula una fórmula en el contexto de la celda (el motor de la hoja); solo
 * se llama cuando el destino no es un texto literal. Devuelve null si no hay enlace o si su
 * esquema no está permitido.
 */
export function enlaceDeCelda(
  html: string,
  evaluar: (formula: string) => CellValue,
): string | null {
  if (html === '') return null;
  const texto = rawText(html).trim();
  if (!texto.startsWith('=')) return hrefSeguro(texto);
  const llamada = llamadaHyperlink(texto);
  if (!llamada) return null;
  const destino = llamada.args[0];
  if (destino?.kind === 'str') return hrefSeguro(destino.value);
  const f = formulaDelDestino(texto);
  if (!f) return null;
  const v = evaluar(f);
  return typeof v === 'string' ? hrefSeguro(v) : null;
}

/** Lo que enseña el chip de un enlace: la máquina de una web o la dirección de un correo. */
export function rotuloDeEnlace(href: string): string {
  try {
    const url = new URL(href);
    if (url.protocol === 'mailto:') return decodeURIComponent(url.pathname);
    return url.hostname.replace(/^www\./, '');
  } catch {
    return href;
  }
}

/**
 * Abre el enlace en otra pestaña sin darle a la página de destino acceso a la nuestra
 * (`noopener`) ni nuestra dirección (`noreferrer`). Se vuelve a filtrar aquí por si alguien
 * llama sin pasar por `enlaceDeCelda`.
 */
export function abrirEnlace(
  href: string,
  abrir: (url: string, destino: string, rasgos: string) => unknown = (u, d, r) =>
    window.open(u, d, r),
): boolean {
  const seguro = hrefSeguro(href);
  if (!seguro) return false;
  abrir(seguro, '_blank', 'noopener,noreferrer');
  return true;
}
