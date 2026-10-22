/*
 * SPDX-License-Identifier: Apache-2.0
 * SPDX-FileCopyrightText: (c) 2026 Daniel Delgado Ramos
 *
 * El panel «Quién puede consultar» de cada conexión (NEG-06): traducción del inglés
 * (`en.ts`), que es la fuente de verdad.
 */
import type { AppCatalog } from '@sytue/i18n';
import type { en } from './en.js';

export const pt: AppCatalog<typeof en> = {
  'acceso.boton': 'Quem pode consultar',
  'acceso.titulo': 'Quem pode consultar {nombre}',
  'acceso.ayuda':
    'As consultas saem com as credenciais da sua organização. A administração e a pessoa que criou a ligação podem sempre consultá-la; adicione as pessoas e os grupos que também precisem.',
  'acceso.vacia':
    'Mais ninguém: só a administração e a pessoa que criou a ligação a podem consultar.',
  'acceso.persona': 'Pessoa',
  'acceso.grupo': 'Grupo',
  'acceso.quitar': 'Remover {nombre}',
  'acceso.anadirPersona': 'Adicionar uma pessoa',
  'acceso.buscar': 'Nome ou e-mail',
  'acceso.sinCandidatos': 'Sem resultados.',
  'acceso.anadirGrupo': 'Adicionar um grupo',
  'acceso.elegirGrupo': 'Escolha um grupo',
  'acceso.sinGrupos': 'A sua organização não tem grupos.',
  'acceso.anadir': 'Adicionar',
  'acceso.guardar': 'Guardar',
  'acceso.cancelar': 'Cancelar',
  'acceso.guardada': 'Lista guardada.',
  'acceso.error': 'Não foi possível guardar a lista.',
  'acceso.errorCarga': 'Não foi possível carregar a lista.',
  'acceso.cargando': 'A carregar…',
};
