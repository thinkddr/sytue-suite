/*
 * SPDX-License-Identifier: Apache-2.0
 * SPDX-FileCopyrightText: (c) 2026 Daniel Delgado Ramos
 *
 * El panel «Quién puede consultar» de cada conexión (NEG-06): traducción del inglés
 * (`en.ts`), que es la fuente de verdad.
 */
import type { AppCatalog } from '@sytue/i18n';
import type { en } from './en.js';

export const cs: AppCatalog<typeof en> = {
  'acceso.boton': 'Kdo se smí dotazovat',
  'acceso.titulo': 'Kdo se smí dotazovat – {nombre}',
  'acceso.ayuda':
    'Dotazy odcházejí s přihlašovacími údaji vaší organizace. Správci a autor připojení se mohou dotazovat vždy; přidejte lidi a skupiny, které to potřebují také.',
  'acceso.vacia': 'Nikdo další: dotazovat se mohou jen správci a autor připojení.',
  'acceso.persona': 'Osoba',
  'acceso.grupo': 'Skupina',
  'acceso.quitar': 'Odebrat {nombre}',
  'acceso.anadirPersona': 'Přidat osobu',
  'acceso.buscar': 'Jméno nebo e-mail',
  'acceso.sinCandidatos': 'Žádné výsledky.',
  'acceso.anadirGrupo': 'Přidat skupinu',
  'acceso.elegirGrupo': 'Vyberte skupinu',
  'acceso.sinGrupos': 'Vaše organizace nemá žádné skupiny.',
  'acceso.anadir': 'Přidat',
  'acceso.guardar': 'Uložit',
  'acceso.cancelar': 'Zrušit',
  'acceso.guardada': 'Seznam uložen.',
  'acceso.error': 'Seznam se nepodařilo uložit.',
  'acceso.errorCarga': 'Seznam se nepodařilo načíst.',
  'acceso.cargando': 'Načítání…',
};
