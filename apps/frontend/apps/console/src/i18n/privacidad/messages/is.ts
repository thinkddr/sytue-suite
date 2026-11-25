/*
 * SPDX-License-Identifier: Apache-2.0
 * SPDX-FileCopyrightText: (c) 2026 Daniel Delgado Ramos
 *
 * El registro de solicitudes RGPD y la supresión física de una cuenta: traducción
 * del inglés (`en.ts`), que es la fuente de verdad.
 */
import type { AppCatalog } from '@sytue/i18n';
import type { en } from './en.js';

export const is: AppCatalog<typeof en> = {
  'privacidad.titulo': 'Persónuverndarbeiðnir',
  'privacidad.descripcion':
    'Það sem fólk biður fyrirtækið þitt um varðandi persónuupplýsingar sínar (GDPR, 15. til 21. grein). Skráðu hverja beiðni, svaraðu innan mánaðar og framkvæmdu eyðingu hér.',
  'privacidad.registrar': 'Skrá beiðni',
  'privacidad.filtro': 'Sýna',
  'privacidad.filtro.abiertas': 'Opnar',
  'privacidad.filtro.vencidas': 'Fram yfir frest',
  'privacidad.filtro.todas': 'Allar',
  'privacidad.resumen.abiertas': 'Opnar beiðnir',
  'privacidad.resumen.vencidas': 'Frestur liðinn',
  'privacidad.vacia': 'Engar beiðnir til að sýna.',
  'privacidad.errorCarga': 'Ekki tókst að hlaða skránni.',
  'privacidad.col.persona': 'Einstaklingur',
  'privacidad.col.tipo': 'Beiðni',
  'privacidad.col.recibida': 'Móttekin',
  'privacidad.col.plazo': 'Frestur',
  'privacidad.col.estado': 'Staða',
  'privacidad.tipo.access': 'Aðgangur að eigin gögnum',
  'privacidad.tipo.rectification': 'Leiðrétting',
  'privacidad.tipo.erasure': 'Eyðing',
  'privacidad.tipo.restriction': 'Takmörkun vinnslu',
  'privacidad.tipo.portability': 'Flutningur gagna',
  'privacidad.tipo.objection': 'Andmæli',
  'privacidad.canal.email': 'Tölvupóstur',
  'privacidad.canal.web': 'Vefeyðublað',
  'privacidad.canal.letter': 'Bréf',
  'privacidad.canal.phone': 'Sími',
  'privacidad.canal.in_person': 'Í eigin persónu',
  'privacidad.canal.other': 'Annað',
  'privacidad.estado.received': 'Móttekin',
  'privacidad.estado.in_progress': 'Í vinnslu',
  'privacidad.estado.completed': 'Svarað',
  'privacidad.estado.rejected': 'Hafnað',
  'privacidad.plazo.vencida': 'Fram yfir frest',
  'privacidad.plazo.pronto': 'Frestur að renna út',
  'privacidad.plazo.prorrogada': 'Framlengd',
  'privacidad.alta.quien': 'Hver spyr',
  'privacidad.alta.miembro': 'Einhver innan fyrirtækisins',
  'privacidad.alta.fuera': 'Einhver utan þess',
  'privacidad.alta.elegirMiembro': 'Veldu einstaklinginn',
  'privacidad.alta.correo': 'Netfang viðkomandi',
  'privacidad.alta.nombre': 'Nafn viðkomandi (valfrjálst)',
  'privacidad.alta.tipo': 'Hvað er beðið um',
  'privacidad.alta.canal': 'Hvernig hún barst',
  'privacidad.alta.recibida': 'Móttökudagur',
  'privacidad.alta.recibidaAyuda':
    'Mánaðarfresturinn reiknast frá þessum degi, ekki frá deginum í dag.',
  'privacidad.alta.identidad': 'Auðkenni staðfest',
  'privacidad.alta.metodo': 'Hvernig það var staðfest',
  'privacidad.alta.notas': 'Athugasemdir',
  'privacidad.alta.error.subject': 'Tilgreindu hver leggur fram beiðnina.',
  'privacidad.alta.error.email': 'Sláðu inn gilt netfang.',
  'privacidad.alta.error.date':
    'Dagsetningin verður að vera innan síðasta árs og ekki í framtíðinni.',
  'privacidad.alta.error.upstream': 'Ekki tókst að skrá beiðnina.',
  'privacidad.alta.hecha': 'Beiðnin hefur verið skráð.',
  'privacidad.detalle.plazo': 'Svara fyrir {fecha}',
  'privacidad.detalle.resolucion': 'Svar sem einstaklingnum var gefið',
  'privacidad.detalle.resolucionAyuda':
    'Nauðsynlegt til að loka beiðninni: þetta er það sem einstaklingnum er sagt.',
  'privacidad.detalle.guardar': 'Vista',
  'privacidad.detalle.guardado': 'Beiðnin hefur verið uppfærð.',
  'privacidad.detalle.cerrada': 'Þessari beiðni hefur verið lokað og henni verður ekki breytt.',
  'privacidad.detalle.prorroga': 'Framlengja um tvo mánuði',
  'privacidad.detalle.prorrogaAyuda':
    'Einu sinni og áður en fresturinn rennur út. Segja þarf einstaklingnum ástæðuna: skrifaðu hana hér.',
  'privacidad.detalle.prorrogar': 'Framlengja',
  'privacidad.detalle.historia': 'Ferill',
  'privacidad.evento.created': 'Skráð',
  'privacidad.evento.updated': 'Uppfærð',
  'privacidad.evento.extended': 'Frestur framlengdur',
  'privacidad.evento.erasureStarted': 'Eyðing hafin',
  'privacidad.evento.erasureCompleted': 'Eyðingu lokið',
  'privacidad.supresion.titulo': 'Eyða aðganginum',
  'privacidad.supresion.explica':
    'Aðganginum er lokað strax og óafturkræft og innskráningarleiðum hans er eytt. Innan fárra mínútna er prófíllinn gerður nafnlaus og því eytt sem aðrar þjónustur geyma um einstaklinginn. Það sem einstaklingurinn bjó til fyrir fyrirtækið (pósthólf, skrár, viðburðir) verður áfram hjá fyrirtækinu.',
  'privacidad.supresion.confirmar': 'Sláðu inn {correo} til að staðfesta',
  'privacidad.supresion.iniciada': 'Eyðing hafin. Henni lýkur innan fárra mínútna.',
  'privacidad.supresion.enCurso': 'Eyðing er í gangi.',
  'privacidad.supresion.hecha': 'Aðganginum hefur verið eytt.',
  'privacidad.supresion.sinCuenta':
    'Einstaklingurinn er ekki með aðgang hjá fyrirtækinu: hér er enginn aðgangur til að eyða.',
  'privacidad.supresion.noMiembro': 'Einstaklingurinn er ekki lengur meðlimur í fyrirtækinu.',
  'privacidad.supresion.error': 'Ekki tókst að eyða aðganginum.',
  'privacidad.error.upstream': 'Ekki tókst að vista breytinguna.',
  'privacidad.cancelar': 'Hætta við',
};
