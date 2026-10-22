/*
 * SPDX-License-Identifier: Apache-2.0
 * SPDX-FileCopyrightText: (c) 2026 Daniel Delgado Ramos
 *
 * Abrir un PDF que está en el ORDENADOR.
 *
 * El visor solo sabía trabajar con un documento ya guardado en Drive, así que
 * para mirar un fichero recién descargado había que subirlo a mano primero y
 * luego buscarlo. En iLovePDF se arrastra y ya está, y es lo que espera
 * cualquiera que llegue aquí con un fichero en el escritorio.
 *
 * También es lo que hace utilizable la app de escritorio: dentro de la webview
 * de Wails, un `<input type="file">` abre el diálogo de ficheros del SISTEMA, así
 * que el menú «Archivo → Abrir…» solo tiene que pulsar este botón.
 */
import { useRef, useState } from 'preact/hooks';
import { docsT } from '../i18n';
// Alias: en este fichero `r` ya nombra la respuesta de cada fetch.
import { r as ruta } from '../lib/rutas';
import type { Locale } from '@sytue/i18n';

export function AbrirPdf({ locale, embebido = false }: { locale: Locale; embebido?: boolean }) {
  const t = docsT(locale);
  const entrada = useRef<HTMLInputElement>(null);
  const camara = useRef<HTMLInputElement>(null);
  const [subiendo, setSubiendo] = useState(false);
  const [error, setError] = useState('');
  const [encima, setEncima] = useState(false);

  /**
   * Escanear: varias fotos → un PDF de varias páginas.
   *
   * Las fotos se mandan en UNA petición porque un escaneo de diez hojas es UN
   * documento; subirlas de una en una dejaría diez PDF sueltos en Drive.
   */
  const escanear = (fotos: File[]): void => {
    if (subiendo || fotos.length === 0) return;
    setSubiendo(true);
    setError('');
    void Promise.all(fotos.map(comoDataURL))
      .then((imagenes) =>
        fetch('/api/pdf-escanear', {
          method: 'POST',
          credentials: 'include',
          headers: { 'content-type': 'application/json' },
          body: JSON.stringify({ imagenes, nombre: t('pdf.open.scanName') }),
        }),
      )
      .then(async (r) => {
        if (!r.ok) {
          const c = (await r.json().catch(() => ({}))) as { error?: string };
          throw new Error(c.error ?? 'fallo');
        }
        const { path } = (await r.json()) as { path: string };
        window.location.href = ruta(`/pdf?path=${encodeURIComponent(path)}`);
      })
      .catch(() => {
        setError(t('pdf.open.scanFailed'));
        setSubiendo(false);
      });
  };

  const abrir = (f: File): void => {
    if (subiendo) return;
    setSubiendo(true);
    setError('');
    // Cuerpo CRUDO y nombre en `?name=`: así no dispara el checkOrigin (CSRF) de
    // Astro, que es el mismo patrón del resto de subidas de la suite.
    void fetch(`/api/pdf-abrir?name=${encodeURIComponent(f.name)}`, {
      method: 'POST',
      credentials: 'include',
      headers: { 'content-type': 'application/octet-stream' },
      body: f,
    })
      .then(async (r) => {
        if (!r.ok) {
          const cuerpo = (await r.json().catch(() => ({}))) as { error?: string };
          throw new Error(cuerpo.error ?? 'fallo');
        }
        const { path } = (await r.json()) as { path: string };
        // Se navega en vez de montar el visor aquí: la ruta ES el estado de esta
        // pantalla, y así recargar o compartir el enlace lleva al mismo sitio.
        window.location.href = ruta(`/pdf?path=${encodeURIComponent(path)}`);
      })
      .catch((e: Error) => {
        setError(t(e.message === 'not_a_pdf' ? 'pdf.open.notPdf' : 'pdf.open.failed'));
        setSubiendo(false);
      });
  };

  const caja = (
    <div
      onDragOver={(e) => {
        e.preventDefault();
        setEncima(true);
      }}
      onDragLeave={() => setEncima(false)}
      onDrop={(e) => {
        e.preventDefault();
        setEncima(false);
        const f = e.dataTransfer?.files?.[0];
        if (f) abrir(f);
      }}
      class={`flex w-full flex-col items-center gap-3 rounded-xl border-2 border-dashed text-center transition-colors ${
        embebido ? 'p-6' : 'max-w-md p-10'
      } ${encima ? 'border-accent bg-selected' : 'border-border bg-surface'}`}
    >
      {/* Embebida en la portada de CertiPDF el título de la vista es «Tus PDF»,
            y esto es una sección. Sola (sin portada) sigue siendo el h1. */}
      {embebido ? (
        <h2 class="text-text text-lg font-semibold">{t('pdf.open.title')}</h2>
      ) : (
        <h1 class="text-text text-lg font-semibold">{t('pdf.open.title')}</h1>
      )}
      <p class="text-text-muted text-sm">{t('pdf.open.drop')}</p>

      <button
        type="button"
        disabled={subiendo}
        onClick={() => entrada.current?.click()}
        class="bg-primary sytue-focus-ring rounded-control px-4 py-2 text-sm font-medium text-[var(--sytue-on-primary)] hover:bg-[var(--sytue-primary-hover)] disabled:opacity-40"
      >
        {subiendo ? t('pdf.open.opening') : t('pdf.open.pick')}
      </button>
      <input
        ref={entrada}
        type="file"
        accept="application/pdf,.pdf"
        hidden
        tabIndex={-1}
        onChange={(e) => {
          const f = (e.target as HTMLInputElement).files?.[0];
          (e.target as HTMLInputElement).value = ''; // volver a elegir el MISMO fichero
          if (f) abrir(f);
        }}
      />

      {/* Escanear con la cámara. `capture` la abre directamente en el móvil;
            en un ordenador el mismo control ofrece elegir imágenes, que es el
            caso de las fotos ya hechas. */}
      <button
        type="button"
        disabled={subiendo}
        onClick={() => camara.current?.click()}
        class="border-border text-text hover:bg-bg-subtle sytue-focus-ring rounded-control border px-4 py-2 text-sm font-medium disabled:opacity-40"
      >
        {t('pdf.open.scan')}
      </button>
      <input
        ref={camara}
        type="file"
        accept="image/*"
        capture="environment"
        multiple
        hidden
        tabIndex={-1}
        onChange={(e) => {
          const el = e.target as HTMLInputElement;
          const fotos = Array.from(el.files ?? []);
          el.value = '';
          escanear(fotos);
        }}
      />

      {error !== '' && (
        <p class="text-danger-text text-xs" role="alert">
          {error}
        </p>
      )}
      {/* Dónde acaba el fichero, dicho antes y no después: se guarda en Drive,
            y quien lo abre tiene derecho a saberlo sin tener que descubrirlo. */}
      <p class="text-text-subtle text-2xs">{t('pdf.open.savedIn')}</p>
    </div>
  );
  return embebido ? caja : <div class="flex h-full items-center justify-center p-6">{caja}</div>;
}

/** Un fichero como `data:…;base64,…`, que es como viaja al BFF. */
function comoDataURL(f: File): Promise<string> {
  return new Promise((resolver, rechazar) => {
    const lector = new FileReader();
    lector.onload = () =>
      typeof lector.result === 'string' ? resolver(lector.result) : rechazar(new Error('ilegible'));
    lector.onerror = () => rechazar(new Error('ilegible'));
    lector.readAsDataURL(f);
  });
}
