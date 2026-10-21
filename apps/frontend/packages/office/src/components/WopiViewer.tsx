/*
 * SPDX-License-Identifier: Apache-2.0
 * SPDX-FileCopyrightText: (c) 2026 Daniel Delgado Ramos
 *
 * WopiViewer — la superficie que abre un fichero que vive en OTRO sitio.
 *
 * A esta página se llega desde el host (hoy Sytue Drive) por el puente
 * `/wopi/view`, que deja `src` y `token` en el FRAGMENTO de la URL. Lo primero
 * que se hace es leerlos y borrar el fragmento: a partir de ahí el token vive en
 * memoria y en ningún sitio más — ni en la barra, ni en el historial, ni en
 * `sessionStorage`, que en un iframe de otro sitio ni siquiera está garantizado.
 *
 * NO HAY SESIÓN DE SYTUE ID aquí, y es lo correcto: la credencial es el token
 * que acuñó el host tras autorizar a esa persona sobre ESE fichero. Por eso las
 * llamadas van directas a `/api/v1/wopi/*` (el servicio Go) y no por el BFF de
 * esta app, que lo que hace es añadir una cookie de sesión que aquí no existe.
 *
 * LO QUE SE PINTA VA DENTRO DE UN IFRAME, siempre. El contenido lo ha subido
 * alguien y volcarlo en el DOM de docs.sytue.eu sería ejecutar su HTML en
 * nuestro origen. Con `sandbox` vacío el navegador le da un origen opaco y sin
 * scripts: es la misma barrera que un saneador, hecha por el navegador, y
 * además impide que el CSS del documento se derrame sobre el cromo.
 */
import { useEffect, useMemo, useRef, useState } from 'preact/hooks';
import { Button, Spinner } from '@sytue/ui';
import { docsT } from '../i18n';
import { modeloAHtml } from './wopiRender';
import type { Locale } from '@sytue/i18n';

interface Props {
  locale: Locale;
}

/** Lo que devuelve `/api/v1/wopi/open`. */
interface Abierto {
  kind: string;
  title: string;
  content?: string;
  raw?: boolean;
  can_write: boolean;
  version?: string;
}

/** Lo que el puente dejó en el fragmento. */
interface Entrada {
  src: string;
  token: string;
  mode: string;
}

/**
 * Lee el fragmento y lo BORRA de la barra en el mismo paso.
 *
 * `replaceState` y no `pushState`: dejar la URL con el token como entrada del
 * historial es exactamente lo que se estaba evitando al no ponerlo en la query.
 */
function tomarEntrada(): Entrada | null {
  const bruto = window.location.hash.slice(1);
  if (!bruto) return null;
  window.history.replaceState(null, '', window.location.pathname);
  const p = new URLSearchParams(bruto);
  const src = p.get('src');
  const token = p.get('token');
  if (!src || !token) return null;
  return { src, token, mode: p.get('mode') === 'edit' ? 'edit' : 'view' };
}

