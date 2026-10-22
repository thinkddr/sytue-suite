/*
 * SPDX-License-Identifier: Apache-2.0
 * SPDX-FileCopyrightText: (c) 2026 Daniel Delgado Ramos
 *
 * El panel «Quién puede consultar» de cada conexión (NEG-06): traducción del inglés
 * (`en.ts`), que es la fuente de verdad.
 */
import type { AppCatalog } from '@sytue/i18n';
import type { en } from './en.js';

export const es: AppCatalog<typeof en> = {
  'acceso.boton': 'Quién consulta',
  'acceso.titulo': 'Quién puede consultar {nombre}',
  'acceso.ayuda':
    'Las consultas salen con las credenciales de tu organización. Quienes administran y quien creó la conexión pueden consultarla siempre; añade a las personas y grupos que también lo necesiten.',
  'acceso.vacia':
    'Nadie más: solo pueden consultarla quienes administran y quien creó la conexión.',
  'acceso.persona': 'Persona',
  'acceso.grupo': 'Grupo',
  'acceso.quitar': 'Quitar {nombre}',
  'acceso.anadirPersona': 'Añadir una persona',
  'acceso.buscar': 'Nombre o correo',
  'acceso.sinCandidatos': 'Sin resultados.',
  'acceso.anadirGrupo': 'Añadir un grupo',
  'acceso.elegirGrupo': 'Elige un grupo',
  'acceso.sinGrupos': 'Tu organización no tiene grupos.',
  'acceso.anadir': 'Añadir',
  'acceso.guardar': 'Guardar',
  'acceso.cancelar': 'Cancelar',
  'acceso.guardada': 'Lista guardada.',
  'acceso.error': 'No se ha podido guardar la lista.',
  'acceso.errorCarga': 'No se ha podido leer la lista.',
  'acceso.cargando': 'Cargando…',
};
