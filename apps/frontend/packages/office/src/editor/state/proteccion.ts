/*
 * SPDX-License-Identifier: Apache-2.0
 * SPDX-FileCopyrightText: (c) 2026 Daniel Delgado Ramos
 *
 * Contraseñas de protección de Calcus (hoja y libro) en tránsito hacia el servidor.
 *
 * La protección la cumple el servicio Go, que guarda solo el HASH de la contraseña y
 * decide si un cambio pasa. Por eso la contraseña NO viaja dentro del modelo (que se
 * guarda, se sirve a todo lector y queda en el historial): se teclea en el diálogo,
 * se anota aquí y el autosave la adjunta como `proteccion` junto a las operaciones
 * (POST /api/ops, mismo contrato que el mensaje WS «ops»). Se olvida al confirmar el
 * servidor, o al rechazarlo.
 *
 * Ámbitos: `libro` y `hoja:<nombre>` (los mismos que `domain.AmbitoLibro/AmbitoHoja`).
 */

export interface SobreProteccion {
  /** Contraseña vigente con la que autorizar (quitar o cambiar una protección). */
  password?: string;
  /** Contraseñas nuevas por ámbito (al proteger o al cambiar la contraseña). */
  new?: Record<string, string>;
}

export const AMBITO_LIBRO = 'libro';
/** Ámbito de la protección de una presentación (`domain.AmbitoDiapos`). */
export const AMBITO_DIAPOS = 'diapos';
export const ambitoHoja = (nombre: string): string => `hoja:${nombre}`;

let vigente = '';
let nuevas: Record<string, string> = {};
let propietario = false;

/** Contraseña con la que autorizar el próximo guardado (desproteger o cambiar). */
export function presentarClave(clave: string): void {
  vigente = clave;
}

/** Contraseña nueva de un ámbito: sale en el próximo guardado. */
export function presentarNueva(ambito: string, clave: string): void {
  if (clave !== '') nuevas[ambito] = clave;
}

/** Lo pendiente de enviar; undefined si no hay nada (no se añade el campo al cuerpo). */
export function sobreProteccion(): SobreProteccion | undefined {
  if (vigente === '' && Object.keys(nuevas).length === 0) return undefined;
  return {
    ...(vigente !== '' ? { password: vigente } : {}),
    ...(Object.keys(nuevas).length > 0 ? { new: { ...nuevas } } : {}),
  };
}

/** El servidor ya lo tiene (o lo rechazó): la contraseña no se guarda ni un instante más. */
export function olvidarProteccion(): void {
  vigente = '';
  nuevas = {};
}

/** Quien abre el documento es su propietario: solo él puede quitar una protección sin contraseña. */
export function fijarPropietario(esPropietario: boolean): void {
  propietario = esPropietario;
}
export const esPropietario = (): boolean => propietario;

let sub = '';
/** Quién es quien edita (su `sub`), para que el editor enseñe desbloqueados sus rangos editables. */
export function fijarYo(userSub: string): void {
  sub = userSub;
}
// ponytail: los grupos solo los conoce el servidor (claims de la sesión); el editor no los
// ve, así que una celda concedida solo por grupo se enseña bloqueada aunque el servicio la acepte.
export const yo = (): { sub: string } => ({ sub });
