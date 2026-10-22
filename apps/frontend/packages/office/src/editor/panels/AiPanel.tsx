/*
 * SPDX-License-Identifier: Apache-2.0
 * SPDX-FileCopyrightText: (c) 2026 Daniel Delgado Ramos
 *
 * Panel «Asistente de IA» (patrón del carril izquierdo, como RevisionPanel):
 * acciones rápidas sobre la SELECCIÓN o el DOCUMENTO —resumir, reescribir,
 * traducir, mejorar, corregir, explicar, continuar— y un chat libre. La petición
 * se compone con `buildAiRequest` (puro, testeado), se envía a POST /api/ai (el
 * BFF añade la sesión y proxya al servicio de IA) y la respuesta se muestra para
 * Insertar / Reemplazar la selección / Copiar. El resultado se inserta como TEXTO
 * PLANO (insertText, canvas/exec): la respuesta de la IA nunca inyecta HTML.
 */
import type { ComponentChildren, JSX } from 'preact';
import { useEffect, useRef, useState } from 'preact/hooks';
import { Button, Spinner, Textarea, useToast } from '@sytue/ui';
import { getEditorRoot, getSelectionText, insertText } from '../canvas/exec';
import {
  dispatchSheetCommand,
  readSheetGrid,
  readSheetState,
  subscribeSheetState,
} from '../sheets/SheetCanvas';
import { cleanFormula } from '../sheets/aifill.js';
import { EditorIcon } from '../icons';
import { aiErrorCode, aiErrorKey, buildAiRequest, parseAiResponse, type AiAction } from '../ai';
import type { DocsKey } from '../../i18n';
import type { EditorContext } from '../types';
import { bloquesDe, type Bloque } from '@sytue/ai';
import { dispatchSlidesCommand } from '../slides/SlidesCanvas';

/** Receta de foco visible (regla de oro §0.6; nunca outline:none sin reemplazo). */
// El anillo canónico vive en `@sytue/ui/styles.css` (.sytue-focus-ring): una sola
// definición para toda la suite, en vez de la copia que cada fichero perdía a su manera.
const FOCUS_RING = 'sytue-focus-ring';

/** Chip de acción: borde sutil + texto, a juego con los del panel Buscar. */
const CHIP =
  'inline-flex h-7 items-center justify-center gap-1 rounded-sm border border-border bg-surface px-2 text-xs text-text ' +
  'transition-colors duration-150 hover:bg-bg-subtle disabled:pointer-events-none disabled:opacity-40 ' +
  FOCUS_RING;

/* Los tres mapas de abajo guardan CLAVES, no texto: si guardaran el rótulo ya
 * traducido, el array se congelaría con el idioma de la primera carga del módulo.
 * El texto lo resuelve la `t` del componente en cada render. */

/** Idiomas destino de «Traducir» (incluye las cooficiales de la UE ES).
 *  `as const satisfies` y no una anotación: así `LANGS[0]` es un literal y no
 *  `DocsKey | undefined` (el tsconfig lleva noUncheckedIndexedAccess). */
const LANGS = [
  'ai.lang.en',
  'ai.lang.fr',
  'ai.lang.de',
  'ai.lang.it',
  'ai.lang.pt',
  'ai.lang.ca',
  'ai.lang.gl',
  'ai.lang.eu',
  'ai.lang.es',
] as const satisfies readonly DocsKey[];

/** Acciones que operan sobre selección/documento (una fila de chips cada grupo). */
const QUICK: readonly { id: AiAction; labelKey: DocsKey }[] = [
  { id: 'resumir', labelKey: 'ai.action.summarise' },
  { id: 'mejorar', labelKey: 'ai.action.improve' },
  { id: 'corregir', labelKey: 'ai.action.proofread' },
  { id: 'explicar', labelKey: 'ai.action.explain' },
  { id: 'continuar', labelKey: 'ai.action.continue' },
];

