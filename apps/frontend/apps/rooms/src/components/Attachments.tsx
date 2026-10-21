/**
 * Attachments — previsualización de los adjuntos de un mensaje (ficheros de Sytue
 * Drive). Las imágenes (content_type image/*) se pintan como miniatura inline y, al
 * pulsarlas, abren un lightbox a tamaño completo con navegación entre las imágenes
 * del mensaje (Esc cierra, ← → cambian). El resto de ficheros se muestran como
 * tarjeta (icono + nombre + tamaño + descargar).
 *
 * Seguridad: los bytes se sirven vía el proxy BFF /api/attachment (nunca se expone
 * el token de Drive), con X-Content-Type-Options: nosniff y CSP sandbox en lo que
 * va inline — un SVG con <script> no ejecuta ni en <img> ni en navegación directa.
 */
import type { JSX } from 'preact';
import { createPortal } from 'preact/compat';
import { useEffect, useRef, useState } from 'preact/hooks';
import { cx, formatBytes, Icon, IconButton, useMobileBack, useToast } from '@sytue/ui';
import type { Locale } from '@sytue/i18n';
import type { Attachment } from '../client/chatApi.js';
import { attachmentUrl, saveAttachmentToDrive } from '../client/chatApi.js';
import type { T } from '../i18n/index.js';
import { roomsMatrix } from '../client/matrix/client.js';
import { DownloadGlyph, FileGlyph } from './glyphs.js';

function isImage(a: Attachment): boolean {
  return a.contentType.startsWith('image/') && (!a.matrixFile || a.contentType !== 'image/svg+xml');
}

/**
 * Adjuntos que el NAVEGADOR ya sabe reproducir: vídeo y audio con sus propios
 * controles. No hace falta reproductor ninguno —ni dependencia— y se hereda gratis
 * el teclado, los subtítulos del sistema, AirPlay y la velocidad de reproducción.
 *
 * `preload="metadata"`: se pide la cabecera para saber la duración y pintar la barra,
 * no el fichero entero. Un canal con tres vídeos de 200 MB no se descarga al abrirse.
 *
 * ponytail: se comprueba el tipo declarado, no el contenido. Drive sirve el suyo con
 * nosniff, así que un fichero mal etiquetado sale como tarjeta de descarga; que es lo
 * que hacía con todo hasta ahora.
 */
function isVideo(a: Attachment): boolean {
  return a.contentType.startsWith('video/');
}

function isAudio(a: Attachment): boolean {
  return a.contentType.startsWith('audio/');
}

/**
 * Tipos que el proxy de adjuntos sirve INLINE (`pages/api/attachment.ts`: nosniff +
 * `Content-Security-Policy: sandbox`), o sea los que se pueden abrir en una pestaña
 * y leer con el visor del navegador en vez de bajar el fichero.
 *
 * ponytail: se ofrece «Abrir», no un `<iframe>` incrustado. La respuesta va con
 * `sandbox`, que quita los scripts, y el visor de PDF de Firefox (pdf.js) ES un
 * script: incrustado se vería en blanco en unos navegadores y bien en otros. Una
 * pestaña usa el visor nativo y funciona en todos. Para incrustarlo habría que
 * relajar la cabecera —o servirlo por otra ruta— y eso ya no es una línea.
 */
function isReadableInline(a: Attachment): boolean {
  return a.contentType === 'application/pdf' || a.contentType.startsWith('text/plain');
}

