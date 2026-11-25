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

export const es: AppCatalog<typeof en> = {
  'gestion.acceso.titulo': 'Qué puede hacer y por qué',
  'gestion.acceso.ver': 'Mostrar',
  'gestion.acceso.error': 'No se ha podido cargar el acceso efectivo.',
  'gestion.acceso.ayuda':
    'Lo calcula Sytue ID con la misma regla con la que autoriza. Cada permiso dice de dónde sale; si quitas esa fuente, mira las demás que aparecen aquí.',
  'gestion.acceso.sinPermisos': 'Sin permisos de administración.',
  'gestion.acceso.fuente.integrado': 'Rol integrado: {role}',
  'gestion.acceso.fuente.rol': 'Rol: {name}',
  'gestion.acceso.fuente.elevacion': 'Elevado a {name} hasta {when}',
  'gestion.acceso.pendientes': 'Elevaciones pendientes de aprobación',
  'gestion.acceso.pendiente': '{role}, pedida el {when}',
  'gestion.acceso.grupos': 'Grupos',
  'gestion.acceso.sinGrupos': 'No está en ningún grupo.',
  'gestion.acceso.calculado': 'Calculado el {when}',
  'gestion.sesiones.titulo': 'Sesiones abiertas',
  'gestion.sesiones.ver': 'Mostrar',
  'gestion.sesiones.error': 'No se han podido cargar las sesiones.',
  'gestion.sesiones.vacia': 'No hay sesiones abiertas.',
  'gestion.sesiones.inicio': 'entró el {when}',
  'gestion.sesiones.caduca': 'caduca el {when}',
  'gestion.sesiones.mfa': 'Segundo factor',
  'gestion.sesiones.sinMfa': 'Solo contraseña',
  'gestion.sesiones.total': 'Sesiones abiertas: {count}',
  'gestion.bajas.titulo': 'Cuentas cerradas que aún puedes restaurar',
  'gestion.bajas.ayuda':
    'Una cuenta cerrada conserva sus datos {days} días. Restaurarla la devuelve suspendida: reactívala en el directorio para devolverle el acceso.',
  'gestion.bajas.cerrada': 'cerrada el {when}',
  'gestion.bajas.motivo': 'motivo: {reason}',
  'gestion.bajas.quedan': 'Días restantes: {count}',
  'gestion.bajas.restaurar': 'Restaurar',
  'gestion.bajas.confirmarTitulo': '¿Restaurar a {name}?',
  'gestion.bajas.confirmar':
    'La cuenta vuelve suspendida, con sus datos. Nadie podrá entrar con ella hasta que la reactives.',
  'gestion.bajas.restaurada': '{name} restaurada',
  'gestion.bajas.reactivar':
    'Está suspendida: reactívala en el directorio para devolverle el acceso.',
  'gestion.bajas.fallo': 'No se ha podido restaurar la cuenta',
  'gestion.directorio.exportar': 'Exportar CSV',
  'gestion.dispositivos.buscar': 'Buscar persona, dispositivo o IP',
  'gestion.dispositivos.confianza': 'Filtrar por confianza',
  'gestion.dispositivos.todos': 'Todos los dispositivos',
  'gestion.dispositivos.recuento': 'Mostrando {count} de {total}',
  'gestion.dispositivos.sinResultados': 'Ningún dispositivo coincide.',
  'gestion.auditoria.rango': 'Rango rápido',
  'gestion.auditoria.hoy': 'Hoy',
  'gestion.auditoria.7d': '7 días',
  'gestion.auditoria.30d': '30 días',
  'gestion.roles.duplicar': 'Duplicar',
  'gestion.roles.copia': '{name} (copia)',
  'gestion.lotes.titulo': 'Acciones en lote recientes',
  'gestion.lotes.ayuda':
    'Lo que se aplicó a varias personas a la vez y a quién no se le pudo aplicar.',
  'gestion.lotes.ver': 'Mostrar',
  'gestion.lotes.error': 'No se han podido cargar las acciones en lote.',
  'gestion.lotes.vacio': 'Todavía no hay acciones en lote.',
  'gestion.lotes.progreso': '{done} de {total}',
  'gestion.lotes.accion.setRole': 'Cambiar rol',
  'gestion.lotes.estado.ejecutando': 'En curso',
  'gestion.lotes.estado.terminado': 'Terminado',
  'gestion.lotes.estado.cancelado': 'Cancelado',
  'gestion.lotes.estado.interrumpido': 'Interrumpido',
  'gestion.lotes.detalle': 'Detalle',
  'gestion.lotes.todosBien': 'Se aplicó a todas las personas.',
  'gestion.lote.fallos': 'No se aplicó a',
  'gestion.lote.cerrar': 'Cerrar',
  'gestion.lote.codigo.not_found': 'no está en esta organización',
  'gestion.lote.codigo.self': 'eres tú',
  'gestion.lote.codigo.forbidden': 'cambiar una cuenta propietaria requiere permisos de propiedad',
  'gestion.lote.codigo.deleted': 'la cuenta está cerrada',
  'gestion.lote.codigo.last_owner': 'es el último propietario de la organización',
  'gestion.lote.codigo.last_admin': 'es el último administrador de la organización',
  'gestion.lote.codigo.error': 'error inesperado; vuelve a intentarlo',
  'gestion.lote.seleccionarVisibles': 'Seleccionar las visibles ({count})',
  'gestion.lote.recortado':
    'Una acción en lote abarca como mucho {max} personas: se han seleccionado las {max} primeras visibles.',
};
