/*
 * SPDX-License-Identifier: Apache-2.0
 * SPDX-FileCopyrightText: (c) 2026 Daniel Delgado Ramos
 *
 * El diálogo de cuentas externas: el asistente, la lista y la vuelta del proveedor.
 * El buzón que llevaba dentro se retiró (fase B, punto 4): «Abrir buzón» y la cuenta
 * recién conectada se abren en el panel principal (`onOpenAccount`), y la cuenta del
 * lateral (initialInstance) entra UNA vez en su inicio de sesión. Ejecutar: `bun test`.
 */
import { GlobalRegistrator } from '@happy-dom/global-registrator';
import { afterEach, expect, test } from 'bun:test';
import { render } from 'preact';
import { act } from 'preact/test-utils';
import { en } from '../i18n/messages/en.js';

if (typeof document === 'undefined') GlobalRegistrator.register({ url: 'https://sytue.test/mail' });
else
  (window as unknown as { happyDOM?: { setURL: (url: string) => void } }).happyDOM?.setURL(
    'https://sytue.test/mail',
  );

const lateral = {
  instance_id: 'lateral',
  provider: 'imap',
  name: 'Trabajo',
  address: 'ana@example.org',
  connected: true,
  services: ['mail'],
  capabilities: {},
};
let cuentas: unknown[] = [lateral];
/** El código del alta a mano: «ya conectada», salvo que se cambie; vacío, se guarda. */
let altaCodigo = 'mail_account_exists';
const nueva = { ...lateral, instance_id: 'nueva', address: 'luis@example.org' };
const pedidas: string[] = [];
const cuerpos: unknown[] = [];
globalThis.fetch = (async (input: RequestInfo | URL, init?: RequestInit) => {
  const url = new URL(String(input));
  // Sin el prefijo de la app: otro fichero de la suite fija `BASE_URL` ('/mail/') y,
  // según el orden, las peticiones salen con él o sin él.
  pedidas.push(`${init?.method ?? 'GET'} ${url.pathname.replace(/^\/mail(?=\/api\/)/, '')}`);
  if (init?.method === 'POST') cuerpos.push(JSON.parse(String(init.body)));
  // Sin URL que seguir: las pruebas miran qué se pidió, no navegan.
  if (url.pathname.endsWith('/external-mail/connect'))
    return new Response('{}', { status: 503, headers: { 'content-type': 'application/json' } });
  if (url.pathname.endsWith('/external-mail/accounts') && init?.method === 'POST' && !altaCodigo) {
    cuentas = [lateral, nueva];
    return new Response(JSON.stringify({ instance_id: 'nueva' }), {
      status: 201,
      headers: { 'content-type': 'application/json' },
    });
  }
  if (url.pathname.endsWith('/external-mail/accounts') && init?.method === 'POST')
    return new Response(
      JSON.stringify({
        error: { code: altaCodigo, params: { instance_id: 'lateral' } },
      }),
      {
        status: altaCodigo === 'mail_account_exists' ? 409 : 422,
        headers: { 'content-type': 'application/json' },
      },
    );
  const body = url.pathname.endsWith('/external-mail/accounts')
    ? {
        items: cuentas,
        providers: [
          { provider: 'imap', available: true, services: [] },
          {
            provider: 'microsoft',
            available: true,
            services: [{ service: 'mail', available: true }],
          },
        ],
      }
    : { items: [] };
  return new Response(JSON.stringify(body), {
    status: 200,
    headers: { 'content-type': 'application/json' },
  });
}) as unknown as typeof fetch;

const { ExternalMail } = await import('./ExternalMail.js');
/** Las cuentas que el diálogo mandó abrir en el panel principal. */
const abiertas: string[] = [];
/** Las que abrió porque acababan de conectarse: MailApp lo avisa con un toast. */
const conectadas: string[] = [];
const alAbrir = (cuenta: { instance_id: string }, conectada: boolean) => {
  abiertas.push(cuenta.instance_id);
  if (conectada) conectadas.push(cuenta.instance_id);
};
const t = ((key: keyof typeof en) => en[key]) as unknown as Parameters<typeof ExternalMail>[0]['t'];

