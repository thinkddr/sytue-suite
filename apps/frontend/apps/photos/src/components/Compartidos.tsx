/**
 * «Compartido conmigo»: los álbumes que otras personas han compartido conmigo y,
 * con `?album=`, uno de ellos. Con «puede ver», solo mirar. Con «puede editar»,
 * renombrarlo y quitar cualquier foto aquí, y añadir fotos propias desde la galería
 * («Añadir a un álbum…»), que se copian al Drive de su dueña.
 *
 * Las imágenes van por `/api/v1/shared/albums/<álbum>/assets/…`: la verja del servicio
 * es que el álbum esté compartido con quien mira y la foto esté en él.
 */
import type { JSX } from 'preact';
import { useEffect, useRef, useState } from 'preact/hooks';
import {
  Badge,
  EmptyState,
  Icon,
  IconButton,
  Illustration,
  Spinner,
  Button,
  usePrompt,
  useMobileBack,
} from '@sytue/ui';
import { translate, type Locale } from '@sytue/i18n';
import type { SytueUser } from '@sytue/auth';
import {
  albumCompartido,
  albumesCompartidos,
  quitarDelAlbum,
  renombrarAlbum,
  urlDeVarianteCompartida,
  type AlbumCompartido,
  type FotoCompartida,
} from '../client/photosApi.js';
import { useT } from '../i18n/index.js';
import { estiloDeTesela } from '../lib/filas.js';
import { RUTA } from '../lib/ruta.js';
import { Marco } from './Marco.js';
import { Nav } from './Nav.js';
import { EscenarioVisor } from './EscenarioVisor.js';