/** Lightbox de imágenes de un mensaje (portal). Esc cierra; ← → navegan si hay más. */
function Lightbox({
  images,
  index,
  channelId,
  onIndex,
  onClose,
  t,
}: {
  images: Attachment[];
  index: number;
  channelId: string;
  onIndex: (i: number) => void;
  onClose: () => void;
  t: T;
}): JSX.Element {
  const closeRef = useRef<HTMLButtonElement>(null);
  const img = images[index]!;
  const many = images.length > 1;

  useMobileBack(true, onClose);

  useEffect(() => {
    closeRef.current?.focus();
    const onKey = (e: KeyboardEvent): void => {
      if (e.key === 'Escape') {
        e.preventDefault();
        onClose();
      } else if (e.key === 'ArrowRight' && many) {
        e.preventDefault();
        onIndex((index + 1) % images.length);
      } else if (e.key === 'ArrowLeft' && many) {
        e.preventDefault();
        onIndex((index - 1 + images.length) % images.length);
      }
    };
    document.addEventListener('keydown', onKey);
    return () => document.removeEventListener('keydown', onKey);
  }, [index, images.length, many, onClose, onIndex]);

  return createPortal(
    // El diálogo se cierra con Escape (listener global del useEffect de arriba); el clic en
    // el backdrop es un atajo de ratón, no la única vía. No es un elemento interactivo.
    // eslint-disable-next-line jsx-a11y/click-events-have-key-events, jsx-a11y/no-noninteractive-element-interactions
    <div
      role="dialog"
      aria-modal="true"
      aria-label={t('attachment.image', { name: img.name })}
      class="sytue-safe-fullscreen fixed inset-0 z-[60] flex flex-col bg-[var(--sytue-overlay)]"
      onClick={onClose}
    >
      {/* Barra: nombre + contador + descargar + cerrar. onClick solo frena la propagación
          para que pulsar la barra no cierre el visor; no es un control. Con el dedo,
          descargar y cerrar miden 44, como el resto de la suite (medían 36). */}
      {/* eslint-disable-next-line jsx-a11y/click-events-have-key-events, jsx-a11y/no-static-element-interactions */}
      <div
        class="text-neutral-0 flex shrink-0 items-center gap-3 px-4 py-3"
        onClick={(e) => e.stopPropagation()}
      >
        <span class="min-w-0 flex-1 truncate text-sm font-medium" title={img.name}>
          {img.name}
        </span>
        {many ? (
          <span class="text-neutral-0/70 shrink-0 text-xs tabular-nums">
            {index + 1} / {images.length}
          </span>
        ) : null}
        <a
          href={attachmentUrl(img, { download: true, channelId })}
          download={img.name}
          aria-label={t('attachment.download', { name: img.name })}
          title={t('action.download')}
          class="text-neutral-0/90 hover:bg-neutral-0/15 sytue-focus-ring rounded-control inline-flex h-9 w-9 items-center justify-center transition-colors duration-150 pointer-coarse:h-11 pointer-coarse:w-11"
        >
          <DownloadGlyph size={18} />
        </a>
        <button
          ref={closeRef}
          type="button"
          aria-label={t('action.close')}
          title={t('action.closeEsc')}
          onClick={onClose}
          class="text-neutral-0/90 hover:bg-neutral-0/15 sytue-focus-ring rounded-control inline-flex h-9 w-9 items-center justify-center transition-colors duration-150 pointer-coarse:h-11 pointer-coarse:w-11"
        >
          <Icon name="close" size={20} />
        </button>
      </div>

      <div class="relative flex min-h-0 flex-1 items-center justify-center px-4 pb-4">
        {many ? (
          <button
            type="button"
            aria-label={t('attachment.previousImage')}
            onClick={(e) => {
              e.stopPropagation();
              onIndex((index - 1 + images.length) % images.length);
            }}
            class="bg-neutral-0/10 text-neutral-0 hover:bg-neutral-0/20 sytue-focus-ring rounded-control absolute top-1/2 left-3 inline-flex h-11 w-11 -translate-y-1/2 items-center justify-center transition-colors duration-150"
          >
            <Icon name="chevronRight" size={22} class="rotate-180" />
          </button>
        ) : null}
        {/* onClick solo frena la propagación (pulsar la imagen no cierra el visor). */}
        {/* eslint-disable-next-line jsx-a11y/click-events-have-key-events, jsx-a11y/no-noninteractive-element-interactions */}
        <img
          src={attachmentUrl(img, { channelId })}
          alt={img.name}
          class="rounded-card max-h-full max-w-full object-contain shadow-[var(--shadow-xl)]"
          onClick={(e) => e.stopPropagation()}
        />
        {many ? (
          <button
            type="button"
            aria-label={t('attachment.nextImage')}
            onClick={(e) => {
              e.stopPropagation();
              onIndex((index + 1) % images.length);
            }}
            class="bg-neutral-0/10 text-neutral-0 hover:bg-neutral-0/20 sytue-focus-ring rounded-control absolute top-1/2 right-3 inline-flex h-11 w-11 -translate-y-1/2 items-center justify-center transition-colors duration-150"
          >
            <Icon name="chevronRight" size={22} />
          </button>
        ) : null}
      </div>
    </div>,
    document.body,
  );
}

