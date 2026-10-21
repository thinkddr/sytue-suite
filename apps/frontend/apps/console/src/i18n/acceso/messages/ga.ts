/*
 * SPDX-License-Identifier: Apache-2.0
 * SPDX-FileCopyrightText: (c) 2026 Daniel Delgado Ramos
 *
 * El panel «Quién puede consultar» de cada conexión (NEG-06): traducción del inglés
 * (`en.ts`), que es la fuente de verdad.
 */
import type { AppCatalog } from '@sytue/i18n';
import type { en } from './en.js';

export const ga: AppCatalog<typeof en> = {
  'acceso.boton': 'Cé atá in ann fiosrú',
  'acceso.titulo': 'Cé atá in ann fiosrú – {nombre}',
  'acceso.ayuda':
    'Imíonn fiosruithe le dintiúir d’eagraíochta. Is féidir le lucht riaracháin agus leis an duine a chruthaigh an ceangal é a fhiosrú i gcónaí; cuir leis na daoine agus na grúpaí a bhfuil sé de dhíth orthu freisin.',
  'acceso.vacia':
    'Níl aon duine eile ann: ní féidir ach le lucht riaracháin agus leis an duine a chruthaigh an ceangal é a fhiosrú.',
  'acceso.persona': 'Duine',
  'acceso.grupo': 'Grúpa',
  'acceso.quitar': 'Bain {nombre}',
  'acceso.anadirPersona': 'Cuir duine leis',
  'acceso.buscar': 'Ainm nó ríomhphost',
  'acceso.sinCandidatos': 'Gan torthaí.',
  'acceso.anadirGrupo': 'Cuir grúpa leis',
  'acceso.elegirGrupo': 'Roghnaigh grúpa',
  'acceso.sinGrupos': 'Níl aon ghrúpa ag d’eagraíocht.',
  'acceso.anadir': 'Cuir leis',
  'acceso.guardar': 'Sábháil',
  'acceso.cancelar': 'Cealaigh',
  'acceso.guardada': 'Sábháladh an liosta.',
  'acceso.error': 'Níorbh fhéidir an liosta a shábháil.',
  'acceso.errorCarga': 'Níorbh fhéidir an liosta a lódáil.',
  'acceso.cargando': 'Á lódáil…',
};
