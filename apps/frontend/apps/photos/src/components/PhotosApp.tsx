/**
 * PhotosApp — la línea de tiempo de Sytue Photos.
 *
 * Monta la Sytue Shell (barra superior, conmutador de aplicaciones) y debajo la
 * cuadrícula de fotos agrupada por mes, de la más reciente a la más antigua.
 */
import { Fragment, type ComponentChildren, type JSX } from 'preact';
import {
  useCallback,
  useEffect,
  useId,
  useLayoutEffect,
  useMemo,
  useRef,
  useState,
} from 'preact/hooks';
import {
  Button,
  ConfirmDialog,
  Dropdown,
  EmptyState,
  FILA_UNICA_EN_MOVIL,
  Icon,
  IconButton,
  Illustration,
  Input,
  MASCARA_SCROLL_X,
  MenuItem,
  MenuSeparator,
  Spinner,
  Toolbar,
  cx,
  formatBytes,
  useScrollRestante,
  usePrompt,
  useMobileBack,
  esAtajoDeApp,
  Glifo,
} from '@sytue/ui';
import { formatDateTime, translate, type Locale } from '@sytue/i18n';
import {
  alaPapelera,
  albumesParaAnyadir,
  anyadirAlAlbum,
  crearAlbum,
  describirFoto,
  preguntarFoto,
  guardarDescripcion,
  guardarReceta,
  lineaDeTiempo,
  marcarFavorita,
  obtenerReceta,
  sinEspacio,
  terminosRelacionados,
  textoAlternativo,
  urlDeExportacion,
  urlDeVariante,
  urlDeVarianteCon,
  type Album,
  type FormatoExportacion,
  type Foto,
} from '../client/photosApi.js';
import { esImagen, subirFotos } from '../client/subida.js';
import type { Receta } from '../editor/receta.js';
import { Nav } from './Nav.js';
import { Recuerdos } from './Recuerdos.js';
import { porMes } from '../lib/agrupar.js';
import { estiloDeTesela } from '../lib/filas.js';
import { CAMPO_TACTIL } from '../lib/tactil.js';
import { RUTA } from '../lib/ruta.js';
import { tituloSugerido } from '../lib/tituloAlbum.js';
import { useT } from '../i18n/index.js';
import {
  obtenerFondoLienzo,
  guardarFondoLienzo,
  estilosFondoLienzo,
  type FondoLienzo,
} from '../lib/fondoLienzo.js';
import { Marco } from './Marco.js';
import { EscenarioVisor } from './EscenarioVisor.js';
import type { SytueUser } from '@sytue/auth';

interface Props {
  locale: Locale;
  user: SytueUser;
  hubBaseUrl: string;
  /**
   * La MISMA vista con el filtro de favoritas puesto. No es una pantalla
   * aparte: la búsqueda, la selección múltiple, el visor y la subida son los de
   * siempre, y duplicarlos para enseñar menos fotos sería mantener dos.
   */
  soloFavoritas?: boolean;
}

