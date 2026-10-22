/* SPDX-License-Identifier: Apache-2.0
 * SPDX-FileCopyrightText: (c) 2026 Daniel Delgado Ramos */
import { createContext } from 'preact';
import type { Registro, ValorCampo } from './tipos.js';

export interface Altas {
  puedeCrear: (modelo: string) => boolean;
  crear: (
    modelo: string,
    creado: (r: Registro) => void,
    semilla?: Record<string, ValorCampo>,
  ) => void;
}
/** Acceso a la misma alta validada desde una relación o desde otra área. */
export const AltasContext = createContext<Altas | null>(null);