afterEach(() => {
  document.body.innerHTML = '';
  abiertas.length = 0;
  conectadas.length = 0;
  pedidas.length = 0;
  cuerpos.length = 0;
  cuentas = [lateral];
  altaCodigo = 'mail_account_exists';
  sessionStorage.clear();
  window.history.replaceState(null, '', '/mail');
});

async function esperar(): Promise<void> {
  for (let i = 0; i < 4; i++)
    await act(async () => {
      await new Promise((r) => setTimeout(r, 0));
    });
}
function boton(texto: string): HTMLButtonElement {
  const found = [...document.querySelectorAll('button')].find(
    (b) => b.textContent?.trim() === texto && !b.hidden,
  );
  if (!found)
    throw new Error(
      `no hay botón «${texto}» en ${[...document.querySelectorAll('button')].map((b) => b.textContent?.trim()).join(' / ')}`,
    );
  return found;
}
async function pulsar(texto: string): Promise<void> {
  await act(async () => boton(texto).click());
  await esperar();
}
async function escribir(id: string, valor: string): Promise<void> {
  const input = document.getElementById(id) as HTMLInputElement;
  await act(async () => {
    input.value = valor;
    input.dispatchEvent(new Event('input', { bubbles: true }));
  });
}
/** El asistente está en el paso de la cuenta (el formulario de IMAP o iCloud). */
function enAsistente(): boolean {
  return document.getElementById('external-mail-email') !== null;
}

// «Volver a iniciar sesión» del lateral lleva al inicio de sesión de ESA cuenta, una
// vez: un 409 posterior vuelve a la lista con su aviso, no al asistente. VISTO EN ROJO
// (2026-11-06) con `initial` reaplicado en cada recarga de la lista.
test('un alta a mano ya conectada vuelve a la lista con su aviso, no a la cuenta del lateral', async () => {
  const host = document.createElement('div');
  document.body.appendChild(host);
  await act(async () => {
    render(
      <ExternalMail t={t} initialInstance="lateral" onOpenAccount={alAbrir} onClose={() => {}} />,
      host,
    );
  });
  await esperar();
  expect(enAsistente()).toBe(true);
  expect((document.getElementById('external-mail-email') as HTMLInputElement).value).toBe(
    'ana@example.org',
  );
  await pulsar('Cancel');
  expect(enAsistente()).toBe(false);

  await pulsar(en['externalMail.add']);
  await act(async () => {
    (
      [...document.querySelectorAll('input[name="mail-provider"]')].find((i) =>
        (i.closest('label')?.textContent ?? '').includes('IMAP'),
      ) as HTMLInputElement
    ).click();
  });
  await esperar();
  await pulsar('Continue');
  await escribir('external-mail-email', 'ana@example.org');
  await escribir('external-mail-incoming_host', 'imap.example.org');
  await escribir('external-mail-incoming_password', 'secreto');
  await escribir('external-mail-smtp_host', 'smtp.example.org');
  await pulsar(en['externalMail.review']);
  await pulsar(en['externalMail.connect']);

  expect(pedidas).toContain('POST /api/external-mail/accounts');
  expect(enAsistente()).toBe(false);
  expect(document.querySelector('[role="status"]')?.textContent).toContain(
    en['externalMail.exists'],
  );
  // El asistente ya no está: el foco va a la cuenta que hay, no a <body>.
  expect(document.activeElement?.textContent?.trim()).toBe(en['externalMail.open']);
  expect(document.activeElement?.closest('[data-instance]')?.getAttribute('data-instance')).toBe(
    'lateral',
  );

  // Abrir el buzón lo lleva al panel principal y responde al aviso. No es un alta:
  // no se avisa de que se conectó nada.
  await pulsar(en['externalMail.open']);
  expect(abiertas).toEqual(['lateral']);
  expect(conectadas).toEqual([]);
  expect(document.body.textContent).not.toContain(en['externalMail.exists']);
});

