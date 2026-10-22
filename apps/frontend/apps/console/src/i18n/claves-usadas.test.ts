/*
 * SPDX-License-Identifier: Apache-2.0
 * SPDX-FileCopyrightText: (c) 2026 Daniel Delgado Ramos
 *
 * NINGUNA CLAVE DEL CATÁLOGO SIN USAR (2026-09-28): el mismo guardián que www, con la
 * mecánica de `@sytue/i18n/claves-usadas`. Al pasar la consola a 26 idiomas había 16
 * claves que ninguna pantalla pedía y que se habrían traducido 21 veces más.
 *
 * Las plantillas dinámicas van en `DINAMICAS` con la lista EXACTA de claves que
 * producen: si alguien añade una sin listarla, el primer test lo dice.
 */
import { join } from 'node:path';
import { expect, test } from 'bun:test';
import { clavesSinUsar, codigoDe, plantillasDinamicas } from '@sytue/i18n/claves-usadas';
import { en } from './messages/en';

const codigo = codigoDe(join(import.meta.dir, '..', '..'));

const pre = (prefijo: string, valores: readonly string[]): string[] =>
  valores.map((v) => `${prefijo}${v}`);

const EDITORES = ['collabora', 'onlyoffice'];
const FIRMAS = ['abierta', 'cerrada', 'confianza'];

/** Cada `t(\`…${…}…\`)` del código, tal cual está escrito, y las claves que produce. */
const DINAMICAS: Record<string, readonly string[]> = {
  'editor.kind.${k.id}': pre('editor.kind.', EDITORES),
  'editor.kind.${k.id}.desc': EDITORES.map((e) => `editor.kind.${e}.desc`),
  'editor.kind.${kind}': pre('editor.kind.', EDITORES),
  'externalSignatures.mode.${m}': pre('externalSignatures.mode.', FIRMAS),
  'externalSignatures.mode.${m}.help': FIRMAS.map((m) => `externalSignatures.mode.${m}.help`),
  'migrations.domain.${d}': pre('migrations.domain.', [
    'drive',
    'contacts',
    'repos',
    'gists',
    'teams',
    'team_files',
    'mail',
    'calendar',
    'cloud',
  ]),
  "platform.action.${cambioEstado?.accion ?? 'suspend'}": pre('platform.action.', [
    'suspend',
    'reactivate',
    'delete',
  ]),
  'platform.approval.action.${a.action}': pre('platform.approval.action.', [
    'org.suspend',
    'org.delete',
    'org.merge',
    'platform.admin.grant',
  ]),
  'platform.approval.state.${a.effective_status}': pre('platform.approval.state.', [
    'pending',
    'executed',
    'rejected',
    'expired',
  ]),
  "platform.audit.family.${f || 'all'}": pre('platform.audit.family.', [
    'all',
    'platform.org.',
    'platform.seats.',
    'platform.merge.',
    'platform.admin.',
    'platform.pim.',
    'platform.approval.',
    'platform.user.',
  ]),
  'platform.merge.state.${m.status}': pre('platform.merge.state.', [
    'pending',
    'running',
    'completed',
    'failed',
    'aborted',
  ]),
  'platform.pim.state.${a.effective_status}': pre('platform.pim.state.', [
    'active',
    'expired',
    'revoked',
  ]),
  'platform.seats.kind.${g.kind}': pre('platform.seats.kind.', [
    'goodwill',
    'employee',
    'trial',
    'migration',
  ]),
  'platform.status.${o.status}': pre('platform.status.', [
    'active',
    'suspended',
    'deleted',
    'merged',
  ]),
  'role.${r}': pre('role.', ['member', 'billing', 'admin']),
};

test('toda plantilla dinámica del código está en DINAMICAS, y ninguna de más', () => {
  expect(plantillasDinamicas(codigo)).toEqual(
    Object.keys(DINAMICAS).sort((a, b) => a.localeCompare(b)),
  );
});

test('toda clave que produce una plantilla dinámica existe en el catálogo', () => {
  const inexistentes = Object.values(DINAMICAS)
    .flat()
    .filter((k) => !(k in en));
  expect(inexistentes).toEqual([]);
});

test('ninguna clave del catálogo se queda sin usar', () => {
  expect(clavesSinUsar(Object.keys(en), codigo, DINAMICAS)).toEqual([]);
});
