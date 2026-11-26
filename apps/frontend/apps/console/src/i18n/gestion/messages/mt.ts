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

export const mt: AppCatalog<typeof en> = {
  'gestion.acceso.titulo': "X'jista' jagħmel, u għaliex",
  'gestion.acceso.ver': 'Uri',
  'gestion.acceso.error': 'L-aċċess effettiv ma setax jitgħabba.',
  'gestion.acceso.ayuda':
    'Jikkalkulah Sytue ID bl-istess regola li juża meta jawtorizza. Kull permess juri minn fejn ġej; jekk tneħħi dak is-sors, iċċekkja l-oħrajn elenkati hawn.',
  'gestion.acceso.sinPermisos': "L-ebda permess ta' amministrazzjoni.",
  'gestion.acceso.fuente.integrado': 'Rwol integrat: {role}',
  'gestion.acceso.fuente.rol': 'Rwol: {name}',
  'gestion.acceso.fuente.elevacion': 'Elevat għal {name} sa {when}',
  'gestion.acceso.pendientes': 'Elevazzjonijiet jistennew l-approvazzjoni',
  'gestion.acceso.pendiente': '{role}, mitlub fi {when}',
  'gestion.acceso.grupos': 'Gruppi',
  'gestion.acceso.sinGrupos': "Mhux f'ebda grupp.",
  'gestion.acceso.calculado': 'Ikkalkulat fi {when}',
  'gestion.sesiones.titulo': 'Sessjonijiet miftuħa',
  'gestion.sesiones.ver': 'Uri',
  'gestion.sesiones.error': 'Is-sessjonijiet ma setgħux jitgħabbew.',
  'gestion.sesiones.vacia': "M'hemmx sessjonijiet miftuħa.",
  'gestion.sesiones.inicio': 'daħal fi {when}',
  'gestion.sesiones.caduca': 'jiskadi fi {when}',
  'gestion.sesiones.mfa': 'It-tieni fattur',
  'gestion.sesiones.sinMfa': 'Password biss',
  'gestion.sesiones.total': 'Sessjonijiet miftuħa: {count}',
  'gestion.bajas.titulo': "Kontijiet magħluqa li għadek tista' terġa' ġġib",
  'gestion.bajas.ayuda':
    "Kont magħluq iżomm id-dejta tiegħu għal {days} jum. Ir-restawr iġibu lura sospiż: erġa' attivah fid-direttorju biex terġa' tagħtih aċċess.",
  'gestion.bajas.cerrada': 'magħluq fi {when}',
  'gestion.bajas.motivo': 'raġuni: {reason}',
  'gestion.bajas.quedan': 'Jiem li fadal: {count}',
  'gestion.bajas.restaurar': 'Irrestawra',
  'gestion.bajas.confirmarTitulo': "Terġa' ġġib lil {name}?",
  'gestion.bajas.confirmar':
    "Il-kont jerġa' lura sospiż, bid-dejta tiegħu. Ħadd ma jista' jidħol bih sakemm terġa' tattivah.",
  'gestion.bajas.restaurada': "{name} reġa' nġieb",
  'gestion.bajas.reactivar':
    "Huwa sospiż: erġa' attivah fid-direttorju biex terġa' tagħtih aċċess.",
  'gestion.bajas.fallo': "Il-kont ma setax jerġa' jinġieb",
  'gestion.directorio.exportar': 'Esporta CSV',
  'gestion.dispositivos.buscar': 'Fittex persuna, apparat jew IP',
  'gestion.dispositivos.confianza': 'Iffiltra skont il-fiduċja',
  'gestion.dispositivos.todos': 'L-apparati kollha',
  'gestion.dispositivos.recuento': 'Qed jintwerew {count} minn {total}',
  'gestion.dispositivos.sinResultados': 'L-ebda apparat ma jaqbel.',
  'gestion.auditoria.rango': 'Medda malajr',
  'gestion.auditoria.hoy': 'Illum',
  'gestion.auditoria.7d': '7 ijiem',
  'gestion.auditoria.30d': '30 jum',
  'gestion.roles.duplicar': 'Idduplika',
  'gestion.roles.copia': '{name} (kopja)',
  'gestion.lotes.titulo': 'Azzjonijiet bl-ingrossa riċenti',
  'gestion.lotes.ayuda':
    "X'ġie applikat lil diversi persuni f'daqqa, u lil min ma setax jiġi applikat.",
  'gestion.lotes.ver': 'Uri',
  'gestion.lotes.error': 'L-azzjonijiet bl-ingrossa ma setgħux jitgħabbew.',
  'gestion.lotes.vacio': "Għad m'hemmx azzjonijiet bl-ingrossa.",
  'gestion.lotes.progreso': '{done} minn {total}',
  'gestion.lotes.accion.setRole': 'Ibdel ir-rwol',
  'gestion.lotes.estado.ejecutando': 'Għaddej',
  'gestion.lotes.estado.terminado': 'Lest',
  'gestion.lotes.estado.cancelado': 'Ikkanċellat',
  'gestion.lotes.estado.interrumpido': 'Interrott',
  'gestion.lotes.detalle': 'Dettalji',
  'gestion.lotes.todosBien': 'Ġie applikat għal kulħadd.',
  'gestion.lote.fallos': 'Ma ġiex applikat għal',
  'gestion.lote.cerrar': 'Agħlaq',
  'gestion.lote.codigo.not_found': "mhux f'din l-organizzazzjoni",
  'gestion.lote.codigo.self': 'int stess',
  'gestion.lote.codigo.forbidden': "biex tibdel kont ta' sid trid drittijiet ta' sid",
  'gestion.lote.codigo.deleted': 'il-kont huwa magħluq',
  'gestion.lote.codigo.last_owner': 'l-aħħar sid tal-organizzazzjoni',
  'gestion.lote.codigo.last_admin': 'l-aħħar amministratur tal-organizzazzjoni',
  'gestion.lote.codigo.error': "żball mhux mistenni; erġa' pprova",
  'gestion.lote.seleccionarVisibles': 'Agħżel dawk kollha viżibbli ({count})',
  'gestion.lote.recortado':
    'Azzjoni bl-ingrossa tkopri l-iktar {max} persuna: intgħażlu l-ewwel {max} viżibbli.',
};
