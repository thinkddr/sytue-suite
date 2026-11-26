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

export const sv: AppCatalog<typeof en> = {
  'gestion.acceso.titulo': 'Vad personen får göra, och varför',
  'gestion.acceso.ver': 'Visa',
  'gestion.acceso.error': 'Den faktiska behörigheten kunde inte läsas in.',
  'gestion.acceso.ayuda':
    'Beräknas av Sytue ID med samma regel som används vid auktorisering. Varje behörighet visar var den kommer ifrån; tar du bort källan, kontrollera de andra som står här.',
  'gestion.acceso.sinPermisos': 'Inga administrationsbehörigheter.',
  'gestion.acceso.fuente.integrado': 'Inbyggd roll: {role}',
  'gestion.acceso.fuente.rol': 'Roll: {name}',
  'gestion.acceso.fuente.elevacion': 'Upphöjd till {name} till {when}',
  'gestion.acceso.pendientes': 'Upphöjningar som väntar på godkännande',
  'gestion.acceso.pendiente': '{role}, begärd {when}',
  'gestion.acceso.grupos': 'Grupper',
  'gestion.acceso.sinGrupos': 'Ingår inte i någon grupp.',
  'gestion.acceso.calculado': 'Beräknad {when}',
  'gestion.sesiones.titulo': 'Öppna sessioner',
  'gestion.sesiones.ver': 'Visa',
  'gestion.sesiones.error': 'Sessionerna kunde inte läsas in.',
  'gestion.sesiones.vacia': 'Inga öppna sessioner.',
  'gestion.sesiones.inicio': 'inloggad {when}',
  'gestion.sesiones.caduca': 'går ut {when}',
  'gestion.sesiones.mfa': 'Andra faktor',
  'gestion.sesiones.sinMfa': 'Endast lösenord',
  'gestion.sesiones.total': 'Öppna sessioner: {count}',
  'gestion.bajas.titulo': 'Stängda konton som du fortfarande kan återställa',
  'gestion.bajas.ayuda':
    'Ett stängt konto behåller sina data i {days} dagar. En återställning ger tillbaka det avstängt: återaktivera det i katalogen för att ge åtkomst igen.',
  'gestion.bajas.cerrada': 'stängt {when}',
  'gestion.bajas.motivo': 'orsak: {reason}',
  'gestion.bajas.quedan': 'Dagar kvar: {count}',
  'gestion.bajas.restaurar': 'Återställ',
  'gestion.bajas.confirmarTitulo': 'Återställa {name}?',
  'gestion.bajas.confirmar':
    'Kontot kommer tillbaka avstängt, med sina data. Ingen kan logga in med det förrän du återaktiverar det.',
  'gestion.bajas.restaurada': '{name} återställt',
  'gestion.bajas.reactivar':
    'Det är avstängt: återaktivera det i katalogen för att ge åtkomst igen.',
  'gestion.bajas.fallo': 'Kontot kunde inte återställas',
  'gestion.directorio.exportar': 'Exportera CSV',
  'gestion.dispositivos.buscar': 'Sök person, enhet eller IP',
  'gestion.dispositivos.confianza': 'Filtrera efter förtroende',
  'gestion.dispositivos.todos': 'Alla enheter',
  'gestion.dispositivos.recuento': 'Visar {count} av {total}',
  'gestion.dispositivos.sinResultados': 'Ingen enhet matchar.',
  'gestion.auditoria.rango': 'Snabbval av period',
  'gestion.auditoria.hoy': 'I dag',
  'gestion.auditoria.7d': '7 dagar',
  'gestion.auditoria.30d': '30 dagar',
  'gestion.roles.duplicar': 'Duplicera',
  'gestion.roles.copia': '{name} (kopia)',
  'gestion.lotes.titulo': 'Senaste massåtgärder',
  'gestion.lotes.ayuda':
    'Vad som tillämpades på flera personer samtidigt, och vilka det inte gick för.',
  'gestion.lotes.ver': 'Visa',
  'gestion.lotes.error': 'Massåtgärderna kunde inte läsas in.',
  'gestion.lotes.vacio': 'Inga massåtgärder än.',
  'gestion.lotes.progreso': '{done} av {total}',
  'gestion.lotes.accion.setRole': 'Byt roll',
  'gestion.lotes.estado.ejecutando': 'Pågår',
  'gestion.lotes.estado.terminado': 'Klar',
  'gestion.lotes.estado.cancelado': 'Avbruten',
  'gestion.lotes.estado.interrumpido': 'Avbruten oväntat',
  'gestion.lotes.detalle': 'Detaljer',
  'gestion.lotes.todosBien': 'Tillämpat på alla.',
  'gestion.lote.fallos': 'Inte tillämpat på',
  'gestion.lote.cerrar': 'Stäng',
  'gestion.lote.codigo.not_found': 'finns inte i den här organisationen',
  'gestion.lote.codigo.self': 'det är du',
  'gestion.lote.codigo.forbidden': 'bara en ägare kan ändra en ägare',
  'gestion.lote.codigo.deleted': 'kontot är stängt',
  'gestion.lote.codigo.last_owner': 'organisationens sista ägare',
  'gestion.lote.codigo.last_admin': 'organisationens sista administratör',
  'gestion.lote.codigo.error': 'oväntat fel; försök igen',
  'gestion.lote.seleccionarVisibles': 'Markera alla synliga ({count})',
  'gestion.lote.recortado':
    'En massåtgärd omfattar högst {max} personer: de första {max} synliga är markerade.',
};
