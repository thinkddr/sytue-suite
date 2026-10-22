/*
 * SPDX-License-Identifier: Apache-2.0
 * SPDX-FileCopyrightText: (c) 2026 Daniel Delgado Ramos
 *
 * El directorio de la organización, con la sesión de Docs: compartir se escribe con
 * una dirección y el permiso se guarda contra el `sub` (ver `@sytue/auth/server`).
 */
import { crearDirectorio } from '@sytue/auth/server';
import { bff } from './auth';

export type { Persona } from '@sytue/auth/server';
export const { resolverPersona, resolverPersonas, agenda } = crearDirectorio(bff, import.meta.env);
