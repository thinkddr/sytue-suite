/*
 * SPDX-License-Identifier: Apache-2.0
 * SPDX-FileCopyrightText: (c) 2026 Daniel Delgado Ramos
 *
 * «Conectar una aplicación» no ofrece lo que se conecta desde un producto (Drive,
 * Rooms, Mail): eran duplicados de Slack, Teams, Google Chat, Dropbox y Google Drive.
 * Ejecutar: `bun test`.
 */
import { GlobalRegistrator } from '@happy-dom/global-registrator';
import { expect, test } from 'bun:test';
import type { CatalogApp } from '../lib/gateway.js';

if (typeof document === 'undefined') GlobalRegistrator.register();

const { conectablesDesdeAdmin } = await import('./ConnectAppWizard.js');

const app = (key: string, name: string): CatalogApp => ({
  key,
  name,
  description: '',
  color: '#000',
  iconUrl: '',
  identity: 'none',
  provisioning: 'none',
  hasData: false,
  fields: [],
  tier: 'tier2',
});

test('el paso «elegir» oculta las plantillas de producto y deja una sola de cada marca', () => {
  const catalogo = [
    app('slack', 'Slack'),
    app('slack-rooms', 'Slack'),
    app('slack-chat', 'Slack'),
    app('microsoft-teams', 'Microsoft Teams'),
    app('microsoft-teams-rooms', 'Microsoft Teams'),
    app('dropbox', 'Dropbox Business'),
    app('dropbox-storage', 'Dropbox'),
    app('google-drive', 'Google Drive'),
    app('google-workspace', 'Google Workspace'),
    app('gmail-mail', 'Gmail'),
  ];
  expect(conectablesDesdeAdmin(catalogo).map((a) => a.key)).toEqual([
    'slack',
    'microsoft-teams',
    'dropbox',
    'google-workspace',
  ]);
});