export function PhotosApp({ locale, user, hubBaseUrl, soloFavoritas = false }: Props): JSX.Element {
  const t = useT(locale);
  const { preguntar, dialogo: dialogoPreguntar } = usePrompt();
  const [fotos, setFotos] = useState<Foto[]>([]);
  const [cursor, setCursor] = useState<string | undefined>(undefined);
  const [cargando, setCargando] = useState(true);
  const [fin, setFin] = useState(false);
  const [error, setError] = useState<string | null>(null);
  const centinela = useRef<HTMLDivElement>(null);
  const [abierta, setAbierta] = useState<number | null>(null);
  const [texto, setTexto] = useState('');
  // Términos alternativos con los que Sytue AI amplía una búsqueda que no
  // encuentra nada. Se declaran AQUÍ y no junto a la función que los pide:
  // `cargarMas` los usa, y en JavaScript una constante de bloque no existe
  // antes de su línea.
  const [tambien, setTambien] = useState<string[]>([]);
  const [ampliando, setAmpliando] = useState(false);
  const generacion = useRef(0);
  // Subida: progreso, aviso final y la zona de soltar.
  const [subida, setSubida] = useState<{ hechas: number; total: number } | null>(null);
  const [aviso, setAviso] = useState<string | null>(null);
  const [arrastrando, setArrastrando] = useState(false);
  const selector = useRef<HTMLInputElement>(null);
  // Selección múltiple: null = fuera del modo; dentro, los ids marcados. Las
  // acciones en lote (álbum, papelera) reutilizan las mismas llamadas que el
  // visor hace de una en una: el servicio no tiene un «borrar varios» y no
  // hace falta para unas decenas de fotos.
  const [seleccion, setSeleccion] = useState<Set<string> | null>(null);
  // La barra de selección, para el aviso de que queda contenido a la derecha.
  // Depende de `seleccion` porque la barra SOLO existe con selección marcada.
  const barraSeleccion = useScrollRestante<HTMLDivElement>(seleccion !== null);
  const [confirmarPapelera, setConfirmarPapelera] = useState(false);
  const [albumesLista, setAlbumesLista] = useState<Album[] | null>(null);
  useEffect(() => {
    if (seleccion === null || confirmarPapelera) return;
    // `esAtajoDeApp`: con un menú o un diálogo abiertos, Escape es SUYO. Sin esto,
    // cerrar «Álbum nuevo» con Escape se llevaba también la selección.
    const alPulsar = (e: KeyboardEvent) => {
      if (e.key === 'Escape' && esAtajoDeApp(e)) setSeleccion(null);
    };
    window.addEventListener('keydown', alPulsar);
    return () => window.removeEventListener('keydown', alPulsar);
  }, [seleccion, confirmarPapelera]);
  // «u» añade fotos: el atajo que pulsa el menú nativo de la app de escritorio.
  useEffect(() => {
    const alPulsar = (e: KeyboardEvent) => {
      if (e.key !== 'u' || !esAtajoDeApp(e) || subida !== null) return;
      e.preventDefault();
      selector.current?.click();
    };
    window.addEventListener('keydown', alPulsar);
    return () => window.removeEventListener('keydown', alPulsar);
  }, [subida]);
  const alternar = (id: string) =>
    setSeleccion((prev) => {
      const s = new Set(prev);
      if (s.has(id)) s.delete(id);
      else s.add(id);
      return s;
    });
  /** Marca (o desmarca, si ya estaban todas) las fotos de un mes. */
  const alternarMes = (ids: string[]) =>
    setSeleccion((prev) => {
      const s = new Set(prev);
      const todas = ids.every((id) => s.has(id));
      for (const id of ids) {
        if (todas) s.delete(id);
        else s.add(id);
      }
      return s;
    });
  // Descargar lo marcado: una exportación JPEG por foto, con un respiro entre
  // ellas para que el navegador no las tome por una ráfaga y las pare.
  const descargarSeleccion = () => {
    if (!seleccion) return;
    [...seleccion].forEach((id, i) => {
      setTimeout(() => descargar(urlDeExportacion(id, 'jpeg')), i * 400);
    });
    setAviso(t('gallery.downloading', { count: seleccion.size }));
  };
  /**
   * Marcar favorita cambia la lista AL VOLVER la llamada, no antes: en la vista
   * de favoritas, desmarcar saca la foto de la lista, y hacerlo optimista la
   * haría desaparecer para reaparecer si la llamada falla.
   */
  const cambiarFavorita = async (f: Foto, valor: boolean) => {
    try {
      await marcarFavorita(f.id, valor);
      setFotos((l) =>
        soloFavoritas && !valor
          ? l.filter((x) => x.id !== f.id)
          : l.map((x) => (x.id === f.id ? { ...x, favorite: valor } : x)),
      );
      if (soloFavoritas && !valor) setAbierta(null);
    } catch (e) {
      setError(String(e));
    }
  };

  const cargarAlbumes = () => {
    if (albumesLista === null)
      void albumesParaAnyadir()
        .then(setAlbumesLista)
        .catch(() => setAlbumesLista([]));
  };
  /** Álbum nuevo con lo marcado: se crea y se le añaden las fotos de una vez. */
  const albumDeLaSeleccion = async () => {
    if (!seleccion || seleccion.size === 0) return;
    const titulo = await preguntar({
      titulo: t('gallery.newAlbumTitle'),
      mensaje: t('gallery.newAlbumPrompt', { count: seleccion.size }),
      confirmLabel: t('gallery.newAlbumCreate'),
      // Propuesto con lo que Sytue AI vio en esas fotos, y editable: llega
      // escrito en el campo, no puesto por detrás. Si no hay nada claro viene
      // vacío, que es como estaba antes.
      valor: tituloSugerido(fotos.filter((f) => seleccion.has(f.id))),
    });
    if (titulo === null || titulo.trim() === '') return;
    try {
      const album = await crearAlbum(titulo.trim());
      await anyadirAlAlbum(album.id, [...seleccion]);
      // La lista de álbumes cacheada se queda vieja: se tira para que la próxima
      // vez que se abra el selector traiga el nuevo.
      setAlbumesLista(null);
      setAviso(t('gallery.addedToAlbum', { count: seleccion.size, title: album.title }));
      setSeleccion(null);
    } catch (e) {
      setError(String(e));
    }
  };
  const anyadirSeleccionA = async (albumId: string) => {
    const al = albumesLista?.find((a) => a.id === albumId);
    if (!al || !seleccion || seleccion.size === 0) return;
    try {
      await anyadirAlAlbum(al.id, [...seleccion]);
      setAviso(t('gallery.addedToAlbum', { count: seleccion.size, title: al.title }));
      setSeleccion(null);
    } catch (e) {
      // En un álbum de otra persona lo añadido se copia a su Drive: si no cabe, se dice.
      setError(sinEspacio(e) ? translate(locale, 'sharedAlbum.quotaFull') : String(e));
    }
  };
  /** Tope por pasada. Cada foto es una llamada de visión, y se cobra. */
  const TOPE_DESCRIBIR = 50;
  const [describiendoLote, setDescribiendoLote] = useState<{
    hechas: number;
    total: number;
  } | null>(null);
  const pararLote = useRef(false);

  /**
   * Describir lo marcado con Sytue AI.
   *
   * DE UNA EN UNA Y EN SERIE, no en paralelo. Cada foto es una llamada de
   * visión: cincuenta a la vez se llevan por delante el límite del servicio de
   * IA y gastan el presupuesto de golpe, además de dejar sin asistente al resto
   * de la suite mientras dura.
   *
   * LAS QUE YA TIENEN DESCRIPCIÓN SE SALTAN. Volver a describirlas costaría lo
   * mismo que la primera vez para acabar con un texto parecido, y quien pulsa
   * esto sobre un álbum entero no está pidiendo pagar dos veces.
   *
   * Se puede parar a mitad: lo descrito hasta ahí ya está guardado, porque cada
   * foto se guarda en cuanto vuelve y no al final.
   */
  const describirSeleccion = async () => {
    if (!seleccion || describiendoLote) return;
    const pendientes = fotos.filter((f) => seleccion.has(f.id) && !f.description?.trim());
    if (pendientes.length === 0) {
      setAviso(t('gallery.describeNothing'));
      return;
    }
    const lote = pendientes.slice(0, TOPE_DESCRIBIR);
    pararLote.current = false;
    setDescribiendoLote({ hechas: 0, total: lote.length });
    let hechas = 0;
    for (const f of lote) {
      if (pararLote.current) break;
      try {
        const { description, tags } = await describirFoto(f.id);
        await guardarDescripcion(f.id, description, tags);
        setFotos((l) => l.map((x) => (x.id === f.id ? { ...x, description, tags } : x)));
        hechas++;
      } catch (e) {
        // Una foto que falla no para el lote: lo que se pidió fue describir el
        // álbum, no describirlo entero o nada.
        setError(
          String(e).includes('ai_not_configured')
            ? t('viewer.describeUnavailable')
            : t('viewer.describeFailed'),
        );
      }
      setDescribiendoLote({ hechas, total: lote.length });
    }
    setDescribiendoLote(null);
    setAviso(
      pendientes.length > TOPE_DESCRIBIR
        ? `${t('gallery.described', { count: hechas })} · ${t('gallery.describeCapped', { count: TOPE_DESCRIBIR })}`
        : t('gallery.described', { count: hechas }),
    );
  };

  const tirarSeleccion = async () => {
    if (!seleccion || seleccion.size === 0) return;
    const ids = [...seleccion];
    setConfirmarPapelera(false);
    const resultados = await Promise.allSettled(ids.map((id) => alaPapelera(id)));
    const hechas = new Set(ids.filter((_, i) => resultados[i]?.status === 'fulfilled'));
    setFotos((l) => l.filter((f) => !hechas.has(f.id)));
    setSeleccion(null);
    setAviso(t('gallery.trashed', { count: hechas.size }));
    if (hechas.size < ids.length)
      setError(t('gallery.trashFailed', { count: ids.length - hechas.size }));
  };
  const anyadir = async (lista: FileList | readonly File[]): Promise<void> => {
    const ficheros = Array.from(lista).filter(esImagen);
    if (ficheros.length === 0) {
      setAviso(t('gallery.onlyImages'));
      return;
    }
    setAviso(null);
    setSubida({ hechas: 0, total: ficheros.length });
    const resultado = await subirFotos(ficheros, (hechas, total) => setSubida({ hechas, total }));
    setSubida(null);
    if (resultado.fotos.length > 0) {
      // La línea de tiempo se vuelve a pedir desde el principio: las fotos nuevas
      // caen donde les toque por fecha, no arriba por haber llegado las últimas.
      generacion.current++;
      const pagina = await lineaDeTiempo(undefined, 100, texto, soloFavoritas);
      setFotos(pagina.items);
      setCursor(pagina.nextCursor);
      setFin(!pagina.nextCursor);
      setAbierta(null);
    }
    setAviso(
      resultado.fallos.length > 0
        ? t('gallery.uploadFailed', { name: resultado.fallos.join(', ') })
        : t('gallery.uploaded', { count: resultado.fotos.length }),
    );
  };
  // La misma zona de soltar para la galería llena y para la vacía.
  const zona = {
    onDragOver: (e: DragEvent) => {
      e.preventDefault();
      setArrastrando(true);
    },
    onDragLeave: () => setArrastrando(false),
    onDrop: (e: DragEvent) => {
      e.preventDefault();
      setArrastrando(false);
      if (e.dataTransfer?.files.length) void anyadir(e.dataTransfer.files);
    },
  };
  const selectorDeFicheros = (
    <input
      ref={selector}
      type="file"
      accept="image/*"
      multiple
      class="hidden"
      onChange={(e) => {
        const el = e.currentTarget;
        if (el.files?.length) void anyadir(el.files);
        el.value = '';
      }}
    />
  );
  /**
   * «Añadir fotos» es LA acción principal de la galería y la única rellena de su
   * barra: lo demás (seleccionar, las parecidas) va de fantasma, que es como se
   * lee cuál es cuál sin pararse a mirarlo.
   *
   * En la barra, en móvil solo el icono: el rótulo mandaba el botón a una tercera
   * fila. En la galería VACÍA el rótulo va siempre: ahí es la única cosa que se
   * puede hacer, y un «+» suelto en mitad de la pantalla no dice qué añade.
   */
  const botonAnyadir = (siempreConRotulo: boolean) => (
    <Button
      variant="primary"
      leadingIcon={<Icon name="plus" />}
      disabled={subida !== null}
      onClick={() => selector.current?.click()}
      title={t('gallery.add')}
    >
      <span class={cx(!siempreConRotulo && 'sr-only sm:not-sr-only')}>{t('gallery.add')}</span>
    </Button>
  );
  const estadoSubida =
    subida || aviso || arrastrando ? (
      <p
        class={cx(
          'mb-4 rounded-md px-3 py-2 text-sm',
          arrastrando
            ? 'border-primary text-primary-text border border-dashed text-center'
            : 'text-text-muted',
        )}
        role="status"
      >
        {arrastrando
          ? t('gallery.dropHere')
          : subida
            ? t('gallery.uploading', { done: subida.hechas, total: subida.total })
            : aviso}
      </p>
    ) : null;

  /**
   * El agrupado por mes se calcula UNA vez por lista y no en cada render.
   *
   * Sin esto, marcar una foto —que solo cambia un `Set`— recorría las miles de
   * la línea de tiempo para volver a partirlas en meses y a formatear cada
   * rótulo. En una galería grande eso se siente al pulsar, que es justo donde
   * no se puede notar nada.
   */
  const grupos = useMemo(() => porMes(fotos, locale), [fotos, locale]);
  /** La posición de cada foto en la lista, para abrir el visor sin recorrerla. */
  const posiciones = useMemo(() => new Map(fotos.map((f, i) => [f.id, i])), [fotos]);

  const cargarMas = useCallback(async () => {
    if (fin) return;
    setCargando(true);
    const gen = generacion.current;
    try {
      const pagina = await lineaDeTiempo(cursor, 100, texto, soloFavoritas, tambien);
      // Si la búsqueda cambió mientras llegaba esta página, es de otra línea.
      if (gen !== generacion.current) return;
      setFotos((previas) => [...previas, ...pagina.items]);
      setCursor(pagina.nextCursor);
      if (!pagina.nextCursor) setFin(true);
      setError(null);
    } catch (e) {
      setError(String(e));
    } finally {
      setCargando(false);
    }
  }, [cursor, fin, texto, soloFavoritas, tambien]);

  useEffect(() => {
    void cargarMas();
    // Solo la primera página: las siguientes las pide el centinela.
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, []);

  /**
   * Ampliar la búsqueda con Sytue AI.
   *
   * SOLO A PETICIÓN, y solo se ofrece cuando la búsqueda literal no encuentra
   * nada. Ampliar siempre gastaría una llamada por búsqueda para no mejorar las
   * que ya funcionan, y quien busca «playa» y ve sus fotos de la playa no está
   * pidiendo nada más.
   *
   * Esto NO es buscar por significado de verdad: eso pediría vectorizar lo
   * escrito con el mismo modelo que vectorizó las fotos. Es preguntarle al
   * modelo de texto de qué otras formas se dice, y buscar también esas.
   */
  const ampliarBusqueda = async () => {
    if (ampliando || !texto.trim()) return;
    setAmpliando(true);
    try {
      const terminos = await terminosRelacionados(texto);
      if (terminos.length === 0) {
        setAviso(t('gallery.searchWiderNone'));
        return;
      }
      generacion.current++;
      setTambien(terminos);
      setFotos([]);
      setCursor(undefined);
      setFin(false);
      setAviso(t('gallery.searchWiderWith', { terms: terminos.join(', ') }));
    } catch {
      setAviso(t('gallery.searchWiderNone'));
    } finally {
      setAmpliando(false);
    }
  };

  /** Al cambiar la búsqueda se empieza otra línea: vacía, sin cursor y sin fin. */
  const buscar = (nuevo: string) => {
    generacion.current++;
    // Escribir otra cosa descarta lo ampliado: los alternativos eran de la
    // consulta anterior y arrastrarlos daría resultados que nadie pidió.
    setTambien([]);
    setAviso(null);
    setTexto(nuevo);
    setFotos([]);
    setCursor(undefined);
    setFin(false);
    setAbierta(null);
  };
  const primeraBusqueda = useRef(true);
  useEffect(() => {
    if (primeraBusqueda.current) {
      primeraBusqueda.current = false;
      return;
    }
    const id = setTimeout(() => void cargarMas(), 250);
    return () => clearTimeout(id);
    // Solo cuando cambia el texto o lo que amplía; `cargarMas` se recrea con
    // cada cursor.
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [texto, tambien]);

  /**
   * Carga infinita con IntersectionObserver y no con un listener de scroll: el
   * observador no corre en cada píxel desplazado, así que no compite con el
   * hilo que está pintando las imágenes que acaban de entrar.
   *
   * `rootMargin` generoso a propósito: pedir la página siguiente cuando el
   * centinela ya se ve deja un hueco en blanco mientras llega. Se pide una
   * pantalla antes.
   */
  useEffect(() => {
    const nodo = centinela.current;
    if (!nodo || fin) return;
    const obs = new IntersectionObserver(
      (entradas) => {
        if (entradas[0]?.isIntersecting && !cargando) void cargarMas();
      },
      { rootMargin: '800px' },
    );
    obs.observe(nodo);
    return () => obs.disconnect();
  }, [cargarMas, cargando, fin]);

  if (!cargando && fotos.length === 0 && !error && !texto) {
    return (
      <Marco user={user} hubBaseUrl={hubBaseUrl} locale={locale} title={t('app.title')}>
        <div
          class="mx-auto flex h-full w-full max-w-screen-2xl flex-col px-4 py-6 lg:px-8"
          {...zona}
        >
          <Nav t={t} locale={locale} activa={soloFavoritas ? 'favorites' : 'photos'} />
          {selectorDeFicheros}
          {estadoSubida}
          <EmptyState
            class="h-full"
            title={t(soloFavoritas ? 'favorites.empty.title' : 'gallery.empty.title')}
            description={t(
              soloFavoritas ? 'favorites.empty.description' : 'gallery.empty.description',
            )}
            illustration={<Illustration name="emptyPhotos" />}
            action={botonAnyadir(true)}
          />
        </div>
      </Marco>
    );
  }

  return (
    <Marco user={user} hubBaseUrl={hubBaseUrl} locale={locale} title={t('app.title')}>
      <div class="mx-auto w-full max-w-screen-2xl px-4 py-6 lg:px-8" {...zona}>
        <div class="flex flex-wrap items-center gap-2">
          <Nav t={t} locale={locale} activa={soloFavoritas ? 'favorites' : 'photos'} />
          {/* El `Input` de la casa, con su lupa: el campo a mano medía 28 px al lado
              de botones de 32, con otro borde y otro radio. La maqueta va en el
              envoltorio porque con icono el `class` de `Input` llega al <input>. */}
          <div class="mb-4 ml-auto min-w-0 flex-1 sm:w-64 sm:flex-none">
            <Input
              type="search"
              value={texto}
              onInput={(e) => buscar((e.target as HTMLInputElement).value)}
              placeholder={t('gallery.search')}
              aria-label={t('gallery.search')}
              leadingIcon={<Icon name="search" size={16} />}
              class={CAMPO_TACTIL}
            />
          </div>
          {/* Solo con una búsqueda hecha Y con aciertos: es el punto donde la
              búsqueda literal se queda corta —solo saca lo que alguien
              escribió— y esos aciertos sirven de punto de partida para llegar a
              las que nadie describió. Sin resultados no hay de dónde partir. */}
          {texto.trim() && fotos.length > 0 && seleccion === null ? (
            <span class="mb-4">
              <Button
                variant="ghost"
                leadingIcon={<Icon name="image" />}
                onClick={() => {
                  window.location.href = `${RUTA}similar?ids=${encodeURIComponent(
                    fotos
                      .slice(0, 12)
                      .map((f) => f.id)
                      .join(','),
                  )}`;
                }}
              >
                {t('gallery.likeTheseResults')}
              </Button>
            </span>
          ) : null}
          {fotos.length > 0 && seleccion === null ? (
            <span class="mb-4">
              <Button
                variant="ghost"
                leadingIcon={<Icon name="check" />}
                onClick={() => setSeleccion(new Set())}
              >
                {t('gallery.select')}
              </Button>
            </span>
          ) : null}
          <span class="mb-4">{botonAnyadir(false)}</span>
          {selectorDeFicheros}
        </div>
        {seleccion !== null ? (
          // Pegajosa: en una galería de miles, la barra de acciones tiene que seguir
          // a mano cuando lo marcado está tres pantallas más arriba.
          // En móvil es UNA fila que se desplaza de lado (ver scrollX.ts): envolver
          // sus controles sobre la rejilla de fotos se comía la galería.
          //
          // UNA BARRA DE CONTEXTO, NO UNA FILA DE BOTONES. Eran seis rellenos del
          // mismo peso más un `<select>` del sistema, y se partía en dos líneas. Ahora
          // se lee de izquierda a derecha: cómo se sale y cuántas van; lo que se
          // puede hacer con ellas, de fantasma y con su icono; y, tras un filete y en
          // el extremo, lo que destruye. La papelera va SOLA a la derecha a propósito:
          // sin vecino a ese lado, un clic que se pasa no la pulsa.
          //
          // `bg-surface-raised` y no `bg-surface`: va PEGADA encima de las fotos, y
          // `surface` es translúcida en el tema traslúcido.
          <Toolbar
            aria-label={t('gallery.select')}
            innerRef={barraSeleccion.ref}
            data-scroll-x
            class={cx(
              'border-border bg-surface-raised sticky top-0 z-20 mb-4 flex flex-wrap items-center gap-1 rounded-md border px-2 py-1.5 shadow-[var(--shadow-sm)]',
              FILA_UNICA_EN_MOVIL,
              barraSeleccion.queda && MASCARA_SCROLL_X,
            )}
          >
            <IconButton
              label={t('gallery.cancelSelection')}
              size="lg"
              onClick={() => setSeleccion(null)}
            >
              <Icon name="close" />
            </IconButton>
            <span
              class="mr-auto px-1 text-sm font-medium tabular-nums"
              role="status"
              aria-live="polite"
            >
              {t('gallery.selectedCount', { count: seleccion.size })}
            </span>
            {/* «Álbum nuevo» y «Añadir a un álbum…» eran dos mandos para la misma
                pregunta —¿a qué álbum van?—, y el segundo un `<select>` del sistema.
                Ahora es UN menú de la casa con el nuevo arriba, como en el visor. */}
            <Dropdown
              label={t('albums.addTo')}
              align="end"
              trigger={(props) => (
                <Button
                  {...props}
                  variant="ghost"
                  size="sm"
                  leadingIcon={<Icon name="album" />}
                  disabled={seleccion.size === 0}
                  title={t('albums.addTo')}
                  onMouseDown={cargarAlbumes}
                  onFocus={cargarAlbumes}
                >
                  <span class={ROTULO_DE_LOTE}>{t('albums.addTo')}</span>
                </Button>
              )}
            >
              {(cerrar) => (
                <>
                  <MenuItem
                    icon={<Icon name="plus" size={16} />}
                    onClick={() => {
                      cerrar();
                      void albumDeLaSeleccion();
                    }}
                  >
                    {t('gallery.newAlbum')}
                  </MenuItem>
                  {albumesLista && albumesLista.length > 0 ? <MenuSeparator /> : null}
                  {(albumesLista ?? []).map((a) => (
                    <MenuItem
                      key={a.id}
                      onClick={() => {
                        cerrar();
                        void anyadirSeleccionA(a.id);
                      }}
                    >
                      {a.compartidoPor ? `${a.title} · ${a.compartidoPor}` : a.title}
                    </MenuItem>
                  ))}
                </>
              )}
            </Dropdown>
            <Button
              variant="ghost"
              size="sm"
              leadingIcon={<Icon name="text" />}
              disabled={seleccion.size === 0 || describiendoLote !== null}
              title={t('gallery.describeSelected')}
              onClick={() => void describirSeleccion()}
            >
              {describiendoLote ? (
                // El progreso se lee siempre, también donde los rótulos se esconden:
                // es un botón que gasta dinero por foto.
                t('gallery.describing', {
                  done: describiendoLote.hechas,
                  total: describiendoLote.total,
                })
              ) : (
                <span class={ROTULO_DE_LOTE}>{t('gallery.describeSelected')}</span>
              )}
            </Button>
            {describiendoLote ? (
              // Parar tiene que estar a mano MIENTRAS corre: es un botón que
              // gasta dinero por foto, y quien se arrepiente a la décima no
              // debería tener que cerrar la pestaña.
              <Button
                variant="ghost"
                size="sm"
                onClick={() => {
                  pararLote.current = true;
                }}
              >
                {t('gallery.describeStop')}
              </Button>
            ) : null}
            <Button
              variant="ghost"
              size="sm"
              // El mismo icono que «Fotos parecidas» en el visor: es la misma
              // pregunta. Llevaba la lupa, que en el visor es el tamaño real.
              leadingIcon={<Icon name="image" />}
              disabled={seleccion.size === 0}
              title={t('gallery.moreLikeThese')}
              onClick={() => {
                window.location.href = `${RUTA}similar?ids=${encodeURIComponent(
                  [...seleccion].slice(0, 12).join(','),
                )}`;
              }}
            >
              <span class={ROTULO_DE_LOTE}>{t('gallery.moreLikeThese')}</span>
            </Button>
            <Button
              variant="ghost"
              size="sm"
              leadingIcon={<Icon name="download" />}
              disabled={seleccion.size === 0}
              title={t('gallery.downloadSelected')}
              onClick={descargarSeleccion}
            >
              <span class={ROTULO_DE_LOTE}>{t('gallery.downloadSelected')}</span>
            </Button>
            <Filete />
            <IconButton
              label={t('viewer.delete')}
              tone="danger"
              size="lg"
              disabled={seleccion.size === 0}
              onClick={() => setConfirmarPapelera(true)}
            >
              <Icon name="trash" />
            </IconButton>
          </Toolbar>
        ) : null}
        {estadoSubida}
        {error ? (
          <p class="text-danger-text mb-4 text-sm" role="alert">
            {error}
          </p>
        ) : null}

        {/* Solo en la galería de todas y sin búsqueda: un recuerdo entre los
            resultados de «playa» no es un recuerdo, es ruido. */}
        {!soloFavoritas && !texto.trim() ? (
          <Recuerdos
            locale={locale}
            onAbrir={(id) => {
              const i = posiciones.get(id);
              // Solo se abre si esa foto está en la página ya cargada. Un
              // recuerdo de 2019 puede estar tres mil filas más abajo, y saltar
              // ahí obligaría a cargar la línea de tiempo entera para enseñar
              // una; en ese caso se va a su pantalla de parecidas, que la
              // enseña grande sin arrastrar nada.
              if (i !== undefined) setAbierta(i);
              else window.location.href = `${RUTA}similar?id=${encodeURIComponent(id)}`;
            }}
          />
        ) : null}

        {/* Solo cuando la búsqueda literal no encuentra NADA: ampliar siempre
            gastaría una llamada por búsqueda sin mejorar las que ya funcionan. */}
        {texto.trim() && !cargando && fotos.length === 0 && tambien.length === 0 ? (
          <div class="border-border mb-6 rounded-md border p-4">
            <p class="text-text-muted mb-2 text-sm">{t('gallery.searchWiderHint')}</p>
            <Button
              variant="secondary"
              size="sm"
              leadingIcon={<Icon name="search" />}
              disabled={ampliando}
              onClick={() => void ampliarBusqueda()}
            >
              {t('gallery.searchWider')}
            </Button>
          </div>
        ) : null}

        {grupos.map((grupo) => (
          <section key={grupo.clave} class="mb-8">
            {/* `sticky`: el mes se queda arriba mientras se recorre, que es lo que
              permite saber dónde estás en una galería de miles. Opaca y del color
              del fondo: pegada tapa las fotos, y suelta no pinta una banda blanca. */}
            <h2
              class={cx(
                'bg-bg text-text-muted sytue-overline sticky z-10 flex items-center gap-3 py-2',
                // Bajo la barra de selección cuando está: si no, el mes se escondía detrás.
                // Su alto: 12 de relleno, 2 de borde y los 32 de un mando —44 con el dedo—.
                seleccion !== null ? 'top-[2.875rem] pointer-coarse:top-[3.625rem]' : 'top-0',
              )}
            >
              {grupo.titulo}
              {(() => {
                const aproximadas = grupo.fotos.filter((f) => !f.takenAtFromExif).length;
                return aproximadas > 0 ? (
                  <span class="text-text-subtle font-normal tracking-normal normal-case">
                    {t('gallery.approxCount', { count: aproximadas })}
                  </span>
                ) : null;
              })()}
              {seleccion !== null ? (
                <BotonDelMes
                  fotos={grupo.fotos}
                  seleccion={seleccion}
                  onToggle={alternarMes}
                  t={t}
                />
              ) : null}
            </h2>
            {/* Filas justificadas (`.filas`, en styles/app.css): cada foto con su
                proporción, y la fila crece con la pantalla. */}
            <ul class="filas">
              {grupo.fotos.map((f) => (
                <Miniatura
                  key={f.id}
                  foto={f}
                  t={t}
                  onOpen={() => setAbierta(posiciones.get(f.id) ?? 0)}
                  onFavorite={() => void cambiarFavorita(f, !f.favorite)}
                  {...(seleccion !== null
                    ? { seleccionada: seleccion.has(f.id), onToggle: () => alternar(f.id) }
                    : {})}
                />
              ))}
            </ul>
          </section>
        ))}

        <div ref={centinela} class="flex h-16 items-center justify-center">
          {cargando ? <Spinner /> : null}
        </div>

        {dialogoPreguntar}
        <ConfirmDialog
          open={confirmarPapelera}
          onClose={() => setConfirmarPapelera(false)}
          onConfirm={() => void tirarSeleccion()}
          title={t('viewer.delete')}
          confirmLabel={t('viewer.delete')}
          tone="danger"
        >
          <p class="text-text text-sm leading-relaxed">
            {t('gallery.trashSelectedConfirm', { count: seleccion?.size ?? 0 })}
          </p>
        </ConfirmDialog>

        {abierta !== null && fotos[abierta] ? (
          <Visor
            foto={fotos[abierta]}
            onFavorite={(f, v) => void cambiarFavorita(f, v)}
            onDescribe={(f, description, tags) =>
              setFotos((l) => l.map((x) => (x.id === f.id ? { ...x, description, tags } : x)))
            }
            t={t}
            locale={locale}
            onClose={() => setAbierta(null)}
            onDelete={(f) =>
              void alaPapelera(f.id)
                .then(() => {
                  setFotos((l) => l.filter((x) => x.id !== f.id));
                  setAbierta(null);
                })
                .catch((e) => setError(String(e)))
            }
            vecinas={[fotos[abierta - 1], fotos[abierta + 1]]}
            onPrev={abierta > 0 ? () => setAbierta(abierta - 1) : undefined}
            onNext={
              abierta < fotos.length - 1
                ? () => setAbierta(abierta + 1)
                : !fin
                  ? () => void cargarMas().then(() => setAbierta(abierta + 1))
                  : undefined
            }
          />
        ) : null}
      </div>
    </Marco>
  );
}

/**
 * El rótulo de un mando de la barra de selección: a la vista desde `xl`, y por
 * debajo solo para el lector de pantalla (el `title` del botón lo enseña al pasar).
 *
 * A 1280 la barra entera cabe en una línea con sitio de sobra (en castellano
 * ocupa unos dos tercios de sus 1198 px). A 1024 solo hay 942, y con los rótulos
 * de los idiomas largos ya no cabría y se partiría en dos líneas, que es justo lo
 * que tenía. Sin rótulos cabe en un teléfono.
 */
const ROTULO_DE_LOTE = 'max-xl:sr-only';

/** El mando de «marcar el mes entero», que también dice si ya lo está. */
function BotonDelMes({
  fotos,
  seleccion,
  onToggle,
  t,
}: {
  fotos: Foto[];
  seleccion: Set<string>;
  onToggle: (ids: string[]) => void;
  t: ReturnType<typeof useT>;
}): JSX.Element {
  const todas = fotos.every((f) => seleccion.has(f.id));
  return (
    // El `Button` de la casa, en su talla de cromo denso. El `<h2>` va en versalitas
    // y el botón las heredaría: se le quitan.
    <Button
      variant="ghost"
      size="xs"
      aria-pressed={todas}
      class="tracking-normal normal-case"
      onClick={() => onToggle(fotos.map((f) => f.id))}
    >
      {todas ? t('gallery.unselectMonth') : t('gallery.selectMonth')}
    </Button>
  );
}

/**
 * El visor: un `<dialog>` NATIVO a pantalla completa con la variante `grande`.
 * `showModal()` trae de serie el foco atrapado, Esc para cerrar y el fondo
 * inerte; las flechas pasan de foto. Se monta solo mientras está abierto, así
 * que la imagen grande no se pide hasta que alguien la quiere ver.
 */
/**
 * Pide la variante grande de las fotos de al lado mientras se mira una. Medido en el arnés
 * a 1440 con la red local: sin esto, cada flecha esperaba a que llegara y se decodificara
 * la foto siguiente; con una red de verdad, eso es el hueco en blanco entre dos fotos.
 * La caché del navegador hace el resto: la URL es la misma que pide el `<img>`.
 *
 * ponytail: solo la anterior y la siguiente; con el pase automático o a golpe de flecha
 * no hace falta más, y cada una son cientos de KB.
 */
function precargar(fotos: (Foto | undefined)[]): void {
  for (const f of fotos) {
    if (!f) continue;
    const i = new Image();
    i.decoding = 'async';
    i.src = urlDeVariante(f.id, 'grande');
    void i.decode().catch(() => undefined);
  }
}

/**
 * Dónde se pulsó al pedir el tamaño real, por escenario: el punto en fracciones de la foto
 * encajada y dónde quedaba dentro del escenario. Fuera del componente porque se escribe en
 * el clic y se lee en el efecto siguiente, y el compilador de React no deja mutar refs ni
 * el DOM desde el cuerpo de un componente.
 */
const ANCLAS = new WeakMap<HTMLElement, { fx: number; fy: number; vx: number; vy: number }>();

function anclar(img: HTMLElement | null, c: HTMLElement | null, e?: MouseEvent): void {
  if (!img || !c) return;
  const r = img.getBoundingClientRect();
  const rc = c.getBoundingClientRect();
  const px = e ? e.clientX : r.left + r.width / 2;
  const py = e ? e.clientY : r.top + r.height / 2;
  ANCLAS.set(c, {
    fx: (px - r.left) / Math.max(r.width, 1),
    fy: (py - r.top) / Math.max(r.height, 1),
    vx: px - rc.left,
    vy: py - rc.top,
  });
}

/** Ya a tamaño real: el punto pulsado, de vuelta bajo el puntero. */
function irAlAncla(img: HTMLElement | null, c: HTMLElement | null): void {
  const a = c ? ANCLAS.get(c) : undefined;
  if (!a || !img || !c) return;
  c.scrollLeft = a.fx * img.offsetWidth - a.vx;
  c.scrollTop = a.fy * img.offsetHeight - a.vy;
  ANCLAS.delete(c);
}

/** Descarga por un enlace efímero: la URL lleva `Content-Disposition: attachment`. */
function descargar(url: string): void {
  const a = document.createElement('a');
  a.href = url;
  a.download = '';
  document.body.appendChild(a);
  a.click();
  a.remove();
}

/**
 * LA FICHA SE PUEDE ESTIRAR, Y SE RECUERDA.
 *
 * Nacía con 18rem —252 px— y dentro hay una descripción escrita por el asistente
 * y una lista de etiquetas: con la columna de la izquierda ocupando lo que pide
 * «Fecha de subida (sin fecha EXIF)», al valor le quedaban unos cincuenta
 * píxeles y las palabras se partían letra a letra («11 de septi embr e de»).
 *
 * Se guarda en `localStorage` y no en el servidor porque es una preferencia de
 * ESTA pantalla en ESTE navegador, como el ancho de una barra lateral en
 * cualquier escritorio: no merece un viaje ni un campo en el perfil.
 *
 * Dos medidas y no una: en escritorio la ficha va a la derecha y lo que se
 * ajusta es el ANCHO; en móvil va debajo y es el ALTO. Compartir número entre
 * las dos daría una ficha de 600 px de alta en un teléfono.
 */
export const FICHA_ANCHO = { clave: 'sytue.fotos.ficha.ancho', min: 240, max: 720, defecto: 340 };
export const FICHA_ALTO = { clave: 'sytue.fotos.ficha.alto', min: 160, max: 560, defecto: 260 };

export function leerMedida({ clave, min, max, defecto }: typeof FICHA_ANCHO): number {
  if (typeof localStorage === 'undefined') return defecto;
  const n = Number(localStorage.getItem(clave));
  return Number.isFinite(n) && n > 0 ? Math.min(Math.max(n, min), max) : defecto;
}

export function Visor({
  foto,
  t,
  locale,
  onClose,
  onPrev,
  onNext,
  accion,
  onDelete,
  onFavorite,
  onDescribe,
  vecinas = [],
}: {
  foto: Foto;
  t: ReturnType<typeof useT>;
  locale: Locale;
  onClose: () => void;
  onPrev?: (() => void) | undefined;
  onNext?: (() => void) | undefined;
  /**
   * La anterior y la siguiente, para pedirlas mientras se mira esta: pasar de foto es
   * entonces enseñar algo que ya está, no esperar a la red. Ver `precargar`.
   */
  vecinas?: (Foto | undefined)[];
  /** Un mando extra en la cabecera (p. ej. «quitar del álbum»). */
  accion?: JSX.Element | undefined;
  /** Mover a la papelera; si no se pasa, el visor no ofrece borrar. */
  onDelete?: ((foto: Foto) => void) | undefined;
  /** Marcar o desmarcar favorita; sin él, el visor no ofrece el corazón. */
  onFavorite?: ((foto: Foto, valor: boolean) => void) | undefined;
  /** Guardar lo que Sytue AI vio en la foto; sin él, no se ofrece describir. */
  onDescribe?: ((foto: Foto, descripcion: string, etiquetas: string[]) => void) | undefined;
}): JSX.Element {
  const ref = useRef<HTMLDialogElement>(null);
  useMobileBack(true, onClose);
  const imagen = useRef<HTMLImageElement>(null);
  useEffect(() => {
    const d = ref.current;
    if (d && !d.open) d.showModal();
  }, []);
  const claveVecinas = vecinas.map((f) => f?.id ?? '').join(',');
  useEffect(() => precargar(vecinas), [claveVecinas]); // eslint-disable-line react-hooks/exhaustive-deps
  // «Añadir a un álbum»: la lista se pide al abrir el selector, no al abrir
  // el visor, para no pagar una petición por cada foto que se mira.
  const [lista, setLista] = useState<Album[] | null>(null);
  const [aviso, setAviso] = useState('');
  // La ficha (fecha, cámara, dimensiones, peso) se queda abierta al pasar de
  // foto: quien la abre para una quiere compararla con la siguiente.
  const [conFicha, setConFicha] = useState(false);
  const [descripcion, setDescripcion] = useState(foto.description ?? '');
  const [etiquetas, setEtiquetas] = useState<string[]>(foto.tags ?? []);
  const [editandoDesc, setEditandoDesc] = useState(false);
  const [textoEditado, setTextoEditado] = useState('');
  const [guardandoDesc, setGuardandoDesc] = useState(false);

  useEffect(() => {
    setDescripcion(foto.description ?? '');
    setEtiquetas(foto.tags ?? []);
    setEditandoDesc(false);
    setTextoEditado('');
  }, [foto.id, foto.description, foto.tags]);
  // Su medida, recordada entre visitas. Se lee de forma perezosa: en el primer
  // render del servidor no hay `localStorage`, y leerlo fuera dejaría el valor
  // del servidor pegado hasta la siguiente interacción.
  const [anchoFicha, setAnchoFicha] = useState(() => leerMedida(FICHA_ANCHO));
  const [altoFicha, setAltoFicha] = useState(() => leerMedida(FICHA_ALTO));
  const [enAncho, setEnAncho] = useState(true);
  useEffect(() => {
    const mq = matchMedia('(min-width: 48rem)');
    const aplicar = () => setEnAncho(mq.matches);
    aplicar();
    mq.addEventListener('change', aplicar);
    return () => mq.removeEventListener('change', aplicar);
  }, []);
  const arrastreFichaRef = useRef<{ desde: number; medida: number } | null>(null);

  /** Fija la medida dentro de sus topes y la recuerda. */
  const fijarMedida = useCallback(
    (valor: number, guardar: boolean) => {
      const cfg = enAncho ? FICHA_ANCHO : FICHA_ALTO;
      const v = Math.min(Math.max(Math.round(valor), cfg.min), cfg.max);
      (enAncho ? setAnchoFicha : setAltoFicha)(v);
      if (guardar && typeof localStorage !== 'undefined') {
        localStorage.setItem(cfg.clave, String(v));
      }
    },
    [enAncho],
  );
  // La confirmación de borrado va DENTRO del <dialog>: con showModal() todo lo
  // de fuera queda inerte, así que un Modal en portal no se podría pulsar. Y el
  // confirm() del navegador habla el idioma del sistema, no el de la app.
  const [borrando, setBorrando] = useState(false);
  useEffect(() => setBorrando(false), [foto.id]);
  // Pase automático (4 s por foto) y tamaño real. El pase se para solo en la
  // última foto; el tamaño real se olvida al cambiar de foto, porque lo que se
  // amplía es UNA foto, no el visor.
  const [pase, setPase] = useState(false);
  // Girar desde el visor: un paso «giro» más en la receta, que el servidor
  // guarda y con cuya huella se vuelve a pedir la imagen. Los lados se
  // intercambian aquí para que la caja del <img> no deje bandas.
  const [huella, setHuella] = useState<string | null>(null);
  const [lados, setLados] = useState<[number, number]>([foto.width, foto.height]);
  const [girando, setGirando] = useState(false);
  useEffect(() => {
    setHuella(null);
    setLados([foto.width, foto.height]);
  }, [foto.id, foto.width, foto.height]);
  const girar = async () => {
    if (girando) return;
    setGirando(true);
    try {
      const { recipe } = await obtenerReceta(foto.id);
      const receta: Receta = {
        ...recipe,
        pasos: [...recipe.pasos, { tipo: 'giro', params: { grados: 90 } }],
      };
      const { fingerprint } = await guardarReceta(foto.id, receta);
      setHuella(fingerprint);
      setLados(([w, h]) => [h, w]);
      setAviso(t('viewer.rotated'));
    } catch {
      setAviso(t('viewer.rotateFailed'));
    } finally {
      setGirando(false);
    }
  };
  const [tamanyoReal, setTamanyoReal] = useState(false);
  useEffect(() => setTamanyoReal(false), [foto.id]);
  /**
   * EL TAMAÑO REAL SE ABRE DONDE SE PULSA. Abría siempre por la esquina de arriba a la
   * izquierda: quien pulsaba en una cara para verla de cerca acababa mirando el cielo y
   * tenía que buscarla desplazando. Ahora el punto pulsado se queda bajo el puntero; con
   * «Z» o con el botón, el centro de la foto se queda en el centro.
   */
  const escenario = useRef<HTMLDivElement>(null);
  const alternarReal = (e?: MouseEvent) => {
    if (!tamanyoReal) anclar(imagen.current, escenario.current, e);
    setTamanyoReal((v) => !v);
  };
  useLayoutEffect(() => {
    if (tamanyoReal) irAlAncla(imagen.current, escenario.current);
  }, [tamanyoReal]);
  const [fondo, setFondo] = useState<FondoLienzo>(obtenerFondoLienzo);
  const cambiarFondo = (f: FondoLienzo) => {
    setFondo(f);
    guardarFondoLienzo(f);
  };
  const estiloFondo = useMemo(() => estilosFondoLienzo(fondo), [fondo]);
  useEffect(() => {
    if (!pase) return;
    if (!onNext) {
      setPase(false);
      return;
    }
    const id = setInterval(onNext, 4000);
    return () => clearInterval(id);
  }, [pase, onNext]);
  const atajosId = useId();
  const cargarAlbumes = () => {
    if (lista === null)
      void albumesParaAnyadir()
        .then(setLista)
        .catch(() => setLista([]));
  };
  const anyadir = async (albumId: string) => {
    const al = lista?.find((a) => a.id === albumId);
    if (!al) return;
    try {
      await anyadirAlAlbum(al.id, [foto.id]);
      setAviso(t('albums.added', { title: al.title }));
    } catch (e) {
      setAviso(sinEspacio(e) ? translate(locale, 'sharedAlbum.quotaFull') : String(e));
    }
  };
  type Fila = [string, ComponentChildren];
  const [describiendo, setDescribiendo] = useState(false);
  /**
   * Describir con Sytue AI. Se guarda EN CUANTO vuelve, sin pedir confirmación:
   * lo que devuelve es el texto alternativo de la foto, y una descripción que
   * hay que aceptar en un diálogo se queda sin aceptar. Se puede volver a pedir
   * cuantas veces haga falta; lo que no se puede es leer la foto con un lector
   * de pantalla mientras la descripción espera aprobación.
   */
  const describir = async () => {
    if (!onDescribe || describiendo) return;
    setDescribiendo(true);
    setAviso(t('viewer.describing'));
    try {
      const { description, tags } = await describirFoto(foto.id);
      await guardarDescripcion(foto.id, description, tags);
      setDescripcion(description);
      setEtiquetas(tags);
      onDescribe(foto, description, tags);
      setConFicha(true);
      setAviso(t('viewer.describedInInfo'));
    } catch (e) {
      setAviso(
        String(e).includes('ai_not_configured')
          ? t('viewer.describeUnavailable')
          : t('viewer.describeFailed'),
      );
    } finally {
      setDescribiendo(false);
    }
  };

  const guardarEdicionDesc = async () => {
    const nueva = textoEditado.trim();
    setGuardandoDesc(true);
    try {
      await guardarDescripcion(foto.id, nueva, etiquetas);
      setDescripcion(nueva);
      onDescribe?.(foto, nueva, etiquetas);
      setEditandoDesc(false);
      setAviso(t('viewer.describedInInfo'));
    } catch {
      setAviso(t('viewer.describeFailed'));
    } finally {
      setGuardandoDesc(false);
    }
  };

  const ficha: Fila[] = [
    [
      foto.takenAtFromExif ? t('viewer.infoTaken') : t('viewer.infoUploaded'),
      formatDateTime(locale, foto.takenAt, { dateStyle: 'long', timeStyle: 'short' }),
    ],
    ...(foto.camera ? [[t('viewer.infoCamera'), foto.camera] as Fila] : []),
    ...(foto.lens ? [[t('viewer.infoLens'), foto.lens] as Fila] : []),
    ...(foto.location
      ? [
          [
            t('viewer.infoPlace'),
            <Lugar key="lugar" id={foto.id} lugar={foto.location} t={t} />,
          ] as Fila,
        ]
      : []),
    [t('viewer.infoDimensions'), `${lados[0]} × ${lados[1]}`],
    [t('viewer.infoSize'), formatBytes(locale, foto.sizeBytes)],
  ];
  return (
    <dialog
      ref={ref}
      onClose={onClose}
      aria-describedby={atajosId}
      onKeyDown={(e) => {
        // Un campo y un menú abierto son de quien los tiene: en un menú las flechas
        // recorren sus opciones, no las fotos.
        if (
          e.target instanceof HTMLElement &&
          e.target.closest('select, input, textarea, [role="menu"]')
        )
          return;
        if (e.key === 'ArrowLeft') onPrev?.();
        else if (e.key === 'ArrowRight') onNext?.();
        else if (e.key === ' ') {
          e.preventDefault();
          setPase((v) => !v);
        } else if (e.key === 'i' || e.key === 'I') setConFicha((v) => !v);
        else if (e.key === 'z' || e.key === 'Z') alternarReal();
        else if ((e.key === 'Delete' || e.key === 'Backspace') && onDelete) setBorrando(true);
      }}
      class="sytue-safe-fullscreen bg-bg text-text h-dvh max-h-none w-screen max-w-none p-0 backdrop:bg-black/80"
    >
      <div class="flex h-full flex-col">
        {/* La barra se separa de la foto con un hairline: sin él, los mandos
            flotaban sobre el lienzo negro y la cabecera no se leía como barra. */}
        {/* En <1024px la barra NO cabe en una línea (medido el 2026-09-15: sacaba
            «Cerrar» de la pantalla). Ahí se parte en dos: nombre y cerrar arriba, y
            los mandos en su propia fila, que se desplaza de lado si aún no caben.
            Los menús de esta fila van en un portal con posición fija, así que el
            desplazamiento no los recorta. En ≥1024px, la línea de siempre. */}
        {/* UNA SOLA TALLA Y CUATRO GRUPOS. Había tres rótulos de texto, nueve iconos
            de 24, una papelera de 28 y un «Cerrar» escrito: cuatro formas de botón
            en una fila. Ahora todo es el mismo `IconButton` de 32 —44 con el dedo—,
            separado por filetes en lo que es de ESTA FOTO (favorita, álbum,
            descargar, parecidas), lo que la CAMBIA (describir, girar), cómo se MIRA
            (pase, tamaño real, ficha, fondo) y, aparte, lo que la tira. El único con
            rótulo es «Editar», que es a lo que se viene a un visor de un editor de
            fotos; cerrar es el aspa de cualquier diálogo de la casa. */}
        <header class="border-border flex items-center gap-2 border-b px-3 py-2 max-lg:flex-wrap">
          <span class="truncate text-sm font-medium max-lg:min-w-0 max-lg:flex-1">{foto.name}</span>
          <span role="status" aria-live="polite" class="text-text-muted ml-auto truncate text-xs">
            {aviso}
          </span>
          <div
            data-scroll-x
            // La máscara en el borde derecho es SOLO discoverability: la fila ya
            // se desplazaba (arriba se explica por qué), pero nada avisaba de que
            // había más mandos fuera de vista —en iOS no hay barra de scroll que
            // lo insinúe— y "no se ven las opciones" es justo lo que se reportó.
            // Se desvanece a partir de los últimos 28px; el resto de la fila
            // sigue opaco.
            //
            // `relative`: la fila DESPLAZA de lado, y el rótulo de «Editar» es un
            // `sr-only` (`absolute`) por debajo de `md`. Sin bloque contenedor aquí se
            // anclaba al diálogo en el sitio que ocuparía sin desplazar —al final de la
            // fila— y lo ensanchaba: 225 px de desplazamiento lateral a 390 y 295 a 320,
            // sin culpable visible (v3.11.1).
            class="relative flex items-center gap-1 max-lg:order-last max-lg:w-full max-lg:overflow-x-auto max-lg:[mask-image:linear-gradient(to_right,black_calc(100%-28px),transparent)] max-lg:[&>*]:shrink-0"
          >
            {accion}
            {accion ? <Filete /> : null}
            {onFavorite ? (
              <IconButton
                label={t(foto.favorite ? 'gallery.unfavorite' : 'gallery.favorite')}
                size="lg"
                pressed={foto.favorite}
                onClick={() => onFavorite(foto, !foto.favorite)}
              >
                <Icon name="corazon" {...(foto.favorite ? { fill: 'currentColor' } : {})} />
              </IconButton>
            ) : null}
            {/* UN DESPLEGABLE DEL SISTEMA Y NO UN <select>. El nativo se pinta con
              el control del sistema operativo —en macOS, su flecha y su marco
              azulado— y en una barra de mandos propios cantaba: ni el mismo
              borde, ni el mismo radio, ni el mismo foco. */}
            <Dropdown
              label={t('albums.addTo')}
              align="end"
              trigger={(props) => (
                <IconButton
                  {...props}
                  label={t('albums.addTo')}
                  size="lg"
                  onMouseDown={cargarAlbumes}
                  onFocus={cargarAlbumes}
                >
                  <Icon name="album" />
                </IconButton>
              )}
            >
              {(lista ?? []).map((a) => (
                <MenuItem key={a.id} onClick={() => void anyadir(a.id)}>
                  {a.compartidoPor ? `${a.title} · ${a.compartidoPor}` : a.title}
                </MenuItem>
              ))}
            </Dropdown>
            {/* Descargar en el formato que haga falta: JPEG para compartir, PNG sin
              pérdida para seguir editando fuera. La receta se aplica en el servidor
              con las mismas fórmulas que la vista previa. */}
            <Dropdown
              label={t('viewer.download')}
              align="end"
              trigger={(props) => (
                <IconButton {...props} label={t('viewer.download')} size="lg">
                  <Icon name="download" />
                </IconButton>
              )}
            >
              {(['jpeg', 'png'] as const).map((formato: FormatoExportacion) => (
                <MenuItem
                  key={formato}
                  onClick={() => descargar(urlDeExportacion(foto.id, formato))}
                >
                  {t(formato === 'png' ? 'viewer.downloadPng' : 'viewer.downloadJpeg')}
                </MenuItem>
              ))}
            </Dropdown>
            {/* Un enlace de verdad (`href` renderiza un <a>): se abre en otra pestaña
              y el lector de pantalla lo anuncia como enlace. */}
            <IconButton
              href={`${RUTA}similar?id=${encodeURIComponent(foto.id)}`}
              label={t('viewer.similar')}
              size="lg"
            >
              <Icon name="image" />
            </IconButton>
            <Filete />
            {onDescribe ? (
              <IconButton
                label={t('viewer.describe')}
                size="lg"
                disabled={describiendo}
                onClick={() => void describir()}
              >
                <Icon name="text" />
              </IconButton>
            ) : null}
            <IconButton
              label={t('viewer.rotate')}
              size="lg"
              disabled={girando}
              onClick={() => void girar()}
            >
              <Icon name="rotate" />
            </IconButton>
            <Filete />
            <IconButton
              label={pase ? t('viewer.slideshowStop') : t('viewer.slideshow')}
              size="lg"
              pressed={pase}
              disabled={!onNext && !pase}
              onClick={() => setPase((v) => !v)}
            >
              {pase ? <GlifoPausa /> : <GlifoPlay />}
            </IconButton>
            <IconButton
              label={tamanyoReal ? t('viewer.zoomOut') : t('viewer.zoomIn')}
              size="lg"
              pressed={tamanyoReal}
              onClick={() => alternarReal()}
            >
              <Icon name="search" />
            </IconButton>
            <IconButton
              label={t('viewer.info')}
              size="lg"
              pressed={conFicha}
              onClick={() => setConFicha((v) => !v)}
            >
              <Icon name="info" />
            </IconButton>
            <Dropdown
              label={t('viewer.canvasBackground')}
              align="end"
              trigger={(props) => (
                <IconButton
                  {...props}
                  label={t('viewer.canvasBackground')}
                  size="lg"
                  pressed={fondo !== 'crema'}
                >
                  <Icon name="grid" />
                </IconButton>
              )}
            >
              {(['crema', 'blanco', 'oscuro', 'damero'] as const).map((opcion) => (
                <MenuItem key={opcion} onClick={() => cambiarFondo(opcion)}>
                  <span class="flex items-center gap-2">
                    <span
                      class={cx(
                        'h-3 w-3 shrink-0 rounded-full border',
                        opcion === 'crema' && 'border-[#d8d3c6] bg-[#faf9f6]',
                        opcion === 'blanco' && 'border-gray-300 bg-white',
                        opcion === 'oscuro' && 'border-gray-700 bg-[#131619]',
                        opcion === 'damero' &&
                          'border-gray-300 bg-white bg-[repeating-conic-gradient(#cbd5e1_0%_25%,transparent_0%_50%)] bg-[length:6px_6px]',
                      )}
                    />
                    <span>{t(`editor.bg_${opcion}`)}</span>
                    {fondo === opcion ? (
                      <Icon name="check" size={14} class="text-primary ml-auto" />
                    ) : null}
                  </span>
                </MenuItem>
              ))}
            </Dropdown>
            {onDelete ? (
              <>
                <Filete />
                <IconButton
                  label={t('viewer.delete')}
                  tone="danger"
                  size="lg"
                  onClick={() => setBorrando(true)}
                >
                  <Icon name="trash" />
                </IconButton>
              </>
            ) : null}
            <Filete />
            <Button
              href={`${RUTA}edit?id=${encodeURIComponent(foto.id)}`}
              variant="ghost"
              size="sm"
              leadingIcon={<Icon name="pen" />}
              title={t('viewer.edit')}
            >
              <span class="max-md:sr-only">{t('viewer.edit')}</span>
            </Button>
          </div>
          <IconButton label={t('viewer.close')} size="lg" onClick={onClose}>
            <Icon name="close" />
          </IconButton>
        </header>
        {borrando && onDelete ? (
          <div
            role="group"
            aria-label={t('viewer.delete')}
            class="border-border bg-surface mx-3 mb-2 flex flex-wrap items-center gap-2 rounded-md border px-3 py-2 text-sm"
          >
            <span class="min-w-0 flex-1">{t('viewer.deleteConfirm')}</span>
            <Button variant="danger" size="sm" onClick={() => onDelete(foto)}>
              {t('viewer.delete')}
            </Button>
            <Button variant="ghost" size="sm" onClick={() => setBorrando(false)}>
              {t('common.cancel')}
            </Button>
          </div>
        ) : null}
        <div class="flex min-h-0 flex-1 max-md:flex-col">
          <EscenarioVisor
            prev={t('viewer.prev')}
            next={t('viewer.next')}
            onPrev={onPrev}
            onNext={onNext}
            real={tamanyoReal}
            foto={imagen}
            clave={foto.id}
            contenedor={escenario}
            class={tamanyoReal ? 'overflow-auto' : 'items-center justify-center overflow-hidden'}
          >
            {/* oxlint-disable-next-line jsx-a11y/click-events-have-key-events, jsx-a11y/no-noninteractive-element-interactions -- «Z» hace lo mismo desde el teclado */}
            <img
              ref={imagen}
              src={
                huella
                  ? urlDeVarianteCon(foto.id, 'grande', huella)
                  : urlDeVariante(foto.id, 'grande')
              }
              alt={descripcion || textoAlternativo(foto)}
              width={lados[0]}
              height={lados[1]}
              decoding="async"
              onClick={(e) => alternarReal(e)}
              style={estiloFondo.style}
              class={cx(
                // `h-auto w-auto`: la caja del <img> es la de la FOTO. Con los `width` y
                // `height` del original puestos y solo los topes, la caja se estiraba al
                // hueco entero y `object-contain` dejaba la foto dentro con bandas a los
                // lados pintadas del fondo del lienzo: crema junto a la foto también en
                // oscuro (medido a 1440: 134 878 px² de banda en una foto 3:2).
                tamanyoReal
                  ? 'max-h-none max-w-none cursor-zoom-out'
                  : 'h-auto max-h-full w-auto max-w-full cursor-zoom-in object-contain',
                estiloFondo.className,
              )}
            />
            {descripcion ? (
              <div
                role="region"
                aria-label={t('viewer.infoDescriptionTitle')}
                // Sobre la foto, como las flechas: negro y blanco, sin desenfoque. Al
                // 85 % un `backdrop-blur` no se ve y se paga en cada fotograma.
                class="pointer-events-auto absolute bottom-4 left-1/2 z-10 flex max-w-[90vw] -translate-x-1/2 items-start gap-2.5 rounded-lg bg-black/85 px-4 py-2.5 text-white md:max-w-xl"
              >
                <span class="mt-0.5 shrink-0 text-white/70" aria-hidden="true">
                  <Icon name="text" size={16} />
                </span>
                <div class="min-w-0 flex-1">
                  <div class="mb-0.5 flex items-center justify-between gap-2">
                    <span class="text-[10px] font-bold tracking-wider text-white/70 uppercase">
                      {t('viewer.infoDescriptionTitle')}
                    </span>
                    {/* Blanco y no `text-primary-text`: ese token es la tinta del tema
                        para SUS superficies —azul oscuro en el claro— y sobre este
                        negro se quedaba en 2,2:1. */}
                    <button
                      type="button"
                      onClick={() => setConFicha(true)}
                      class="sytue-focus-ring flex shrink-0 cursor-pointer items-center gap-1 rounded-sm text-xs font-medium text-white underline-offset-2 hover:underline"
                    >
                      <Icon name="info" size={12} />
                      {t('viewer.viewInInfo')}
                    </button>
                  </div>
                  <p class="line-clamp-2 text-xs leading-snug text-white/95 transition-all select-text hover:line-clamp-none sm:text-sm">
                    {descripcion}
                  </p>
                </div>
              </div>
            ) : null}
          </EscenarioVisor>
          {conFicha ? (
            // A la derecha en escritorio y debajo en móvil, sin tapar la foto en
            // ninguno de los dos: la ficha se consulta MIRANDO la imagen.
            <>
              {/* EL ASA. `pointer` y no `mouse` + `touch`: es el mismo código
                  para ratón, dedo y lápiz, y con `setPointerCapture` el arrastre
                  sigue funcionando aunque el puntero se salga del asa —que es lo
                  que pasa siempre al estirar deprisa—.
                  Con teclado también: es un `separator` y responde a las flechas,
                  porque un panel que solo se ajusta arrastrando no lo ajusta
                  quien no puede arrastrar. */}
              <div
                role="separator"
                aria-orientation={enAncho ? 'vertical' : 'horizontal'}
                aria-label={t('viewer.infoResize')}
                aria-valuenow={enAncho ? anchoFicha : altoFicha}
                aria-valuemin={enAncho ? FICHA_ANCHO.min : FICHA_ALTO.min}
                aria-valuemax={enAncho ? FICHA_ANCHO.max : FICHA_ALTO.max}
                tabIndex={0}
                class={cx(
                  'sytue-focus-ring bg-border/40 hover:bg-primary/40 active:bg-primary/60 shrink-0 touch-none transition-colors',
                  // 6 px de agarre visible, pero el área que responde al dedo es
                  // mayor por el relleno negativo: 6 px no se aciertan en un
                  // teléfono.
                  enAncho
                    ? 'w-1.5 cursor-col-resize max-md:hidden'
                    : 'h-1.5 w-full cursor-row-resize md:hidden',
                )}
                onPointerDown={(e: PointerEvent) => {
                  const el = e.currentTarget as HTMLElement;
                  el.setPointerCapture(e.pointerId);
                  arrastreFichaRef.current = {
                    desde: enAncho ? e.clientX : e.clientY,
                    medida: enAncho ? anchoFicha : altoFicha,
                  };
                }}
                onPointerMove={(e: PointerEvent) => {
                  const a = arrastreFichaRef.current;
                  if (!a) return;
                  // La ficha está a la derecha (o abajo): al arrastrar hacia la
                  // izquierda (o hacia arriba) CRECE, de ahí el signo.
                  const delta = a.desde - (enAncho ? e.clientX : e.clientY);
                  fijarMedida(a.medida + delta, false);
                }}
                onPointerUp={() => {
                  if (!arrastreFichaRef.current) return;
                  arrastreFichaRef.current = null;
                  fijarMedida(enAncho ? anchoFicha : altoFicha, true);
                }}
                onKeyDown={(e: KeyboardEvent) => {
                  const paso = e.shiftKey ? 48 : 16;
                  const crece = enAncho ? 'ArrowLeft' : 'ArrowUp';
                  const mengua = enAncho ? 'ArrowRight' : 'ArrowDown';
                  if (e.key !== crece && e.key !== mengua) return;
                  e.preventDefault();
                  const actual = enAncho ? anchoFicha : altoFicha;
                  fijarMedida(actual + (e.key === crece ? paso : -paso), true);
                }}
              />
              <aside
                aria-label={t('viewer.info')}
                class="border-border shrink-0 space-y-4 overflow-y-auto border-l p-4 max-md:border-t max-md:border-l-0"
                style={enAncho ? { width: `${anchoFicha}px` } : { height: `${altoFicha}px` }}
              >
                {/* Sección destacada: Descripción y etiquetas (Sytue AI) */}
                <section class="bg-bg-subtle border-border rounded-lg border p-3">
                  <div class="mb-1.5 flex items-center justify-between gap-2">
                    <h3 class="text-text flex items-center gap-1.5 text-xs font-semibold tracking-wider uppercase">
                      <Icon name="text" size={14} class="text-primary shrink-0" />
                      {t('viewer.infoDescriptionTitle')}
                    </h3>
                    {onDescribe && !editandoDesc ? (
                      <div class="flex items-center gap-1">
                        {descripcion ? (
                          <IconButton
                            label={t('viewer.editDescription')}
                            size="sm"
                            onClick={() => {
                              setTextoEditado(descripcion);
                              setEditandoDesc(true);
                            }}
                          >
                            <Icon name="pen" size={14} />
                          </IconButton>
                        ) : null}
                        <Button
                          variant="ghost"
                          size="xs"
                          loading={describiendo}
                          title={t('viewer.describe')}
                          onClick={() => void describir()}
                        >
                          {describiendo
                            ? t('viewer.describing')
                            : descripcion
                              ? t('viewer.describeAgain')
                              : t('viewer.describe')}
                        </Button>
                      </div>
                    ) : null}
                  </div>

                  <p class="text-text-muted mb-2.5 text-[11px]">
                    {t('viewer.infoDescriptionHint')}
                  </p>

                  {editandoDesc ? (
                    <div class="space-y-2">
                      <textarea
                        value={textoEditado}
                        onInput={(e) => setTextoEditado((e.target as HTMLTextAreaElement).value)}
                        rows={3}
                        class="border-border bg-bg text-text w-full resize-none rounded-md border px-2.5 py-1.5 text-xs sm:text-sm"
                        placeholder={t('viewer.infoDescriptionEmpty')}
                      />
                      <div class="flex justify-end gap-2">
                        <Button
                          size="sm"
                          variant="ghost"
                          onClick={() => setEditandoDesc(false)}
                          disabled={guardandoDesc}
                        >
                          {t('common.cancel')}
                        </Button>
                        <Button
                          size="sm"
                          variant="primary"
                          onClick={() => void guardarEdicionDesc()}
                          disabled={guardandoDesc}
                        >
                          {guardandoDesc
                            ? t('viewer.savingDescription')
                            : t('viewer.saveDescription')}
                        </Button>
                      </div>
                    </div>
                  ) : descripcion ? (
                    <div class="space-y-2">
                      <p class="text-text bg-bg/70 border-border/60 rounded border p-2.5 text-xs leading-relaxed select-text sm:text-sm">
                        {descripcion}
                      </p>
                      {etiquetas && etiquetas.length > 0 ? (
                        <div class="flex flex-wrap gap-1 pt-0.5">
                          {etiquetas.map((tag) => (
                            <span
                              key={tag}
                              class="bg-surface text-text-muted border-border rounded-full border px-2 py-0.5 text-[11px]"
                            >
                              #{tag}
                            </span>
                          ))}
                        </div>
                      ) : null}
                    </div>
                  ) : (
                    <div class="py-2 text-center">
                      <p class="text-text-muted mb-2 text-xs">{t('viewer.infoDescriptionEmpty')}</p>
                      {onDescribe ? (
                        <Button
                          size="sm"
                          variant="secondary"
                          disabled={describiendo}
                          onClick={() => void describir()}
                          leadingIcon={<Icon name="text" />}
                        >
                          {describiendo ? t('viewer.describing') : t('viewer.describe')}
                        </Button>
                      ) : null}
                    </div>
                  )}
                </section>

                {/* Metadatos técnicos */}
                <h4 class="text-text-muted pt-1 text-xs font-medium tracking-wider uppercase">
                  {t('viewer.infoTechnical')}
                </h4>

                <dl class="grid grid-cols-[minmax(0,auto)_minmax(9rem,1fr)] gap-x-4 gap-y-2 text-sm">
                  {ficha.map(([clave, valor]) => (
                    <Fragment key={clave}>
                      <dt class="text-text-muted">{clave}</dt>
                      <dd class="min-w-0 break-words tabular-nums">{valor}</dd>
                    </Fragment>
                  ))}
                </dl>

                <PreguntarPorLaFoto key={foto.id} id={foto.id} t={t} />
              </aside>
            </>
          ) : null}
        </div>
      </div>
      <p id={atajosId} class="sr-only">
        {t('viewer.shortcuts')}
      </p>
    </dialog>
  );
}

/**
 * Preguntarle a Sytue AI algo sobre esta foto.
 *
 * LA RESPUESTA NO SE GUARDA, y se dice debajo. Describir sí escribe en la foto
 * —es su texto alternativo, un dato suyo—, pero «¿qué pone en ese cartel?» es
 * una conversación sobre la foto: guardarla la convertiría en una ficha de
 * datos que nadie revisó y que además saldría en las búsquedas.
 *
 * Es un `<form>` de verdad y no un botón suelto porque escribir y pulsar Intro
 * es como se hace una pregunta; con un `onClick` habría que reimplementar esa
 * tecla, y peor.
 */
function PreguntarPorLaFoto({ id, t }: { id: string; t: ReturnType<typeof useT> }): JSX.Element {
  const campo = useId();
  const [pregunta, setPregunta] = useState('');
  const [respuesta, setRespuesta] = useState('');
  const [pensando, setPensando] = useState(false);

  const enviar = async (e: Event) => {
    e.preventDefault();
    const texto = pregunta.trim();
    if (!texto || pensando) return;
    setPensando(true);
    setRespuesta('');
    try {
      setRespuesta(await preguntarFoto(id, texto));
    } catch (err) {
      setRespuesta(
        String(err).includes('ai_not_configured')
          ? t('viewer.describeUnavailable')
          : t('viewer.askFailed'),
      );
    } finally {
      setPensando(false);
    }
  };

  return (
    <form onSubmit={(e) => void enviar(e)} class="border-border mt-4 border-t pt-4">
      <label for={campo} class="text-text-muted mb-1 block text-sm">
        {t('viewer.ask')}
      </label>
      <div class="flex gap-2">
        <Input
          id={campo}
          type="text"
          value={pregunta}
          maxLength={300}
          placeholder={t('viewer.askPlaceholder')}
          onInput={(e) => setPregunta((e.target as HTMLInputElement).value)}
          class={cx('min-w-0 flex-1', CAMPO_TACTIL)}
        />
        <Button type="submit" variant="secondary" disabled={pensando || pregunta.trim() === ''}>
          {t('viewer.askSend')}
        </Button>
      </div>
      <p class="text-text-muted mt-2 text-sm" role="status" aria-live="polite">
        {pensando ? t('viewer.describing') : respuesta}
      </p>
      <p class="text-text-muted mt-2 text-xs">{t('viewer.askNotSaved')}</p>
    </form>
  );
}

/**
 * Dónde se hizo la foto: las coordenadas, y un enlace a un mapa.
 *
 * Cuatro decimales y no los que vengan: son unos once metros, que es lo que de
 * verdad sabe un GPS de móvil, y más cifras darían una precisión que el dato no
 * tiene.
 *
 * El mapa es un ENLACE, no un mapa incrustado: incrustarlo llamaría a un
 * servicio de fuera nada más abrir la ficha, con la posición de la foto dentro,
 * sin que nadie lo haya pedido. Así solo sale de aquí si se pulsa, y va sin
 * referente.
 */
function Lugar({
  id,
  lugar,
  t,
}: {
  id: string;
  lugar: { lat: number; lon: number };
  t: ReturnType<typeof useT>;
}): JSX.Element {
  const { lat, lon } = lugar;
  return (
    <>
      <span class="tabular-nums">
        {lat.toFixed(4)}, {lon.toFixed(4)}
      </span>{' '}
      <a
        href={`https://www.openstreetmap.org/?mlat=${lat}&mlon=${lon}#map=15/${lat}/${lon}`}
        target="_blank"
        rel="noreferrer noopener"
        class="sytue-focus-ring rounded-sm underline"
      >
        {t('viewer.infoPlaceMap')}
      </a>{' '}
      {/* «Por aquí» se pregunta desde la foto, no desde un mapa: quien mira una
          foto de una playa quiere las OTRAS de esa playa, y eso no se pide con
          unas coordenadas en la mano. */}
      <a
        href={`${RUTA}nearby?id=${encodeURIComponent(id)}`}
        class="sytue-focus-ring rounded-sm underline"
      >
        {t('viewer.infoPlaceNearby')}
      </a>
    </>
  );
}

function GlifoPlay(): JSX.Element {
  return <Glifo d="M7 5v14l12-7z" size={16} fill="currentColor" stroke="none" />;
}

function GlifoPausa(): JSX.Element {
  return <Glifo d="M7 5h4v14H7zm6 0h4v14h-4z" size={16} fill="currentColor" stroke="none" />;
}

/** El filete que separa dos grupos de mandos en una barra. */
export function Filete(): JSX.Element {
  return <span class="bg-border mx-1 h-5 w-px shrink-0" aria-hidden="true" />;
}

/**
 * Lo que va SOBRE UNA FOTO en una tesela: negro al 60 % con la tinta en blanco.
 *
 * Son literales y no tokens de rol a propósito. Un token cambia con el tema y lo
 * de debajo no: una foto es blanca o negra en los tres. Sobre el peor caso —una
 * foto blanca— el disco queda en gris 102 y el blanco a 5,7:1. Con el 30 % y el
 * 40 % que había, el corazón medía 2,1:1 y 2,9:1 ahí, por debajo del 3:1 que pide
 * WCAG 1.4.11 para un icono. Es el mismo par que el tono `media` de `IconButton`,
 * que es el de las flechas del visor.
 */
const SOBRE_FOTO = 'bg-black/60 text-white';

/**
 * Una tesela de las filas justificadas (`.filas`, en styles/app.css).
 *
 * EL HUECO SE RESERVA ANTES DE QUE LLEGUE LA IMAGEN. Sin eso, cada miniatura que
 * carga empuja a las de abajo y la galería da saltos mientras se desplaza: es el
 * defecto más visible de una galería y el más fácil de evitar. Lo reserva la
 * proporción de la foto, que viene del servidor con ella (`estiloDeTesela`).
 *
 * CON SU PROPORCIÓN, y no cuadrada. Fue cuadrada porque en una REJILLA la
 * proporción real dejaba filas de alturas distintas, con una banda en blanco bajo
 * las fotos más bajas. En filas justificadas ese problema no existe —en una fila
 * todas miden lo mismo de alto— y lo que se gana es la foto entera: medido con una
 * biblioteca de proporciones corrientes, el cuadrado enseñaba el 70 % de cada foto.
 * `object-cover` se queda para las únicas que se acotan, las extremas.
 *
 * EL RECORTE VA EN EL BOTÓN, NO EN EL `<li>`. Con `overflow-hidden` en el `<li>`,
 * el aro de foco del botón —una sombra por fuera— se recortaba entero: se podía
 * recorrer la galería con el tabulador sin ver por dónde se iba. El aro lleva
 * hueco (`-inset`) para que se distinga del anillo de una foto marcada, que es
 * del mismo azul.
 *
 * `loading="lazy"` y `decoding="async"`: en una galería de miles, decodificar en
 * el hilo principal lo bloquea justo cuando se está desplazando.
 */
export function Miniatura({
  foto,
  t,
  onOpen,
  seleccionada,
  onToggle,
  onFavorite,
}: {
  foto: Foto;
  t: ReturnType<typeof useT>;
  onOpen: () => void;
  /** En modo selección la tesela marca y desmarca en vez de abrir. */
  seleccionada?: boolean;
  onToggle?: () => void;
  /** Marcar favorita sin abrir la foto; sin él, la tesela solo la enseña. */
  onFavorite?: (() => void) | undefined;
}): JSX.Element {
  const seleccionando = onToggle !== undefined;
  return (
    <li
      style={estiloDeTesela(foto)}
      class={cx('group bg-bg-subtle relative rounded-sm', seleccionada && 'ring-primary ring-2')}
    >
      <button
        type="button"
        onClick={seleccionando ? onToggle : onOpen}
        {...(seleccionando ? {} : { 'aria-haspopup': 'dialog' as const })}
        class="sytue-focus-ring-inset block h-full w-full overflow-hidden rounded-sm"
        title={foto.description?.trim() ? `${foto.name} — ${foto.description}` : foto.name}
        aria-label={
          seleccionando
            ? t('gallery.selectTile', { name: foto.name })
            : t('gallery.open', { name: foto.name })
        }
        {...(seleccionando ? { 'aria-pressed': seleccionada === true } : {})}
      >
        <img
          src={urlDeVariante(foto.id, 'miniatura')}
          alt={textoAlternativo(foto)}
          width={foto.width}
          height={foto.height}
          loading="lazy"
          decoding="async"
          class={cx(
            'h-full w-full object-cover transition-opacity duration-150',
            seleccionada && 'opacity-70',
          )}
        />
      </button>
      {seleccionando ? (
        // Un círculo en la esquina, como en cualquier galería: se ve de un
        // vistazo cuáles van y cuáles no, sin depender del tinte de la imagen.
        // Sin marcar va al 50 %: con el 30 % que había, sobre una foto clara el
        // disco se quedaba a 2,1:1 de ella y el círculo vacío no se veía.
        <span
          aria-hidden="true"
          class={cx(
            'pointer-events-none absolute top-1 left-1 grid h-6 w-6 place-items-center rounded-full border-2 border-white shadow-[var(--shadow-sm)]',
            seleccionada ? 'bg-primary text-[var(--sytue-on-primary)]' : 'bg-black/50',
          )}
        >
          {seleccionada ? <Icon name="check" size={16} /> : null}
        </span>
      ) : null}
      {/* El corazón se ve SIEMPRE si está marcada y solo al pasar por encima si
          no: en una galería, un mando en cada tesela es ruido, pero saber cuáles
          son favoritas de un vistazo es justo lo que se busca.
          CON EL DEDO SE VE SIEMPRE (`pointer-coarse:opacity-100`) y se pulsa: ahí
          no hay «pasar por encima», y el botón de las no favoritas estaba en su
          esquina INVISIBLE, robándole el toque a la foto. Lo que fallaba no era
          que se pudiera pulsar, sino que no se viera.
          EL OBJETIVO MIDE 32, NO 44. El fondo que se ve mide 24; el botón le suma
          4 px de aire por cada lado y va pegado a la esquina. Con 44 no cabe: en
          un teléfono de 390 la fila más baja mide 98, el centro de la foto queda
          a 49 del borde y un objetivo de 44 llegaría a 5 px de él —quien toca la
          foto para abrirla la marcaría—; en uno de 320 la fila mide 79 y lo
          pisaría. Con 32 quedan 17 y 7 px de margen. */}
      {onFavorite && !seleccionando ? (
        <button
          type="button"
          onClick={onFavorite}
          aria-label={t(foto.favorite ? 'gallery.unfavorite' : 'gallery.favorite')}
          aria-pressed={foto.favorite}
          class={cx(
            'group/corazon sytue-focus-ring rounded-control absolute bottom-0 left-0 p-1',
            !foto.favorite &&
              'opacity-0 group-hover:opacity-100 focus-visible:opacity-100 pointer-coarse:opacity-100',
          )}
        >
          <span
            class={cx(
              'rounded-control block p-1 transition-colors duration-150 group-hover/corazon:bg-black/75 group-active/corazon:bg-black/85',
              SOBRE_FOTO,
            )}
          >
            <Icon name="corazon" size={16} {...(foto.favorite ? { fill: 'currentColor' } : {})} />
          </span>
        </button>
      ) : null}
      {!foto.takenAtFromExif ? (
        // La foto está colocada por su fecha de SUBIDA. Decirlo evita que una
        // descarga vieja parezca de hoy sin explicación.
        // Solo al pasar o enfocar: en una galería subida desde el móvil TODAS las
        // fotos la llevaban y una marca en cada tesela deja de decir nada. El dato
        // sigue a la vista en la cabecera del mes (cuántas van por fecha de subida).
        // `bg-surface-raised`, que es opaca en los tres temas: `bg-surface/80` es
        // casi transparente en el traslúcido y la marca se perdía sobre la foto.
        <span
          class="bg-surface-raised text-text-subtle pointer-events-none absolute right-1 bottom-1 rounded px-1 text-[10px] opacity-0 transition-opacity group-focus-within:opacity-100 group-hover:opacity-100"
          title={t('gallery.noExifDate')}
        >
          ~
        </span>
      ) : null}
    </li>
  );
}
