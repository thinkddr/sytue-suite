/*
 * SPDX-License-Identifier: Apache-2.0
 * SPDX-FileCopyrightText: (c) 2026 Daniel Delgado Ramos
 *
 * Las herramientas de gestión de personas de Sytue Admin (acceso efectivo,
 * sesiones, bajas recuperables, lotes, export del directorio, dispositivos,
 * auditoría y roles): traducción del inglés (`en.ts`), que es la fuente de verdad.
 */
import type { AppCatalog } from '@sytue/i18n';
import type { en } from './en.js';

export const ga: AppCatalog<typeof en> = {
  'gestion.acceso.titulo': 'Cad is féidir leo a dhéanamh, agus cén fáth',
  'gestion.acceso.ver': 'Taispeáin',
  'gestion.acceso.error': 'Níorbh fhéidir an rochtain éifeachtach a lódáil.',
  'gestion.acceso.ayuda':
    'Ríomhann Sytue ID é leis an riail chéanna a chuireann sé i bhfeidhm agus é ag údarú. Taispeánann gach cead cá as a dtagann sé; má bhaineann tú an fhoinse sin, seiceáil na cinn eile atá liostaithe anseo.',
  'gestion.acceso.sinPermisos': 'Gan aon cheadanna riaracháin.',
  'gestion.acceso.fuente.integrado': 'Ról ionsuite: {role}',
  'gestion.acceso.fuente.rol': 'Ról: {name}',
  'gestion.acceso.fuente.elevacion': 'Ardaithe go {name} go dtí {when}',
  'gestion.acceso.pendientes': 'Ardaithe ag fanacht le ceadú',
  'gestion.acceso.pendiente': '{role}, iarrtha {when}',
  'gestion.acceso.grupos': 'Grúpaí',
  'gestion.acceso.sinGrupos': 'Níl i ngrúpa ar bith.',
  'gestion.acceso.calculado': 'Ríofa {when}',
  'gestion.sesiones.titulo': 'Seisiúin oscailte',
  'gestion.sesiones.ver': 'Taispeáin',
  'gestion.sesiones.error': 'Níorbh fhéidir na seisiúin a lódáil.',
  'gestion.sesiones.vacia': 'Níl aon seisiún oscailte.',
  'gestion.sesiones.inicio': 'logáilte isteach {when}',
  'gestion.sesiones.caduca': 'rachaidh in éag {when}',
  'gestion.sesiones.mfa': 'Dara fachtóir',
  'gestion.sesiones.sinMfa': 'Pasfhocal amháin',
  'gestion.sesiones.total': 'Seisiúin oscailte: {count}',
  'gestion.bajas.titulo': 'Cuntais dúnta is féidir leat a athchóiriú',
  'gestion.bajas.ayuda':
    'Coimeádann cuntas dúnta a shonraí ar feadh {days} lá. Tugann athchóiriú ar ais é ar fionraí: athghníomhachtaigh san eolaire é chun rochtain a thabhairt ar ais.',
  'gestion.bajas.cerrada': 'dúnta {when}',
  'gestion.bajas.motivo': 'cúis: {reason}',
  'gestion.bajas.quedan': 'Laethanta fágtha: {count}',
  'gestion.bajas.restaurar': 'Athchóirigh',
  'gestion.bajas.confirmarTitulo': '{name} a athchóiriú?',
  'gestion.bajas.confirmar':
    'Tagann an cuntas ar ais ar fionraí, lena shonraí. Ní féidir le haon duine logáil isteach leis go dtí go n-athghníomhachtaíonn tú é.',
  'gestion.bajas.restaurada': '{name} athchóirithe',
  'gestion.bajas.reactivar':
    'Tá sé ar fionraí: athghníomhachtaigh san eolaire é chun rochtain a thabhairt ar ais.',
  'gestion.bajas.fallo': 'Níorbh fhéidir an cuntas a athchóiriú',
  'gestion.directorio.exportar': 'Easpórtáil CSV',
  'gestion.dispositivos.buscar': 'Cuardaigh duine, gléas nó IP',
  'gestion.dispositivos.confianza': 'Scag de réir iontaobhais',
  'gestion.dispositivos.todos': 'Gach gléas',
  'gestion.dispositivos.recuento': '{count} as {total} á dtaispeáint',
  'gestion.dispositivos.sinResultados': 'Níl aon ghléas ag teacht leis.',
  'gestion.auditoria.rango': 'Raon tapa',
  'gestion.auditoria.hoy': 'Inniu',
  'gestion.auditoria.7d': '7 lá',
  'gestion.auditoria.30d': '30 lá',
  'gestion.roles.duplicar': 'Dúblaigh',
  'gestion.roles.copia': '{name} (cóip)',
  'gestion.lotes.titulo': 'Gníomhartha bulc le déanaí',
  'gestion.lotes.ayuda':
    'An méid a cuireadh i bhfeidhm ar roinnt daoine ag an am céanna, agus cé nárbh fhéidir é a chur i bhfeidhm orthu.',
  'gestion.lotes.ver': 'Taispeáin',
  'gestion.lotes.error': 'Níorbh fhéidir na gníomhartha bulc a lódáil.',
  'gestion.lotes.vacio': 'Níl aon ghníomhartha bulc fós.',
  'gestion.lotes.progreso': '{done} as {total}',
  'gestion.lotes.accion.setRole': 'Athraigh ról',
  'gestion.lotes.estado.ejecutando': 'Ag rith',
  'gestion.lotes.estado.terminado': 'Críochnaithe',
  'gestion.lotes.estado.cancelado': 'Cealaithe',
  'gestion.lotes.estado.interrumpido': 'Briste isteach air',
  'gestion.lotes.detalle': 'Sonraí',
  'gestion.lotes.todosBien': 'Curtha i bhfeidhm ar gach duine.',
  'gestion.lote.fallos': 'Níor cuireadh i bhfeidhm ar',
  'gestion.lote.cerrar': 'Dún',
  'gestion.lote.codigo.not_found': 'níl san eagraíocht seo',
  'gestion.lote.codigo.self': 'is tusa atá ann',
  'gestion.lote.codigo.forbidden': 'ní féidir ach le húinéir úinéir a athrú',
  'gestion.lote.codigo.deleted': 'tá an cuntas dúnta',
  'gestion.lote.codigo.last_owner': 'an t-úinéir deireanach den eagraíocht',
  'gestion.lote.codigo.last_admin': 'an riarthóir deireanach den eagraíocht',
  'gestion.lote.codigo.error': 'earráid gan choinne; bain triail eile as',
  'gestion.lote.seleccionarVisibles': 'Roghnaigh gach ceann infheicthe ({count})',
  'gestion.lote.recortado':
    'Clúdaíonn gníomh bulc {max} duine ar a mhéad: roghnaíodh an chéad {max} atá infheicthe.',
};
