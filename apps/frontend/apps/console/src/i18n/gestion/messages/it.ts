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

export const it: AppCatalog<typeof en> = {
  'gestion.acceso.titulo': 'Cosa può fare e perché',
  'gestion.acceso.ver': 'Mostra',
  'gestion.acceso.error': 'Impossibile caricare l’accesso effettivo.',
  'gestion.acceso.ayuda':
    'Calcolato da Sytue ID con la stessa regola che applica per autorizzare. Ogni permesso indica da dove arriva; se rimuovi quella fonte, controlla le altre elencate qui.',
  'gestion.acceso.sinPermisos': 'Nessun permesso di amministrazione.',
  'gestion.acceso.fuente.integrado': 'Ruolo integrato: {role}',
  'gestion.acceso.fuente.rol': 'Ruolo: {name}',
  'gestion.acceso.fuente.elevacion': 'Elevato a {name} fino al {when}',
  'gestion.acceso.pendientes': 'Elevazioni in attesa di approvazione',
  'gestion.acceso.pendiente': '{role}, richiesta il {when}',
  'gestion.acceso.grupos': 'Gruppi',
  'gestion.acceso.sinGrupos': 'Non è in nessun gruppo.',
  'gestion.acceso.calculado': 'Calcolato il {when}',
  'gestion.sesiones.titulo': 'Sessioni aperte',
  'gestion.sesiones.ver': 'Mostra',
  'gestion.sesiones.error': 'Impossibile caricare le sessioni.',
  'gestion.sesiones.vacia': 'Nessuna sessione aperta.',
  'gestion.sesiones.inicio': 'accesso il {when}',
  'gestion.sesiones.caduca': 'scade il {when}',
  'gestion.sesiones.mfa': 'Secondo fattore',
  'gestion.sesiones.sinMfa': 'Solo password',
  'gestion.sesiones.total': 'Sessioni aperte: {count}',
  'gestion.bajas.titulo': 'Account chiusi che puoi ancora ripristinare',
  'gestion.bajas.ayuda':
    'Un account chiuso conserva i suoi dati per {days} giorni. Il ripristino lo riporta sospeso: riattivalo nella directory per restituire l’accesso.',
  'gestion.bajas.cerrada': 'chiuso il {when}',
  'gestion.bajas.motivo': 'motivo: {reason}',
  'gestion.bajas.quedan': 'Giorni rimanenti: {count}',
  'gestion.bajas.restaurar': 'Ripristina',
  'gestion.bajas.confirmarTitulo': 'Ripristinare {name}?',
  'gestion.bajas.confirmar':
    'L’account torna sospeso, con i suoi dati. Nessuno potrà accedere finché non lo riattivi.',
  'gestion.bajas.restaurada': '{name} ripristinato',
  'gestion.bajas.reactivar': 'È sospeso: riattivalo nella directory per restituire l’accesso.',
  'gestion.bajas.fallo': 'Impossibile ripristinare l’account',
  'gestion.directorio.exportar': 'Esporta CSV',
  'gestion.dispositivos.buscar': 'Cerca persona, dispositivo o IP',
  'gestion.dispositivos.confianza': 'Filtra per attendibilità',
  'gestion.dispositivos.todos': 'Tutti i dispositivi',
  'gestion.dispositivos.recuento': '{count} di {total} mostrati',
  'gestion.dispositivos.sinResultados': 'Nessun dispositivo corrisponde.',
  'gestion.auditoria.rango': 'Intervallo rapido',
  'gestion.auditoria.hoy': 'Oggi',
  'gestion.auditoria.7d': '7 giorni',
  'gestion.auditoria.30d': '30 giorni',
  'gestion.roles.duplicar': 'Duplica',
  'gestion.roles.copia': '{name} (copia)',
  'gestion.lotes.titulo': 'Azioni in blocco recenti',
  'gestion.lotes.ayuda':
    'Cosa è stato applicato a più persone contemporaneamente e a chi non è stato possibile.',
  'gestion.lotes.ver': 'Mostra',
  'gestion.lotes.error': 'Impossibile caricare le azioni in blocco.',
  'gestion.lotes.vacio': 'Ancora nessuna azione in blocco.',
  'gestion.lotes.progreso': '{done} di {total}',
  'gestion.lotes.accion.setRole': 'Cambia ruolo',
  'gestion.lotes.estado.ejecutando': 'In corso',
  'gestion.lotes.estado.terminado': 'Completato',
  'gestion.lotes.estado.cancelado': 'Annullato',
  'gestion.lotes.estado.interrumpido': 'Interrotto',
  'gestion.lotes.detalle': 'Dettagli',
  'gestion.lotes.todosBien': 'Applicato a tutti.',
  'gestion.lote.fallos': 'Non applicato a',
  'gestion.lote.cerrar': 'Chiudi',
  'gestion.lote.codigo.not_found': 'non fa parte di questa organizzazione',
  'gestion.lote.codigo.self': 'sei tu',
  'gestion.lote.codigo.forbidden':
    'modificare un account proprietario richiede permessi di proprietà',
  'gestion.lote.codigo.deleted': 'l’account è chiuso',
  'gestion.lote.codigo.last_owner': 'ultimo proprietario dell’organizzazione',
  'gestion.lote.codigo.last_admin': 'ultimo amministratore dell’organizzazione',
  'gestion.lote.codigo.error': 'errore imprevisto; riprova',
  'gestion.lote.seleccionarVisibles': 'Seleziona tutti i visibili ({count})',
  'gestion.lote.recortado':
    'Un’azione in blocco riguarda al massimo {max} persone: sono selezionate le prime {max} visibili.',
};
