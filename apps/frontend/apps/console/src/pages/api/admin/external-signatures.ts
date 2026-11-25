/*
 * SPDX-License-Identifier: Apache-2.0
 * SPDX-FileCopyrightText: (c) 2026 Daniel Delgado Ramos
 *
 * BFF de Sytue Admin — peticiones de firma que llegan de otras organizaciones
 * (ADR 0059).
 *
 *   GET  /api/admin/external-signatures                     → { mode, trusted }
 *   POST /api/admin/external-signatures { mode, trusted }   → guarda
 *
 * Proxya a `services/docs` (GET/PUT /api/v1/pdf/signature-policy); el proxy es el de
 * todas las políticas con gente de fuera (lib/politicaDeFuera.ts).
 */
import { docsBaseUrl } from '../../../lib/auth.js';
import { rutasDePolitica } from '../../../lib/politicaDeFuera.js';

export const prerender = false;

export const { GET, POST } = rutasDePolitica(
  () => `${docsBaseUrl().replace(/\/$/, '')}/api/v1/pdf/signature-policy`,
);