async function abrir(query = ''): Promise<void> {
  window.history.replaceState(null, '', `/mail${query}`);
  const host = document.createElement('div');
  document.body.appendChild(host);
  await act(async () => {
    render(<ExternalMail t={t} onOpenAccount={alAbrir} onClose={() => {}} />, host);
  });
  await esperar();
}
async function elegir(proveedor: string): Promise<void> {
  await act(async () => {
    (
      [...document.querySelectorAll('input[name="mail-provider"]')].find((i) =>
        (i.closest('label')?.textContent ?? '').includes(proveedor),
      ) as HTMLInputElement
    ).click();
  });
  await esperar();
}

// Los borradores no se listan: al volver de un Outlook cancelado el asistente
// enseñaba Gmail, y «Continuar al proveedor» llevaba a Google.
test('al volver de un inicio cancelado se sigue con el proveedor que se eligió', async () => {
  sessionStorage.setItem(
    'sytue.external-mail.consent',
    JSON.stringify({ instance_id: 'draft-ms', provider: 'microsoft', services: ['mail'] }),
  );
  await abrir('?connect-mail=1&instance=draft-ms&oauth_error=denied');
  expect(document.body.textContent).toContain(en['externalMail.cancelled']);
  // Atendida la vuelta, se olvida: otra vuelta ilegible no la nombraría.
  expect(sessionStorage.getItem('sytue.external-mail.consent')).toBeNull();
  await pulsar(en['externalMail.authorize']);
  expect(cuerpos).toEqual([{ provider: 'microsoft', return_to: 'mail', services: ['mail'] }]);
});

// Una vuelta correcta también lo olvida: si no, volver más tarde de un inicio con el
// estado ilegible preseleccionaba el proveedor de una conexión ya terminada.
test('volver bien del proveedor olvida el inicio que se mandó', async () => {
  sessionStorage.setItem(
    'sytue.external-mail.consent',
    JSON.stringify({ instance_id: 'lateral', provider: 'microsoft', services: ['mail'] }),
  );
  await abrir('?connect-mail=1&instance=lateral');
  // Conectada: su Entrada en el panel principal, no un buzón dentro del diálogo, y
  // como recién conectada (fase B, punto 7: MailApp lo avisa).
  expect(abiertas).toEqual(['lateral']);
  expect(conectadas).toEqual(['lateral']);
  expect(sessionStorage.getItem('sytue.external-mail.consent')).toBeNull();
});

test('iCloud pide solo la dirección y la contraseña de app, y guarda sus servidores', async () => {
  await abrir();
  await pulsar(en['externalMail.add']);
  await elegir('iCloud');
  await pulsar('Continue');
  expect(document.body.textContent).toContain(en['externalMail.icloudBody']);
  expect(document.getElementById('external-mail-incoming_host')).toBeNull();
  expect(document.getElementById('external-mail-smtp_host')).toBeNull();
  await escribir('external-mail-email', 'ana@icloud.com');
  await escribir('external-mail-incoming_password', 'abcd-efgh-ijkl-mnop');
  await pulsar(en['externalMail.review']);
  await pulsar(en['externalMail.connect']);
  expect(cuerpos).toEqual([
    {
      provider: 'imap',
      name: '',
      address: 'ana@icloud.com',
      incoming: {
        host: 'imap.mail.me.com',
        port: 993,
        username: 'ana@icloud.com',
        security: 'tls',
      },
      outgoing: {
        host: 'smtp.mail.me.com',
        port: 587,
        username: 'ana@icloud.com',
        security: 'starttls',
      },
      password: 'abcd-efgh-ijkl-mnop',
    },
  ]);

  // Pasar a IMAP no se lleva los servidores de iCloud.
  await pulsar(en['externalMail.add']);
  await elegir('iCloud');
  await elegir('IMAP');
  await pulsar('Continue');
  expect((document.getElementById('external-mail-incoming_host') as HTMLInputElement).value).toBe(
    '',
  );
});

