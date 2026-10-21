/*
 * SPDX-License-Identifier: Apache-2.0
 * SPDX-FileCopyrightText: (c) 2026 Daniel Delgado Ramos
 *
 * Abrir un fichero de Drive en el editor externo (WOPI), estilo Office Online.
 *
 * El editor lo despliega quien opera la suite (su Collabora, su Office for the web): la
 * suite no incorpora motores AGPL. Si no hay ninguno configurado, `wopiExtensions()`
 * devuelve un conjunto vacío y la UI simplemente no ofrece la acción.
 */

/** Extensiones que el editor sabe abrir. Vacío = no hay editor configurado. */
let cache: Promise<ReadonlySet<string>> | null = null;

/**
 * Extensiones abribles, pedidas UNA vez por carga de página.
 *
 * Se memoriza porque el menú de acciones se monta una vez por fila: sin esto, una
 * carpeta con doscientos ficheros dispararía doscientas peticiones idénticas.
 */
export function wopiExtensions(): Promise<ReadonlySet<string>> {
  cache ??= fetch('/api/wopi', { credentials: 'include', headers: { accept: 'application/json' } })
    .then((r) => (r.ok ? r.json() : { extensions: [] }))
    .then(
      (d: { extensions?: string[] }) => new Set((d.extensions ?? []).map((e) => e.toLowerCase())),
    )
    .catch(() => new Set<string>());
  return cache;
}

/** Extensión de un nombre de fichero, en minúsculas y sin punto ('' si no tiene). */
export function extensionOf(name: string): string {
  const i = name.lastIndexOf('.');
  return i < 0 || i === name.length - 1 ? '' : name.slice(i + 1).toLowerCase();
}

/**
 * Formatos que Sytue Docs sabe abrir (services/docs/internal/domain/import.go).
 *
 * Es una lista fija y no una pregunta al servidor: son los formatos que el propio
 * editor de la suite convierte, no los de un editor que despliegue otro, así que no
 * cambian de una instalación a otra. Si allí se añade uno, se añade aquí.
 */
const DOCS_EXTENSIONS: ReadonlySet<string> = new Set([
  'docx',
  'xlsx',
  'pptx',
  'odt',
  'ods',
  'odp',
  'csv',
  'tsv',
  'md',
  'markdown',
  'html',
  'htm',
  'txt',
  'text',
  'pdf',
  // iWork: se convierten al abrir; lo que no, se abre con su vista previa y un aviso.
  'pages',
  'numbers',
  'key',
]);

/** ¿Puede Sytue Docs abrir este fichero? */
export function openableInDocs(name: string): boolean {
  return DOCS_EXTENSIONS.has(extensionOf(name));
}

/**
 * Abre el fichero en el editor.
 *
 * El editor exige recibir el `access_token` por POST de formulario, no en la URL: en la
 * query quedaría en el historial del navegador y en los registros de cualquier proxy por
 * el que pase. Por eso se envía un formulario y no un `window.open`.
 *
 * `target` es dónde se abre: por defecto una pestaña nueva, o el `name` de un iframe
 * para previsualizar sin salir de la carpeta. Es el mismo POST en los dos casos —lo
 * único que cambia es la ventana que lo recibe—, así que no hacen falta dos caminos.
 *
 * Devuelve false si no hay editor para ese fichero, para que quien llama pueda explicarlo
 * en vez de dejar una pestaña (o un marco) en blanco.
 */
export async function openInEditor(
  id: string,
  mode: 'edit' | 'view' = 'edit',
  target = '_blank',
): Promise<boolean> {
  const res = await fetch('/api/wopi', {
    method: 'POST',
    credentials: 'include',
    headers: { 'content-type': 'application/json', accept: 'application/json' },
    body: JSON.stringify({ id, ...(mode === 'view' ? { mode: 'view' } : {}) }),
  });
  if (!res.ok) return false;
  const data = (await res.json()) as {
    editorUrl?: string;
    accessToken?: string;
    accessTokenTtl?: number;
  };
  if (!data.editorUrl || !data.accessToken) return false;

  const form = document.createElement('form');
  form.method = 'POST';
  form.action = data.editorUrl;
  form.target = target;
  // Solo tiene sentido en una ventana nueva: sin esto el editor tendría
  // `window.opener` sobre la pestaña de Drive y podría navegarla a donde quisiera.
  // En un iframe no hay `opener`, y ponerlo ahí no haría nada.
  if (target.startsWith('_')) form.rel = 'noopener';
  form.style.display = 'none';
  form.append(hidden('access_token', data.accessToken));
  if (data.accessTokenTtl) form.append(hidden('access_token_ttl', String(data.accessTokenTtl)));
  document.body.append(form);
  form.submit();
  form.remove();
  return true;
}

function hidden(name: string, value: string): HTMLInputElement {
  const el = document.createElement('input');
  el.type = 'hidden';
  el.name = name;
  el.value = value;
  return el;
}