export function WopiViewer({ locale }: Props) {
  const t = docsT(locale);
  const [doc, setDoc] = useState<Abierto | null>(null);
  const [error, setError] = useState<string | null>(null);
  const [blobUrl, setBlobUrl] = useState<string | null>(null);
  // El token NO entra en el estado: no tiene por qué provocar renders ni acabar
  // en las herramientas de desarrollo de Preact.
  const entrada = useRef<Entrada | null>(null);

  useEffect(() => {
    const e = tomarEntrada();
    if (!e) {
      setError(t('wopi.error.link'));
      return;
    }
    entrada.current = e;
    let vivo = true;
    let creada: string | null = null;

    void (async () => {
      try {
        const res = await fetch('/api/v1/wopi/open', {
          method: 'POST',
          headers: { 'content-type': 'application/json', accept: 'application/json' },
          body: JSON.stringify({ src: e.src, token: e.token, locale }),
        });
        if (!res.ok) throw new Error(String(res.status));
        const abierto = (await res.json()) as Abierto;
        if (!vivo) return;
        setDoc(abierto);
        document.title = `${abierto.title} — Sytue Docus`;

        // Lo que no se convierte se trae tal cual y se pinta con el motor del
        // propio navegador: un PDF que volviera distinto de como entró no sería
        // una previsualización, sería una alteración.
        if (abierto.raw) {
          const bin = await fetch('/api/v1/wopi/content', {
            method: 'POST',
            headers: { 'content-type': 'application/json' },
            body: JSON.stringify({ src: e.src, token: e.token }),
          });
          if (!bin.ok) throw new Error(String(bin.status));
          creada = URL.createObjectURL(await bin.blob());
          if (!vivo) return;
          setBlobUrl(creada);
        }
      } catch {
        if (vivo) setError(t('wopi.error.open'));
      }
    })();

    return () => {
      vivo = false;
      // Sin esto, el fichero entero se queda retenido en memoria mientras dure la
      // pestaña: un `createObjectURL` no lo suelta el recolector.
      if (creada) URL.revokeObjectURL(creada);
    };
    // Solo al montar: el fragmento se consume una vez y ya no está.
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, []);

  // Antes de los cortes de arriba: un hook no puede quedar detrás de un `return`.
  // Se memoriza porque pintar una hoja de 200x40 es construir 8.000 celdas de
  // cadena, y sin esto se rehace en cada render del cromo.
  const pintado = useMemo(
    () =>
      doc && !doc.raw && doc.content
        ? modeloAHtml(doc.kind, doc.content, t('wopi.truncated'))
        : null,
    // `t` está atada al idioma, que no cambia mientras la página vive.
    // eslint-disable-next-line react-hooks/exhaustive-deps
    [doc],
  );

  if (error) {
    return (
      <Marco titulo={t('wopi.page.title')}>
        <p class="text-text-muted max-w-prose text-center text-sm">{error}</p>
      </Marco>
    );
  }
  if (!doc) {
    return (
      <Marco titulo={t('wopi.page.title')}>
        <Spinner size={22} />
        <p class="text-text-muted text-sm">{t('wopi.loading')}</p>
      </Marco>
    );
  }

  return (
    <div class="bg-bg-subtle flex h-full min-h-0 flex-col">
      <header class="border-border bg-surface flex items-center gap-3 border-b px-4 py-2">
        <h1 class="text-text truncate text-sm font-medium">{doc.title}</h1>
        {/* Siempre visible mientras el guardado no exista: quien abre un fichero
            tiene que saber ANTES de escribir que lo que escriba no se guarda. */}
        <span class="border-border text-text-subtle rounded-full border px-2 py-0.5 text-xs">
          {t('wopi.readonly')}
        </span>
        <div class="ml-auto flex items-center gap-2">
          {blobUrl && (
            <Button
              variant="ghost"
              size="sm"
              onClick={() => window.open(blobUrl, '_blank', 'noopener')}
            >
              {t('wopi.openTab')}
            </Button>
          )}
        </div>
      </header>

      <main class="min-h-0 flex-1">
        {doc.raw ? (
          blobUrl ? (
            // Sin `sandbox`, y no por capricho: un marco en sandbox tiene origen
            // opaco, y un `blob:` solo lo puede cargar el origen que lo creó — con
            // sandbox el visor saldría vacío. El PDF lo pinta el navegador en su
            // propio proceso, no en el DOM de esta página.
            <iframe src={blobUrl} title={doc.title} class="h-full w-full border-0" />
          ) : (
            <Marco titulo="">
              <Spinner size={22} />
            </Marco>
          )
        ) : pintado ? (
          // `sandbox` vacío: origen opaco y sin scripts. El contenido lo ha subido
          // alguien, y esta es una barrera más dura que cualquier lista de
          // etiquetas permitidas — además de impedir que su CSS se derrame sobre
          // el cromo de la página.
          <iframe
            sandbox=""
            srcdoc={pintado}
            title={doc.title}
            class="mx-auto h-full w-full max-w-[1100px] border-0 bg-white"
          />
        ) : (
          <Marco titulo="">
            <p class="text-text-muted max-w-prose text-center text-sm">{t('wopi.notYet')}</p>
          </Marco>
        )}
      </main>
    </div>
  );
}

/** Centrado para los tres estados que no son «hay documento». */
function Marco({ titulo, children }: { titulo: string; children: preact.ComponentChildren }) {
  return (
    <div class="bg-bg-subtle flex h-full flex-col items-center justify-center gap-3 p-8">
      {titulo && <h1 class="text-text text-sm font-medium">{titulo}</h1>}
      {children}
    </div>
  );
}
