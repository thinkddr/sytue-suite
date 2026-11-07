/**
 * Un álbum: su cuadrícula (las mismas teselas que la galería), el visor con los
 * mandos de «usar como portada» y «quitar del álbum», renombrarlo, compartirlo con
 * personas de la organización o por enlace, y borrarlo con el diálogo de la casa:
 * las fotos se quedan, así que no es destructivo de verdad.
 */
import type { JSX } from 'preact';
import { useEffect, useState } from 'preact/hooks';
import { Button, Icon, IconButton, Spinner, useConfirm, usePrompt, withBase } from '@sytue/ui';
import { translate, type Locale } from '@sytue/i18n';
import { CompartirConPersonas } from '@sytue/shell';
import {
  album as pedirAlbum,
  borrarAlbum,
  compartirAlbum,
  dejarDeCompartirAlbum,
  enlaceDelAlbum,
  fotosDelAlbum,
  ponerPortada,
  quitarDelAlbum,
  renombrarAlbum,
  type Album,
  type Foto,
} from '../client/photosApi.js';
import { useT } from '../i18n/index.js';
import { posicionVisor } from '../lib/posicionVisor.js';
import { esPortada, mandoPortada, tituloRenombrado } from '../lib/edicionAlbum.js';
import { RUTA } from '../lib/ruta.js';
import { Marco } from './Marco.js';
import type { SytueUser } from '@sytue/auth';
import { Filete, Miniatura, Visor } from './PhotosApp.js';
import { Nav } from './Nav.js';

