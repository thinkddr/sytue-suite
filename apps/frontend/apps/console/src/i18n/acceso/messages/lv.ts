/*
 * SPDX-License-Identifier: Apache-2.0
 * SPDX-FileCopyrightText: (c) 2026 Daniel Delgado Ramos
 *
 * El panel «Quién puede consultar» de cada conexión (NEG-06): traducción del inglés
 * (`en.ts`), que es la fuente de verdad.
 */
import type { AppCatalog } from '@sytue/i18n';
import type { en } from './en.js';

export const lv: AppCatalog<typeof en> = {
  'acceso.boton': 'Kas drīkst vaicāt',
  'acceso.titulo': 'Kas drīkst vaicāt – {nombre}',
  'acceso.ayuda':
    'Vaicājumi tiek sūtīti ar jūsu organizācijas pieteikšanās datiem. Administratori un savienojuma autors var vaicāt vienmēr; pievienojiet cilvēkus un grupas, kam tas arī vajadzīgs.',
  'acceso.vacia': 'Neviens cits: vaicāt drīkst tikai administratori un savienojuma autors.',
  'acceso.persona': 'Persona',
  'acceso.grupo': 'Grupa',
  'acceso.quitar': 'Noņemt {nombre}',
  'acceso.anadirPersona': 'Pievienot personu',
  'acceso.buscar': 'Vārds vai e-pasts',
  'acceso.sinCandidatos': 'Nav rezultātu.',
  'acceso.anadirGrupo': 'Pievienot grupu',
  'acceso.elegirGrupo': 'Izvēlieties grupu',
  'acceso.sinGrupos': 'Jūsu organizācijai nav grupu.',
  'acceso.anadir': 'Pievienot',
  'acceso.guardar': 'Saglabāt',
  'acceso.cancelar': 'Atcelt',
  'acceso.guardada': 'Saraksts saglabāts.',
  'acceso.error': 'Sarakstu neizdevās saglabāt.',
  'acceso.errorCarga': 'Sarakstu neizdevās ielādēt.',
  'acceso.cargando': 'Notiek ielāde…',
};
