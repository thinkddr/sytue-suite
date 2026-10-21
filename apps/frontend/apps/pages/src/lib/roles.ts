/*
 * SPDX-License-Identifier: Apache-2.0
 * SPDX-FileCopyrightText: (c) 2026 Daniel Delgado Ramos
 */
import type { SytueUser } from '@sytue/auth';

/**
 * ¿Administra esta persona la organización en Sytue Web? `owner` o `admin`: LA MISMA regla
 * que `esAdmin` del servicio (services/pages/internal/http/publicar.go), que es quien de
 * verdad decide. Aquí sirve para no ofrecer —ni PEDIR— lo que el servicio va a negar: los
 * dominios de un sitio, la clave de IA de la organización.
 *
 * No es una barrera de seguridad: si esto y el servicio discreparan, manda el servicio y la
 * interfaz ya sabe enseñar su 403.
 */
export function administra(user: Pick<SytueUser, 'roles'>): boolean {
  return user.roles.some((rol) => rol === 'owner' || rol === 'admin');
}
