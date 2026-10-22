/*
 * SPDX-License-Identifier: Apache-2.0
 * SPDX-FileCopyrightText: (c) 2026 Daniel Delgado Ramos
 *
 * El panel «Quién puede consultar» de cada conexión (NEG-06): traducción del inglés
 * (`en.ts`), que es la fuente de verdad.
 */
import type { AppCatalog } from '@sytue/i18n';
import type { en } from './en.js';

export const sk: AppCatalog<typeof en> = {
  'acceso.boton': 'Kto sa môže dopytovať',
  'acceso.titulo': 'Kto sa môže dopytovať – {nombre}',
  'acceso.ayuda':
    'Dopyty odchádzajú s prihlasovacími údajmi vašej organizácie. Správcovia a autor pripojenia sa môžu dopytovať vždy; pridajte ľudí a skupiny, ktoré to tiež potrebujú.',
  'acceso.vacia': 'Nikto ďalší: dopytovať sa môžu len správcovia a autor pripojenia.',
  'acceso.persona': 'Osoba',
  'acceso.grupo': 'Skupina',
  'acceso.quitar': 'Odobrať {nombre}',
  'acceso.anadirPersona': 'Pridať osobu',
  'acceso.buscar': 'Meno alebo e-mail',
  'acceso.sinCandidatos': 'Žiadne výsledky.',
  'acceso.anadirGrupo': 'Pridať skupinu',
  'acceso.elegirGrupo': 'Vyberte skupinu',
  'acceso.sinGrupos': 'Vaša organizácia nemá žiadne skupiny.',
  'acceso.anadir': 'Pridať',
  'acceso.guardar': 'Uložiť',
  'acceso.cancelar': 'Zrušiť',
  'acceso.guardada': 'Zoznam uložený.',
  'acceso.error': 'Zoznam sa nepodarilo uložiť.',
  'acceso.errorCarga': 'Zoznam sa nepodarilo načítať.',
  'acceso.cargando': 'Načítava sa…',
};
