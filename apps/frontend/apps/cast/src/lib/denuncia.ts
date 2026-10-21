/*
 * SPDX-License-Identifier: Apache-2.0
 * SPDX-FileCopyrightText: (c) 2026 Daniel Delgado Ramos
 *
 * A dónde va una denuncia y con qué cuerpo. Con sesión, a la ruta con sesión y sin
 * prueba de trabajo; sin ella, a la pública y con la prueba resuelta. Aparte del
 * cliente para poder probarlo sin red: equivocarse aquí es mandar la denuncia de
 * quien no ha entrado a una ruta que le contesta 401.
 */
import type { Motivo, ObjetivoDeDenuncia } from './tipos.js';

export interface PeticionDeDenuncia {
  /** Relativa a `/api/v1`. */
  ruta: string;
  cuerpo: Record<string, string>;
}

export function peticionDeDenuncia(
  objetivo: ObjetivoDeDenuncia,
  datos: { reason: Motivo; details: string },
  /** La prueba de trabajo resuelta; `null` con sesión. */
  altcha: string | null,
): PeticionDeDenuncia {
  const destino =
    objetivo.kind === 'channel'
      ? `/channels/${encodeURIComponent(objetivo.org)}/reports`
      : `/videos/${encodeURIComponent(objetivo.videoId)}/reports`;
  const cuerpo: Record<string, string> = { kind: objetivo.kind, ...datos };
  if ('target' in objetivo) cuerpo.target = objetivo.target;
  if (altcha === null) return { ruta: destino, cuerpo };
  return { ruta: `/public${destino}`, cuerpo: { ...cuerpo, altcha } };
}
