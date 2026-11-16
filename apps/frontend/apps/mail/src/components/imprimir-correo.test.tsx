/*
 * SPDX-License-Identifier: Apache-2.0
 * SPDX-FileCopyrightText: (c) 2026 Daniel Delgado Ramos
 *
 * «Imprimir» en el lector (D-MAIL-09): imprime el hilo ENTERO —también los
 * mensajes plegados, cuyo cuerpo pide en ese momento— desde un documento propio
 * en un marco aislado, no la página de Mail. Ejecutar: `bun test`.
 */
import { GlobalRegistrator } from '@happy-dom/global-registrator';
import { afterEach, expect, test } from 'bun:test';
import { render } from 'preact';
import { act } from 'preact/test-utils';
import type { EmailSummary } from '@sytue/api-clients';

if (typeof document === 'undefined') GlobalRegistrator.register();
process.env.BASE_URL ??= '/mail/';

const { ThemeProvider } = await import('@sytue/ui');
const { MessageReader } = await import('./MessageReader.js');
const { groupByThread } = await import('./utils.js');
const { documentoDeImpresion, imprimirEnMarco, ATRIBUTO_MARCO } =
  await import('./imprimirCorreo.js');

const CUERPOS: Record<string, { text: string; html: string }> = {
  m1: { text: 'Primera parte del hilo, plegada en el lector.', html: '' },
  m2: { text: '', html: '<table><tr><td>Factura <b>de noviembre</b></td></tr></table>' },
};
const ADJUNTOS = [
  { name: 'factura.pdf', type: 'application/pdf', size: 1200 },
  { name: 'signature.asc', type: 'application/pgp-signature', size: 300 },
];
const pedidos: string[] = [];
globalThis.fetch = (async (input: RequestInfo | URL) => {
  const url = String(input);
  pedidos.push(url);
  const id = new URL(url, 'https://mail.sytue.eu').searchParams.get('emailId') ?? '';
  const cuerpo = url.includes('/api/body')
    ? (CUERPOS[id] ?? {})
    : url.includes('/api/attachments')
      ? { attachments: id === 'm2' ? ADJUNTOS : [] }
      : {};
  return new Response(JSON.stringify(cuerpo), {
    status: 200,
    headers: { 'content-type': 'application/json' },
  });
}) as unknown as typeof fetch;

function correo(id: string, extra: Partial<EmailSummary> = {}): EmailSummary {
  return {
    id,
    threadId: 'h1',
    subject: 'Factura <noviembre>',
    from: [{ name: 'Cobros', email: 'cobros@banco.eu' }],
    to: [{ email: 'yo@ejemplo.eu' }],
    cc: [],
    preview: `Extracto de ${id}`,
    receivedAt: '2026-11-05T10:00:00Z',
    keywords: { $seen: true },
    hasAttachment: false,
    ...extra,
  };
}

const esperar = (): Promise<void> => new Promise((r) => setTimeout(r, 0));

afterEach(() => {
  document.body.innerHTML = '';
  pedidos.length = 0;
  montados.length = 0;
});

async function montar(readOnly = false): Promise<HTMLElement> {
  const host = document.createElement('div');
  document.body.appendChild(host);
  const nada = (): void => {};
  // Reciente→antiguo, como llega de la lista: m2 es el último y el único expandido.
  const hilo = [
    correo('m2', { receivedAt: '2026-11-06T09:00:00Z', hasAttachment: true }),
    correo('m1', { cc: [{ email: 'jefa@ejemplo.eu' }] }),
  ];
  await act(async () => {
    render(
      <ThemeProvider>
        <MessageReader
          readOnly={readOnly}
          locale="es"
          conversation={groupByThread(hilo)[0] ?? null}
          onReply={nada}
          onCompose={nada}
          onToggleFlag={nada}
          onMarkUnread={nada}
          onArchive={null}
          onTrash={null}
          onSpam={null}
          onNotSpam={null}
          onPurge={null}
          onMove={nada}
          moveTargets={[]}
          labels={[]}
          onToggleLabel={nada}
        />
      </ThemeProvider>,
      host,
    );
  });
  await act(esperar);
  return host;
}

function botonPorTexto(raiz: ParentNode, texto: string): HTMLElement {
  const b = Array.from(raiz.querySelectorAll<HTMLElement>('button,[role="menuitem"]')).find(
    (el) => el.textContent?.trim() === texto,
  );
  if (!b) throw new Error(`no hay «${texto}»`);
  return b;
}

/**
 * Los marcos de impresión que se han montado. Se apuntan al entrar en el DOM: el
 * de happy-dom carga e «imprime» en seguida y puede haberse retirado ya.
 */
const montados: HTMLIFrameElement[] = [];
new MutationObserver((cambios) => {
  for (const c of cambios)
    for (const n of Array.from(c.addedNodes))
      if (n instanceof HTMLIFrameElement && n.hasAttribute(ATRIBUTO_MARCO)) montados.push(n);
}).observe(document.body, { childList: true });

async function marcoDeImpresion(): Promise<HTMLIFrameElement> {
  for (let i = 0; i < 50; i++) {
    const marco = montados.at(-1);
    if (marco) return marco;
    await act(esperar);
  }
  throw new Error('no se montó el marco de impresión');
}

