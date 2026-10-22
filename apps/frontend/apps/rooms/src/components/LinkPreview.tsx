/**
 * LinkPreview — tarjeta de vista previa (OpenGraph) bajo un mensaje de Sytue Chat
 * (Ola 16). Pide al BFF los metadatos de una URL (GET /api/unfurl, cacheado + SSRF-safe
 * en el backend) y pinta site_name + título + descripción + miniatura. Degrada a NADA
 * si la URL no expone metadatos útiles.
 *
 * IMAGEN: la sirve NUESTRO servidor, no el host remoto. `image_url` ya no es la URL
 * de origen sino la ruta del proxy (/api/v1/unfurl/imagen/<hash>), que devuelve una
 * miniatura generada aquí. Antes se pintaba directamente desde su host y eso hacía que
 * el tercero elegido por quien pegó el enlace viera la IP de CADA persona que leía el
 * canal; `referrerpolicy` no evitaba nada de eso, solo ocultaba de dónde venía.
 *
 * Además carga menos: la miniatura ronda las decenas de KB frente a los megas de una
 * portada, y deja de romperse cuando el origen está lento o caído. Un fallo de carga la
 * oculta igual (la tarjeta sigue mostrando el texto).
 *
 * Estética sobria (Mattermost/Linear): borde fino, sin degradados. Accesible: la
 * tarjeta es un enlace con etiqueta clara, target nueva pestaña y rel de seguridad.
 */
import type { JSX } from 'preact';
import { useEffect, useState } from 'preact/hooks';
import { unfurlLink, type LinkPreview as LinkPreviewData } from '../client/chatApi.js';
import { nombreDeProveedor, proveedorDeReunion } from '../lib/enlacesExternos.js';
import type { T } from '../i18n/index.js';
import { VideoGlyph } from './glyphs.js';

/**
 * Caché de módulo por URL: dedupe las peticiones entre re-renders y mensajes distintos
 * que citen el mismo enlace. Guarda la PROMESA (no solo el valor) para colapsar las
 * peticiones en vuelo. null = sin metadatos útiles (no se pinta tarjeta).
 */
const cache = new Map<string, Promise<LinkPreviewData | null>>();

function loadPreview(url: string): Promise<LinkPreviewData | null> {
  let p = cache.get(url);
  if (!p) {
    p = unfurlLink(url)
      .then(({ unfurl }) => (unfurl.title || unfurl.imageUrl ? unfurl : null))
      .catch(() => null); // degrada en silencio: sin tarjeta
    cache.set(url, p);
  }
  return p;
}

/** Hostname legible de la URL, como respaldo del site_name (p. ej. «example.com»). */
function hostOf(url: string): string {
  try {
    return new URL(url).hostname.replace(/^www\./, '');
  } catch {
    return '';
  }
}

export function LinkPreview({ url, t }: { url: string; t: T }): JSX.Element | null {
  const [preview, setPreview] = useState<LinkPreviewData | null>(null);
  const [imgOk, setImgOk] = useState(true);

  useEffect(() => {
    let live = true;
    setImgOk(true);
    void loadPreview(url).then((p) => live && setPreview(p));
    return () => {
      live = false;
    };
  }, [url]);

  if (!preview) return null;
  const site = preview.siteName || hostOf(preview.url);
  const showImage = Boolean(preview.imageUrl) && imgOk;

  return (
    <a
      href={preview.url}
      target="_blank"
      rel="noopener noreferrer"
      aria-label={preview.title ? t('link.previewOf', { title: preview.title }) : t('link.preview')}
      class="border-border bg-surface hover:bg-bg-subtle sytue-focus-ring mt-1.5 flex max-w-xl gap-3 rounded-md border p-2.5 no-underline transition-colors duration-150"
    >
      <div class="min-w-0 flex-1">
        {site ? <span class="sytue-meta block truncate">{site}</span> : null}
        {preview.title ? (
          <span class="text-text mt-0.5 block truncate text-sm font-semibold">{preview.title}</span>
        ) : null}
        {preview.description ? (
          <span class="text-text-muted mt-0.5 [display:-webkit-box] block overflow-hidden text-xs leading-snug [-webkit-box-orient:vertical] [-webkit-line-clamp:2]">
            {preview.description}
          </span>
        ) : null}
      </div>
      {showImage ? (
        <img
          src={preview.imageUrl}
          alt=""
          referrerpolicy="no-referrer"
          loading="lazy"
          decoding="async"
          onError={() => setImgOk(false)}
          class="h-16 w-16 shrink-0 rounded object-cover"
        />
      ) : null}
    </a>
  );
}

/**
 * Enlace a una reunión de Zoom, Google Meet, Teams o Webex: en vez de la vista previa
 * —esas páginas no exponen metadatos útiles sin JavaScript, y pedirlas avisaría al
 * proveedor de cada lectura—, una tarjeta que dice de quién es la reunión y deja
 * unirse con un toque. Devuelve null si el enlace no es de ninguna.
 */
export function MeetingLinkCard({ url, t }: { url: string; t: T }): JSX.Element | null {
  const proveedor = proveedorDeReunion(url);
  if (!proveedor) return null;
  const titulo = t('link.joinMeeting', { provider: nombreDeProveedor(proveedor) });
  return (
    <a
      href={url}
      target="_blank"
      rel="noopener noreferrer"
      aria-label={`${t('link.join')}: ${titulo}`}
      class="border-border bg-surface hover:bg-bg-subtle sytue-focus-ring mt-1.5 flex max-w-xl items-center gap-3 rounded-md border p-2.5 no-underline transition-colors duration-150"
    >
      <span class="bg-bg-subtle text-text-muted grid h-9 w-9 shrink-0 place-items-center rounded-md">
        <VideoGlyph size={18} />
      </span>
      <span class="text-text min-w-0 flex-1 truncate text-sm font-semibold">{titulo}</span>
      <span class="text-primary-text shrink-0 text-sm font-semibold">{t('link.join')}</span>
    </a>
  );
}