export function AlbumView({
  locale,
  id,
  user,
  hubBaseUrl,
}: {
  locale: Locale;
  id: string;
  user: SytueUser;
  hubBaseUrl: string;
}): JSX.Element {
  const { confirmar, dialogo: dialogoConfirmar } = useConfirm();
  const { preguntar, dialogo: dialogoPreguntar } = usePrompt();
  const t = useT(locale);
  const [al, setAl] = useState<Album | null>(null);
  const [fotos, setFotos] = useState<Foto[] | null>(null);
  const [abierta, setAbierta] = useState<number | null>(null);
  const [error, setError] = useState('');
  const [aviso, setAviso] = useState('');
  // «Compartir con personas»: para ver o para editar (lo añadido se copia a su Drive).
  const [conPersonas, setConPersonas] = useState(false);

  useEffect(() => {
    Promise.all([pedirAlbum(id), fotosDelAlbum(id)])
      .then(([a, f]) => {
        setAl(a);
        setFotos(f);
      })
      .catch((e) => setError(String(e)));
  }, [id]);

  /** Renombrar: el diálogo de la casa con el título de ahora escrito. */
  const renombrar = async () => {
    if (!al) return;
    const nuevo = tituloRenombrado(
      al.title,
      await preguntar({
        titulo: t('album.rename'),
        mensaje: t('album.renamePrompt'),
        valor: al.title,
      }),
    );
    if (nuevo === null) return;
    try {
      setAl(await renombrarAlbum(id, nuevo));
    } catch (e) {
      setError(String(e));
    }
  };

  /**
   * Elegir portada desde el visor: es la foto que se está mirando, que es como se
   * decide cuál representa al álbum. Antes la portada era siempre la primera que
   * entró, sin forma de cambiarla. El resultado se dice también DENTRO del visor
   * (`avisar`), como en Fyne: la cabecera del álbum queda detrás del modal y mientras
   * está abierto ni se ve ni se anuncia.
   */
  const portada = async (foto: Foto, avisar: (texto: string) => void) => {
    if (!al || esPortada(al, foto.id)) return;
    try {
      setAl(await ponerPortada(id, al.title, foto.id));
      setAviso(t('album.coverSet'));
      avisar(t('album.coverSet'));
    } catch (e) {
      setError(String(e));
      avisar(String(e));
    }
  };

  const quitar = async (foto: Foto) => {
    try {
      await quitarDelAlbum(id, foto.id);
      setFotos((f) => (f ?? []).filter((x) => x.id !== foto.id));
      setAbierta(null);
    } catch (e) {
      setError(String(e));
    }
  };

  /**
   * El servicio solo guarda la huella del enlace, así que no puede volver a
   * darlo: compartir un álbum ya compartido crea uno NUEVO y el anterior deja de
   * valer (FILE-10, decisión del dueño). Antes de eso se pregunta.
   */
  const compartir = async () => {
    if (al?.shared) {
      const ok = await confirmar({
        mensaje: t('album.newLinkConfirm'),
        confirmLabel: t('album.newLink'),
        tone: 'danger',
      });
      if (!ok) return;
    }
    try {
      const { token } = await compartirAlbum(id);
      setAl((a) => (a ? { ...a, shared: true } : a));
      await copiar(token);
    } catch (e) {
      setError(String(e));
    }
  };

  const copiar = async (token: string) => {
    const enlace = enlaceDelAlbum(token);
    try {
      await navigator.clipboard.writeText(enlace);
      setAviso(t('album.linkCopied'));
    } catch {
      // Sin permiso de portapapeles —o sin HTTPS— se ENSEÑA el enlace para
      // copiarlo a mano: quedarse callado dejaría a la persona sin nada.
      setAviso(enlace);
    }
  };

  const dejarDeCompartir = async () => {
    const ok = await confirmar({
      mensaje: t('album.unshareConfirm'),
      confirmLabel: t('album.unshare'),
      tone: 'danger',
    });
    if (!ok) return;
    try {
      await dejarDeCompartirAlbum(id);
      // La clave se QUITA en vez de ponerse a `undefined`: con
      // `exactOptionalPropertyTypes`, «ausente» y «undefined» no son lo mismo.
      setAl((a) => {
        if (!a) return a;
        const { shared: _apagado, ...resto } = a;
        return resto;
      });
      setAviso('');
    } catch (e) {
      setError(String(e));
    }
  };

  const borrar = async () => {
    // El diálogo del sistema de diseño y no `confirm()`: aquel pinta sus botones
    // en el idioma del sistema operativo y sin la estética de la casa.
    const ok = await confirmar({
      mensaje: t('albums.deleteConfirm'),
      confirmLabel: t('albums.delete'),
      tone: 'danger',
    });
    if (!ok) return;
    try {
      await borrarAlbum(id);
      // Con `RUTA` y no relativo («albums»): resolvía igual contra el directorio
      // de la página, pero era el único enlace interno que se le escapaba a la
      // verja de ruta.test.ts —y con la org en el path, `${RUTA}…` es quien
      // conserva el segmento sin importar desde qué URL se pulse.
      window.location.href = `${RUTA}albums`;
    } catch (e) {
      setError(String(e));
    }
  };

  /**
   * Refrescar la lista tras describir una foto con Sytue AI.
   *
   * EL VISOR SOLO PINTA EL BOTÓN SI RECIBE ESTA PROP, así que sin ella la misma
   * foto se podía describir desde la línea de tiempo pero no desde un álbum:
   * dos comportamientos según por dónde llegues, y el que faltaba era el del
   * texto alternativo. Guardar lo hace el visor; esto solo actualiza la lista de
   * aquí para que el `alt` de las miniaturas cambie sin recargar.
   */
  const describir = (f: Foto, description: string, tags: string[]): void => {
    setFotos((l) => (l ?? []).map((x) => (x.id === f.id ? { ...x, description, tags } : x)));
  };

  return (
    <Marco user={user} hubBaseUrl={hubBaseUrl} locale={locale} title={al?.title ?? t('nav.albums')}>
      <div class="mx-auto w-full max-w-7xl px-4 py-6">
        <Nav t={t} locale={locale} activa="albums" />
        <header class="mb-4 flex flex-wrap items-center gap-2">
          <Button
            href={`${RUTA}albums`}
            variant="ghost"
            size="sm"
            leadingIcon={<Icon name="chevronLeft" />}
          >
            {t('albums.back')}
          </Button>
          <h1 class="truncate text-lg font-semibold">{al?.title ?? ''}</h1>
          {fotos ? (
            <span class="text-text-muted text-sm">
              {t('albums.count', { count: fotos.length })}
            </span>
          ) : null}
          {/* UNA acción principal —compartir con personas, que es para lo que se
              abre un álbum además de mirarlo—, las demás de fantasma y borrar
              aparte, tras un filete y sin gritar: eran cuatro botones seguidos, dos
              de ellos rellenos y del mismo peso. */}
          <span class="ml-auto flex flex-wrap items-center gap-1">
            {aviso ? (
              <span
                role="status"
                aria-live="polite"
                class="text-text-muted max-w-xs truncate text-xs"
              >
                {aviso}
              </span>
            ) : null}
            <Button
              variant="primary"
              size="sm"
              leadingIcon={<Icon name="users" />}
              disabled={!al}
              onClick={() => setConPersonas(true)}
            >
              {translate(locale, 'sharePeople.open')}
            </Button>
            <Button
              variant="ghost"
              size="sm"
              leadingIcon={<Icon name="link" />}
              disabled={!al}
              onClick={() => void compartir()}
            >
              {al?.shared ? t('album.newLink') : t('album.share')}
            </Button>
            {al?.shared ? (
              <Button variant="ghost" size="sm" onClick={() => void dejarDeCompartir()}>
                {t('album.unshare')}
              </Button>
            ) : null}
            <Button
              variant="ghost"
              size="sm"
              leadingIcon={<Icon name="pen" />}
              disabled={!al}
              onClick={() => void renombrar()}
            >
              {t('album.rename')}
            </Button>
            <Filete />
            <IconButton
              label={t('albums.delete')}
              tone="danger"
              size="lg"
              disabled={!al}
              onClick={() => void borrar()}
            >
              <Icon name="trash" />
            </IconButton>
          </span>
        </header>
        {error ? (
          <p class="text-danger-text mb-4 text-sm" role="alert">
            {error}
          </p>
        ) : null}
        {fotos === null ? (
          <Spinner />
        ) : (
          <ul class="filas">
            {fotos.map((f, i) => (
              <Miniatura key={f.id} foto={f} t={t} onOpen={() => setAbierta(i)} />
            ))}
          </ul>
        )}
        {abierta !== null && fotos?.[abierta] ? (
          <Visor
            foto={fotos[abierta]}
            t={t}
            locale={locale}
            onClose={() => setAbierta(null)}
            vecinas={[fotos[abierta - 1], fotos[abierta + 1]]}
            posicion={posicionVisor(abierta, fotos.length)}
            onIr={setAbierta}
            onPrev={abierta > 0 ? () => setAbierta(abierta - 1) : undefined}
            onNext={abierta < fotos.length - 1 ? () => setAbierta(abierta + 1) : undefined}
            onDescribe={describir}
            accion={(avisar) => (
              <>
                {/* En la que ya es la portada dice que lo es y va `aria-disabled`: no es
                    un conmutador (pulsarlo otra vez no quita nada) y no se apaga del
                    todo, porque un botón `disabled` se salta con el tabulador y el
                    lector de pantalla no lo anuncia (`mandoPortada`). */}
                {(() => {
                  const mando = mandoPortada(al, fotos[abierta]!.id);
                  return (
                    <IconButton
                      label={t(mando.clave)}
                      size="lg"
                      aria-disabled={mando.apagado ? 'true' : undefined}
                      class={mando.apagado ? 'opacity-40' : undefined}
                      disabled={!al}
                      onClick={() => void portada(fotos[abierta]!, avisar)}
                    >
                      <Icon name="image" />
                    </IconButton>
                  );
                })()}
                <Button
                  variant="ghost"
                  size="sm"
                  leadingIcon={<Icon name="minus" />}
                  title={t('albums.remove')}
                  onClick={() => void quitar(fotos[abierta]!)}
                >
                  <span class="max-md:sr-only">{t('albums.remove')}</span>
                </Button>
              </>
            )}
          />
        ) : null}
        {conPersonas && al ? (
          <CompartirConPersonas
            endpoint={withBase(
              import.meta.env.BASE_URL,
              `/api/people?album=${encodeURIComponent(id)}`,
            )}
            nombre={al.title}
            niveles={['read', 'write']}
            locale={locale}
            onClose={() => setConPersonas(false)}
          />
        ) : null}
        {dialogoConfirmar}
        {dialogoPreguntar}
      </div>
    </Marco>
  );
}
