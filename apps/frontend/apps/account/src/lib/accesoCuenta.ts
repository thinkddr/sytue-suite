/*
 * SPDX-License-Identifier: Apache-2.0
 * SPDX-FileCopyrightText: (c) 2026 Daniel Delgado Ramos
 *
 * Qué decir cuando Sytue ID no deja cambiar el @usuario o el móvil (me_cuenta.go): cada
 * negativa viaja con su código, y aquí se convierte en la clave del aviso de la
 * tarjeta «Acceso y recuperación». Lo que no se reconoce es «no se ha podido».
 */

export type FalloUsuario =
  'invalid' | 'taken' | 'recent' | 'is_address' | 'wrong_password' | 'failed';

export type FalloMovil =
  | 'invalid'
  | 'same'
  | 'wait'
  | 'limit'
  | 'code_wrong'
  | 'code_expired'
  | 'has_account'
  | 'unavailable'
  | 'wrong_password'
  | 'failed';

/** El código del error de Sytue ID (`{ error: { code } }`), o "" si no lo hay. */
export async function leerCodigo(res: Response | null | undefined): Promise<string> {
  if (!res) return '';
  try {
    const b = (await res.json()) as { error?: { code?: unknown } };
    return typeof b.error?.code === 'string' ? b.error.code : '';
  } catch {
    return '';
  }
}

export function falloDeUsuario(status: number, code: string): FalloUsuario {
  switch (code) {
    case 'username_invalid':
      return 'invalid';
    case 'username_taken':
      return 'taken';
    case 'username_recent':
      return 'recent';
    case 'username_is_address':
      return 'is_address';
  }
  if (status === 401) return 'wrong_password';
  if (status === 400) return 'invalid';
  return 'failed';
}

export function falloDeMovil(status: number, code: string): FalloMovil {
  switch (code) {
    case 'phone_invalid':
      return 'invalid';
    case 'phone_same':
      return 'same';
    case 'code_wait':
      return 'wait';
    case 'sms_limit':
      return 'limit';
    case 'code_wrong':
      return 'code_wrong';
    case 'code_expired':
      return 'code_expired';
    case 'phone_has_account':
      return 'has_account';
    case 'sms_unavailable':
      return 'unavailable';
  }
  if (status === 401) return 'wrong_password';
  if (status === 400) return 'invalid';
  return 'failed';
}
