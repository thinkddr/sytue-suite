// El cliente de intents tiene que hablar la MISMA forma que el Hub.
//
// Regresión: `dispatchIntent` mandaba `{action, target, payload}` mientras el
// servidor lee `{type, params, ids}` (services/hub/internal/domain: IntentRequest).
// Con eso al Hub le llegaba un `type` vacío y respondía «intent desconocido», así
// que este método no podía funcionar NUNCA. No se notó porque las dos superficies
// que usan intents —Mail y Drive— se compusieron la llamada a mano en su BFF con
// la forma correcta, y este cliente se quedó de adorno.
//
// El test mira el CUERPO que sale por el cable, no el tipo: un tipo se puede
// renombrar y seguir mandando la forma equivocada. Ejecutar: `bun test`.
import { expect, test } from 'bun:test';
import { HubClient } from './index.js';

/** Captura la petición que el cliente pone en el cable y responde 200. */
function clienteQueCaptura(respuesta: unknown): {
  hub: HubClient;
  visto: () => { url: string; body: unknown };
} {
  let url = '';
  let body: unknown;
  const fetchImpl = (async (input: string | URL | Request, init?: RequestInit) => {
    url = String(input);
    body = init?.body ? JSON.parse(String(init.body)) : undefined;
    return new Response(JSON.stringify(respuesta), {
      status: 200,
      headers: { 'content-type': 'application/json' },
    });
  }) as unknown as typeof fetch;
  const hub = new HubClient({
    baseUrl: 'https://hub.test',
    auth: { getAccessToken: () => 'tok' },
    fetchImpl,
  });
  return { hub, visto: () => ({ url, body }) };
}

test('dispatchIntent manda {type, params, ids}, que es lo que el Hub lee', async () => {
  const { hub, visto } = clienteQueCaptura({
    type: 'attach-drive-to-mail',
    url: 'https://mail.test/compose?action=compose&attach_drive=f1',
    target: 'mail',
    label: 'Adjuntar en un correo',
  });

  await hub.dispatchIntent({
    type: 'attach-drive-to-mail',
    ids: ['f1', 'f2'],
    params: { subject: 'Informe' },
  });

  const { url, body } = visto();
  expect(url).toBe('https://hub.test/api/v1/intents');
  expect(body).toEqual({
    type: 'attach-drive-to-mail',
    ids: ['f1', 'f2'],
    params: { subject: 'Informe' },
  });

  // Y lo que NO puede volver a salir: la forma vieja. Si alguien reintroduce
  // `action`/`target`/`payload`, el Hub responderá «intent desconocido» y el
  // fallo aparecerá en producción, no aquí.
  const enviado = body as Record<string, unknown>;
  expect(enviado.action).toBeUndefined();
  expect(enviado.target).toBeUndefined();
  expect(enviado.payload).toBeUndefined();
});

test('dispatchIntent devuelve el deep-link tal cual lo compone el Hub', async () => {
  const destino = 'https://mail.test/compose?action=compose&attach_drive=f1&subject=Informe';
  const { hub } = clienteQueCaptura({ type: 'attach-drive-to-mail', url: destino, target: 'mail' });

  const res = await hub.dispatchIntent({ type: 'attach-drive-to-mail', ids: ['f1'] });

  // La URL es lo único imprescindible: es lo que materializa el intent. Que se
  // pierda al deserializar deja al usuario con un botón que no hace nada.
  expect(res.url).toBe(destino);
  expect(res.type).toBe('attach-drive-to-mail');
});
