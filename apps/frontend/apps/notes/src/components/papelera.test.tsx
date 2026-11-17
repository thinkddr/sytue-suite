/*
 * SPDX-License-Identifier: Apache-2.0
 * SPDX-FileCopyrightText: (c) 2026 Daniel Delgado Ramos
 *
 * La papelera dice de qué cuaderno viene cada nota y cuándo se vaciará sola.
 */
import { GlobalRegistrator } from '@happy-dom/global-registrator';
import { expect, test } from 'bun:test';
import { render } from 'preact';
import type { TrashedNote } from '@sytue/api-clients';
import { PanelPapelera } from './Papelera.js';

if (typeof document === 'undefined') GlobalRegistrator.register();

const t = ((k: string, v?: { count?: number }) =>
  v?.count === undefined ? k : `${k}:${v.count}`) as never;
const AHORA = new Date('2026-11-12T10:00:00Z');
const DIA = 24 * 60 * 60 * 1000;
const nota = (id: string, extra: Partial<TrashedNote>): TrashedNote =>
  ({ id, kind: 'text', title: id, deleted_at: AHORA.toISOString(), ...extra }) as TrashedNote;

test('cada fila lleva su cuaderno y cuándo se borra', () => {
  const raiz = document.createElement('div');
  render(
    <PanelPapelera
      t={t}
      ahora={AHORA}
      notes={[
        nota('billetes', {
          notebook_title: 'Viaje',
          purge_at: new Date(AHORA.getTime() + 30 * DIA - 1000).toISOString(),
        }),
        // Retención legal: sin purge_at. No se dice por qué.
        nota('contrato', { notebook_title: 'Legal' }),
      ]}
      onRestore={() => undefined}
      onPurge={() => undefined}
      onVaciar={() => undefined}
    />,
    raiz,
  );
  const metas = [...raiz.querySelectorAll('li .sytue-meta')].map((m) => m.textContent);
  expect(metas).toEqual(['Viaje · trash.purgeIn:30', 'Legal · trash.kept']);
});
