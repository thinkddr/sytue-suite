/*
 * SPDX-License-Identifier: Apache-2.0
 * SPDX-FileCopyrightText: (c) 2026 Daniel Delgado Ramos
 *
 * El panel «Quién puede consultar» de cada conexión (NEG-06): traducción del inglés
 * (`en.ts`), que es la fuente de verdad.
 */
import type { AppCatalog } from '@sytue/i18n';
import type { en } from './en.js';

export const lt: AppCatalog<typeof en> = {
  'acceso.boton': 'Kas gali siųsti užklausas',
  'acceso.titulo': 'Kas gali siųsti užklausas – {nombre}',
  'acceso.ayuda':
    'Užklausos siunčiamos su jūsų organizacijos prisijungimo duomenimis. Administratoriai ir ryšio autorius gali siųsti užklausas visada; pridėkite žmones ir grupes, kuriems to irgi reikia.',
  'acceso.vacia': 'Daugiau niekas: užklausas gali siųsti tik administratoriai ir ryšio autorius.',
  'acceso.persona': 'Asmuo',
  'acceso.grupo': 'Grupė',
  'acceso.quitar': 'Pašalinti {nombre}',
  'acceso.anadirPersona': 'Pridėti asmenį',
  'acceso.buscar': 'Vardas arba el. paštas',
  'acceso.sinCandidatos': 'Rezultatų nėra.',
  'acceso.anadirGrupo': 'Pridėti grupę',
  'acceso.elegirGrupo': 'Pasirinkite grupę',
  'acceso.sinGrupos': 'Jūsų organizacija neturi grupių.',
  'acceso.anadir': 'Pridėti',
  'acceso.guardar': 'Įrašyti',
  'acceso.cancelar': 'Atšaukti',
  'acceso.guardada': 'Sąrašas įrašytas.',
  'acceso.error': 'Nepavyko įrašyti sąrašo.',
  'acceso.errorCarga': 'Nepavyko įkelti sąrašo.',
  'acceso.cargando': 'Įkeliama…',
};
