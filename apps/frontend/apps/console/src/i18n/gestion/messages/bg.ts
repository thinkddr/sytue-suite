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

export const bg: AppCatalog<typeof en> = {
  'gestion.acceso.titulo': 'Какво може да прави и защо',
  'gestion.acceso.ver': 'Показване',
  'gestion.acceso.error': 'Действителният достъп не можа да се зареди.',
  'gestion.acceso.ayuda':
    'Изчислява се от Sytue ID по същото правило, по което разрешава достъпа. Всяко разрешение показва откъде идва; ако премахнете този източник, проверете останалите, изброени тук.',
  'gestion.acceso.sinPermisos': 'Без администраторски разрешения.',
  'gestion.acceso.fuente.integrado': 'Вградена роля: {role}',
  'gestion.acceso.fuente.rol': 'Роля: {name}',
  'gestion.acceso.fuente.elevacion': 'Повишен до {name} до {when}',
  'gestion.acceso.pendientes': 'Повишения, чакащи одобрение',
  'gestion.acceso.pendiente': '{role}, поискано на {when}',
  'gestion.acceso.grupos': 'Групи',
  'gestion.acceso.sinGrupos': 'Не е в никоя група.',
  'gestion.acceso.calculado': 'Изчислено на {when}',
  'gestion.sesiones.titulo': 'Отворени сесии',
  'gestion.sesiones.ver': 'Показване',
  'gestion.sesiones.error': 'Сесиите не можаха да се заредят.',
  'gestion.sesiones.vacia': 'Няма отворени сесии.',
  'gestion.sesiones.inicio': 'влязъл на {when}',
  'gestion.sesiones.caduca': 'изтича на {when}',
  'gestion.sesiones.mfa': 'Втори фактор',
  'gestion.sesiones.sinMfa': 'Само парола',
  'gestion.sesiones.total': 'Отворени сесии: {count}',
  'gestion.bajas.titulo': 'Закрити акаунти, които още можете да възстановите',
  'gestion.bajas.ayuda':
    'Закритият акаунт пази данните си {days} дни. Възстановяването го връща спрян: активирайте го отново в директорията, за да върнете достъпа.',
  'gestion.bajas.cerrada': 'закрит на {when}',
  'gestion.bajas.motivo': 'причина: {reason}',
  'gestion.bajas.quedan': 'Оставащи дни: {count}',
  'gestion.bajas.restaurar': 'Възстановяване',
  'gestion.bajas.confirmarTitulo': 'Да се възстанови ли {name}?',
  'gestion.bajas.confirmar':
    'Акаунтът се връща спрян, с данните си. Никой не може да влезе с него, докато не го активирате отново.',
  'gestion.bajas.restaurada': '{name} е възстановен',
  'gestion.bajas.reactivar':
    'Спрян е: активирайте го отново в директорията, за да върнете достъпа.',
  'gestion.bajas.fallo': 'Акаунтът не можа да се възстанови',
  'gestion.directorio.exportar': 'Експорт в CSV',
  'gestion.dispositivos.buscar': 'Търсене на човек, устройство или IP',
  'gestion.dispositivos.confianza': 'Филтриране по доверие',
  'gestion.dispositivos.todos': 'Всички устройства',
  'gestion.dispositivos.recuento': 'Показани {count} от {total}',
  'gestion.dispositivos.sinResultados': 'Няма съвпадащи устройства.',
  'gestion.auditoria.rango': 'Бърз период',
  'gestion.auditoria.hoy': 'Днес',
  'gestion.auditoria.7d': '7 дни',
  'gestion.auditoria.30d': '30 дни',
  'gestion.roles.duplicar': 'Дублиране',
  'gestion.roles.copia': '{name} (копие)',
  'gestion.lotes.titulo': 'Последни групови действия',
  'gestion.lotes.ayuda':
    'Какво е приложено към няколко души наведнъж и към кого не е могло да се приложи.',
  'gestion.lotes.ver': 'Показване',
  'gestion.lotes.error': 'Груповите действия не можаха да се заредят.',
  'gestion.lotes.vacio': 'Все още няма групови действия.',
  'gestion.lotes.progreso': '{done} от {total}',
  'gestion.lotes.accion.setRole': 'Смяна на роля',
  'gestion.lotes.estado.ejecutando': 'Изпълнява се',
  'gestion.lotes.estado.terminado': 'Завършено',
  'gestion.lotes.estado.cancelado': 'Отменено',
  'gestion.lotes.estado.interrumpido': 'Прекъснато',
  'gestion.lotes.detalle': 'Подробности',
  'gestion.lotes.todosBien': 'Приложено към всички.',
  'gestion.lote.fallos': 'Не е приложено към',
  'gestion.lote.cerrar': 'Затваряне',
  'gestion.lote.codigo.not_found': 'не е в тази организация',
  'gestion.lote.codigo.self': 'това сте вие',
  'gestion.lote.codigo.forbidden':
    'акаунт с права на собственост се променя само с права на собственост',
  'gestion.lote.codigo.deleted': 'акаунтът е закрит',
  'gestion.lote.codigo.last_owner': 'последният собственик на организацията',
  'gestion.lote.codigo.last_admin': 'последният администратор на организацията',
  'gestion.lote.codigo.error': 'неочаквана грешка; опитайте отново',
  'gestion.lote.seleccionarVisibles': 'Избор на всички видими ({count})',
  'gestion.lote.recortado':
    'Груповото действие обхваща най-много {max} души: избрани са първите {max} видими.',
};