export function Compartidos({
  locale,
  album,
  user,
  hubBaseUrl,
}: {
  locale: Locale;
  /** El álbum abierto (`?album=`), o null para la lista. */
  album: string | null;
  user: SytueUser;
  hubBaseUrl: string;
}): JSX.Element {
  const t = useT(locale);
  const tc = (k: Parameters<typeof translate>[1], v?: Record<string, string>) =>
    translate(locale, k, v);
  const [lista, setLista] = useState<AlbumCompartido[] | null>(null);
  const [abierto, setAbierto] = useState<{
    album: AlbumCompartido;
    items: FotoCompartida[];
  } | null>(null);
  const [foto, setFoto] = useState<number | null>(null);
  const [error, setError] = useState(false);
  const [fallo, setFallo] = useState('');
  const { preguntar, dialogo: dialogoPreguntar } = usePrompt();
  const editable = abierto?.album.permission === 'write';

  useEffect(() => {
    setError(false);
    if (album) {
      albumCompartido(album)
        .then(setAbierto)
        .catch(() => setError(true));
    } else {
      albumesCompartidos()
        .then(setLista)
        .catch(() => setError(true));
    }
  }, [album]);

  const titulo = abierto?.album.title ?? tc('sharedWithMe.title');
  const quien = (a: AlbumCompartido) =>
    tc('sharedWithMe.by', { name: a.sharedByName ?? tc('sharePeople.someone') });

  const renombrar = async () => {
    if (!abierto || !album) return;
    const nuevo = (
      await preguntar({ mensaje: tc('sharedAlbum.renamePrompt'), valor: abierto.album.title })
    )?.trim();
    if (!nuevo || nuevo === abierto.album.title) return;
    try {
      await renombrarAlbum(album, nuevo);
      setAbierto((a) => (a ? { ...a, album: { ...a.album, title: nuevo } } : a));
    } catch (e) {
      setFallo(String(e));
    }
  };

  const quitar = async (f: FotoCompartida) => {
    if (!album) return;
    try {
      await quitarDelAlbum(album, f.id);
      setAbierto((a) => (a ? { ...a, items: a.items.filter((x) => x.id !== f.id) } : a));
      setFoto(null);
    } catch (e) {
      setFallo(String(e));
    }
  };

  return (
    <Marco user={user} hubBaseUrl={hubBaseUrl} locale={locale} title={titulo}>
      <div class="mx-auto w-full max-w-7xl px-4 py-6">
        <Nav t={t} locale={locale} activa="shared" />
        {album ? (
          <header class="mb-4 flex flex-wrap items-center gap-2">
            <Button
              href={`${RUTA}shared`}
              variant="ghost"
              size="sm"
              leadingIcon={<Icon name="chevronLeft" />}
            >
              {tc('sharedWithMe.title')}
            </Button>
            <h1 class="truncate text-lg font-semibold">{abierto?.album.title ?? ''}</h1>
            {abierto ? (
              <>
                <span class="text-text-muted text-sm">
                  {t('albums.count', { count: abierto.items.length })}
                </span>
                <span class="text-text-muted text-sm">{quien(abierto.album)}</span>
                <Badge>{tc(editable ? 'sharePeople.write' : 'sharedWithMe.readOnly')}</Badge>
                {editable ? (
                  <span class="ml-auto">
                    <Button
                      variant="ghost"
                      size="sm"
                      leadingIcon={<Icon name="pen" />}
                      onClick={() => void renombrar()}
                    >
                      {tc('sharedAlbum.rename')}
                    </Button>
                  </span>
                ) : null}
              </>
            ) : null}
          </header>
        ) : (
          <h1 class="mb-4 text-lg font-semibold">{tc('sharedWithMe.title')}</h1>
        )}
        {editable && abierto ? (
          <p class="text-text-muted mb-4 text-sm">
            {tc('sharedAlbum.addHint', {
              name: abierto.album.sharedByName ?? tc('sharePeople.someone'),
              action: t('albums.addTo'),
            })}
          </p>
        ) : null}
        {fallo ? (
          <p class="text-danger-text mb-4 text-sm" role="alert">
            {fallo}
          </p>
        ) : null}
        {error ? (
          <EmptyState
            title={tc('sharedWithMe.empty')}
            illustration={<Illustration name="emptyPhotos" />}
          />
        ) : album ? (
          abierto === null ? (
            <Spinner />
          ) : (
            // Las mismas filas justificadas que la galería, y la misma tesela por
            // dentro: el recorte va en el botón para que su aro de foco se vea.
            <ul class="filas">
              {abierto.items.map((f, i) => (
                <li key={f.id} style={estiloDeTesela(f)} class="bg-bg-subtle relative rounded-sm">
                  <button
                    type="button"
                    aria-haspopup="dialog"
                    onClick={() => setFoto(i)}
                    class="sytue-focus-ring-inset block h-full w-full overflow-hidden rounded-sm"
                    title={f.name}
                    aria-label={t('gallery.open', { name: f.name })}
                  >
                    <img
                      src={urlDeVarianteCompartida(album, f.id, 'miniatura')}
                      alt={f.name}
                      width={f.width}
                      height={f.height}
                      loading="lazy"
                      decoding="async"
                      class="h-full w-full object-cover"
                    />
                  </button>
                </li>
              ))}
            </ul>
          )
        ) : lista === null ? (
          <Spinner />
        ) : lista.length === 0 ? (
          <EmptyState
            class="flex-1"
            title={tc('sharedWithMe.empty')}
            illustration={<Illustration name="emptyPhotos" />}
          />
        ) : (
          <ul class="grid grid-cols-[repeat(auto-fill,minmax(11rem,1fr))] gap-3">
            {lista.map((al) => (
              <li key={al.id}>
                <a
                  href={`${RUTA}shared?album=${encodeURIComponent(al.id)}`}
                  class="sytue-focus-ring block rounded-sm"
                >
                  <div class="bg-bg-subtle aspect-square overflow-hidden rounded-sm">
                    {al.coverAssetId ? (
                      <img
                        src={urlDeVarianteCompartida(al.id, al.coverAssetId, 'miniatura')}
                        alt=""
                        loading="lazy"
                        decoding="async"
                        class="h-full w-full object-cover"
                      />
                    ) : null}
                  </div>
                  <div class="mt-1 truncate text-sm font-medium">{al.title}</div>
                  <div class="text-text-muted truncate text-xs">
                    {t('albums.count', { count: al.count })} · {quien(al)}
                  </div>
                </a>
              </li>
            ))}
          </ul>
        )}
        {album && abierto && foto !== null && abierto.items[foto] ? (
          <VisorCompartido
            album={album}
            foto={abierto.items[foto]}
            t={t}
            onClose={() => setFoto(null)}
            onPrev={foto > 0 ? () => setFoto(foto - 1) : undefined}
            onNext={foto < abierto.items.length - 1 ? () => setFoto(foto + 1) : undefined}
            {...(editable ? { onQuitar: (f: FotoCompartida) => void quitar(f) } : {})}
          />
        ) : null}
        {dialogoPreguntar}
      </div>
    </Marco>
  );
}