export function Attachments({
  attachments: providedAttachments,
  channelId,
  t,
  locale,
}: {
  attachments?: Attachment[] | undefined;
  /** Traductor ya atado al idioma. */
  t: T;
  /** Idioma de quien lee: el tamaño se escribe con su separador decimal. */
  locale: Locale;
  /** Canal del mensaje: los adjuntos viven en el ámbito de su canal y Drive exige
   *  membresía para servirlos, así que viaja en todas sus URLs. */
  channelId: string;
}): JSX.Element | null {
  const encryptedRef = useRef<HTMLDivElement>(null);
  const [visible, setVisible] = useState(false);
  const [resolved, setResolved] = useState<Map<string, string>>(new Map());
  const [decryptFailed, setDecryptFailed] = useState(false);
  const [decryptTick, setDecryptTick] = useState(0);
  const attachments = providedAttachments?.map((file) =>
    resolved.has(file.fileId) ? { ...file, objectUrl: resolved.get(file.fileId)! } : file,
  );
  const pending = attachments?.some((file) => file.matrixFile && !file.objectUrl) === true;
  useEffect(() => {
    if (!pending || !encryptedRef.current) return;
    const observer = new IntersectionObserver((entries) => {
      if (entries.some((entry) => entry.isIntersecting)) setVisible(true);
    });
    observer.observe(encryptedRef.current);
    return () => observer.disconnect();
  }, [pending]);
  useEffect(() => {
    if (!visible) return;
    let stopped = false;
    const client = roomsMatrix();
    const created: string[] = [];
    setDecryptFailed(false);
    const files = providedAttachments?.filter((file) => file.matrixFile && !file.objectUrl) ?? [];
    void Promise.all(
      files.map(async (file) => {
        const url = await client.download(file);
        created.push(url);
        if (stopped) {
          client.releaseAttachment(url);
          return;
        }
        setResolved((previous) => new Map(previous).set(file.fileId, url));
      }),
    ).catch(() => {
      if (!stopped) setDecryptFailed(true);
    });
    return () => {
      stopped = true;
      for (const url of created) client.releaseAttachment(url);
    };
  }, [providedAttachments, visible, decryptTick]);
  const [lightbox, setLightbox] = useState<number | null>(null);
  const openerRef = useRef<HTMLElement | null>(null);
  const toast = useToast();
  /** fileId que se está guardando ahora: deshabilita su botón y frena el doble clic. */
  const [guardando, setGuardando] = useState<string | null>(null);

  /** Copia el adjunto a «Mi Drive». El fallo se cuenta: es una acción explícita y
   *  quedarse en silencio le haría creer al usuario que ya lo tiene guardado. */
  async function guardarEnDrive(a: Attachment): Promise<void> {
    if (guardando !== null || a.matrixFile) return;
    setGuardando(a.fileId);
    try {
      await saveAttachmentToDrive(a.fileId, channelId);
      toast.notify({ tone: 'success', title: t('attachment.savedToDrive', { name: a.name }) });
    } catch {
      toast.notify({ tone: 'danger', title: t('attachment.saveToDriveFailed') });
    } finally {
      setGuardando(null);
    }
  }

  if (!attachments || attachments.length === 0) return null;
  if (pending)
    return (
      <div ref={encryptedRef} class="mt-2">
        <button
          type="button"
          class="text-text-muted text-sm"
          onClick={() => {
            setVisible(true);
            setDecryptTick((value) => value + 1);
          }}
        >
          {t(decryptFailed ? 'action.retry' : 'state.loading')}
        </button>
      </div>
    );
  const images = attachments.filter(isImage);
  const media = attachments.filter((a) => isVideo(a) || isAudio(a));
  const files = attachments.filter((a) => !isImage(a) && !isVideo(a) && !isAudio(a));

  function openLightbox(imageIndex: number): void {
    openerRef.current = (document.activeElement as HTMLElement) ?? null;
    setLightbox(imageIndex);
  }
  function closeLightbox(): void {
    setLightbox(null);
    openerRef.current?.focus?.();
  }

  return (
    <div class="mt-1.5 flex flex-col gap-2">
      {images.length > 0 ? (
        <div class="flex flex-wrap gap-2">
          {images.map((a, i) => (
            <button
              key={a.fileId}
              type="button"
              onClick={() => openLightbox(i)}
              aria-label={t('attachment.enlarge', { name: a.name })}
              title={a.name}
              class="border-border bg-surface-sunken hover:border-border-strong sytue-focus-ring block max-w-[16rem] overflow-hidden rounded-lg border transition-colors duration-150"
            >
              {/* Miniatura generada por Drive (con ETag/304), no el original: un canal
                  con 30 fotos de 8 MB ya no descarga ~240 MB al abrirse. El original
                  se baja solo al ampliar en el lightbox.

                  `max-h-64` = 256 px CSS contra una fuente de 512 px (thumbMaxDim en
                  Drive): esa proporción de 2× es lo que la hace nítida en pantallas
                  retina. Si alguna vez se agranda este hueco, hay que subir también
                  aquella constante, o la foto se verá a media resolución. */}
              <img
                src={attachmentUrl(a, { channelId, thumb: true })}
                alt={a.name}
                loading="lazy"
                class="max-h-64 w-auto max-w-full object-cover"
              />
            </button>
          ))}
        </div>
      ) : null}

      {/* Vídeo y audio, reproducibles en el sitio. El nombre va debajo y con enlace de
          descarga: quien quiera el fichero sigue teniéndolo a un clic. */}
      {media.map((a) => (
        <div key={a.fileId} class="max-w-sm">
          {/* Sin `<track>`: los subtítulos de un adjunto viajan DENTRO del propio
              fichero (un .mp4 con su pista), y un `<track>` vacío solo silenciaría
              la regla sin añadir un solo subtítulo. */}
          {isVideo(a) ? (
            // eslint-disable-next-line jsx-a11y/media-has-caption
            <video
              src={attachmentUrl(a, { channelId })}
              controls
              preload="metadata"
              class="border-border bg-surface-sunken max-h-64 w-full rounded-lg border"
            />
          ) : (
            // eslint-disable-next-line jsx-a11y/media-has-caption
            <audio
              src={attachmentUrl(a, { channelId })}
              controls
              preload="metadata"
              class="w-full"
            />
          )}
          <a
            href={attachmentUrl(a, { download: true, channelId })}
            download={a.name}
            class="sytue-focus-ring text-text-muted hover:text-text mt-0.5 flex items-center gap-1.5 rounded text-xs"
          >
            <DownloadGlyph size={12} />
            <span class="truncate" title={a.name}>
              {a.name}
            </span>
            <span class="sytue-meta shrink-0">{formatBytes(locale, a.sizeBytes)}</span>
          </a>
        </div>
      ))}

      {files.map((a) => (
        <div
          key={a.fileId}
          class={cx(
            'group border-border bg-surface flex max-w-sm items-center gap-1 rounded-lg border pr-1.5',
            'hover:border-border-strong hover:bg-bg-subtle transition-colors duration-150',
          )}
        >
          {/* El enlace ocupa la tarjeta menos las acciones. Va aparte del botón y no
              envolviéndolo: un <button> dentro de un <a> es HTML inválido y deja al
              lector de pantalla sin saber qué está anunciando. */}
          <a
            href={attachmentUrl(a, { download: true, channelId })}
            download={a.name}
            class="sytue-focus-ring flex min-w-0 flex-1 items-center gap-3 rounded-lg py-2 pl-3"
          >
            <span class="bg-bg-subtle text-text-muted grid h-9 w-9 shrink-0 place-items-center rounded-md">
              <FileGlyph size={18} />
            </span>
            <span class="min-w-0 flex-1">
              <span class="text-text block truncate text-sm font-medium" title={a.name}>
                {a.name}
              </span>
              <span class="sytue-meta-dense block">
                {a.sizeBytes > 0 ? formatBytes(locale, a.sizeBytes) : t('attachment.file')}
              </span>
            </span>
            <span
              class="text-text-subtle group-hover:text-text shrink-0 transition-colors duration-150"
              aria-hidden="true"
            >
              <DownloadGlyph size={16} />
            </span>
          </a>
          {/* «Guardar en mi Drive»: copia la referencia al espacio del usuario para que
              el fichero le sobreviva al mensaje. No duplica bytes (el servidor reusa el
              blob), así que no hay que avisar de consumo. */}
          {/* Abrir en pestaña: para un PDF o un texto, leerlo es lo que se quiere
              hacer nueve de cada diez veces, y bajarlo era el único camino. */}
          {isReadableInline(a) ? (
            <a
              href={attachmentUrl(a, { channelId })}
              target="_blank"
              rel="noopener noreferrer"
              aria-label={t('attachment.open', { name: a.name })}
              title={t('action.open')}
              class="text-text-muted hover:bg-bg-subtle hover:text-text sytue-focus-ring rounded-control inline-flex h-8 w-8 shrink-0 items-center justify-center transition-colors duration-150"
            >
              <Icon name="arrowUpRight" size={16} />
            </a>
          ) : null}
          {!a.matrixFile ? (
            <IconButton
              size="sm"
              label={t('attachment.saveToDrive', { name: a.name })}
              disabled={guardando === a.fileId}
              onClick={() => void guardarEnDrive(a)}
            >
              <Icon name="cloud" size={16} />
            </IconButton>
          ) : null}
        </div>
      ))}

      {lightbox !== null ? (
        <Lightbox
          images={images}
          index={lightbox}
          channelId={channelId}
          onIndex={setLightbox}
          onClose={closeLightbox}
          t={t}
        />
      ) : null}
    </div>
  );
}
