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

export const pt: AppCatalog<typeof en> = {
  'gestion.acceso.titulo': 'O que pode fazer e porquê',
  'gestion.acceso.ver': 'Mostrar',
  'gestion.acceso.error': 'Não foi possível carregar o acesso efetivo.',
  'gestion.acceso.ayuda':
    'Calculado pelo Sytue ID com a mesma regra que aplica ao autorizar. Cada permissão indica de onde vem; se retirar essa fonte, verifique as outras aqui listadas.',
  'gestion.acceso.sinPermisos': 'Sem permissões de administração.',
  'gestion.acceso.fuente.integrado': 'Função integrada: {role}',
  'gestion.acceso.fuente.rol': 'Função: {name}',
  'gestion.acceso.fuente.elevacion': 'Elevado a {name} até {when}',
  'gestion.acceso.pendientes': 'Elevações a aguardar aprovação',
  'gestion.acceso.pendiente': '{role}, pedida em {when}',
  'gestion.acceso.grupos': 'Grupos',
  'gestion.acceso.sinGrupos': 'Não pertence a nenhum grupo.',
  'gestion.acceso.calculado': 'Calculado em {when}',
  'gestion.sesiones.titulo': 'Sessões abertas',
  'gestion.sesiones.ver': 'Mostrar',
  'gestion.sesiones.error': 'Não foi possível carregar as sessões.',
  'gestion.sesiones.vacia': 'Não há sessões abertas.',
  'gestion.sesiones.inicio': 'entrou em {when}',
  'gestion.sesiones.caduca': 'expira em {when}',
  'gestion.sesiones.mfa': 'Segundo fator',
  'gestion.sesiones.sinMfa': 'Só palavra-passe',
  'gestion.sesiones.total': 'Sessões abertas: {count}',
  'gestion.bajas.titulo': 'Contas encerradas que ainda pode restaurar',
  'gestion.bajas.ayuda':
    'Uma conta encerrada mantém os dados durante {days} dias. Restaurá-la devolve-a suspensa: reative-a no diretório para devolver o acesso.',
  'gestion.bajas.cerrada': 'encerrada em {when}',
  'gestion.bajas.motivo': 'motivo: {reason}',
  'gestion.bajas.quedan': 'Dias restantes: {count}',
  'gestion.bajas.restaurar': 'Restaurar',
  'gestion.bajas.confirmarTitulo': 'Restaurar {name}?',
  'gestion.bajas.confirmar':
    'A conta volta suspensa, com os seus dados. Ninguém pode entrar com ela até a reativar.',
  'gestion.bajas.restaurada': '{name} restaurada',
  'gestion.bajas.reactivar': 'Está suspensa: reative-a no diretório para devolver o acesso.',
  'gestion.bajas.fallo': 'Não foi possível restaurar a conta',
  'gestion.directorio.exportar': 'Exportar CSV',
  'gestion.dispositivos.buscar': 'Procurar pessoa, dispositivo ou IP',
  'gestion.dispositivos.confianza': 'Filtrar por confiança',
  'gestion.dispositivos.todos': 'Todos os dispositivos',
  'gestion.dispositivos.recuento': 'A mostrar {count} de {total}',
  'gestion.dispositivos.sinResultados': 'Nenhum dispositivo corresponde.',
  'gestion.auditoria.rango': 'Intervalo rápido',
  'gestion.auditoria.hoy': 'Hoje',
  'gestion.auditoria.7d': '7 dias',
  'gestion.auditoria.30d': '30 dias',
  'gestion.roles.duplicar': 'Duplicar',
  'gestion.roles.copia': '{name} (cópia)',
  'gestion.lotes.titulo': 'Ações em massa recentes',
  'gestion.lotes.ayuda':
    'O que foi aplicado a várias pessoas de uma vez e a quem não foi possível aplicar.',
  'gestion.lotes.ver': 'Mostrar',
  'gestion.lotes.error': 'Não foi possível carregar as ações em massa.',
  'gestion.lotes.vacio': 'Ainda não há ações em massa.',
  'gestion.lotes.progreso': '{done} de {total}',
  'gestion.lotes.accion.setRole': 'Alterar função',
  'gestion.lotes.estado.ejecutando': 'Em curso',
  'gestion.lotes.estado.terminado': 'Concluído',
  'gestion.lotes.estado.cancelado': 'Cancelado',
  'gestion.lotes.estado.interrumpido': 'Interrompido',
  'gestion.lotes.detalle': 'Detalhes',
  'gestion.lotes.todosBien': 'Aplicado a todas as pessoas.',
  'gestion.lote.fallos': 'Não aplicado a',
  'gestion.lote.cerrar': 'Fechar',
  'gestion.lote.codigo.not_found': 'não pertence a esta organização',
  'gestion.lote.codigo.self': 'é a sua própria conta',
  'gestion.lote.codigo.forbidden':
    'alterar uma conta proprietária requer permissões de propriedade',
  'gestion.lote.codigo.deleted': 'a conta está encerrada',
  'gestion.lote.codigo.last_owner': 'último proprietário da organização',
  'gestion.lote.codigo.last_admin': 'último administrador da organização',
  'gestion.lote.codigo.error': 'erro inesperado; tente novamente',
  'gestion.lote.seleccionarVisibles': 'Selecionar todas as visíveis ({count})',
  'gestion.lote.recortado':
    'Uma ação em massa abrange no máximo {max} pessoas: foram selecionadas as primeiras {max} visíveis.',
};