/**
 * La foto sola, sobre negro, como la del enlace en la app nativa: anterior, siguiente
 * y cerrar, con las flechas y Escape. Sin ficha: no es de quien la mira. Con edición,
 * «Quitar del álbum».
 */
function VisorCompartido({
  album,
  foto,
  t,
  onClose,
  onPrev,
  onNext,
  onQuitar,
}: {
  album: string;
  foto: FotoCompartida;
  t: ReturnType<typeof useT>;
  onClose: () => void;
  onPrev?: (() => void) | undefined;
  onNext?: (() => void) | undefined;
  /** «Quitar del álbum»: solo con edición. */
  onQuitar?: ((f: FotoCompartida) => void) | undefined;
}): JSX.Element {
  const ref = useRef<HTMLDialogElement>(null);
  useMobileBack(true, onClose);
  const imagen = useRef<HTMLImageElement>(null);
  useEffect(() => {
    const d = ref.current;
    if (d && !d.open) d.showModal();
  }, []);
  return (
    <dialog
      ref={ref}
      onClose={onClose}
      onKeyDown={(e) => {
        if (e.key === 'ArrowLeft') onPrev?.();
        else if (e.key === 'ArrowRight') onNext?.();
      }}
      class="sytue-safe-fullscreen bg-bg text-text h-dvh max-h-none w-screen max-w-none p-0 backdrop:bg-black/80"
    >
      <div class="flex h-full flex-col">
        <header class="border-border flex items-center gap-2 border-b px-3 py-2">
          <span class="min-w-0 flex-1 truncate text-sm font-medium">{foto.name}</span>
          {onQuitar ? (
            <Button
              variant="ghost"
              size="sm"
              leadingIcon={<Icon name="minus" />}
              title={t('albums.remove')}
              onClick={() => onQuitar(foto)}
            >
              <span class="max-md:sr-only">{t('albums.remove')}</span>
            </Button>
          ) : null}
          <IconButton label={t('viewer.close')} size="lg" onClick={onClose}>
            <Icon name="close" />
          </IconButton>
        </header>
        {/* Las flechas, sobre la foto y no en la cabecera: son las del visor de la galería
            (`EscenarioVisor`), con su talla, su filete y el deslizamiento con el dedo. */}
        <EscenarioVisor
          prev={t('viewer.prev')}
          next={t('viewer.next')}
          onPrev={onPrev}
          onNext={onNext}
          foto={imagen}
          clave={foto.id}
          class="items-center justify-center overflow-hidden bg-[#0e0e0e] p-2"
        >
          <img
            ref={imagen}
            src={urlDeVarianteCompartida(album, foto.id, 'grande')}
            alt={foto.name}
            width={foto.width}
            height={foto.height}
            class="h-auto max-h-full w-auto max-w-full object-contain"
          />
        </EscenarioVisor>
      </div>
    </dialog>
  );
}
