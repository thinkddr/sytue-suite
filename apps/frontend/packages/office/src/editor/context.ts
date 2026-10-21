/*
 * SPDX-License-Identifier: Apache-2.0
 * SPDX-FileCopyrightText: (c) 2026 Daniel Delgado Ramos
 *
 * Contexto Preact del editor (BLUEPRINT.md §2). Las zonas reciben `ctx` por
 * props (firma universal del blueprint); este contexto ofrece el MISMO
 * contrato para piezas profundas que no quieran taladrar props. El valor lo
 * provee EditorApp (A1): <EditorCtx.Provider value={ctx}>.
 */
import { createContext } from 'preact';
import { useContext } from 'preact/hooks';
import { DEFAULT_LOCALE } from '@sytue/i18n';
import { docsT, type DocsT } from '../i18n/index';
import type { EditorContext } from './types';

export type { EditorContext } from './types';

/** Contexto con el contrato completo del editor; `null` fuera del proveedor. */
export const EditorCtx = createContext<EditorContext | null>(null);

/** Devuelve el EditorContext activo; lanza si falta el proveedor (error de integración, no de usuario). */
export function useEditor(): EditorContext {
  const ctx = useContext(EditorCtx);
  if (ctx === null) {
    throw new Error('useEditor() requiere <EditorCtx.Provider> (lo monta EditorApp).');
  }
  return ctx;
}

/**
 * Contexto de IDIOMA del editor, separado del `EditorContext` a propósito: la `t`
 * la necesitan piezas muy profundas de las cintas —una paleta de color, un menú—
 * que no tienen nada que ver con el contrato del editor, y taladrarles props sería
 * ruido en veinte firmas.
 *
 * El valor lo provee `EditorApp` con el idioma que negoció el servidor. El defecto
 * es el catálogo inglés, que es el idioma base (§13.1): así una pieza montada
 * fuera del proveedor —una prueba, un ejemplo— sigue devolviendo texto legible en
 * vez de reventar.
 */
export const EditorI18nCtx = createContext<DocsT>(docsT(DEFAULT_LOCALE));

/** `t` del editor, atada al idioma activo. */
export function useT(): DocsT {
  return useContext(EditorI18nCtx);
}