// Con un dominio propio de iCloud+ o un alias, Apple pide como usuario la dirección
// principal de iCloud: sin campo para ella, la contraseña correcta daba authError.
test('iCloud admite otro usuario en «Avanzado» y lo usa en los dos servidores', async () => {
  await abrir();
  await pulsar(en['externalMail.add']);
  await elegir('iCloud');
  await pulsar('Continue');
  await escribir('external-mail-email', 'ana@ana.example');
  await escribir('external-mail-incoming_password', 'abcd-efgh-ijkl-mnop');
  await escribir('external-mail-incoming_user', 'ana@icloud.com');
  await pulsar(en['externalMail.review']);
  await pulsar(en['externalMail.connect']);
  const [cuerpo] = cuerpos as {
    address: string;
    incoming: { username: string };
    outgoing: { username: string };
  }[];
  expect(cuerpo?.address).toBe('ana@ana.example');
  expect(cuerpo?.incoming.username).toBe('ana@icloud.com');
  expect(cuerpo?.outgoing.username).toBe('ana@icloud.com');
});

// Reconectar conservaba, oculto, el usuario guardado: editar la dirección seguía
// autenticando con la antigua.
test('reconectar iCloud con otra dirección se autentica con la nueva', async () => {
  const servidor = (host: string, port: number, security: string) => ({
    host,
    port,
    username: 'ana@icloud.com',
    security,
  });
  cuentas = [
    {
      instance_id: 'icloud',
      provider: 'imap',
      name: 'iCloud',
      address: 'ana@icloud.com',
      connected: false,
      services: ['mail'],
      capabilities: {},
      configuration: {
        incoming: servidor('imap.mail.me.com', 993, 'tls'),
        outgoing: servidor('smtp.mail.me.com', 587, 'starttls'),
      },
    },
  ];
  await abrir();
  await pulsar(en['externalMail.connect']);
  expect((document.getElementById('external-mail-incoming_user') as HTMLInputElement).value).toBe(
    '',
  );
  await escribir('external-mail-email', 'ana@me.com');
  await escribir('external-mail-incoming_password', 'abcd-efgh-ijkl-mnop');
  await pulsar(en['externalMail.review']);
  await pulsar(en['externalMail.connect']);
  const [cuerpo] = cuerpos as {
    instance_id?: string;
    incoming: { username: string };
    outgoing: { username: string };
  }[];
  expect(cuerpo?.instance_id).toBe('icloud');
  expect(cuerpo?.incoming.username).toBe('ana@me.com');
  expect(cuerpo?.outgoing.username).toBe('ana@me.com');
});

const servidorICloud = (host: string, port: number, security: string) => ({
  host,
  port,
  username: 'ana@icloud.com',
  security,
});
const cuentaICloud = {
  instance_id: 'icloud',
  provider: 'imap',
  name: 'Personal',
  address: 'ana@icloud.com',
  connected: true,
  services: ['mail'],
  capabilities: {},
  configuration: {
    address: 'ana@icloud.com',
    incoming: servidorICloud('imap.mail.me.com', 993, 'tls'),
    outgoing: servidorICloud('smtp.mail.me.com', 587, 'starttls'),
  },
};

// La ayuda decía dónde crear la contraseña de app, pero sin enlace ni la sección
// exacta. El enlace abre account.apple.com en otra pestaña, sin `opener`.
test('el formulario de iCloud enlaza a account.apple.com para crear la contraseña de app', async () => {
  await abrir();
  await pulsar(en['externalMail.add']);
  await elegir('iCloud');
  await pulsar('Continue');
  const enlace = [...document.querySelectorAll('a')].find(
    (a) => a.textContent?.trim() === en['externalMail.icloudLink'],
  );
  expect(enlace?.getAttribute('href')).toBe('https://account.apple.com/');
  expect(enlace?.getAttribute('target')).toBe('_blank');
  expect(enlace?.getAttribute('rel')).toContain('noopener');
  expect(en['externalMail.icloudBody']).toContain('App-Specific Passwords');
  // IMAP no lo lleva: allí la contraseña es la del proveedor que sea.
  await pulsar('Back');
  await elegir('IMAP');
  await pulsar('Continue');
  expect(document.body.textContent).not.toContain(en['externalMail.icloudLink']);
});

