/*
 * SPDX-License-Identifier: Apache-2.0
 * SPDX-FileCopyrightText: (c) 2026 Daniel Delgado Ramos
 *
 * Bloque de EMBED L4 (Sytue Integration Gateway, ola 5): incrusta un informe de
 * Power BI dentro de un documento como contenido propio de la suite. Igual que la
 * imagen, el HTML del folio ES el modelo (§6.5): lo PERSISTIDO es solo un placeholder
 * `[data-sytue-embed]` con la REFERENCIA (instancia + reportId), contenteditable=false
 * (objeto atómico) y una leyenda estática que hace de SNAPSHOT de respaldo (impresión/
 * export). El iframe vivo se HIDRATA aparte y jamás entra en el modelo (stripEmbedLive).
 *
 * MODELO DE SEGURIDAD (innegociable):
 *  - El token/credencial del SaaS NUNCA llega al navegador: el servidor (gateway)
 *    mintea un artefacto de embed acotado (embed token de Power BI, TTL corto). El BFF
 *    (/api/embed) lo pide server-to-server con el token del usuario (passthrough).
 *  - El embed token NO viaja en query string ni se persiste (localStorage/DOM): vive
 *    en memoria y se entrega al iframe por postMessage con TARGET ORIGIN ESTRICTO.
 *  - El iframe va SANDBOX y su src se restringe a los orígenes permitidos (EMBED_ORIGINS
 *    = «frame-src» del embed; hoy app.powerbi.com — follow-up: derivarlo del manifiesto).
 *
 * ponytail: camino ZERO-DEP. El render PLENO de app-owns-data lo secuencia el SDK
 * `powerbi-client` (protocolo postMessage versionado con el host del embed); aquí se
 * hace el handshake MÍNIMO (entrega del token por canal seguro) y se enmarca la
 * embedUrl. Añadir `powerbi-client` queda como follow-up MARCADO (no se toca package.json).
 */
import { editorT } from '../../i18n';

/** Orígenes permitidos para enmarcar (equivalente a CSP frame-src del embed). Hoy la
 *  nube pública de Power BI; las soberanas añadirían el suyo (follow-up: del manifiesto). */
const EMBED_ORIGINS = new Set(['https://app.powerbi.com']);

/** Referencia persistida de un embed (data-* del placeholder). */
interface EmbedRef {
  instanceId: string;
  reportId: string;
  groupId: string;
}

/** Respuesta del mint (BFF → gateway). El token vive en memoria; nunca al DOM. */
interface EmbedResponse {
  embed_url?: string;
  token?: string;
  token_type?: string;
  expires_at?: string;
}

/* ---------- inserción (placeholder = modelo persistido) ---------- */

const ATTR_ESCAPES: Record<string, string> = {
  '&': '&amp;',
  '<': '&lt;',
  '>': '&gt;',
  '"': '&quot;',
  "'": '&#39;',
};

/** Escapa un valor para contexto de atributo HTML (frontera de confianza: report/group
 *  son entrada del usuario que se interpola en la cadena de insertHTML). */
