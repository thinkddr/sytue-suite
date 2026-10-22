/*
 * SPDX-License-Identifier: Apache-2.0
 * SPDX-FileCopyrightText: (c) 2026 Daniel Delgado Ramos
 *
 * El directorio de la organización, con la sesión de Drive. La lógica vive en
 * `@sytue/auth/server` (crearDirectorio): la comparte con Docs.
 */
import { crearDirectorio } from '@sytue/auth/server';
import { bff } from './auth';

export type { Persona } from '@sytue/auth/server';
export const { resolverPersona, resolverPersonas, agenda } = crearDirectorio(bff, import.meta.env);