// Con la contraseña del Apple ID, iCloud rechaza el LOGIN: «revisa el usuario y la
// contraseña» no dice lo que de verdad falla.
test('un LOGIN rechazado en iCloud pide la contraseña específica de app', async () => {
  altaCodigo = 'mail_auth';
  await abrir();
  await pulsar(en['externalMail.add']);
  await elegir('iCloud');
  await pulsar('Continue');
  await escribir('external-mail-email', 'ana@icloud.com');
  await escribir('external-mail-incoming_password', 'la-del-apple-id');
  await pulsar(en['externalMail.review']);
  await pulsar(en['externalMail.connect']);
  expect(document.body.textContent).toContain(en['externalMail.icloudAuthError']);
  expect(document.body.textContent).not.toContain(en['externalMail.authError']);
});

test('el mismo rechazo en un IMAP cualquiera sigue diciendo el aviso general', async () => {
  altaCodigo = 'mail_auth';
  await abrir();
  await pulsar(en['externalMail.add']);
  await elegir('IMAP');
  await pulsar('Continue');
  await escribir('external-mail-email', 'ana@example.org');
  await escribir('external-mail-incoming_host', 'imap.example.org');
  await escribir('external-mail-incoming_password', 'secreta');
  await escribir('external-mail-smtp_host', 'smtp.example.org');
  await pulsar(en['externalMail.review']);
  await pulsar(en['externalMail.connect']);
  expect(document.body.textContent).toContain(en['externalMail.authError']);
  expect(document.body.textContent).not.toContain(en['externalMail.icloudAuthError']);
});

test('la lista nombra la cuenta de iCloud como iCloud Mail, no como IMAP', async () => {
  cuentas = [cuentaICloud, lateral];
  await abrir();
  const filas = [...document.querySelectorAll('[data-instance]')].map((f) => f.textContent ?? '');
  expect(filas[0]).toContain('ana@icloud.com · iCloud Mail');
  expect(filas[1]).toContain('ana@example.org · IMAP');
});

// Una contraseña de app revocada en account.apple.com: «Volver a iniciar sesión»
// (desde el lateral o desde la franja de la carpeta, AvisoExterno) abre el formulario
// de iCloud con su dirección, no el de IMAP. VISTO EN ROJO (2026-11-06) sin
// `reconnect` para la cuenta pedida: se quedaba en la lista.
test('volver a iniciar sesión en iCloud abre su formulario con la dirección puesta', async () => {
  cuentas = [{ ...cuentaICloud, connected: false }];
  const host = document.createElement('div');
  document.body.appendChild(host);
  await act(async () => {
    render(
      <ExternalMail t={t} initialInstance="icloud" onOpenAccount={alAbrir} onClose={() => {}} />,
      host,
    );
  });
  await esperar();
  expect(document.body.textContent).toContain(en['externalMail.icloudBody']);
  expect((document.getElementById('external-mail-email') as HTMLInputElement).value).toBe(
    'ana@icloud.com',
  );
  expect(document.getElementById('external-mail-incoming_host')).toBeNull();
  expect(abiertas).toEqual([]);
});

// Fase B, punto 7: el alta a mano que el servidor guarda cierra la hoja con la
// Entrada de la cuenta nueva en el panel principal, y la marca como recién
// conectada para que MailApp lo avise. VISTO EN ROJO (2026-11-07) con
// `openMailbox(added)` sin el `true`.
test('un alta a mano que se guarda abre la cuenta nueva como recién conectada', async () => {
  altaCodigo = '';
  await abrir();
  await pulsar(en['externalMail.add']);
  await elegir('IMAP');
  await pulsar('Continue');
  await escribir('external-mail-email', 'luis@example.org');
  await escribir('external-mail-incoming_host', 'imap.example.org');
  await escribir('external-mail-incoming_password', 'secreto');
  await escribir('external-mail-smtp_host', 'smtp.example.org');
  await pulsar(en['externalMail.review']);
  await pulsar(en['externalMail.connect']);
  expect(pedidas).toContain('POST /api/external-mail/accounts');
  expect(abiertas).toEqual(['nueva']);
  expect(conectadas).toEqual(['nueva']);
});
