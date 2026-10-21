/*
 * SPDX-License-Identifier: Apache-2.0
 * SPDX-FileCopyrightText: (c) 2026 Daniel Delgado Ramos
 *
 * El panel «Quién puede consultar» de cada conexión (NEG-06): traducción del inglés
 * (`en.ts`), que es la fuente de verdad.
 */
import type { AppCatalog } from '@sytue/i18n';
import type { en } from './en.js';

export const bg: AppCatalog<typeof en> = {
  'acceso.boton': 'Кой може да изпраща заявки',
  'acceso.titulo': 'Кой може да изпраща заявки – {nombre}',
  'acceso.ayuda':
    'Заявките се изпращат с идентификационните данни на организацията ви. Администрацията и авторът на връзката могат винаги да изпращат заявки; добавете хората и групите, на които също им е нужно.',
  'acceso.vacia':
    'Никой друг: заявки могат да изпращат само администрацията и авторът на връзката.',
  'acceso.persona': 'Лице',
  'acceso.grupo': 'Група',
  'acceso.quitar': 'Премахване на {nombre}',
  'acceso.anadirPersona': 'Добавяне на лице',
  'acceso.buscar': 'Име или имейл',
  'acceso.sinCandidatos': 'Няма резултати.',
  'acceso.anadirGrupo': 'Добавяне на група',
  'acceso.elegirGrupo': 'Изберете група',
  'acceso.sinGrupos': 'Организацията ви няма групи.',
  'acceso.anadir': 'Добавяне',
  'acceso.guardar': 'Запазване',
  'acceso.cancelar': 'Отказ',
  'acceso.guardada': 'Списъкът е запазен.',
  'acceso.error': 'Списъкът не можа да бъде запазен.',
  'acceso.errorCarga': 'Списъкът не можа да бъде зареден.',
  'acceso.cargando': 'Зареждане…',
};
