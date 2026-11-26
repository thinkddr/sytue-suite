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

export const da: AppCatalog<typeof en> = {
  'gestion.acceso.titulo': 'Hvad personen må, og hvorfor',
  'gestion.acceso.ver': 'Vis',
  'gestion.acceso.error': 'Den faktiske adgang kunne ikke indlæses.',
  'gestion.acceso.ayuda':
    'Beregnes af Sytue ID med samme regel, som bruges ved godkendelse. Hver tilladelse viser, hvor den kommer fra; fjerner du kilden, så tjek de andre, der står her.',
  'gestion.acceso.sinPermisos': 'Ingen administrationstilladelser.',
  'gestion.acceso.fuente.integrado': 'Indbygget rolle: {role}',
  'gestion.acceso.fuente.rol': 'Rolle: {name}',
  'gestion.acceso.fuente.elevacion': 'Hævet til {name} indtil {when}',
  'gestion.acceso.pendientes': 'Hævninger, der afventer godkendelse',
  'gestion.acceso.pendiente': '{role}, anmodet {when}',
  'gestion.acceso.grupos': 'Grupper',
  'gestion.acceso.sinGrupos': 'Er ikke med i nogen gruppe.',
  'gestion.acceso.calculado': 'Beregnet {when}',
  'gestion.sesiones.titulo': 'Åbne sessioner',
  'gestion.sesiones.ver': 'Vis',
  'gestion.sesiones.error': 'Sessionerne kunne ikke indlæses.',
  'gestion.sesiones.vacia': 'Ingen åbne sessioner.',
  'gestion.sesiones.inicio': 'logget ind {when}',
  'gestion.sesiones.caduca': 'udløber {when}',
  'gestion.sesiones.mfa': 'Anden faktor',
  'gestion.sesiones.sinMfa': 'Kun adgangskode',
  'gestion.sesiones.total': 'Åbne sessioner: {count}',
  'gestion.bajas.titulo': 'Lukkede konti, du stadig kan gendanne',
  'gestion.bajas.ayuda':
    'En lukket konto beholder sine data i {days} dage. Gendannelse bringer den tilbage suspenderet: genaktivér den i kataloget for at give adgang igen.',
  'gestion.bajas.cerrada': 'lukket {when}',
  'gestion.bajas.motivo': 'årsag: {reason}',
  'gestion.bajas.quedan': 'Dage tilbage: {count}',
  'gestion.bajas.restaurar': 'Gendan',
  'gestion.bajas.confirmarTitulo': 'Gendan {name}?',
  'gestion.bajas.confirmar':
    'Kontoen kommer tilbage suspenderet med sine data. Ingen kan logge ind med den, før du genaktiverer den.',
  'gestion.bajas.restaurada': '{name} gendannet',
  'gestion.bajas.reactivar':
    'Den er suspenderet: genaktivér den i kataloget for at give adgang igen.',
  'gestion.bajas.fallo': 'Kontoen kunne ikke gendannes',
  'gestion.directorio.exportar': 'Eksportér CSV',
  'gestion.dispositivos.buscar': 'Søg person, enhed eller IP',
  'gestion.dispositivos.confianza': 'Filtrér efter tillid',
  'gestion.dispositivos.todos': 'Alle enheder',
  'gestion.dispositivos.recuento': 'Viser {count} af {total}',
  'gestion.dispositivos.sinResultados': 'Ingen enheder matcher.',
  'gestion.auditoria.rango': 'Hurtig periode',
  'gestion.auditoria.hoy': 'I dag',
  'gestion.auditoria.7d': '7 dage',
  'gestion.auditoria.30d': '30 dage',
  'gestion.roles.duplicar': 'Duplikér',
  'gestion.roles.copia': '{name} (kopi)',
  'gestion.lotes.titulo': 'Seneste massehandlinger',
  'gestion.lotes.ayuda':
    'Hvad der blev anvendt på flere personer på én gang, og hvem det ikke kunne anvendes på.',
  'gestion.lotes.ver': 'Vis',
  'gestion.lotes.error': 'Massehandlingerne kunne ikke indlæses.',
  'gestion.lotes.vacio': 'Ingen massehandlinger endnu.',
  'gestion.lotes.progreso': '{done} af {total}',
  'gestion.lotes.accion.setRole': 'Skift rolle',
  'gestion.lotes.estado.ejecutando': 'Kører',
  'gestion.lotes.estado.terminado': 'Afsluttet',
  'gestion.lotes.estado.cancelado': 'Annulleret',
  'gestion.lotes.estado.interrumpido': 'Afbrudt',
  'gestion.lotes.detalle': 'Detaljer',
  'gestion.lotes.todosBien': 'Anvendt på alle.',
  'gestion.lote.fallos': 'Ikke anvendt på',
  'gestion.lote.cerrar': 'Luk',
  'gestion.lote.codigo.not_found': 'er ikke i denne organisation',
  'gestion.lote.codigo.self': 'det er dig',
  'gestion.lote.codigo.forbidden': 'kun en ejer kan ændre en ejer',
  'gestion.lote.codigo.deleted': 'kontoen er lukket',
  'gestion.lote.codigo.last_owner': 'er organisationens sidste ejer',
  'gestion.lote.codigo.last_admin': 'er organisationens sidste administrator',
  'gestion.lote.codigo.error': 'uventet fejl; prøv igen',
  'gestion.lote.seleccionarVisibles': 'Vælg alle synlige ({count})',
  'gestion.lote.recortado':
    'En massehandling omfatter højst {max} personer: de første {max} synlige er valgt.',
};