const REWRITE: readonly { id: AiAction; labelKey: DocsKey }[] = [
  { id: 'reescribir-claro', labelKey: 'ai.rewrite.clearer' },
  { id: 'reescribir-formal', labelKey: 'ai.rewrite.formal' },
  { id: 'reescribir-corto', labelKey: 'ai.rewrite.shorter' },
];

export function AiPanel({ ctx }: { ctx: EditorContext }): JSX.Element {
  const t = ctx.t;
  const offline = ctx.connection === 'offline';
  const toast = useToast();
  const [busy, setBusy] = useState(false);
  const [result, setResult] = useState<string | null>(null);
  const [error, setError] = useState<string | null>(null);
  /** La última petición partió de una selección → habilita «Reemplazar selección». */
  const [hadSelection, setHadSelection] = useState(false);
  const [hasSelection, setHasSelection] = useState(false);
  const [chat, setChat] = useState('');
  /** Clave del idioma destino; el nombre traducido es lo que viaja en el prompt. */
  const [langKey, setLangKey] = useState<DocsKey>(LANGS[0]);
  /** Petición en vuelo, para poder cancelarla (o descartarla al desmontar). */
  const abortRef = useRef<AbortController | null>(null);

  /** ¿Estamos en una hoja? Ahí insertar no es escribir texto en un lienzo. */
  const enHoja = ctx.kind === 'spreadsheet';

  // Indicador «sobre la selección / sobre el documento»: refresca con la selección
  // (rebote 150 ms para no re-evaluar en cada tecla del arrastre). Sin toString del
  // rango: solo comprueba presencia, barato aunque la selección sea enorme.
  useEffect(() => {
    if (enHoja) {
      const update = (): void => {
        setHasSelection(Boolean(readSheetState()?.selection));
      };
      update();
      return subscribeSheetState(update);
    }
    let timer = 0;
    const update = (): void => {
      window.clearTimeout(timer);
      timer = window.setTimeout(() => setHasSelection(getSelectionText().trim() !== ''), 150);
    };
    update();
    document.addEventListener('selectionchange', update);
    return () => {
      window.clearTimeout(timer);
      document.removeEventListener('selectionchange', update);
    };
  }, [ctx.contentEpoch, enHoja]);

  // Descartar la petición en vuelo si el panel se cierra/desmonta.
  useEffect(() => () => abortRef.current?.abort(), []);

  /** Texto del documento vivo (el HTML del lienzo ES el modelo); '' sin lienzo. */
  const documentText = (): string => getEditorRoot()?.innerText ?? '';

  /** Lanza una acción/chat: compone la petición, la envía y muestra la respuesta. */
  const run = async (action: AiAction | 'chat', prompt = ''): Promise<void> => {
    if (offline) return;
    let selection = '';
    let context = '';
    if (enHoja) {
      const sheetState = readSheetState();
      selection = sheetState?.selection ?? '';
      if (selection) {
        const grid = readSheetGrid(selection);
        if (grid && grid.length > 0) {
          context =
            `Selection: ${selection}\nData:\n` + grid.map((row) => row.join('\t')).join('\n');
        } else {
          context = `Selection: ${selection}`;
        }
      }
    } else {
      selection = getSelectionText().trim();
      context = selection || (action === 'chat' ? '' : documentText());
    }
    if (action === 'chat' && prompt.trim() === '') return;
    if (action !== 'chat' && context === '') {
      setResult(null);
      setError(t('ai.needsText'));
      return;
    }

    const body = buildAiRequest({ action, prompt, context, kind: ctx.kind });
    abortRef.current?.abort(); // solo una en vuelo
    const ctrl = new AbortController();
    abortRef.current = ctrl;
    setBusy(true);
    setError(null);
    setResult(null);
    setHadSelection(selection !== '');
    try {
      const res = await fetch('/api/ai', {
        method: 'POST',
        headers: { 'content-type': 'application/json' },
        body: JSON.stringify(body),
        signal: ctrl.signal,
      });
      const data: unknown = await res.json().catch(() => null);
      if (!res.ok) {
        setError(t(aiErrorKey(aiErrorCode(data))));
        return; // el `finally` de abajo sigue corriendo: suelta el candado igual
      }
      const text = parseAiResponse(data);
      if (text === null) setError(t('ai.error.badResponse'));
      else setResult(text);
    } catch {
      if (ctrl.signal.aborted) return; // cancelado a propósito: sin mensaje de error
      setError(t(aiErrorKey()));
    } finally {
      if (abortRef.current === ctrl) {
        abortRef.current = null;
        setBusy(false);
      }
    }
  };

  /**
   * Escribe los bloques del asistente CON EL FORMATO del documento, usando los
   * comandos del propio editor.
   *
   * Antes se insertaba la respuesta como texto plano, y los modelos escriben en
   * Markdown aunque se les pida que no: el documento acababa lleno de
   * `**Introducción**` y `1. **Ámbito**` en crudo, con los asteriscos a la vista
   * y sin un solo título de verdad. Es lo que se reportó.
   *
   * NO SE INYECTA HTML, que era —y sigue siendo— la regla de este panel: la
   * respuesta es texto de un tercero. Se traduce a una estructura de tipos
   * cerrados en `@sytue/ai` y aquí se pinta llamando a `heading`, `listBullet`,
   * `listNumber` y `bold`, los mismos comandos que usa quien escribe a mano.
   */
  const escribirBloques = (bloques: Bloque[]): void => {
    let listaAbierta: 'ul' | 'ol' | null = null;
    bloques.forEach((bloque, i) => {
      if (i > 0) insertText('\n', false);

      if (bloque.tipo === 'li') {
        const quiere = bloque.ordenada ? 'ol' : 'ul';
        // Solo se conmuta al ENTRAR y al SALIR de la lista: llamar al comando en
        // cada punto la cerraría y volvería a abrir, y saldrían N listas de uno.
        if (listaAbierta !== quiere) {
          if (listaAbierta) ctx.exec(listaAbierta === 'ol' ? 'listNumber' : 'listBullet');
          ctx.exec(quiere === 'ol' ? 'listNumber' : 'listBullet');
          listaAbierta = quiere;
        }
      } else {
        if (listaAbierta) {
          ctx.exec(listaAbierta === 'ol' ? 'listNumber' : 'listBullet');
          listaAbierta = null;
        }
        ctx.exec('heading', bloque.tipo === 'p' ? 'p' : bloque.tipo);
      }

      for (const trozo of bloque.trozos) {
        // El énfasis se enciende, se escribe y se apaga: `insertText` hereda el
        // formato activo en el cursor, así que no hace falta seleccionar nada.
        if (trozo.negrita) ctx.exec('bold');
        if (trozo.cursiva) ctx.exec('italic');
        insertText(trozo.texto, false);
        if (trozo.cursiva) ctx.exec('italic');
        if (trozo.negrita) ctx.exec('bold');
      }
    });
    // Se cierra lo que quede abierto: si no, lo siguiente que escriba la persona
    // sale dentro de la lista del asistente.
    if (listaAbierta) ctx.exec(listaAbierta === 'ol' ? 'listNumber' : 'listBullet');
  };

  /** Inserta (o reemplaza la selección con) el resultado; lo consume si lo logra. */
  const apply = (replace: boolean): void => {
    if (result === null) return;

    // EN UNA HOJA NO SE INSERTA TEXTO, SE RELLENA. `insertText` escribe en el
    // lienzo `contenteditable` del editor de texto, que en Calcus no es la
    // rejilla: el resultado no llegaba a ninguna celda. Y aunque llegara, pegar
    // la misma fórmula en toda una columna deja veinte veces `=SUMA(A1:B1)`.
    //
    // El comando `aifill` lo resuelve donde corresponde: la hoja SÍ sabe qué
    // tiene seleccionado, así que rellena ella y adapta las referencias con la
    // misma pieza que el autorrelleno.
    if (enHoja) {
      dispatchSheetCommand('aifill', cleanFormula(result));
      toast.notify({ tone: 'success', title: t('ai.filled') });
      setResult(null);
      return;
    }

    // EN DIAPOS, INSERTAR ES CREAR LÁMINAS. Escribir la respuesta dentro de la
    // caja de texto seleccionada dejaba una diapositiva con seiscientas palabras
    // y ningún título. La reparte el lienzo, que es quien sabe dónde está la
    // diapositiva actual y con qué plantilla nacen las nuevas.
    if (ctx.kind === 'presentation' && !replace) {
      dispatchSlidesCommand('iafill', result);
      toast.notify({ tone: 'success', title: t('ai.slidesAdded') });
      setResult(null);
      return;
    }

    const bloques = bloquesDe(result);
    // Con un solo párrafo sin formato no se monta nada: se inserta como siempre,
    // que además conserva «Reemplazar selección» tal cual funcionaba.
    const conFormato =
      bloques.length > 1 ||
      bloques.some((b) => b.tipo !== 'p' || b.trozos.some((x) => x.negrita || x.cursiva));

    if (conFormato && !replace) {
      escribirBloques(bloques);
      toast.notify({ tone: 'success', title: t('ai.inserted') });
      setResult(null);
      return;
    }

    if (insertText(result, replace)) {
      toast.notify({
        tone: 'success',
        title: replace ? t('ai.replaced') : t('ai.inserted'),
      });
      setResult(null);
    } else {
      toast.notify({ tone: 'danger', title: t('ai.insertError') });
    }
  };

  const copy = (): void => {
    if (result === null) return;
    void navigator.clipboard
      .writeText(result)
      .then(() => toast.notify({ tone: 'success', title: t('ai.copied') }))
      .catch(() => toast.notify({ tone: 'danger', title: t('ai.copyError') }));
  };

  return (
    <div class="flex min-h-full flex-col gap-4 p-3">
      {/* Ámbito de la acción: la selección si la hay, si no el documento entero. */}
      <p class="sytue-meta flex items-center gap-1.5">
        <EditorIcon name="sparkles" size="sm" />
        {enHoja
          ? t('ai.scope.sheet')
          : hasSelection
            ? t('ai.scope.selection')
            : t('ai.scope.document')}
      </p>

      <Section label={t('ai.section.actions')}>
        <div class="flex flex-wrap gap-1.5">
          {QUICK.map((a) => (
            <button
              key={a.id}
              type="button"
              class={CHIP}
              disabled={busy || offline}
              title={t(a.labelKey)}
              onMouseDown={(e) => e.preventDefault()} // no robar la selección del lienzo
              onClick={() => void run(a.id)}
            >
              {t(a.labelKey)}
            </button>
          ))}
        </div>
      </Section>

      <Section label={t('ai.section.rewrite')}>
        <div class="flex flex-wrap gap-1.5">
          {REWRITE.map((a) => (
            <button
              key={a.id}
              type="button"
              class={CHIP}
              disabled={busy || offline}
              // Frase entera del catálogo: en alemán el complemento no va detrás.
              title={t('ai.rewrite.title', { name: t(a.labelKey) })}
              onMouseDown={(e) => e.preventDefault()}
              onClick={() => void run(a.id)}
            >
              {t(a.labelKey)}
            </button>
          ))}
        </div>
      </Section>

      <Section label={t('ai.section.translate')}>
        <div class="flex flex-wrap items-center gap-1.5">
          {/* Select nativo: idioma destino; viaja como `prompt` de buildAiRequest. */}
          <select
            class={
              'border-border bg-surface text-text hover:bg-bg-subtle h-7 cursor-pointer rounded-sm border px-1.5 text-xs ' +
              FOCUS_RING
            }
            aria-label={t('ai.lang.label')}
            value={langKey}
            disabled={busy || offline}
            onChange={(e) => setLangKey(e.currentTarget.value as DocsKey)}
          >
            {LANGS.map((key) => (
              <option key={key} value={key}>
                {t(key)}
              </option>
            ))}
          </select>
          <button
            type="button"
            class={CHIP}
            disabled={busy || offline}
            title={t('ai.translate.title', { language: t(langKey) })}
            onMouseDown={(e) => e.preventDefault()}
            onClick={() => void run('traducir', t(langKey))}
          >
            {t('ai.translate')}
          </button>
        </div>
      </Section>

      <Section label={t('ai.section.chat')}>
        <Textarea
          rows={3}
          value={chat}
          placeholder={t('ai.chat.placeholder')}
          aria-label={t('ai.chat.label')}
          disabled={busy || offline}
          onInput={(e) => setChat(e.currentTarget.value)}
          onKeyDown={(e) => {
            if (e.key === 'Enter' && (e.metaKey || e.ctrlKey)) {
              e.preventDefault();
              void run('chat', chat);
            }
          }}
        />
        <div class="mt-1.5 flex justify-end">
          <Button
            size="sm"
            disabled={busy || offline || chat.trim() === ''}
            onClick={() => void run('chat', chat)}
          >
            {t('ai.chat.send')}
          </Button>
        </div>
      </Section>

      {/* Zona de respuesta: cargando · error · resultado con acciones. */}
      <div class="border-border mt-auto border-t pt-3" role="status" aria-live="polite">
        {busy ? (
          <div class="text-text-muted flex items-center gap-2 text-sm">
            <Spinner size={16} />
            <span>{t('ai.generating')}</span>
            <button
              type="button"
              class={CHIP + ' ml-auto'}
              title={t('action.cancel')}
              onClick={() => abortRef.current?.abort()}
            >
              {t('action.cancel')}
            </button>
          </div>
        ) : error !== null ? (
          // -text y no el token a secas: --sytue-danger es el color de RELLENO (para
          // fondos con texto blanco) y no se remapea en tema oscuro, así que como
          // color de texto se queda en 2,95:1. --sytue-danger-text sí invierte.
          <p class="text-danger-text text-sm" role="alert">
            {error}
          </p>
        ) : result !== null ? (
          <div class="flex flex-col gap-2">
            {/* Resultado seleccionable; preserva saltos de línea del modelo. */}
            <div class="border-border bg-bg-subtle text-text max-h-64 overflow-y-auto rounded-md border p-2 text-sm whitespace-pre-wrap">
              {result}
            </div>
            <div class="flex flex-wrap gap-1.5">
              <button
                type="button"
                class={CHIP}
                title={t('ai.result.insert.title')}
                onMouseDown={(e) => e.preventDefault()}
                onClick={() => apply(false)}
              >
                <EditorIcon name="plus" size="sm" />
                {t('ai.result.insert')}
              </button>
              <button
                type="button"
                class={CHIP}
                disabled={!hadSelection}
                title={hadSelection ? t('ai.result.replace.title') : t('ai.result.replace.none')}
                onMouseDown={(e) => e.preventDefault()}
                onClick={() => apply(true)}
              >
                {t('ai.result.replace')}
              </button>
              <button type="button" class={CHIP} title={t('ai.result.copy')} onClick={copy}>
                <EditorIcon name="copy" size="sm" />
                {t('ai.result.copy')}
              </button>
              <button
                type="button"
                class={CHIP}
                title={t('ai.result.discard')}
                onClick={() => setResult(null)}
              >
                {t('ai.result.discard')}
              </button>
            </div>
          </div>
        ) : (
          <p class="sytue-meta">{t('ai.empty')}</p>
        )}
      </div>
    </div>
  );
}

/** Bloque con rótulo pequeño encima (densidad de panel). */
function Section({ label, children }: { label: string; children: ComponentChildren }): JSX.Element {
  return (
    <section class="flex flex-col gap-2">
      <h3 class="sytue-overline text-text-subtle">{label}</h3>
      {children}
    </section>
  );
}
