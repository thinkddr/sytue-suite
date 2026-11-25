/*
 * SPDX-License-Identifier: Apache-2.0
 * SPDX-FileCopyrightText: (c) 2026 Daniel Delgado Ramos
 *
 * BFF de Sytue Admin — con quién de otras organizaciones se habla en Rooms (mig 0083
 * de Rooms).
 *
 *   GET  /api/admin/rooms-external-contact                     → { mode, trusted }
 *   POST /api/admin/rooms-external-contact { mode, trusted }   → guarda
 *
 * Proxya a `services/rooms` (GET/PUT /api/v1/admin/external-contact); el proxy es el de
 * todas las políticas con gente de fuera (lib/politicaDeFuera.ts).
 */
import { roomsBaseUrl } from '../../../lib/auth.js';
import { rutasDePolitica } from '../../../lib/politicaDeFuera.js';

export const prerender = false;

export const { GET, POST } = rutasDePolitica(
  () => `${roomsBaseUrl().replace(/\/$/, '')}/api/v1/admin/external-contact`,
);
