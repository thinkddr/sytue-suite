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
import { useEffect, useState } from 'preact/hooks';
import { Badge, EmptyState, Icon, Button, usePrompt } from '@sytue/ui';
import { translate, type Locale } from '@sytue/i18n';
import type { SytueUser } from '@sytue/auth';
import {
  albumCompartido,
  albumesCompartidos,
  ErrorDeFotos,
  quitarDelAlbum,
  renombrarAlbum,
  urlDeVarianteCompartida,
  type AlbumCompartido,
  type FotoCompartida,
} from '../client/photosApi.js';
import { useT } from '../i18n/index.js';
import { estiloDeTesela } from '../lib/filas.js';
import { posicionVisor } from '../lib/posicionVisor.js';
import { RUTA } from '../lib/ruta.js';
import { conAvisos, FalloDeCarga, useAvisoDeFallo } from './Fallo.js';
import { Marco } from './Marco.js';
import { Nav } from './Nav.js';
import { EsqueletoAlbumes, EsqueletoFilas } from './Esqueletos.js';
import { VisorCompartido } from './VisorCompartido.js';

// El visor vive en su fichero para que la página pública del álbum lo hidrate sin
// cargar con la vista de compartidos entera; aquí se reexporta como siempre.
export { VisorCompartido };

export const Compartidos = conAvisos(CompartidoConmigo);

function CompartidoConmigo({
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
  const [fallo, setFallo] = useState<unknown>(null);
  // Reintentar vuelve a lanzar la misma carga: el efecto depende de esta vuelta.
  const [vuelta, setVuelta] = useState(0);
  const avisarFallo = useAvisoDeFallo(locale);
  const { preguntar, dialogo: dialogoPreguntar } = usePrompt();
  const editable = abierto?.album.permission === 'write';

  useEffect(() => {
    setFallo(null);
    if (album) {
      albumCompartido(album).then(setAbierto).catch(setFallo);
    } else {
      albumesCompartidos().then(setLista).catch(setFallo);
    }
  }, [album, vuelta]);
  // Un 404 no es una avería: el álbum ya no está compartido con quien mira (o
  // nunca lo estuvo), y lo honesto es la lista vacía, no «Reintentar».
  const yaNoCompartido = fallo instanceof ErrorDeFotos && fallo.status === 404;

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
      avisarFallo(e);
    }
  };

  const quitar = async (f: FotoCompartida) => {
    if (!album) return;
    try {
      await quitarDelAlbum(album, f.id);
      setAbierto((a) => (a ? { ...a, items: a.items.filter((x) => x.id !== f.id) } : a));
      setFoto(null);
    } catch (e) {
      avisarFallo(e);
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
        {fallo !== null && !yaNoCompartido ? (
          <FalloDeCarga
            locale={locale}
            fallo={fallo}
            alReintentar={() => setVuelta((v) => v + 1)}
          />
        ) : yaNoCompartido ? (
          <EmptyState title={tc('sharedWithMe.empty')} spot="emptyPhotos" />
        ) : album ? (
          abierto === null ? (
            <EsqueletoFilas />
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
          <EsqueletoAlbumes cuadradas />
        ) : lista.length === 0 ? (
          <EmptyState class="flex-1" title={tc('sharedWithMe.empty')} spot="emptyPhotos" />
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
            src={urlDeVarianteCompartida(album, abierto.items[foto].id, 'grande')}
            foto={abierto.items[foto]}
            t={t}
            onClose={() => setFoto(null)}
            // El álbum llega entero, sin páginas por pedir: nunca lleva el «+».
            posicion={posicionVisor(foto, abierto.items.length)}
            onIr={setFoto}
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
