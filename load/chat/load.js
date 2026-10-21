// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

// Sytue Chat — prueba de carga REST (k6). Somete al hot path de lectura del chat
// (listar teams -> canales -> mensajes con paginacion keyset) a carga sostenida,
// verificando el SLO p95 < 200 ms. El fan-out en tiempo real se prueba aparte en
// ws.js (WebSocket).
//
// Requiere ACCESS_TOKEN (JWT de Sytue ID). Sin token, solo /healthz.
//
//   k6 run -e ACCESS_TOKEN=... load/chat/load.js
//   k6 run -e CHAT_URL=https://chat.staging.sytue.eu -e ACCESS_TOKEN=... -e VUS=100 load/chat/load.js

import http from 'k6/http';
import { group, sleep } from 'k6';
import { BASE, headers, ACCESS_TOKEN, loadThresholds, loadStages } from '../lib/config.js';
import { expectOK } from '../lib/checks.js';

export const options = {
  scenarios: {
    chat_load: {
      executor: 'ramping-vus',
      startVUs: 0,
      stages: loadStages(),
      gracefulRampDown: '20s',
    },
  },
  thresholds: loadThresholds(),
};

export default function () {
  const base = BASE.chat;

  if (!ACCESS_TOKEN) {
    expectOK(http.get(`${base}/healthz`, { headers: headers() }), 'healthz');
    sleep(1);
    return;
  }

  group('teams -> channels -> messages (hot path de lectura)', () => {
    const teamsRes = http.get(`${base}/api/v1/teams`, { headers: headers() });
    expectOK(teamsRes, 'teams');

    let teamID = '';
    try {
      const teams = teamsRes.json();
      if (Array.isArray(teams) && teams.length > 0) teamID = teams[0].id;
    } catch {
      teamID = '';
    }

    if (teamID) {
      const chRes = http.get(`${base}/api/v1/teams/${teamID}/channels`, { headers: headers() });
      expectOK(chRes, 'channels');
      expectOK(http.get(`${base}/api/v1/teams/${teamID}/unread`, { headers: headers() }), 'unread');

      let channelID = '';
      try {
        const channels = chRes.json();
        if (Array.isArray(channels) && channels.length > 0) channelID = channels[0].id;
      } catch {
        channelID = '';
      }

      if (channelID) {
        expectOK(
          http.get(`${base}/api/v1/channels/${channelID}/messages?limit=50`, { headers: headers() }),
          'messages',
        );
      }
    }
  });

  sleep(1);
}