function escapeAttr(text: string): string {
  return text.replace(/[&<>"']/g, (ch) => ATTR_ESCAPES[ch] ?? ch);
}

/**
 * HTML del bloque a insertar en el caret. Persiste SOLO la referencia + la leyenda
 * estática (snapshot de respaldo para impresión/export). El iframe lo pone hydrate.
 * El `<p><br></p>` final da destino al caret tras un objeto atómico (como la tabla).
 */
export function buildEmbedHtml(instanceId: string, reportId: string, groupId = ''): string {
  const t = editorT();
  const ref = escapeAttr(reportId);
  return (
    `<div data-sytue-embed data-provider="powerbi" data-instance="${escapeAttr(instanceId)}" ` +
    `data-report="${ref}" data-group="${escapeAttr(groupId)}" contenteditable="false">` +
    `<div class="sytue-embed-fallback">` +
    `<span class="sytue-embed-fallback-title">${t('embed.powerbi.title')}</span>` +
    `<span class="sytue-embed-fallback-ref">${ref}</span>` +
    `<span class="sytue-embed-fallback-note">${t('embed.powerbi.note')}</span>` +
    `</div></div><p><br></p>`
  );
}

/* ---------- serialización: fuera el iframe vivo (mantiene el modelo limpio) ---------- */

/**
 * Devuelve el HTML del folio SIN las capas vivas del embed (`[data-embed-live]`), de
 * modo que el autosave persiste solo el placeholder + snapshot (jamás el iframe ni,
 * por construcción, el token —que ni siquiera está en el DOM—). Fast-path: si no hay
 * ningún embed vivo, no se parsea (coste cero en el tecleo normal).
 */
export function stripEmbedLive(html: string): string {
  if (!html.includes('data-embed-live')) return html;
  const doc = new DOMParser().parseFromString(html, 'text/html');
  doc.querySelectorAll('[data-embed-live]').forEach((n) => n.remove());
  return doc.body.innerHTML;
}

/* ---------- hidratación (iframe vivo, fuera del modelo) ---------- */

/**
 * Hidrata todos los placeholders de embed del folio: mintea el artefacto server-side
 * (BFF) y enmarca el informe en un iframe sandbox. Idempotente por época (el folio se
 * re-monta desde el HTML en cada contentEpoch → capas vivas frescas). `signal` cancela
 * fetches y listeners al re-hidratar/desmontar (lo pasa el efecto de PageCanvas).
 */
export function hydrateEmbeds(root: HTMLElement, signal: AbortSignal): void {
  ensureEmbedStyle();
  root
    .querySelectorAll<HTMLElement>('[data-sytue-embed]')
    .forEach((host) => hydrateOne(host, signal));
}

function hydrateOne(host: HTMLElement, signal: AbortSignal): void {
  if (host.querySelector(':scope > [data-embed-live]')) return; // ya hidratado esta época
  const ref: EmbedRef = {
    instanceId: host.getAttribute('data-instance') ?? '',
    reportId: host.getAttribute('data-report') ?? '',
    groupId: host.getAttribute('data-group') ?? '',
  };
  if (!ref.instanceId || !ref.reportId) return; // placeholder incompleto: solo snapshot

  const live = document.createElement('div');
  live.className = 'sytue-embed-live';
  live.setAttribute('data-embed-live', '');
  live.append(stateNode('spinner', 'Cargando el informe…'));
  host.appendChild(live);

  void mintAndFrame(live, ref, signal);
}

async function mintAndFrame(live: HTMLElement, ref: EmbedRef, signal: AbortSignal): Promise<void> {
  let embed: EmbedResponse;
  try {
    const res = await fetch('/api/embed', {
      method: 'POST',
      headers: { 'content-type': 'application/json' },
      body: JSON.stringify({
        instance_id: ref.instanceId,
        report_id: ref.reportId,
        group_id: ref.groupId || undefined,
      }),
      signal,
    });
    if (!res.ok) {
      // Quien no está en la lista de la conexión (NEG-06) tiene que saber por qué y a
      // quién pedírselo, no ver un «no se pudo» que parece una avería.
      const cuerpo = (await res.json().catch(() => null)) as { error?: { code?: string } } | null;
      const fuera = res.status === 403 && cuerpo?.error?.code === 'connection_not_allowed';
      showState(
        live,
        'error',
        fuera ? editorT()('extdata.msg.notAllowed') : 'No se pudo autorizar el informe.',
      );
      return;
    }
    embed = (await res.json()) as EmbedResponse;
  } catch {
    if (signal.aborted) return;
    showState(live, 'error', 'No se pudo cargar el informe.');
    return;
  }

  const url = typeof embed.embed_url === 'string' ? embed.embed_url : '';
  const origin = safeEmbedOrigin(url);
  if (!origin) {
    showState(live, 'error', 'El origen del informe no está permitido.');
    return;
  }

  const iframe = document.createElement('iframe');
  iframe.className = 'sytue-embed-frame';
  iframe.title = 'Informe de Power BI'; // a11y: nombre accesible del marco
  // Sandbox: Power BI necesita scripts + su propio origen (almacenamiento/cookies del
  // embed); SIN allow-top-navigation (el marco no puede navegar nuestra página).
  iframe.setAttribute(
    'sandbox',
    'allow-scripts allow-same-origin allow-popups allow-forms allow-downloads',
  );
  iframe.setAttribute('referrerpolicy', 'no-referrer');
  iframe.setAttribute('allow', 'fullscreen');
  iframe.setAttribute('loading', 'lazy');
  // Canal seguro: al cargar, entrega el token acotado por postMessage al ORIGEN ESTRICTO
  // (jamás '*' ni en la query). ponytail: handshake mínimo; el render pleno de
  // app-owns-data lo secuencia powerbi-client (follow-up marcado).
  iframe.addEventListener(
    'load',
    () => {
      if (embed.token) {
        iframe.contentWindow?.postMessage(
          {
            type: 'sytue.embed.load',
            tokenType: embed.token_type ?? 'Embed',
            accessToken: embed.token,
          },
          origin,
        );
      }
    },
    { signal },
  );
  iframe.src = url; // embedUrl SIN token
  live.replaceChildren(iframe);
}

/** Origen del embedUrl si está en la lista permitida; null si no (no se enmarca). */
function safeEmbedOrigin(raw: string): string | null {
  try {
    const u = new URL(raw);
    return EMBED_ORIGINS.has(u.origin) ? u.origin : null;
  } catch {
    return null;
  }
}

/* ---------- estados (carga / error), a11y y dark por tokens ---------- */

function stateNode(kind: 'spinner' | 'error', message: string): HTMLElement {
  const wrap = document.createElement('div');
  wrap.className = 'sytue-embed-state';
  wrap.setAttribute('role', kind === 'error' ? 'alert' : 'status');
  if (kind !== 'error') wrap.setAttribute('aria-live', 'polite');
  if (kind === 'spinner') {
    const spin = document.createElement('div');
    spin.className = 'sytue-embed-spinner';
    spin.setAttribute('aria-hidden', 'true');
    wrap.append(spin);
  }
  const label = document.createElement('span');
  label.textContent = message;
  wrap.append(label);
  return wrap;
}

function showState(live: HTMLElement, kind: 'spinner' | 'error', message: string): void {
  live.replaceChildren(stateNode(kind, message));
}

/* Estilo del bloque, inyectado una vez (mismo patrón que ensureHeaderFooterStyle de
 * PageCanvas). Sobrio; dark automático vía los tokens --sytue-* (re-asignados por
 * [data-theme='dark'] / prefers-color-scheme). */
const EMBED_STYLE_ID = 'sytue-editor-embed';
function ensureEmbedStyle(): void {
  if (typeof document === 'undefined' || document.getElementById(EMBED_STYLE_ID)) return;
  const el = document.createElement('style');
  el.id = EMBED_STYLE_ID;
  el.textContent =
    // --embed-h: alto del bloque en UN sitio (lo hereda la leyenda); radios y cuerpos de
    // texto por la escala del sistema (--radius-*/--text-* de theme.css), no px sueltos.
    '.sytue-page [data-sytue-embed]{--embed-h:460px;position:relative;display:block;' +
    'min-height:var(--embed-h);margin:12px 0;border:1px solid var(--sytue-border);' +
    'border-radius:var(--radius-lg);background:var(--sytue-surface-sunken);overflow:hidden;}' +
    '.sytue-embed-fallback{display:flex;flex-direction:column;align-items:center;justify-content:center;' +
    'gap:4px;min-height:var(--embed-h);padding:24px;text-align:center;color:var(--sytue-text-muted);' +
    'font-size:var(--text-sm);}' +
    '.sytue-embed-fallback-title{font-weight:600;}' +
    '.sytue-embed-fallback-ref{font-family:var(--sytue-font-mono,ui-monospace,monospace);font-size:var(--text-xs);opacity:.85;}' +
    '.sytue-embed-fallback-note{font-size:var(--text-xs);opacity:.7;}' +
    '.sytue-embed-live{position:absolute;inset:0;background:var(--sytue-surface);}' +
    '.sytue-embed-frame{display:block;width:100%;height:100%;border:0;}' +
    '.sytue-embed-state{display:flex;flex-direction:column;align-items:center;justify-content:center;' +
    'gap:10px;height:100%;padding:24px;text-align:center;color:var(--sytue-text-muted);font-size:var(--text-sm);}' +
    // Anillo de 2 px enteros: 2,5 px cae entre píxeles y se ve borroso a DPR 1.
    '.sytue-embed-spinner{width:24px;height:24px;border-radius:var(--radius-pill);border:2px solid var(--sytue-border);' +
    'border-top-color:var(--sytue-text-muted);animation:sytue-embed-spin .7s linear infinite;}' +
    '@keyframes sytue-embed-spin{to{transform:rotate(360deg);}}' +
    '@media (prefers-reduced-motion:reduce){.sytue-embed-spinner{animation:none;}}' +
    // Impresión del navegador: el iframe no imprime; se muestra el snapshot estático.
    '@media print{.sytue-embed-live{display:none !important;}}';
  document.head.appendChild(el);
}
