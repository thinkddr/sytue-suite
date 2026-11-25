/*
 * SPDX-License-Identifier: Apache-2.0
 * SPDX-FileCopyrightText: (c) 2026 Daniel Delgado Ramos
 *
 * El registro de solicitudes RGPD y la supresión física de una cuenta: traducción
 * del inglés (`en.ts`), que es la fuente de verdad.
 */
import type { AppCatalog } from '@sytue/i18n';
import type { en } from './en.js';

export const ga: AppCatalog<typeof en> = {
  'privacidad.titulo': 'Iarratais phríobháideachais',
  'privacidad.descripcion':
    'An méid a iarrann daoine ar d’eagraíocht maidir lena sonraí pearsanta (RGCS, Airteagail 15 go 21). Cláraigh gach iarratas, freagair laistigh de mhí agus, más léirscriosadh é, déan anseo é.',
  'privacidad.registrar': 'Cláraigh iarratas',
  'privacidad.filtro': 'Taispeáin',
  'privacidad.filtro.abiertas': 'Oscailte',
  'privacidad.filtro.vencidas': 'Thar téarma',
  'privacidad.filtro.todas': 'Gach ceann',
  'privacidad.resumen.abiertas': 'Iarratais oscailte',
  'privacidad.resumen.vencidas': 'Thar an spriocdháta',
  'privacidad.vacia': 'Níl aon iarratas le taispeáint.',
  'privacidad.errorCarga': 'Níorbh fhéidir an clár a lódáil.',
  'privacidad.col.persona': 'Duine',
  'privacidad.col.tipo': 'Iarratas',
  'privacidad.col.recibida': 'Faighte',
  'privacidad.col.plazo': 'Spriocdháta',
  'privacidad.col.estado': 'Stádas',
  'privacidad.tipo.access': 'Rochtain ar a sonraí',
  'privacidad.tipo.rectification': 'Ceartú',
  'privacidad.tipo.erasure': 'Léirscriosadh',
  'privacidad.tipo.restriction': 'Srian ar phróiseáil',
  'privacidad.tipo.portability': 'Iniomparthacht sonraí',
  'privacidad.tipo.objection': 'Agóid',
  'privacidad.canal.email': 'Ríomhphost',
  'privacidad.canal.web': 'Foirm ghréasáin',
  'privacidad.canal.letter': 'Litir',
  'privacidad.canal.phone': 'Teileafón',
  'privacidad.canal.in_person': 'Go pearsanta',
  'privacidad.canal.other': 'Eile',
  'privacidad.estado.received': 'Faighte',
  'privacidad.estado.in_progress': 'Ar siúl',
  'privacidad.estado.completed': 'Freagartha',
  'privacidad.estado.rejected': 'Diúltaithe',
  'privacidad.plazo.vencida': 'Thar téarma',
  'privacidad.plazo.pronto': 'Ag dul in éag go luath',
  'privacidad.plazo.prorrogada': 'Síneadh',
  'privacidad.alta.quien': 'Cé atá ag iarraidh',
  'privacidad.alta.miembro': 'Duine san eagraíocht',
  'privacidad.alta.fuera': 'Duine lasmuigh di',
  'privacidad.alta.elegirMiembro': 'Roghnaigh an duine',
  'privacidad.alta.correo': 'A seoladh ríomhphoist',
  'privacidad.alta.nombre': 'A ainm (roghnach)',
  'privacidad.alta.tipo': 'Cad atá á iarraidh',
  'privacidad.alta.canal': 'Conas a tháinig sé',
  'privacidad.alta.recibida': 'Dáta a fuarthas é',
  'privacidad.alta.recibidaAyuda': 'Áirítear an téarma míosa ón dáta seo, ní ón lá inniu.',
  'privacidad.alta.identidad': 'Céannacht seiceáilte',
  'privacidad.alta.metodo': 'Conas a seiceáladh í',
  'privacidad.alta.notas': 'Nótaí',
  'privacidad.alta.error.subject': 'Abair cé atá ag déanamh an iarratais.',
  'privacidad.alta.error.email': 'Cuir isteach seoladh ríomhphoist bailí.',
  'privacidad.alta.error.date':
    'Caithfidh an dáta a bheith laistigh den bhliain seo caite agus gan a bheith sa todhchaí.',
  'privacidad.alta.error.upstream': 'Níorbh fhéidir an t-iarratas a chlárú.',
  'privacidad.alta.hecha': 'Iarratas cláraithe.',
  'privacidad.detalle.plazo': 'Freagair faoi {fecha}',
  'privacidad.detalle.resolucion': 'Freagra a tugadh don duine',
  'privacidad.detalle.resolucionAyuda':
    'Riachtanach chun an t-iarratas a dhúnadh: is é seo a insítear don duine.',
  'privacidad.detalle.guardar': 'Sábháil',
  'privacidad.detalle.guardado': 'Iarratas nuashonraithe.',
  'privacidad.detalle.cerrada': 'Tá an t-iarratas seo dúnta agus ní féidir é a athrú a thuilleadh.',
  'privacidad.detalle.prorroga': 'Cuir dhá mhí leis',
  'privacidad.detalle.prorrogaAyuda':
    'Uair amháin agus roimh an spriocdháta. Caithfear an chúis a insint don duine: scríobh anseo í.',
  'privacidad.detalle.prorrogar': 'Cuir síneadh leis',
  'privacidad.detalle.historia': 'Stair',
  'privacidad.evento.created': 'Cláraithe',
  'privacidad.evento.updated': 'Nuashonraithe',
  'privacidad.evento.extended': 'Síneadh leis an spriocdháta',
  'privacidad.evento.erasureStarted': 'Léirscriosadh tosaithe',
  'privacidad.evento.erasureCompleted': 'Léirscriosadh críochnaithe',
  'privacidad.supresion.titulo': 'Léirscrios an cuntas',
  'privacidad.supresion.explica':
    'Dúntar an cuntas láithreach agus go do-aisiompaithe, agus scriostar a mhodhanna síniú isteach. Laistigh de chúpla nóiméad déantar an phróifíl a anaithnidiú agus scriostar a gcoinníonn seirbhísí eile faoin duine. Fanann a chruthaigh an duine don eagraíocht (bosca poist, comhaid, imeachtaí) leis an eagraíocht.',
  'privacidad.supresion.confirmar': 'Clóscríobh {correo} chun deimhniú',
  'privacidad.supresion.iniciada':
    'Léirscriosadh tosaithe. Críochnóidh sé laistigh de chúpla nóiméad.',
  'privacidad.supresion.enCurso': 'Tá an léirscriosadh ar siúl.',
  'privacidad.supresion.hecha': 'Léirscriosadh an cuntas.',
  'privacidad.supresion.sinCuenta':
    'Níl cuntas ag an duine san eagraíocht: níl aon chuntas le léirscriosadh anseo.',
  'privacidad.supresion.noMiembro': 'Ní ball den eagraíocht a thuilleadh é an duine.',
  'privacidad.supresion.error': 'Níorbh fhéidir an cuntas a léirscriosadh.',
  'privacidad.error.upstream': 'Níorbh fhéidir an t-athrú a shábháil.',
  'privacidad.cancelar': 'Cealaigh',
};