test('«Imprimir» del menú compone el hilo entero, plegados incluidos, en un marco aislado', async () => {
  const host = await montar();
  await act(async () => host.querySelector<HTMLElement>('[aria-label="Más acciones"]')?.click());
  await act(async () => botonPorTexto(document, 'Imprimir').click());
  const marco = await marcoDeImpresion();

  // Aislado como el lector: sin scripts; `allow-modals` para poder abrir el diálogo.
  const sandbox = marco.getAttribute('sandbox') ?? '';
  expect(sandbox).toContain('allow-modals');
  expect(sandbox).not.toContain('allow-scripts');

  const doc = marco.getAttribute('srcdoc') ?? '';
  // El asunto, escapado: un `<` del asunto no abre una etiqueta.
  expect(doc).toContain('<title>Factura &lt;noviembre&gt;</title>');
  // El mensaje plegado (m1) también sale, con su cuerpo pedido ahora y su Cc.
  expect(pedidos.some((u) => u.includes('/api/body?emailId=m1'))).toBe(true);
  expect(doc).toContain('Primera parte del hilo, plegada en el lector.');
  expect(doc).toContain('CC: jefa@ejemplo.eu');
  // El último, con su maqueta HTML y sus adjuntos, pero sin la firma del sobre.
  expect(doc).toContain('Factura <b>de noviembre</b>');
  expect(doc).toContain('<li>factura.pdf</li>');
  expect(doc).not.toContain('signature.asc');
  expect(doc).toContain('De: Cobros &lt;cobros@banco.eu&gt;');
  // En orden de lectura: antiguo → reciente.
  expect(doc.indexOf('Primera parte')).toBeLessThan(doc.indexOf('<b>de noviembre</b>'));
});

test('en solo lectura se sigue pudiendo imprimir, con un botón propio', async () => {
  const host = await montar(true);
  expect(host.querySelector('[aria-label="Más acciones"]')).toBeNull();
  await act(async () => host.querySelector<HTMLElement>('[aria-label="Imprimir"]')?.click());
  const marco = await marcoDeImpresion();
  expect(marco.getAttribute('srcdoc')).toContain('Primera parte del hilo');
});

test('el documento escapa cabeceras y texto y lleva la CSP del lector', () => {
  const html = documentoDeImpresion({
    idioma: 'es',
    asunto: 'Hola',
    mensajes: [
      {
        remitente: 'De: <script>alert(1)</script>',
        fecha: '5 de noviembre de 2026, 10:00',
        destinatarios: ['Para: a@b.eu'],
        texto: '<img src=x onerror=alert(1)>',
        adjuntos: { titulo: 'Adjuntos · 1', nombres: ['<b>.pdf'] },
      },
    ],
  });
  expect(html).not.toContain('<script>');
  expect(html).not.toContain('<img src=x');
  expect(html).toContain('&lt;img src=x onerror=alert(1)&gt;');
  expect(html).toContain('<li>&lt;b&gt;.pdf</li>');
  expect(html).toContain("default-src 'none'");
  expect(html).toContain('<html lang="es">');
});

test('imprimirEnMarco imprime al cargar y retira el marco anterior', async () => {
  let impresos = 0;
  const primero = imprimirEnMarco('<p>uno</p>');
  const viejo = document.querySelector<HTMLIFrameElement>(`iframe[${ATRIBUTO_MARCO}]`);
  void primero.catch(() => {});
  const segundo = imprimirEnMarco('<p>dos</p>');
  expect(viejo?.isConnected).toBe(false);
  const marco = document.querySelector<HTMLIFrameElement>(`iframe[${ATRIBUTO_MARCO}]`)!;
  expect(document.querySelectorAll(`iframe[${ATRIBUTO_MARCO}]`)).toHaveLength(1);
  const ventana = marco.contentWindow!;
  ventana.print = () => {
    impresos++;
  };
  marco.dispatchEvent(new Event('load'));
  await segundo;
  expect(impresos).toBe(1);
  // Se queda montado hasta que el diálogo se cierra (Safari no bloquea en print()).
  expect(marco.isConnected).toBe(true);
  ventana.dispatchEvent(new Event('afterprint'));
  expect(marco.isConnected).toBe(false);
});

test('un correo mal formado no se traga ni esconde los mensajes que vienen detrás', () => {
  const base = { fecha: '5 de noviembre', destinatarios: [] };
  const html = documentoDeImpresion({
    idioma: 'es',
    asunto: 'Hilo',
    mensajes: [
      { ...base, remitente: 'De: a', html: '<body bgcolor="#000"><div>Hola <!-- sin cerrar' },
      { ...base, remitente: 'De: b', html: '<p>Antes</p><plaintext>crudo' },
      { ...base, remitente: 'De: c', texto: 'Respuesta que debe verse' },
    ],
  });
  // Se vuelve a analizar como lo haría el navegador en el marco.
  const doc = new DOMParser().parseFromString(html, 'text/html');
  const mensajes = doc.querySelectorAll('article.sytue-imp-mensaje');
  expect(mensajes).toHaveLength(3);
  expect(mensajes[2]?.textContent).toContain('Respuesta que debe verse');
  expect(doc.body.getAttribute('bgcolor')).toBeNull();
  expect(doc.querySelector('plaintext')).toBeNull();
});
