/*
 * SPDX-License-Identifier: Apache-2.0
 * SPDX-FileCopyrightText: (c) 2026 Daniel Delgado Ramos
 *
 * SyERP — la interfaz entera.
 *
 * NO HAY UNA PANTALLA POR MÓDULO. Hay una lista y una ficha, y las dos se
 * generan del catálogo que publica el servicio: qué modelos existen, qué campos
 * tienen y cómo se llaman en el idioma de quien mira. Por eso encender
 * Fabricación o Flota no añade ni un fichero aquí, y por eso las veinticuatro
 * áreas se manejan igual — que es la mitad de lo que hace a un ERP aprendible.
 *
 * Las tres decisiones de interfaz, que son deliberadas:
 *
 *  1. La barra lateral enseña SOLO lo que la organización ha encendido. Un ERP
 *     que abre con ochenta entradas es el motivo por el que la gente acaba
 *     pidiendo formación para dar de alta un cliente.
 *  2. Una sola acción principal por pantalla. El resto vive en la ficha, donde
 *     tiene contexto.
 *  3. ⌘K va a cualquier sitio. Sustituye a los menús anidados: no hay que saber
 *     en qué submenú vive «albaranes», basta con escribirlo.
 */
import { useCallback, useContext, useEffect, useId, useMemo, useRef, useState } from 'preact/hooks';
import type { JSX, TargetedSubmitEvent } from 'preact';
import { FranjaPeticion, SytueShell } from '@sytue/shell';
import { getAccessToken } from '@sytue/auth/client';
import {
  Modal,
  Badge,
  Button,
  Card,
  EmptyState,
  FILA_UNICA_EN_MOVIL,
  Icon,
  MASCARA_SCROLL_X,
  PaletaDeComandos,
  Spinner,
  cx,
  useScrollRestante,
  esAtajoDeApp,
  ToastProvider,
  useConfirm,
  useNavDrawer,
  useMobileBack,
  usePrompt,
  useToast,
  withBase,
  Checkbox,
  useAtajo,
} from '@sytue/ui';
import type { Locale } from '@sytue/i18n';
import { contiene, formatDate, translate } from '@sytue/i18n';
import type { SytueUser } from '@sytue/auth';
import { useT } from '../i18n/index.js';
import type {
  Actividad,
  Asiento,
  Campo,
  Catalogo,
  EstadisticasModelo,
  MisPermisos,
  Modelo,
  Modulo,
  Obligacion,
  Organizacion,
  Pagina,
  Registro,
  RequisitosFactura,
  RespuestaActivarModulo,
  RespuestaObligaciones,
  ResultadoImportacion,
  ResumenOperativo,
  Tablero,
} from './tipos.js';
import {
  aCentimos,
  alineadoDerecha,
  celda,
  deCentimos,
  formatearDinero,
  formatearFecha,
  nombreDe,
} from './formato.js';
import { api, baseDeLaApp, consulta, ErrorApi, mensajeDeError, esSinPermiso } from '../lib/api.js';
import { claveDeFicha } from '../lib/ficha';
import { SelectorRelacion } from './SelectorRelacion.js';
import { AsientoCompacto, Balance, LibroDiario } from './Libro.js';
import { Onboarding } from './Onboarding.js';
import { Obligaciones, diasHasta, faltas, rotuloDePeriodo } from './Obligaciones.js';
import { Banco } from './Banco.js';
import { AltasContext } from './AltasContext.js';
import { PuntoVenta } from './PuntoVenta.js';
import { AsistenteFacturas } from './AsistenteFacturas.js';
import { Tareas } from './Tareas.js';
import {
  DocumentosRegistro,
  HistorialRegistro,
  RequisitosDeFactura,
  TareasRegistro,
  hoyISO,
} from './Seguimiento.js';
import { PanelIA, RellenarIA, VistaTablero } from './SytueIA.js';
import { EstadoVerifactuFactura, TarjetaVerifactu } from './Verifactu.js';

export interface ErpAppProps {
  user: SytueUser;
  hubBaseUrl: string;
  locale: Locale;
  modeloInicial?: string | undefined;
  registroInicial?: string | undefined;
}

/**
 * Los últimos modelos abiertos aceleran el regreso a una tarea sin guardar
 * información de negocio ni cruzar sesiones. El valor neutro se pinta primero
 * para que el árbol SSR e hidratado coincidan; se recupera solo tras montar.
 */
const CLAVE_RECIENTES = 'sytue-erp-recent-models';

/**
 * Envuelve la app en el proveedor de avisos.
 *
 * `useToast()` devuelve un no-op cuando no hay proveedor, así que sin esto los
 * avisos no fallarían: simplemente no saldrían, y nadie se enteraría de que
 * faltan. Va fuera del componente principal para que TODO lo de dentro —incluida
 * la ficha, que es quien más avisa— comparta la misma pila.
 */
export function ErpApp(props: ErpAppProps): JSX.Element {
  return (
    <ToastProvider>
      <ErpAppInterna {...props} />
    </ToastProvider>
  );
}

function ErpAppInterna({
  user,
  hubBaseUrl,
  locale,
  modeloInicial,
  registroInicial,
}: ErpAppProps): JSX.Element {
  const t = useT(locale);
  const aviso = useToast();
  const [catalogo, setCatalogo] = useState<Catalogo | null>(null);
  const [altasRapidas, setAltasRapidas] = useState<
    { modelo: Modelo; creado: (r: Registro) => void; semilla?: Registro['data'] }[]
  >([]);
  const [asistenciaFacturas, setAsistenciaFacturas] = useState(false);
  const [modelo, setModelo] = useState<Modelo | null>(null);
  const [pagina, setPagina] = useState<Pagina | null>(null);
  const [cargandoLista, setCargandoLista] = useState(false);
  // Lo ya visto, por (modelo, búsqueda). Volver a una lista la pinta AL INSTANTE
  // con lo que había y la refresca por detrás: es la diferencia entre una
  // aplicación que se siente instantánea y una que parpadea en cada clic.
  //
  // En memoria y sin caducidad: vive lo que la pestaña, se invalida en cuanto se
  // escribe algo de ese modelo, y no pretende ser una caché de verdad. Un
  // `Map` basta —son unas pocas listas de cincuenta filas—.
  const cache = useRef(new Map<string, Pagina>());
  const [busqueda, setBusqueda] = useState('');
  // Filtro por estado. Es LA vista que pide cualquiera que use un ERP —«las
  // facturas pendientes», «los pedidos sin entregar»— y el servicio la servía
  // desde el principio sin que la interfaz la ofreciera.
  const [estado, setEstado] = useState('');
  // Orden pedido a mano. Vacío = el de por defecto (fecha de creación), que es el
  // único que va por índice: medido, ordenar por un campo del jsonb cuesta 20 ms
  // frente a 0,6 ms con 20.000 filas. Aceptable al pulsar una cabecera,
  // inaceptable como vista por defecto — por eso arranca vacío.
  const [orden, setOrden] = useState<{ campo: string; asc: boolean } | null>(null);
  const [papelera, setPapelera] = useState(false);
  // Cuenta las escrituras (alta, edición, borrado, importación): la lista
  // vuelve a pedir sus estadísticas cuando cambia, y no en cada búsqueda.
  const [revision, setRevision] = useState(0);
  const [ficha, setFicha] = useState<Registro | 'nueva' | null>(null);
  // De dónde copiar al crear. Duplicar un documento es LA acción que ahorra más
  // tiempo en un ERP —la factura de este mes es la del anterior con otra fecha—
  // y hasta ahora había que teclearlo todo otra vez.
  const [semilla, setSemilla] = useState<Registro | null>(null);
  const [paleta, setPaleta] = useState(false);
  // Sytue AI: el panel, los tableros guardados y el que se está viendo.
  const [panelIA, setPanelIA] = useState(false);
  const [tableros, setTableros] = useState<Tablero[]>([]);
  const [tablero, setTablero] = useState<Tablero | null>(null);
  const [recientes, setRecientes] = useState<string[]>([]);
  const [error, setError] = useState('');
  // La lista del módulo abierto contestó 403 `sin_permiso`: se dice en su sitio, en
  // lugar de la lista, y no con la franja de error encima de «Aquí todavía no hay
  // nada. Crea el primero con Nuevo», que es lo que se veía (y era falso).
  const [sinPermiso, setSinPermiso] = useState(false);
  // Qué ocupa el panel principal. El libro y el balance NO son modelos del
  // catálogo —un asiento no es un registro con campos declarados—, así que no
  // pueden ser «otro modelo abierto» y necesitan su propio estado.
  const [vista, setVista] = useState<Vista>('modelo');
  // El perfil de la organización: `undefined` mientras no se sabe, `null` si
  // aún no se ha hecho el asistente. Distinguirlos evita abrir el asistente a
  // quien ya lo hizo solo porque el servicio tardó en contestar.
  const [organizacion, setOrganizacion] = useState<Organizacion | null | undefined>(undefined);
  // Qué puede hacer esta persona en cada módulo. Nulo mientras no se sabe: se
  // enseña todo y manda el servicio, que es quien responde 403.
  const [permisos, setPermisos] = useState<MisPermisos | null>(null);
  const [requisitos, setRequisitos] = useState<RequisitosFactura | null>(null);
  // Tareas propias que vencen hoy o ya vencieron: el número de la barra lateral.
  const [tareasHoy, setTareasHoy] = useState(0);
  const [masCargando, setMasCargando] = useState(false);
  // EN MÓVIL NO HABÍA NAVEGACIÓN. La barra lateral era `hidden md:block`, así que
  // por debajo de 768 px no había forma de cambiar de módulo: se veía la lista
  // con la que se entró y nada más. La mecánica —trampa de foco, Esc, cierre al
  // pasar a escritorio, devolver el foco al botón— la pone `useNavDrawer` del
  // design system, que ya la comparten AdminShell, AppShell y Rooms.
  const [cajon, setCajon] = useState(false);
  const cajonBoton = useRef('erp-cajon-boton');
  useNavDrawer(cajon, 'erp-cajon', cajonBoton.current, () => setCajon(false), {
    desktopDrawer: true,
  });

  const modelos = useMemo(() => (catalogo?.modulos ?? []).flatMap((m) => m.modelos), [catalogo]);

  const puedeLeer = useCallback(
    (modulo: string) => !permisos || (permisos.modulos[modulo] ?? 'ninguno') !== 'ninguno',
    [permisos],
  );
  const puedeEscribir = useCallback(
    (modulo: string) =>
      !permisos || ['usuario', 'administrador'].includes(permisos.modulos[modulo] ?? 'ninguno'),
    [permisos],
  );

  const puedeAccion = useCallback(
    (modulo: string, verbo: string) =>
      Boolean(permisos && puedeEscribir(modulo) && permisos.verbos?.[modulo]?.includes(verbo)),
    [permisos, puedeEscribir],
  );

  const recargarPermisos = useCallback(() => {
    api<MisPermisos>('mis-permisos')
      .then(setPermisos)
      .catch(() => undefined);
  }, []);

  // El aviso de una petición de edición (`hubnotify.EnlacePeticionesERP`): la
  // administración ve en la franja quién la pide y la concede con un clic.
  const [moduloDelAviso] = useState(() =>
    typeof window === 'undefined'
      ? ''
      : (new URLSearchParams(window.location.search).get('peticion') ?? ''),
  );

  const contarTareas = useCallback(() => {
    api<{ actividades: Actividad[] }>(
      `actividades${consulta({ estado: 'pendiente', mias: '1', hasta: hoyISO() })}`,
    )
      .then((r) => setTareasHoy((r.actividades ?? []).length))
      .catch(() => undefined);
  }, []);
  useEffect(contarTareas, [contarTareas]);

  // --- Catálogo: una vez, al entrar ---------------------------------------
  useEffect(() => {
    let vivo = true;
    api<Catalogo>('catalogo')
      .then((c) => {
        if (!vivo) return;
        setCatalogo(c);
        const inicial = c.modulos.flatMap((m) => m.modelos).find((m) => m.clave === modeloInicial);
        if (inicial) setModelo(inicial);
      })
      .catch(() => vivo && setError(t('error.offline')));
    api<{ tableros: Tablero[] | null }>('tableros')
      .then((r) => vivo && setTableros(r.tableros ?? []))
      // Sin tableros la app sigue igual: la barra simplemente no los lista.
      .catch(() => undefined);
    api<MisPermisos>('mis-permisos')
      .then((p) => vivo && setPermisos(p))
      .catch(() => undefined);
    api<RequisitosFactura>('organizacion/requisitos')
      .then((r) => vivo && setRequisitos(r))
      .catch(() => undefined);
    // Un enlace de un correo o de una notificación puede traer la vista
    // («?vista=obligaciones», «?vista=tareas»).
    const pedida = new URLSearchParams(window.location.search).get('vista');
    if (pedida === 'tareas' || pedida === 'obligaciones' || pedida === 'banco') setVista(pedida);
    api<Organizacion>('organizacion')
      .then((o) => vivo && setOrganizacion(o))
      .catch((err) => {
        // 404 con código propio = todavía no hay perfil. Cualquier otro fallo
        // deja el estado en «no se sabe»: no se abre el asistente por un corte.
        if (vivo && err instanceof ErrorApi && err.estado === 404) setOrganizacion(null);
      });
    return () => {
      vivo = false;
    };
    // Solo al montar: el catálogo no cambia durante la sesión.
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, []);

  // Primera vez: sin perfil y sin ningún área encendida, lo que toca es el
  // asistente, no una cuadrícula de treinta y tres tarjetas apagadas. Solo si
  // no se entró por un enlace directo a un modelo: quien viene a una factura
  // viene a la factura.
  useEffect(() => {
    if (
      catalogo &&
      organizacion === null &&
      (catalogo.activos ?? []).length === 0 &&
      !modeloInicial
    ) {
      setVista('onboarding');
    }
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [catalogo, organizacion]);

  // --- Lista del modelo abierto -------------------------------------------
  const cargarLista = useCallback(
    async (
      m: Modelo,
      q: string,
      est: string,
      ord: { campo: string; asc: boolean } | null,
      pap: boolean,
    ) => {
      // El estado y el orden entran en la clave: sin ellos, filtrar u ordenar
      // servía la lista cacheada sin filtrar ni ordenar, y parecería que el
      // control no hace nada.
      const clave = `${m.clave}|${q}|${est}|${ord ? `${ord.campo}:${ord.asc}` : ''}|${pap ? 'p' : ''}`;
      const guardada = cache.current.get(clave);
      // Si ya se vio, se pinta sin esperar y sin rueda: la petición sigue, pero
      // por detrás. Quien vuelve a una lista no debería ver un hueco.
      if (guardada) setPagina(guardada);
      else setCargandoLista(true);
      try {
        const fresca = await api<Pagina>(
          `${m.clave}${consulta({
            q,
            estado: est,
            orden: ord?.campo,
            dir: ord?.asc === true ? 'asc' : undefined,
            papelera: pap ? '1' : undefined,
          })}`,
        );
        cache.current.set(clave, fresca);
        setPagina(fresca);
        setError('');
        setSinPermiso(false);
      } catch (err) {
        // Con algo ya pintado, un fallo de red no borra la pantalla: se avisa y
        // se deja lo que había, que sigue siendo cierto hasta hace un momento.
        if (!guardada) setPagina(null);
        setSinPermiso(esSinPermiso(err));
        setError(esSinPermiso(err) ? '' : mensajeDeError(err, t));
      } finally {
        setCargandoLista(false);
      }
    },
    [t],
  );

  /**
   * La página siguiente, por cursor keyset. Se AÑADE a lo que ya hay en vez de
   * sustituirlo: quien pide «ver más» quiere más, no otra pantalla.
   *
   * Hasta ahora el cursor que devuelve el servicio no lo usaba nadie, así que una
   * lista se acababa en cincuenta filas y no había forma de llegar a la 51 — en
   * un ERP eso son las facturas del mes pasado.
   */
  const cargarMas = useCallback(async () => {
    if (!modelo || !pagina?.siguiente || masCargando) return;
    setMasCargando(true);
    try {
      const siguiente = await api<Pagina>(
        `${modelo.clave}${consulta({
          q: busqueda,
          estado,
          orden: orden?.campo,
          dir: orden?.asc === true ? 'asc' : undefined,
          papelera: papelera ? '1' : undefined,
          cursor: pagina.siguiente,
        })}`,
      );
      const unida: Pagina = {
        registros: [...(pagina.registros ?? []), ...(siguiente.registros ?? [])],
        // Las etiquetas de las relaciones se acumulan: las de la página anterior
        // siguen haciendo falta para pintar sus filas.
        etiquetas: { ...pagina.etiquetas, ...siguiente.etiquetas },
        ...(siguiente.siguiente === undefined ? {} : { siguiente: siguiente.siguiente }),
      };
      cache.current.set(
        `${modelo.clave}|${busqueda}|${estado}|${orden ? `${orden.campo}:${orden.asc}` : ''}|${papelera ? 'p' : ''}`,
        unida,
      );
      setPagina(unida);
    } catch (err) {
      setError(mensajeDeError(err, t));
    } finally {
      setMasCargando(false);
    }
  }, [modelo, pagina, busqueda, estado, orden, papelera, masCargando, t]);

  useEffect(() => {
    if (!modelo) return;
    // Espera a que se deje de teclear: una lista por pulsación es una consulta
    // por pulsación contra la base de datos de la organización.
    const id = setTimeout(
      () => void cargarLista(modelo, busqueda, estado, orden, papelera),
      busqueda ? 220 : 0,
    );
    return () => clearTimeout(id);
  }, [modelo, busqueda, estado, orden, papelera, cargarLista]);

  // Deep-link `?r=`: abre la ficha en cuanto la lista la trae.
  useEffect(() => {
    if (!registroInicial || !pagina?.registros) return;
    const r = pagina.registros.find((x) => x.id === registroInicial);
    if (r) setFicha(r);
  }, [registroInicial, pagina]);

  // --- ⌘K -----------------------------------------------------------------
  useEffect(() => {
    const alPulsar = (ev: KeyboardEvent): void => {
      if ((ev.metaKey || ev.ctrlKey) && ev.key.toLowerCase() === 'k') {
        ev.preventDefault();
        setPaleta(true);
        return;
      }
      if (ev.key === 'Escape') {
        setPaleta(false);
        return;
      }
      // Las teclas sueltas solo mandan si no se está escribiendo y no hay nada
      // flotando delante. `esAtajoDeApp` lo comprueba en un sitio para toda la
      // suite: sin ella, teclear «n» en el nombre de un cliente abriría una ficha
      // nueva encima.
      if (!esAtajoDeApp(ev)) return;
      if (ev.key === 'n' && modelo && vista === 'modelo') {
        ev.preventDefault();
        setSemilla(null);
        setFicha('nueva');
      } else if (ev.key === '/') {
        ev.preventDefault();
        document.querySelector<HTMLInputElement>('input[type="search"]')?.focus();
      }
    };
    window.addEventListener('keydown', alPulsar);
    return () => window.removeEventListener('keydown', alPulsar);
  }, [modelo, vista]);

  useEffect(() => {
    try {
      const guardados = JSON.parse(sessionStorage.getItem(CLAVE_RECIENTES) ?? '[]') as unknown;
      if (Array.isArray(guardados) && guardados.every((clave) => typeof clave === 'string')) {
        setRecientes(guardados.slice(0, 6));
      }
    } catch {
      // La comodidad nunca es requisito para abrir el ERP: un almacenamiento
      // bloqueado o corrupto simplemente deja la sección de recientes vacía.
    }
  }, []);

  const recordarModelo = useCallback((m: Modelo) => {
    setRecientes((anteriores) => {
      const siguientes = [m.clave, ...anteriores.filter((clave) => clave !== m.clave)].slice(0, 6);
      try {
        sessionStorage.setItem(CLAVE_RECIENTES, JSON.stringify(siguientes));
      } catch {
        // El estado de esta pestaña sigue siendo suficiente aunque el navegador
        // no permita guardar la preferencia de sesión.
      }
      return siguientes;
    });
  }, []);

  const abrirModelo = useCallback(
    (m: Modelo) => {
      recordarModelo(m);
      setCajon(false);
      setVista('modelo');
      setModelo(m);
      setBusqueda('');
      // Los estados los declara CADA modelo: arrastrar «pagado» a una lista de
      // vehículos pediría un estado que ese modelo no tiene, y el servicio lo
      // rechazaría con un error que nadie sabría de dónde sale.
      setEstado('');
      setOrden(null);
      setPapelera(false);
      setFicha(null);
      setPaleta(false);
      // Lo de ESTE modelo, o nada. Sin esta línea se quedaban en pantalla las filas
      // del modelo anterior mientras llegaba la nueva lista, pintadas con las
      // columnas del nuevo: una tabla de guiones que parecía un error de datos.
      setPagina(cache.current.get(`${m.clave}||||`) ?? null);
    },
    [recordarModelo],
  );

  // Abrir el registro del que cuelga una tarea: su lista y su ficha.
  const abrirRegistro = useCallback(
    (clave: string, id: string) => {
      const m = modelos.find((x) => x.clave === clave);
      if (!m) return;
      abrirModelo(m);
      api<Registro>(`${clave}/${id}`)
        .then(setFicha)
        .catch(() => aviso.notify({ tone: 'danger', title: t('error.generic') }));
    },
    [modelos, abrirModelo, aviso, t],
  );

  const activos = catalogo?.activos ?? null;
  const puedeCrear = useCallback(
    (clave: string) => {
      const m = modelos.find((x) => x.clave === clave);
      return Boolean(m && activos?.includes(m.modulo) && puedeAccion(m.modulo, 'crear'));
    },
    [modelos, activos, puedeAccion],
  );
  const modulosVisibles = useMemo(
    () => (catalogo?.modulos ?? []).filter((m) => activos?.includes(m.clave) && puedeLeer(m.clave)),
    [catalogo, activos, puedeLeer],
  );

  /**
   * Encender o APAGAR un módulo. La interfaz solo sabía encender, así que activar
   * uno por error lo dejaba en la barra lateral para siempre — y esconder lo que
   * no se usa es la mitad de por qué esto se maneja mejor que la competencia.
   *
   * Apagar no borra nada: los registros siguen ahí y vuelven en cuanto se
   * reactive. Es una decisión de qué se enseña, no de qué se guarda.
   */
  async function cambiarModulo(modulo: Modulo, activo: boolean): Promise<void> {
    try {
      const res = await api<RespuestaActivarModulo>(`modulos/${modulo.clave}`, {
        method: 'POST',
        body: JSON.stringify({ activo }),
      });
      setCatalogo((c) => {
        if (!c) return c;
        return { ...c, activos: res.activos };
      });

      if (activo && res.activados && res.activados.length > 0 && catalogo) {
        const nombresActivados = res.activados
          .map((k) => catalogo.modulos.find((m) => m.clave === k)?.etiqueta ?? k)
          .join(', ');
        aviso.notify({
          tone: 'info',
          title: t('modules.dependencyActivated', {
            module: modulo.etiqueta,
            deps: nombresActivados,
          }),
        });
      }

      if (!activo && res.desactivados && res.desactivados.length > 0 && catalogo) {
        const nombresDesactivados = res.desactivados
          .map((k) => catalogo.modulos.find((m) => m.clave === k)?.etiqueta ?? k)
          .join(', ');
        aviso.notify({
          tone: 'info',
          title: t('modules.dependencyDeactivated', {
            module: modulo.etiqueta,
            deps: nombresDesactivados,
          }),
        });
      }

      // Si se apaga el módulo del modelo abierto (o alguno de los desactivados en cascada),
      // la lista de detrás dejaría de ser alcanzable desde la barra lateral: se vuelve al inicio.
      if (
        !activo &&
        (modelo?.modulo === modulo.clave || (res.desactivados ?? []).includes(modelo?.modulo ?? ''))
      ) {
        setModelo(null);
        setPagina(null);
      }
    } catch (err) {
      setError(mensajeDeError(err, t));
    }
  }

  return (
    <AltasContext.Provider
      value={{
        puedeCrear,
        crear: (clave, creado, semilla) => {
          const m = modelos.find((x) => x.clave === clave);
          if (m && activos?.includes(m.modulo) && puedeAccion(m.modulo, 'crear'))
            setAltasRapidas((prev) => [
              ...prev,
              { modelo: m, creado, ...(semilla ? { semilla } : {}) },
            ]);
        },
      }}
    >
      <SytueShell
        currentApp="erp"
        user={user}
        hubBaseUrl={hubBaseUrl}
        accessToken={getAccessToken}
        locale={locale}
      >
        {/* `sytue-frame` y, envolviendo al `main`, `sytue-sheet`: el marco de color y la
            hoja de la forma «Marco», como en AppShell (tokens/appearance.css). El `main`
            desplaza por dentro de la hoja, que recorta las esquinas. En «Carril» las dos
            clases no hacen nada.

            El alto es el de la ventana MENOS la barra superior, como en AppShell. Era
            `h-full`, el alto entero del documento: la carcasa medía 56 px de más, el
            documento desplazaba esos 56 px y el borde inferior de la hoja quedaba fuera
            de la vista. */}
        <div class="erp-workspace sytue-frame sytue-safe-workspace flex h-[calc(100dvh-var(--spacing-topbar))] min-h-0">
          {cajon ? (
            // EMPIEZA BAJO LA TOPBAR, no en el borde de la pantalla: con `inset-0`
            // el velo taparía el buscador, el selector de apps y la cuenta de la
            // Sytue Shell, porque la capa de velo va por encima de la de topbar.
            <div
              class="fixed inset-x-0 bottom-0"
              style={{
                top: 'var(--spacing-topbar)',
                zIndex: 'var(--z-overlay)',
                background: 'var(--sytue-overlay)',
              }}
              onClick={() => setCajon(false)}
              aria-hidden="true"
            />
          ) : null}
          {/* `bg-rail`: es un carril, y esa clase le da los tokens del carril. */}
          <nav class="erp-rail bg-rail" aria-label={t('nav.menu')}>
            <button
              id="erp-cajon-boton-escritorio"
              type="button"
              class="erp-rail-action sytue-focus-ring"
              title={t('nav.menu')}
              aria-label={t('nav.menu')}
              aria-expanded={cajon}
              aria-controls="erp-cajon"
              onClick={() => {
                cajonBoton.current = 'erp-cajon-boton-escritorio';
                setCajon(true);
              }}
            >
              <Icon name="menu" size={20} />
            </button>
            <button
              type="button"
              class="erp-rail-action sytue-focus-ring"
              title={t('nav.home')}
              aria-label={t('nav.home')}
              aria-current={vista === 'modelo' && !modelo ? 'page' : undefined}
              onClick={() => {
                setVista('modelo');
                setModelo(null);
              }}
            >
              <Icon name="grid" size={20} />
            </button>
            <button
              type="button"
              class="erp-rail-action sytue-focus-ring"
              title={t('nav.tasks')}
              aria-label={t('nav.tasks')}
              aria-current={vista === 'tareas' ? 'page' : undefined}
              onClick={() => setVista('tareas')}
            >
              <Icon name="check" size={20} />
              {tareasHoy > 0 ? (
                <span class="erp-rail-count" aria-label={t('nav.tasksDue', { count: tareasHoy })}>
                  {tareasHoy}
                </span>
              ) : null}
            </button>
            {modulosVisibles.some((m) => m.clave === 'tpv') &&
            puedeLeer('facturacion') &&
            puedeLeer('productos') ? (
              <button
                type="button"
                class="erp-rail-action sytue-focus-ring"
                title={t('pos.title')}
                aria-label={t('pos.title')}
                aria-current={vista === 'tpv' ? 'page' : undefined}
                onClick={() => setVista('tpv')}
              >
                <Icon name="receipt" size={20} />
              </button>
            ) : null}
            {puedeLeer('contabilidad') ? (
              <button
                type="button"
                class="erp-rail-action sytue-focus-ring"
                title={t('nav.bank')}
                aria-label={t('nav.bank')}
                aria-current={vista === 'banco' ? 'page' : undefined}
                onClick={() => setVista('banco')}
              >
                <Icon name="building" size={20} />
              </button>
            ) : null}
            {puedeLeer('obligaciones') ? (
              <button
                type="button"
                class="erp-rail-action sytue-focus-ring"
                title={t('nav.obligations')}
                aria-label={t('nav.obligations')}
                aria-current={
                  vista === 'obligaciones' || vista === 'onboarding' ? 'page' : undefined
                }
                onClick={() => setVista('obligaciones')}
              >
                <Icon name="audit" size={20} />
              </button>
            ) : null}
            <div class="erp-rail-bottom">
              <button
                type="button"
                class="erp-rail-action sytue-focus-ring"
                title={t('palette.placeholder')}
                aria-label={t('palette.placeholder')}
                onClick={() => setPaleta(true)}
              >
                <Icon name="search" size={20} />
              </button>
              <button
                type="button"
                class="erp-rail-action sytue-focus-ring"
                title="Sytue AI"
                aria-label="Sytue AI"
                onClick={() => setPanelIA(true)}
              >
                <Icon name="ai" size={20} />
              </button>
            </div>
          </nav>
          <BarraLateral
            abierto={cajon}
            modulos={modulosVisibles}
            modeloActivo={vista === 'modelo' ? modelo : null}
            vista={vista}
            onElegir={(m) => {
              setCajon(false);
              abrirModelo(m);
            }}
            onCerrar={() => setCajon(false)}
            onInicio={() => {
              setCajon(false);
              setVista('modelo');
              setModelo(null);
            }}
            onVista={(v) => {
              setCajon(false);
              setVista(v);
            }}
            onPaleta={() => {
              setCajon(false);
              setPaleta(true);
            }}
            tableros={tableros}
            tableroActivo={vista === 'tablero' ? tablero : null}
            onIA={() => {
              setCajon(false);
              setPanelIA(true);
            }}
            tareas={tareasHoy}
            puedeLeer={puedeLeer}
            onTablero={(tb) => {
              setCajon(false);
              setTablero(tb);
              setVista('tablero');
            }}
            t={t}
          />

          {/* La hoja lleva el `main` y, a su lado, la FICHA del registro abierto: es
              contenido, no cromo, y fuera de la hoja quedaba a sangre sobre el marco. */}
          <div class="sytue-sheet flex min-h-0 min-w-0 flex-1">
            {/* `relative`: bloque contenedor de lo que desplaza dentro, como el `main` de
                AdminShell y AppShell. */}
            {/* Las esquinas de la hoja las redondea lo de dentro (ver AppShell): el `main`,
                todas; con la ficha abierta a su lado (≥768px), solo las de la izquierda. */}
            <main
              class={cx(
                'erp-main sytue-sheet-corners relative min-h-0 min-w-0 flex-1 overflow-auto',
                ficha &&
                  modelo &&
                  vista === 'modelo' &&
                  'md:[--sytue-sheet-br:0px] md:[--sytue-sheet-tr:0px]',
              )}
            >
              {moduloDelAviso && permisos?.administrador ? (
                <FranjaPeticion
                  endpoint={`/api/edit-requests?modulo=${encodeURIComponent(moduloDelAviso)}`}
                  soloLectura={false}
                  puedeConceder
                  mensaje=""
                  locale={locale}
                />
              ) : null}
              {/* El único mando de navegación que hay por debajo de 768 px. Va aquí y
              no dentro de cada pantalla para que exista también en el libro, en
              el balance y en la bienvenida. */}
              <button
                id="erp-cajon-boton"
                type="button"
                onClick={() => {
                  cajonBoton.current = 'erp-cajon-boton';
                  setCajon(true);
                }}
                aria-label={t('nav.menu')}
                aria-expanded={cajon}
                aria-controls="erp-cajon"
                class="sytue-focus-ring border-border text-text-muted hover:text-text rounded-control mt-4 ml-6 inline-grid min-h-[44px] min-w-[44px] place-items-center border text-lg md:hidden"
              >
                <span aria-hidden="true">☰</span>
              </button>
              {error ? (
                <p class="bg-danger-bg text-danger-text mx-6 mt-4 rounded-md px-4 py-3 text-sm">
                  {error}
                </p>
              ) : null}

              {!catalogo ? (
                <div class="flex h-full items-center justify-center">
                  <Spinner size={24} />
                </div>
              ) : vista === 'onboarding' || (vista === 'obligaciones' && !organizacion) ? (
                <Onboarding
                  locale={locale}
                  organizacion={organizacion ?? null}
                  modulos={catalogo.modulos}
                  activos={activos ?? []}
                  t={t}
                  onGuardado={setOrganizacion}
                  onVerCalendario={() => setVista('obligaciones')}
                  onActivarModulos={async (claves) => {
                    for (const clave of claves) {
                      const mod = catalogo.modulos.find((m) => m.clave === clave);
                      if (mod) await cambiarModulo(mod, true);
                    }
                  }}
                  onCancelar={() => {
                    setVista('modelo');
                    setModelo(null);
                  }}
                />
              ) : vista === 'obligaciones' && organizacion ? (
                <Obligaciones
                  locale={locale}
                  organizacion={organizacion}
                  t={t}
                  onEditarPerfil={() => setVista('onboarding')}
                  onBanco={() => setVista('banco')}
                  soloLectura={!puedeEscribir('obligaciones')}
                  puedeLeer={puedeLeer}
                  puedeAccion={puedeAccion}
                  onTareas={contarTareas}
                />
              ) : vista === 'tareas' ? (
                <Tareas locale={locale} t={t} onAbrir={abrirRegistro} onCambio={contarTareas} />
              ) : vista === 'tpv' ? (
                <PuntoVenta
                  key={`${user.tenant}:${user.id}`}
                  cuenta={`${user.tenant}:${user.id}`}
                  revision={revision}
                  puedeEditarProductos={puedeAccion('productos', 'escribir')}
                  locale={locale}
                  t={t}
                  vendedor={user.name || user.email}
                  soloLectura={
                    !puedeAccion('tpv', 'crear') ||
                    !['crear', 'asentar', 'pagar'].every((v) => puedeAccion('facturacion', v))
                  }
                  onFactura={(id) => abrirRegistro('factura_venta', id)}
                  onPresupuesto={(r) => {
                    const m = modelos.find((x) => x.clave === r.modelo);
                    if (m) {
                      setModelo(m);
                      setFicha(r);
                    }
                  }}
                  onProductos={() => {
                    const m = modelos.find((x) => x.clave === 'producto');
                    if (m) abrirModelo(m);
                  }}
                />
              ) : vista === 'banco' ? (
                <Banco
                  locale={locale}
                  t={t}
                  organizacion={organizacion ?? null}
                  requisitos={requisitos}
                  soloLectura={!puedeEscribir('contabilidad')}
                  onTareas={contarTareas}
                />
              ) : vista === 'libro' ? (
                <LibroDiario locale={locale} t={t} />
              ) : vista === 'balance' ? (
                <Balance locale={locale} t={t} />
              ) : vista === 'tablero' && tablero ? (
                <VistaTablero
                  tablero={tablero}
                  locale={locale}
                  t={t}
                  onBorrado={(id) => {
                    setTableros((ts) => ts.filter((x) => x.id !== id));
                    setTablero(null);
                    setVista('modelo');
                    setModelo(null);
                  }}
                />
              ) : modelo ? (
                <>
                  {/* En lectura: la franja con «Pedir acceso de edición» a la administración,
                  que se quita sola por el aviso del Hub al concederlo. */}
                  {permisos && permisos.modulos[modelo.modulo] === 'lectura' ? (
                    <FranjaPeticion
                      endpoint={`/api/edit-requests?modulo=${encodeURIComponent(modelo.modulo)}`}
                      soloLectura
                      puedeConceder={false}
                      pedida={(permisos.pedidos ?? []).includes(modelo.modulo)}
                      mensaje={translate(locale, 'readOnly.module', {
                        name:
                          catalogo?.modulos.find((m) => m.clave === modelo.modulo)?.etiqueta ??
                          modelo.modulo,
                      })}
                      locale={locale}
                      concedido="erp.access.granted"
                      recurso={modelo.modulo}
                      alConceder={recargarPermisos}
                    />
                  ) : null}
                  {modelo.clave === 'factura_venta' && organizacion?.pais === 'ES' && !papelera ? (
                    <TarjetaVerifactu
                      administrador={permisos?.administrador ?? false}
                      locale={locale}
                      revision={revision}
                      t={t}
                    />
                  ) : null}
                  {modelo.modulo === 'facturacion' &&
                  !papelera &&
                  (puedeCrear('factura_venta') || puedeCrear('factura_compra')) ? (
                    <div class="px-6 pt-4">
                      <Button variant="secondary" onClick={() => setAsistenciaFacturas(true)}>
                        {t('invoiceAssist.title')}
                      </Button>
                    </div>
                  ) : null}
                  <Lista
                    modelo={modelo}
                    pagina={pagina}
                    cargando={cargandoLista}
                    masCargando={masCargando}
                    busqueda={busqueda}
                    estado={estado}
                    orden={orden}
                    papelera={papelera}
                    sinPermiso={sinPermiso}
                    locale={locale}
                    onBuscar={setBusqueda}
                    onEstado={setEstado}
                    onOrden={setOrden}
                    onPapelera={setPapelera}
                    onAbrir={setFicha}
                    onDuplicar={(r) => {
                      if (modelo) {
                        api<Registro>(`${modelo.clave}/${r.id}/duplicar`, { method: 'POST' })
                          .then((nuevaCopia) => {
                            aviso.notify({ tone: 'success', title: t('toast.created') });
                            setFicha(nuevaCopia);
                          })
                          .catch(() => {
                            setSemilla(r);
                            setFicha('nueva');
                          });
                      } else {
                        setSemilla(r);
                        setFicha('nueva');
                      }
                    }}
                    onNueva={() => {
                      setSemilla(null);
                      setFicha('nueva');
                    }}
                    onMas={() => void cargarMas()}
                    soloLectura={!puedeEscribir(modelo.modulo)}
                    revision={revision}
                    onImportado={() => {
                      setRevision((r) => r + 1);
                      for (const clave of cache.current.keys()) {
                        if (clave.startsWith(`${modelo.clave}|`)) cache.current.delete(clave);
                      }
                      void cargarLista(modelo, busqueda, estado, orden, papelera);
                    }}
                    t={t}
                  />
                </>
              ) : (
                <Bienvenida
                  modulos={catalogo.modulos}
                  activos={activos ?? []}
                  recientes={recientes}
                  locale={locale}
                  onCambiar={cambiarModulo}
                  onAbrirModelo={abrirModelo}
                  onNuevaFicha={(m) => {
                    abrirModelo(m);
                    setSemilla(null);
                    setFicha('nueva');
                  }}
                  onVista={setVista}
                  onPaleta={() => setPaleta(true)}
                  userName={user?.name || user?.email || ''}
                  organizacion={organizacion ?? null}
                  onConfigurar={() => setVista('onboarding')}
                  tareas={tareasHoy}
                  verObligaciones={puedeLeer('obligaciones')}
                  t={t}
                />
              )}
            </main>

            {ficha && modelo && vista === 'modelo' ? (
              <Ficha
                // Cada registro, cada alta y cada duplicado es una ficha DISTINTA: su
                // formulario nace de su registro al montarse, y sin esta clave lo de una
                // ficha se quedaba en la siguiente (lib/ficha.ts).
                key={claveDeFicha(modelo.clave, ficha, semilla)}
                modelo={modelo}
                registro={ficha === 'nueva' ? null : ficha}
                semilla={ficha === 'nueva' ? semilla : null}
                etiquetas={pagina?.etiquetas ?? {}}
                locale={locale}
                enPapelera={papelera}
                facturae={organizacion?.pais === 'ES'}
                requisitos={requisitos}
                soloLectura={!puedeEscribir(modelo.modulo)}
                onTareas={contarTareas}
                onCerrar={() => setFicha(null)}
                onConvertido={(r) => abrirRegistro(r.modelo, r.id)}
                onDuplicar={(r) => {
                  setSemilla(r);
                  setFicha('nueva');
                }}
                onGuardado={() => {
                  setRevision((r) => r + 1);
                  setFicha(null);
                  // Lo escrito invalida lo cacheado de ESE modelo —todas sus
                  // búsquedas—: servir una lista vieja justo después de crear algo es
                  // la forma más rápida de que alguien crea que no se ha guardado.
                  // Sin copiar las claves: borrar durante la iteración de un `Map` es
                  // seguro —la entrada retirada simplemente no se visita—.
                  for (const clave of cache.current.keys()) {
                    if (clave.startsWith(`${modelo.clave}|`)) cache.current.delete(clave);
                  }
                  void cargarLista(modelo, busqueda, estado, orden, papelera);
                }}
                t={t}
              />
            ) : null}
          </div>

          {panelIA ? (
            <PanelIA
              modelo={vista === 'modelo' ? modelo : null}
              modelos={modelos}
              locale={locale}
              t={t}
              onCerrar={() => setPanelIA(false)}
              onCreado={(tareasCreadas) => {
                // Lo creado cambia listas de varios modelos: se olvida lo cacheado.
                cache.current.clear();
                setRevision((r) => r + 1);
                contarTareas();
                if (tareasCreadas.length > 0) {
                  setPanelIA(false);
                  setVista('tareas');
                } else if (modelo) {
                  void cargarLista(modelo, busqueda, estado, orden, papelera);
                }
              }}
              onGuardado={(tb) => {
                setTableros((ts) => [tb, ...ts]);
                setTablero(tb);
                setVista('tablero');
                setPanelIA(false);
              }}
            />
          ) : null}

          {paleta ? (
            <Paleta
              modelos={modelos}
              modulos={catalogo?.modulos ?? []}
              onElegir={abrirModelo}
              onCerrar={() => setPaleta(false)}
              t={t}
            />
          ) : null}
        </div>
        {asistenciaFacturas ? (
          <AsistenteFacturas
            t={t}
            locale={locale}
            modelos={modelos}
            puedeCrear={puedeCrear}
            puedeAccion={puedeAccion}
            onCerrar={() => setAsistenciaFacturas(false)}
            onAbrir={(m, r) => {
              setAsistenciaFacturas(false);
              abrirRegistro(m.clave, r.id);
            }}
            renderFormulario={({
              modelo: m,
              semilla: datos,
              etiquetas: rotulos,
              onCreado,
              onCancelar,
            }) => (
              <AltaRapida
                abierto={altasRapidas.length === 0}
                modelo={m}
                semilla={datos}
                etiquetas={rotulos}
                t={t}
                onCerrar={onCancelar}
                onCreado={(r) => {
                  onCreado(r);
                  cache.current.clear();
                  setRevision((n) => n + 1);
                  contarTareas();
                }}
              />
            )}
          />
        ) : null}
        {altasRapidas.map((alta, i) => (
          <AltaRapida
            key={`${i}-${alta.modelo.clave}`}
            abierto={i === altasRapidas.length - 1}
            modelo={alta.modelo}
            semilla={alta.semilla}
            t={t}
            onCerrar={() => setAltasRapidas((prev) => prev.slice(0, -1))}
            onCreado={(r) => {
              alta.creado(r);
              setAltasRapidas((prev) => prev.slice(0, -1));
              cache.current.clear();
              setRevision((n) => n + 1);
            }}
          />
        ))}
      </SytueShell>
    </AltasContext.Provider>
  );
}

// ---------------------------------------------------------------------------

type Traducir = ReturnType<typeof useT>;

/** Qué ocupa el panel principal. */
type Vista =
  | 'modelo'
  | 'libro'
  | 'balance'
  | 'obligaciones'
  | 'onboarding'
  | 'tablero'
  | 'banco'
  | 'tareas'
  | 'tpv';

/** Columna por la que se ordena y en qué sentido. */
interface Orden {
  campo: string;
  asc: boolean;
}

/** Una entrada de la barra lateral. Todas se pintan igual: activa o no. */
function Entrada({
  activa,
  onClick,
  children,
}: {
  activa: boolean;
  onClick: () => void;
  children: JSX.Element | string;
}): JSX.Element {
  return (
    <button
      type="button"
      onClick={onClick}
      aria-current={activa ? 'page' : undefined}
      // Tokens de navegación: superficie blanca, selección y marcador azules.
      class={
        activa
          ? 'bg-sidebar-active text-sidebar-active-fg before:bg-indicator sytue-focus-ring-chrome rounded-nav relative w-full px-3 py-1.5 text-left text-base [font-weight:var(--sytue-nav-active-weight)] before:absolute before:top-1.5 before:bottom-1.5 before:left-0 before:w-[min(2px,var(--sytue-nav-indicator))] before:rounded-full before:content-[""] focus-visible:outline-none'
          : 'text-sidebar-fg hover:bg-sidebar-hover sytue-focus-ring-chrome rounded-nav w-full px-3 py-1.5 text-left text-base transition-colors focus-visible:outline-none'
      }
    >
      {children}
    </button>
  );
}

function BarraLateral({
  abierto,
  onCerrar,
  modulos,
  modeloActivo,
  vista,
  onElegir,
  onInicio,
  onVista,
  onPaleta,
  tableros,
  tableroActivo,
  onIA,
  tareas,
  puedeLeer,
  onTablero,
  t,
}: {
  abierto: boolean;
  onCerrar: () => void;
  modulos: Modulo[];
  modeloActivo: Modelo | null;
  vista: Vista;
  onElegir: (m: Modelo) => void;
  onInicio: () => void;
  onVista: (v: Vista) => void;
  onPaleta: () => void;
  tableros: Tablero[];
  tableroActivo: Tablero | null;
  onIA: () => void;
  /** Tareas propias para hoy o vencidas. */
  tareas: number;
  puedeLeer: (modulo: string) => boolean;
  onTablero: (t: Tablero) => void;
  t: Traducir;
}): JSX.Element {
  const atajoPaleta = useAtajo('Ctrl K');
  return (
    <nav
      id="erp-cajon"
      tabIndex={-1}
      class={cx(
        // El ancho del menú de la suite (`--spacing-sidebar`, 260 px), como `Sidebar`. Era
        // `w-60`, que con la raíz a 14 son 210 px: el ERP era la única app con el menú
        // 50 px más estrecho (medido en producción, 2026-09-21).
        'sytue-nav-drawer border-border-subtle bg-sidebar text-sidebar-fg w-[var(--spacing-sidebar)] shrink-0 overflow-auto border-r px-3 py-4 focus-visible:outline-none',
        // En <768px es un panel flotante bajo la topbar; en ≥768px vuelve al
        // flujo, como siempre.
        'fixed top-[var(--spacing-topbar)] bottom-0 left-0',
        'z-[var(--z-modal)]',
        abierto ? 'sytue-drawer-in' : 'hidden',
      )}
    >
      <Button
        variant="ghost"
        iconOnly
        class="mb-3 ml-auto flex"
        aria-label={t('action.close')}
        onClick={onCerrar}
      >
        <Icon name="close" size={20} />
      </Button>
      <button
        type="button"
        onClick={onPaleta}
        aria-label={`${t('palette.placeholder')} — ${t('palette.hint', { keys: atajoPaleta || 'Ctrl+K' })}`}
        class="border-chrome-border bg-chrome-hover text-sidebar-fg-muted hover:bg-chrome-active hover:text-sidebar-fg sytue-focus-ring-chrome rounded-control mb-4 flex w-full items-center justify-between border px-3 py-2 text-sm transition-colors focus-visible:outline-none"
      >
        <span aria-hidden="true">{t('palette.placeholder')}</span>
        {/* El `<kbd>` es decoración: se lee en el `aria-label` de arriba, donde
            además se nombra la tecla sin depender de que se entienda el símbolo. */}
        <kbd aria-hidden="true" class="text-sidebar-fg-muted text-xs">
          {atajoPaleta}
        </kbd>
      </button>

      {/* Sytue AI arriba y en TODAS las pantallas: se entiende sobre lo que haya
          abierto, así que el mismo botón sirve en cada módulo. */}
      {/* Perfilado sobre la barra de tinta y no el botón secundario blanco: un bloque
          blanco macizo en mitad del menú pesaría más que la acción principal de la
          página. */}
      <button
        type="button"
        onClick={onIA}
        class="border-chrome-border bg-chrome-hover text-sidebar-fg hover:bg-chrome-active sytue-focus-ring-chrome rounded-control mb-4 inline-flex h-8 w-full items-center justify-center gap-1.5 border px-3 text-sm font-semibold transition-colors focus-visible:outline-none pointer-coarse:min-h-[max(44px,2.75rem)]"
      >
        <Icon name="ai" size={16} />
        Sytue AI
      </button>

      <ul class="mb-5">
        <li>
          <Entrada activa={vista === 'modelo' && modeloActivo === null} onClick={onInicio}>
            {t('nav.home')}
          </Entrada>
        </li>
        {/* El banco y las obligaciones, lo PRIMERO: son las dos tareas de cada
            semana de quien lleva un negocio sin saber de contabilidad. El libro y
            el balance van también arriba y fuera de los módulos: no pertenecen a
            ninguno —son la contabilidad de todo lo demás— y meterlos dentro de
            «Contabilidad» los escondería justo para quien tiene ese módulo
            apagado y aun así factura. */}
        <li>
          <Entrada activa={vista === 'tareas'} onClick={() => onVista('tareas')}>
            <span class="flex items-center justify-between gap-2">
              {t('nav.tasks')}
              {tareas > 0 ? (
                <Badge tone="warning" aria-label={t('nav.tasksDue', { count: tareas })}>
                  {tareas}
                </Badge>
              ) : null}
            </span>
          </Entrada>
        </li>
        {modulos.some((m) => m.clave === 'tpv') &&
        puedeLeer('facturacion') &&
        puedeLeer('productos') ? (
          <li>
            <Entrada activa={vista === 'tpv'} onClick={() => onVista('tpv')}>
              {t('pos.title')}
            </Entrada>
          </li>
        ) : null}
        {puedeLeer('contabilidad') ? (
          <li>
            <Entrada activa={vista === 'banco'} onClick={() => onVista('banco')}>
              {t('nav.bank')}
            </Entrada>
          </li>
        ) : null}
        {puedeLeer('obligaciones') ? (
          <li>
            <Entrada
              activa={vista === 'obligaciones' || vista === 'onboarding'}
              onClick={() => onVista('obligaciones')}
            >
              {t('nav.obligations')}
            </Entrada>
          </li>
        ) : null}
        {puedeLeer('contabilidad') ? (
          <>
            <li>
              <Entrada activa={vista === 'libro'} onClick={() => onVista('libro')}>
                {t('nav.ledger')}
              </Entrada>
            </li>
            <li>
              <Entrada activa={vista === 'balance'} onClick={() => onVista('balance')}>
                {t('nav.balance')}
              </Entrada>
            </li>
          </>
        ) : null}
      </ul>

      {tableros.length > 0 ? (
        <>
          <p class="sytue-overline text-sidebar-fg-muted px-3 py-1.5">{t('nav.dashboards')}</p>
          <ul class="mb-5">
            {tableros.map((tb) => (
              <li key={tb.id}>
                <Entrada activa={tableroActivo?.id === tb.id} onClick={() => onTablero(tb)}>
                  {tb.titulo}
                </Entrada>
              </li>
            ))}
          </ul>
        </>
      ) : null}

      {/* Cada módulo se PLIEGA: con quince áreas encendidas la barra eran
          cuarenta filas y lo que se usaba quedaba bajo el pliegue. `<details>`
          nativo —teclado, lector de pantalla y estado sin código—, abierto de
          inicio solo el módulo de lo que se está viendo. */}
      {modulos.map((modulo) => (
        <details
          key={modulo.clave}
          class="group/modulo mb-1"
          open={modulo.modelos.some((m) => m.clave === modeloActivo?.clave)}
        >
          <summary class="sytue-overline text-sidebar-fg-muted hover:text-sidebar-fg sytue-focus-ring-chrome flex cursor-pointer list-none items-center justify-between rounded-sm px-3 py-1.5 select-none focus-visible:outline-none [&::-webkit-details-marker]:hidden">
            {modulo.etiqueta}
            <span
              aria-hidden="true"
              class="text-sidebar-fg-muted transition-transform duration-150 group-open/modulo:rotate-90"
            >
              ›
            </span>
          </summary>
          <ul class="mb-2">
            {modulo.modelos.map((m) => (
              <li key={m.clave}>
                <Entrada activa={modeloActivo?.clave === m.clave} onClick={() => onElegir(m)}>
                  {m.etiqueta}
                </Entrada>
              </li>
            ))}
          </ul>
        </details>
      ))}
    </nav>
  );
}

/** Un botón del filtro por estado; el activo lleva relleno. */
function FiltroEstado({
  activo,
  onClick,
  children,
}: {
  activo: boolean;
  onClick: () => void;
  children: JSX.Element | string;
}): JSX.Element {
  return (
    <button
      type="button"
      onClick={onClick}
      aria-pressed={activo}
      // `min-h-[24px]`: con `py-1` y texto de 10,5 las pastillas medían 21 de alto en
      // producción (2026-09-21), y van en fila con `gap-1`.
      class={
        activo
          ? 'sytue-focus-ring bg-selected text-text rounded-control min-h-[24px] px-3 py-1 text-xs font-semibold'
          : 'sytue-focus-ring text-text-muted hover:bg-bg-subtle hover:text-text rounded-control min-h-[24px] px-3 py-1 text-xs font-medium transition-colors'
      }
    >
      {children}
    </button>
  );
}

function Lista({
  modelo,
  pagina,
  cargando,
  masCargando,
  busqueda,
  estado,
  orden,
  papelera,
  sinPermiso = false,
  locale,
  onBuscar,
  onEstado,
  onOrden,
  onPapelera,
  onAbrir,
  onDuplicar,
  onNueva,
  onMas,
  onImportado,
  soloLectura,
  revision,
  t,
}: {
  modelo: Modelo;
  pagina: Pagina | null;
  cargando: boolean;
  masCargando: boolean;
  busqueda: string;
  estado: string;
  orden: Orden | null;
  papelera: boolean;
  /** La lista contestó 403 `sin_permiso`: ni está vacía ni se puede crear nada. */
  sinPermiso?: boolean;
  locale: string;
  onBuscar: (q: string) => void;
  onEstado: (e: string) => void;
  onOrden: (o: Orden | null) => void;
  onPapelera: (p: boolean) => void;
  onAbrir: (r: Registro) => void;
  onDuplicar?: (r: Registro) => void;
  onNueva: () => void;
  /** Sin permiso de escritura en su módulo: ni «Nuevo» ni importar. */
  soloLectura: boolean;
  onMas: () => void;
  /** Tras una importación: lo cacheado de este modelo ya no vale. */
  onImportado: () => void;
  /** Sube con cada escritura: es lo que hace que el recuento no se quede viejo. */
  revision: number;
  t: Traducir;
}): JSX.Element {
  const porClave = useMemo(() => new Map(modelo.campos.map((c) => [c.clave, c])), [modelo]);
  const registros = useMemo(() => pagina?.registros ?? [], [pagina]);
  const { confirmar: confirmarLote, dialogo: dialogoLote } = useConfirm();
  const [vistaModo, setVistaModo] = useState<'tabla' | 'tarjetas'>('tabla');
  const [stats, setStats] = useState<EstadisticasModelo | null>(null);

  useEffect(() => {
    let montado = true;
    api<EstadisticasModelo>(`${modelo.clave}/estadisticas`)
      .then((data) => {
        if (montado) setStats(data);
      })
      .catch(() => {
        // Fallback silencioso si no hay conexión
      });
    return () => {
      montado = false;
    };
  }, [modelo.clave, revision]);

  const colDinero = useMemo(
    () => modelo.campos.find((c) => c.tipo === 'dinero' && modelo.lista.includes(c.clave)),
    [modelo],
  );
  const totalDinero = useMemo(() => {
    if (!colDinero || registros.length === 0) return null;
    return registros.reduce((acc, r) => {
      const val = r.data[colDinero.clave];
      return acc + (typeof val === 'number' ? val : 0);
    }, 0);
  }, [colDinero, registros]);

  const [seleccionados, setSeleccionados] = useState<Set<string>>(new Set());
  // La barra de lote solo existe con selección marcada: su hook depende de ello.
  const { ref: barraLoteRef, queda: barraLoteQueda } = useScrollRestante<HTMLDivElement>(
    seleccionados.size > 0,
  );
  const [ejecutandoLote, setEjecutandoLote] = useState(false);
  const aviso = useToast();

  // Importar un CSV: el mismo fichero que sale por «Exportar» vuelve a entrar,
  // y con él el de cualquier otro ERP. En las facturas recibidas entran también
  // sus facturas electrónicas, varias de una vez. El resultado se cuenta en un
  // aviso y los fallos, fila a fila, en otro: quien importa mil filas quiere
  // saber cuáles no entraron, no que «algo falló».
  const selectorCSV = useRef<HTMLInputElement>(null);
  const [importando, setImportando] = useState(false);
  async function importarFicheros(ficheros: File[]): Promise<void> {
    setImportando(true);
    const total = { creados: 0, actualizados: 0, ignoradas: new Set<string>() };
    const fallos: string[] = [];
    for (const fichero of ficheros) {
      const xml = /\.xml$/i.test(fichero.name) || fichero.type.includes('xml');
      try {
        const res = await api<ResultadoImportacion>(`${modelo.clave}/importar`, {
          method: 'POST',
          headers: { 'content-type': xml ? 'application/xml' : 'text/csv' },
          body: await fichero.text(),
        });
        total.creados += res.creados;
        total.actualizados += res.actualizados;
        for (const c of res.ignoradas) total.ignoradas.add(c);
        for (const f of res.fallos ?? []) {
          const fila = t('list.importFailedRow', { row: f.fila, error: f.error });
          fallos.push(ficheros.length > 1 ? `${fichero.name} · ${fila}` : fila);
        }
      } catch (err) {
        const motivo = mensajeDeError(err, t);
        fallos.push(ficheros.length > 1 ? `${fichero.name}: ${motivo}` : motivo);
      }
    }
    aviso.notify({
      tone: fallos.length > 0 ? 'info' : 'success',
      title: t('list.importResult', {
        created: total.creados,
        updated: total.actualizados,
        failed: fallos.length,
      }),
      ...(total.ignoradas.size > 0
        ? { description: t('list.importIgnored', { columns: [...total.ignoradas].join(', ') }) }
        : {}),
    });
    for (const f of fallos.slice(0, 5)) aviso.notify({ tone: 'danger', title: f });
    if (total.creados + total.actualizados > 0) onImportado();
    setImportando(false);
  }

  const todosSeleccionados = registros.length > 0 && seleccionados.size === registros.length;

  function toggleTodos(): void {
    if (seleccionados.size === registros.length) {
      setSeleccionados(new Set());
    } else {
      setSeleccionados(new Set(registros.map((r) => r.id)));
    }
  }

  function toggleFila(id: string, e: MouseEvent): void {
    e.stopPropagation();
    setSeleccionados((prev) => {
      const next = new Set(prev);
      if (next.has(id)) next.delete(id);
      else next.add(id);
      return next;
    });
  }

  async function ejecutarLote(
    accion: 'estado' | 'borrar' | 'restaurar',
    nuevoEstado?: string,
  ): Promise<void> {
    if (seleccionados.size === 0) return;
    setEjecutandoLote(true);
    try {
      await api<{ afectados: number }>(`${modelo.clave}/lote`, {
        method: 'POST',
        body: JSON.stringify({
          accion,
          ids: Array.from(seleccionados),
          estado: nuevoEstado,
        }),
      });
      aviso.notify({
        tone: 'success',
        title: accion === 'borrar' ? t('toast.deleted') : t('toast.saved'),
      });
      setSeleccionados(new Set());
      onEstado(estado);
    } catch (err) {
      aviso.notify({
        tone: 'danger',
        title: mensajeDeError(err, t),
      });
    } finally {
      setEjecutandoLote(false);
    }
  }

  return (
    <div class="mx-auto max-w-5xl space-y-4 px-6 py-6">
      {/* UNA FILA QUE SALTA, y no `flex-col` + `sm:flex-row`. El punto de corte mira la
          VENTANA, pero con la ficha abierta a la lista le queda una fracción: a 880 px
          eran 278, la cabecera seguía en fila, el título se partía palabra a palabra
          encima del buscador y la barra acababa debajo del panel. Así cabe con
          cualquier ancho: si título y barra no caben juntos, la barra baja. Medido en
          producción a 1440, 1280, 1024 y 880, con y sin ficha: nada tapado ni partido. */}
      <header class="border-border flex flex-wrap items-center justify-between gap-3 border-b pb-3">
        <div class="mr-auto min-w-0">
          <div class="flex flex-wrap items-center gap-2">
            <h1 class="text-text sytue-h4">{modelo.etiqueta}</h1>
            <Badge tone="neutral">
              {stats
                ? t('list.recordsLoaded', { count: stats.total })
                : t('list.recordsLoaded', { count: registros.length })}
            </Badge>
            {stats?.suma_dinero !== undefined && stats.suma_dinero > 0 ? (
              <Badge tone="info">
                {t('list.summaryTotal', {
                  amount: formatearDinero(stats.suma_dinero, locale),
                })}
              </Badge>
            ) : totalDinero !== null && colDinero ? (
              <Badge tone="info">
                {t('list.summaryTotal', { amount: formatearDinero(totalDinero, locale) })}
              </Badge>
            ) : null}
          </div>
        </div>

        {/* `flex-wrap`: lo que no cabe salta de fila; los botones ya no parten su rótulo. */}
        <div class="flex flex-wrap items-center gap-2">
          <input
            type="search"
            value={busqueda}
            onInput={(e) => onBuscar((e.target as HTMLInputElement).value)}
            placeholder={t('list.searchPlaceholder')}
            aria-label={t('list.searchPlaceholder')}
            class="sytue-focus-ring border-border bg-surface text-text placeholder-text-subtle w-44 rounded-md border px-3 py-1.5 text-sm"
          />

          <div
            role="group"
            aria-label={t('list.viewOptions')}
            class="border-border bg-surface-raised hidden rounded-md border p-0.5 sm:inline-flex"
          >
            <button
              type="button"
              onClick={() => setVistaModo('tabla')}
              aria-pressed={vistaModo === 'tabla'}
              title={t('list.viewTable')}
              class={cx(
                'sytue-focus-ring inline-flex min-h-[24px] items-center gap-1 rounded px-2.5 py-1 text-xs font-medium whitespace-nowrap transition-colors',
                vistaModo === 'tabla'
                  ? 'bg-selected text-text font-semibold shadow-xs'
                  : 'text-text-muted hover:text-text',
              )}
            >
              <Icon name="table" size={13} />
              <span>{t('list.viewTable')}</span>
            </button>
            <button
              type="button"
              onClick={() => setVistaModo('tarjetas')}
              aria-pressed={vistaModo === 'tarjetas'}
              title={t('list.viewCards')}
              class={cx(
                'sytue-focus-ring inline-flex min-h-[24px] items-center gap-1 rounded px-2.5 py-1 text-xs font-medium whitespace-nowrap transition-colors',
                vistaModo === 'tarjetas'
                  ? 'bg-selected text-text font-semibold shadow-xs'
                  : 'text-text-muted hover:text-text',
              )}
            >
              <Icon name="grid" size={13} />
              <span>{t('list.viewCards')}</span>
            </button>
          </div>

          {/* En móvil solo el icono: «Exportar CSV» partía en dos líneas y
              empujaba «Nuevo» fuera de la fila. El rótulo sigue en `title`. */}
          <Button
            variant="secondary"
            size="sm"
            title={t('list.exportCsv')}
            href={withBase(
              baseDeLaApp(),
              `/api/${modelo.clave}/export${consulta({ q: busqueda, estado })}`,
            )}
          >
            <Icon name="download" size={14} class="sm:mr-1" />
            <span class="hidden sm:inline">{t('list.exportCsv')}</span>
          </Button>
          {soloLectura ? null : (
            <>
              <input
                ref={selectorCSV}
                type="file"
                accept={
                  modelo.recibe ? '.csv,text/csv,.xml,application/xml,text/xml' : '.csv,text/csv'
                }
                multiple={modelo.recibe}
                class="hidden"
                onChange={(e) => {
                  const ficheros = [...(e.currentTarget.files ?? [])];
                  e.currentTarget.value = '';
                  if (ficheros.length > 0) void importarFicheros(ficheros);
                }}
              />
              <Button
                variant="secondary"
                size="sm"
                title={modelo.recibe ? t('list.importInvoicesHint') : t('list.importHint')}
                loading={importando}
                onClick={() => selectorCSV.current?.click()}
              >
                <Icon name="upload" size={14} class="sm:mr-1" />
                <span class="hidden sm:inline">
                  {modelo.recibe ? t('list.importInvoices') : t('list.import')}
                </span>
              </Button>
              <Button variant="primary" size="sm" onClick={onNueva}>
                <Icon name="plus" size={14} class="mr-1" />
                {t('action.new')}
              </Button>
            </>
          )}
        </div>
      </header>

      {/* Barra de Acciones en Lote (Estilo Odoo). En móvil, UNA fila que se
          desplaza de lado (ver scrollX.ts): envolver robaba dos filas al listado. */}
      {seleccionados.size > 0 ? (
        <div
          data-scroll-x
          ref={barraLoteRef}
          class={cx(
            'bg-surface-raised border-border rounded-card flex flex-wrap items-center justify-between gap-3 border px-4 py-2.5 shadow-sm',
            FILA_UNICA_EN_MOVIL,
            barraLoteQueda && MASCARA_SCROLL_X,
          )}
        >
          <div class="flex items-center gap-2">
            <Badge tone="info">{t('list.selectedCount', { n: String(seleccionados.size) })}</Badge>
            <Button variant="ghost" size="xs" onClick={() => setSeleccionados(new Set())}>
              {t('list.deselect')}
            </Button>
          </div>

          <div class="flex flex-wrap items-center gap-2">
            {modelo.estados && modelo.estados.length > 0 ? (
              <div class="flex items-center gap-1">
                <span class="text-text-subtle text-xs">{t('list.batchState')}:</span>
                <select
                  class="sytue-focus-ring border-border bg-surface text-text rounded px-2 py-1 text-xs"
                  onChange={(e) => {
                    const target = (e.target as HTMLSelectElement).value;
                    if (target) {
                      void ejecutarLote('estado', target);
                      (e.target as HTMLSelectElement).value = '';
                    }
                  }}
                  disabled={ejecutandoLote}
                >
                  <option value="">{t('relation.choose')}</option>
                  {modelo.estados.map((st) => (
                    <option key={st.valor} value={st.valor}>
                      {st.etiqueta}
                    </option>
                  ))}
                </select>
              </div>
            ) : null}

            <Button
              variant="secondary"
              size="xs"
              href={withBase(
                baseDeLaApp(),
                `/api/${modelo.clave}/export?ids=${Array.from(seleccionados).join(',')}`,
              )}
            >
              <Icon name="download" size={12} class="mr-1" />
              {t('list.exportCsv')}
            </Button>

            <Button
              variant="ghost"
              size="xs"
              // El diálogo de la suite, de peligro, y no `window.confirm`: el nativo salía
              // con el título del navegador, sin tono y sin foco atrapado (Fyne ya lo hacía).
              onClick={() => {
                void confirmarLote({
                  titulo: t('list.batchDelete'),
                  mensaje: t('list.batchDeleteConfirm', { n: String(seleccionados.size) }),
                  confirmLabel: t('list.batchDelete'),
                  tone: 'danger',
                }).then((si) => {
                  if (si) void ejecutarLote('borrar');
                });
              }}
              disabled={ejecutandoLote}
              class="text-danger hover:bg-danger-bg"
            >
              {t('list.batchDelete')}
            </Button>
            {dialogoLote}
          </div>
        </div>
      ) : null}

      <div class="mb-4 flex flex-wrap items-center justify-between gap-2">
        <div
          role="group"
          aria-label={t('list.filterByStatus')}
          class="flex flex-wrap items-center gap-1 text-xs"
        >
          {modelo.estados && modelo.estados.length > 0 ? (
            <>
              <span class="text-text-subtle mr-1">{t('list.filterByStatus')}:</span>
              <FiltroEstado activo={estado === ''} onClick={() => onEstado('')}>
                {stats ? `${t('list.all')} (${stats.total})` : t('list.all')}
              </FiltroEstado>
              {modelo.estados.map((e) => {
                const cuenta = stats?.por_estado[e.valor];
                const texto = cuenta !== undefined ? `${e.etiqueta} (${cuenta})` : e.etiqueta;
                return (
                  <FiltroEstado
                    key={e.valor}
                    activo={estado === e.valor}
                    onClick={() => onEstado(e.valor)}
                  >
                    {texto}
                  </FiltroEstado>
                );
              })}
            </>
          ) : null}
          <span class={modelo.estados?.length ? 'ml-2' : ''}>
            <FiltroEstado activo={papelera} onClick={() => onPapelera(!papelera)}>
              {t('list.trash')}
            </FiltroEstado>
          </span>
        </div>

        {estado !== '' || busqueda !== '' || papelera ? (
          <Button
            size="xs"
            variant="ghost"
            onClick={() => {
              onEstado('');
              onBuscar('');
              onPapelera(false);
            }}
            class="text-xs"
          >
            {t('action.resetView')}
          </Button>
        ) : null}
      </div>

      {cargando && registros.length === 0 ? (
        <div class="py-16 text-center">
          <Spinner size={22} />
        </div>
      ) : sinPermiso ? (
        <EmptyState
          title={t('error.sinPermiso.titulo')}
          description={t('error.sinPermiso')}
          illustration={<Icon name="lock" size={32} class="text-text-subtle" />}
        />
      ) : registros.length === 0 ? (
        <EmptyState
          title={t('list.empty')}
          {...(!estado && !busqueda
            ? { description: t('list.emptyHint', { action: t('action.new') }) }
            : {})}
          illustration={<Icon name="building" size={32} class="text-text-subtle" />}
          action={
            estado || busqueda ? (
              <Button
                variant="secondary"
                size="sm"
                onClick={() => {
                  onEstado('');
                  onBuscar('');
                  onPapelera(false);
                }}
              >
                {t('action.resetView')}
              </Button>
            ) : soloLectura ? undefined : (
              <Button variant="primary" size="sm" onClick={onNueva}>
                {t('action.new')}
              </Button>
            )
          }
        />
      ) : vistaModo === 'tarjetas' ? (
        <div class="grid gap-3 sm:grid-cols-2 lg:grid-cols-3">
          {registros.map((r) => (
            <Card
              key={r.id}
              class="hover:border-border-strong flex cursor-pointer flex-col justify-between p-4 transition-colors"
              onClick={() => onAbrir(r)}
            >
              <div>
                <div class="mb-2 flex items-start justify-between gap-2">
                  <div class="flex items-center gap-2">
                    <Checkbox
                      checked={seleccionados.has(r.id)}
                      onClick={(e) => e.stopPropagation()}
                      onChange={(e) => toggleFila(r.id, e as unknown as MouseEvent)}
                      aria-label={nombreDe(r)}
                      class="border-border text-primary cursor-pointer rounded"
                    />
                    <span class="text-text-muted font-mono text-xs">{r.ref || '—'}</span>
                  </div>
                  {r.estado ? (
                    <Badge tone={tonoDeEstado(r.estado)}>
                      {modelo.estados?.find((e) => e.valor === r.estado)?.etiqueta ?? r.estado}
                    </Badge>
                  ) : null}
                </div>
                <h3 class="text-text mb-2 text-sm font-semibold">{nombreDe(r)}</h3>
                <div class="text-text-muted mb-4 space-y-1 text-xs">
                  {modelo.lista
                    .filter((col) => col !== 'ref' && col !== 'estado')
                    .slice(0, 3)
                    .map((col) => {
                      const campo = porClave.get(col);
                      if (!campo) return null;
                      const v = celda(campo, r.data[col], locale, pagina?.etiquetas ?? {});
                      if (v === '—') return null;
                      return (
                        <div key={col} class="flex items-center justify-between gap-2">
                          <span class="text-text-subtle">{campo.etiqueta}:</span>
                          <span
                            class={
                              campo.tipo === 'dinero'
                                ? 'text-text font-medium tabular-nums'
                                : 'text-text'
                            }
                          >
                            {v}
                          </span>
                        </div>
                      );
                    })}
                </div>
              </div>
              <div class="border-border mt-auto flex items-center justify-between gap-2 border-t pt-3">
                {onDuplicar ? (
                  <Button
                    size="xs"
                    variant="ghost"
                    type="button"
                    onClick={(e) => {
                      e.stopPropagation();
                      onDuplicar(r);
                    }}
                    title={t('action.duplicateDraft')}
                  >
                    {t('action.duplicate')}
                  </Button>
                ) : (
                  <span />
                )}
                <Button
                  size="xs"
                  variant="secondary"
                  type="button"
                  onClick={(e) => {
                    e.stopPropagation();
                    onAbrir(r);
                  }}
                >
                  {t('action.open')}
                </Button>
              </div>
            </Card>
          ))}
        </div>
      ) : (
        <>
          <div class="border-border bg-surface rounded-card overflow-x-auto border shadow-xs max-md:hidden">
            <table class="w-full border-collapse text-sm">
              <thead>
                {/* La cabecera del Table de @sytue/ui: blanca, con los rótulos en tinta
                    y negrita. */}
                <tr class="border-border bg-surface border-b">
                  <th scope="col" class="w-10 px-3 py-2.5 text-center">
                    <Checkbox
                      checked={todosSeleccionados}
                      onChange={toggleTodos}
                      aria-label={t('list.all')}
                      class="border-border text-primary cursor-pointer rounded"
                    />
                  </th>
                  {modelo.lista.map((col) => {
                    const campo = porClave.get(col);
                    const rotulo =
                      col === 'ref' ? '#' : col === 'estado' ? '' : (campo?.etiqueta ?? col);
                    const activa = orden?.campo === col;
                    return (
                      <th
                        key={col}
                        scope="col"
                        aria-sort={activa ? (orden.asc ? 'ascending' : 'descending') : 'none'}
                        class={cx(
                          'sytue-overline text-text px-3 py-2.5 font-bold',
                          campo && alineadoDerecha(campo.tipo) ? 'text-right' : 'text-left',
                        )}
                      >
                        <button
                          type="button"
                          onClick={() =>
                            onOrden(
                              !activa
                                ? { campo: col, asc: false }
                                : orden.asc
                                  ? null
                                  : { campo: col, asc: true },
                            )
                          }
                          class="sytue-focus-ring hover:text-text -mx-1 inline-flex items-center rounded px-1 tracking-wider"
                        >
                          {rotulo || t('list.sortBy')}
                          <span
                            aria-hidden="true"
                            class="text-indicator ml-1 inline-block w-2 font-mono"
                          >
                            {activa ? (orden.asc ? '↑' : '↓') : ''}
                          </span>
                        </button>
                      </th>
                    );
                  })}
                  {onDuplicar ? <th scope="col" class="w-16 px-2 py-2.5" /> : null}
                </tr>
              </thead>
              <tbody class="divide-border-subtle divide-y">
                {registros.map((r) => {
                  const seleccionado = seleccionados.has(r.id);
                  return (
                    <tr
                      key={r.id}
                      tabIndex={0}
                      onClick={() => onAbrir(r)}
                      onKeyDown={(e) => {
                        if (e.key === 'Enter' || e.key === ' ') {
                          e.preventDefault();
                          onAbrir(r);
                        }
                      }}
                      class={cx(
                        'sytue-focus-ring-inset hover:bg-bg-subtle cursor-pointer transition-colors',
                        seleccionado && 'bg-surface-raised',
                      )}
                    >
                      <td class="w-10 px-3 py-2.5 text-center">
                        <Checkbox
                          checked={seleccionado}
                          onClick={(e) => e.stopPropagation()}
                          onChange={(e) => toggleFila(r.id, e as unknown as MouseEvent)}
                          aria-label={nombreDe(r)}
                          class="border-border text-primary cursor-pointer rounded"
                        />
                      </td>
                      {modelo.lista.map((col) => {
                        if (col === 'ref') {
                          return (
                            <td key={col} class="text-text-muted px-3 py-2.5 font-mono text-xs">
                              {r.ref || '—'}
                            </td>
                          );
                        }
                        if (col === 'estado') {
                          const etiqueta =
                            modelo.estados?.find((e) => e.valor === r.estado)?.etiqueta ?? r.estado;
                          return (
                            <td key={col} class="px-3 py-2.5">
                              {etiqueta ? (
                                <Badge tone={tonoDeEstado(r.estado)}>{etiqueta}</Badge>
                              ) : null}
                            </td>
                          );
                        }
                        const campo = porClave.get(col);
                        return (
                          <td
                            key={col}
                            class={
                              campo && alineadoDerecha(campo.tipo)
                                ? 'text-text px-3 py-2.5 text-right font-mono tabular-nums'
                                : 'text-text px-3 py-2.5'
                            }
                          >
                            {campo
                              ? celda(campo, r.data[col], locale, pagina?.etiquetas ?? {})
                              : '—'}
                          </td>
                        );
                      })}
                      {onDuplicar ? (
                        <td class="px-2 py-2.5 text-right">
                          <Button
                            size="xs"
                            variant="ghost"
                            type="button"
                            onClick={(e) => {
                              e.stopPropagation();
                              onDuplicar(r);
                            }}
                            title={t('action.duplicateDraft')}
                          >
                            {t('action.duplicate')}
                          </Button>
                        </td>
                      ) : null}
                    </tr>
                  );
                })}
              </tbody>
            </table>
          </div>

          {/* EN MÓVIL, TARJETAS RESUMEN */}
          <ul class="divide-border-subtle divide-y md:hidden">
            {registros.map((r) => (
              <li key={r.id}>
                <button
                  type="button"
                  onClick={() => onAbrir(r)}
                  class="sytue-focus-ring hover:bg-bg-subtle w-full px-2 py-3 text-left transition-colors"
                >
                  <span class="flex items-baseline gap-2">
                    <span class="text-text mr-auto truncate text-sm font-semibold">
                      {nombreDe(r)}
                    </span>
                    {r.estado ? (
                      <Badge tone={tonoDeEstado(r.estado)}>
                        {modelo.estados?.find((e) => e.valor === r.estado)?.etiqueta ?? r.estado}
                      </Badge>
                    ) : null}
                  </span>
                  <span class="text-text-muted mt-1 flex flex-wrap gap-x-3 text-xs">
                    {modelo.lista
                      .filter((col) => col !== 'estado' && col !== 'ref')
                      .map((col) => porClave.get(col))
                      .filter((campo): campo is Campo => campo !== undefined)
                      .map((campo) => {
                        const v = celda(
                          campo,
                          r.data[campo.clave],
                          locale,
                          pagina?.etiquetas ?? {},
                        );
                        return v === '—' ? null : (
                          <span key={campo.clave}>
                            <span class="text-text-subtle">{campo.etiqueta}: </span>
                            <span class="text-text font-medium">{v}</span>
                          </span>
                        );
                      })}
                  </span>
                </button>
              </li>
            ))}
          </ul>
        </>
      )}

      {registros.length > 0 ? (
        <footer class="mt-4 flex items-center gap-3">
          <span class="text-text-subtle text-xs">
            {t('list.showing', { n: String(registros.length) })}
          </span>
          {pagina?.siguiente ? (
            <Button variant="secondary" size="sm" onClick={onMas} loading={masCargando}>
              {t('action.more')}
            </Button>
          ) : null}
        </footer>
      ) : null}
    </div>
  );
}

function tonoDeEstado(estado?: string): 'success' | 'info' | 'danger' | 'neutral' {
  switch (estado) {
    case 'pagado':
    case 'hecho':
    case 'ganada':
    case 'aceptado':
    case 'entregado':
    case 'aprobado':
    case 'contratado':
      return 'success';
    case 'confirmado':
    case 'en_curso':
    case 'activa':
    case 'publicado':
      return 'info';
    case 'cancelado':
    case 'rechazado':
    case 'perdida':
    case 'anulado':
      return 'danger';
    default:
      return 'neutral';
  }
}

// ---------------------------------------------------------------------------

/**
 * La ficha: un formulario generado de los campos que declara el modelo.
 *
 * Va en un panel lateral y no en un diálogo modal a pantalla completa: la lista
 * sigue visible detrás, así que se ve en qué se está trabajando y cerrar no
 * desorienta. Un solo botón principal.
 */
function Ficha({
  modelo,
  registro,
  semilla,
  etiquetas,
  locale,
  onCerrar,
  onDuplicar,
  onConvertido,
  onGuardado,
  enPapelera,
  facturae,
  requisitos,
  soloLectura,
  onTareas,
  t,
}: {
  modelo: Modelo;
  registro: Registro | null;
  /** De dónde copiar los valores al CREAR. Nulo al crear en blanco. */
  semilla: Registro | null;
  etiquetas: Record<string, string>;
  locale: Locale;
  onCerrar: () => void;
  onDuplicar: (r: Registro) => void;
  onConvertido: (r: Registro) => void;
  /** La ficha está abierta desde la papelera: lo que procede es restaurar. */
  enPapelera: boolean;
  /** La organización es española: la factura emitida sale también como Facturae. */
  facturae: boolean;
  /** Lo que tiene que llevar una factura válida, para los documentos con factura. */
  requisitos: RequisitosFactura | null;
  soloLectura: boolean;
  /** Algo cambió en las tareas (el contador de la barra lateral). */
  onTareas: () => void;
  onGuardado: () => void;
  t: Traducir;
}): JSX.Element {
  const altas = useContext(AltasContext);
  useMobileBack(true, onCerrar, { nivel: 2 });
  const [valores, setValores] = useState<Record<string, string>>(() =>
    // `registro ?? semilla`: al duplicar se copian los VALORES pero no la
    // identidad. El documento nuevo no hereda ni el id ni la referencia —esa la
    // asigna la secuencia al guardar—, así que sale como lo que es: otro
    // documento, no una copia del mismo.
    valoresIniciales(modelo, registro ?? semilla),
  );
  // El nombre legible de cada relación. Arranca con lo que resolvió la lista y
  // se completa al elegir en el selector: sin esto, acabar de elegir un cliente
  // dejaba el control enseñando un identificador hasta recargar.
  const [rotulos, setRotulos] = useState<Record<string, string>>(etiquetas);
  const [guardando, setGuardando] = useState(false);
  // La barra de estado/flujo, para el aviso de que queda contenido a la derecha.
  const { ref: barraEstadoRef, queda: barraEstadoQueda } = useScrollRestante<HTMLDivElement>();
  const [rellenando, setRellenando] = useState(false);
  const [aviso, setAviso] = useState('');
  const [fallo, setFallo] = useState('');
  // Los asientos que generó ESTE documento. Solo para los modelos que llevan
  // libro, y solo si el documento ya existe: una factura sin guardar no ha
  // generado nada todavía.
  const [asientos, setAsientos] = useState<Asiento[]>([]);
  // Sube con cada documento subido: sus tareas se vuelven a pedir (una subida puede
  // haber cerrado la de «sube la factura»).
  const [docs, setDocs] = useState(0);
  // Qué campo falló, para pintarlo DEBAJO de él. El 422 del servicio los trae
  // uno a uno; antes se aplastaban en una frase única arriba del formulario y
  // quien rellenaba diez campos tenía que releerlos todos para dar con los dos
  // malos.
  const [errores, setErrores] = useState<Record<string, string>>({});
  const [estadoActual, setEstadoActual] = useState<string>(
    registro?.estado || modelo.estados?.[0]?.valor || '',
  );

  async function convertir(): Promise<void> {
    if (!registro || guardando) return;
    setGuardando(true);
    setFallo('');
    try {
      const r = await api<Registro>(`${modelo.clave}/${registro.id}/convertir`, { method: 'POST' });
      onConvertido(r);
    } catch (err) {
      setFallo(mensajeDeError(err, t));
    } finally {
      setGuardando(false);
    }
  }

  async function cambiarEstadoFicha(nuevoEstado: string): Promise<void> {
    if (!registro) return;
    setGuardando(true);
    setFallo('');
    try {
      const actualizado = await api<Registro>(`${modelo.clave}/${registro.id}/estado`, {
        method: 'POST',
        body: JSON.stringify({ estado: nuevoEstado }),
      });
      // Sin escribir sobre `registro`, que es del padre: `onGuardado` cierra la ficha
      // y vuelve a pedir la lista, que ya trae el estado nuevo.
      setEstadoActual(actualizado.estado || nuevoEstado);
      const nombreEstado =
        modelo.estados?.find((e) => e.valor === nuevoEstado)?.etiqueta ?? nuevoEstado;
      aviso2.notify({ tone: 'success', title: t('form.stateUpdated', { status: nombreEstado }) });
      onGuardado();
    } catch (err) {
      setFallo(mensajeDeError(err, t));
    } finally {
      setGuardando(false);
    }
  }
  // El diálogo de la suite y no `window.prompt`: pide el motivo con el mismo
  // aspecto, el mismo foco y la misma accesibilidad que el resto.
  const { preguntar, dialogo } = usePrompt();
  const { confirmar, dialogo: dialogoConfirmar } = useConfirm();
  // Guardar cerraba el panel sin decir nada: quedabas mirando la lista sin saber
  // si había entrado. El aviso es la única confirmación de que pasó algo.
  const aviso2 = useToast();

  const cargarAsientos = useCallback(() => {
    if (!modelo.contable || !registro) return;
    api<{ asientos: Asiento[] | null }>(`${modelo.clave}/${registro.id}/asientos`)
      .then((r) => setAsientos(r.asientos ?? []))
      // Un fallo aquí no puede romper la ficha: el documento se sigue pudiendo
      // leer y editar aunque su contabilidad no se haya podido consultar.
      .catch(() => setAsientos([]));
  }, [modelo, registro]);

  useEffect(() => {
    cargarAsientos();
  }, [cargarAsientos]);
  const panel = useRef<HTMLFormElement | null>(null);

  // El foco entra en el panel al abrirlo: sin esto, el teclado se queda en la
  // lista de detrás y Tab recorre lo que no se está mirando. Se busca el primer
  // control en vez de enhebrar un ref por props hasta el campo: es el mismo
  // efecto con un tercio del cableado.
  useEffect(() => {
    panel.current?.querySelector<HTMLElement>('input, select, textarea')?.focus();
  }, []);

  function fijar(clave: string, valor: string): void {
    setValores((v) => ({ ...v, [clave]: valor }));
  }

  async function guardar(ev: Event): Promise<void> {
    ev.preventDefault();

    // LAS RELACIONES OBLIGATORIAS LAS COMPRUEBA ESTE CÓDIGO, no el navegador.
    // El control de una relación es un botón con un `input[type=hidden]` detrás,
    // y los campos ocultos están EXENTOS de la validación de formularios por
    // especificación: el `required` no hace nada ahí. Sin esto, dejar el cliente
    // sin elegir enviaba la petición y el 422 volvía del servidor —correcto,
    // pero después de un viaje y sin señalar el control—.
    const faltan: Record<string, string> = {};
    for (const campo of modelo.campos) {
      if (campo.tipo === 'relacion' && campo.requerido && !(valores[campo.clave] ?? '').trim()) {
        faltan[campo.clave] = t('form.required');
      }
    }
    if (Object.keys(faltan).length > 0) {
      setErrores(faltan);
      setFallo('');
      // Al primero que falta: en una ficha larga el mensaje puede quedar fuera
      // de la pantalla y parecería que el botón no hace nada.
      const primero = Object.keys(faltan)[0];
      if (primero) {
        document.getElementById(`campo-${primero}`)?.scrollIntoView({ block: 'center' });
      }
      return;
    }

    setGuardando(true);
    setFallo('');
    setErrores({});
    try {
      const data = aCuerpo(modelo, valores);
      const ruta = registro ? `${modelo.clave}/${registro.id}` : modelo.clave;
      await api(ruta, {
        method: registro ? 'PUT' : 'POST',
        body: JSON.stringify({ data }),
      });
      aviso2.notify({ tone: 'success', title: registro ? t('toast.saved') : t('toast.created') });
      onGuardado();
    } catch (err) {
      if (err instanceof ErrorApi && Object.keys(err.porCampo).length > 0) {
        // Con los campos señalados, el mensaje de arriba sobra: repetirlo sería
        // decir dos veces lo mismo y esconder los que sí importan.
        setErrores(err.porCampo);
      } else {
        setFallo(mensajeDeError(err, t));
      }
    } finally {
      setGuardando(false);
    }
  }

  async function asentar(): Promise<void> {
    if (!registro) return;
    setGuardando(true);
    setFallo('');
    try {
      const a = await api<{ ref: string }>(`${modelo.clave}/${registro.id}/asentar`, {
        method: 'POST',
        body: JSON.stringify({}),
      });
      setAviso(t('form.posted', { ref: a.ref }));
      // Y se enseña, no solo se anuncia: el asiento aparece debajo en el acto.
      cargarAsientos();
    } catch (err) {
      setFallo(mensajeDeError(err, t));
    } finally {
      setGuardando(false);
    }
  }

  /**
   * Anular un asiento. Es la ÚNICA forma de corregir contabilidad, y hasta ahora
   * no existía en la interfaz: se podía asentar una factura mal y no había manera
   * de deshacerlo sin entrar en la base de datos.
   *
   * No borra nada —la base lo impide— y exige un motivo, que es lo que convierte
   * «alguien tocó esto» en «alguien lo corrigió por esto». Solo lo puede hacer
   * quien administra la organización; si no, el servicio contesta 403 y se
   * enseña tal cual en vez de fingir que salió bien.
   */
  async function anular(asiento: Asiento): Promise<void> {
    // El título dice QUÉ asiento: «Anular» a secas no lo decía.
    const motivo = await preguntar({
      titulo: t('ledger.voidTitle', { ref: asiento.ref }),
      mensaje: t('ledger.voidReason'),
      confirmLabel: t('ledger.void'),
    });
    if (!motivo) return;
    setGuardando(true);
    setFallo('');
    try {
      await api(`libro/${asiento.id}/anular`, {
        method: 'POST',
        body: JSON.stringify({ motivo }),
      });
      cargarAsientos();
      setAviso(t('ledger.voided', { ref: asiento.ref }));
    } catch (err) {
      setFallo(mensajeDeError(err, t));
    } finally {
      setGuardando(false);
    }
  }

  async function restaurar(): Promise<void> {
    if (!registro) return;
    setGuardando(true);
    try {
      await api(`${modelo.clave}/${registro.id}/restaurar`, { method: 'POST', body: '{}' });
      aviso2.notify({ tone: 'success', title: t('toast.restored') });
      onGuardado();
    } catch (err) {
      setFallo(mensajeDeError(err, t));
      setGuardando(false);
    }
  }

  async function borrar(): Promise<void> {
    if (!registro) return;
    // PREGUNTAR ANTES. Borrar estaba a un clic y sin vuelta atrás visible: el
    // botón vive al lado de «Duplicar», y confundirlos se paga con un documento
    // que desaparece de todas las listas. Va a la papelera y no se destruye, pero
    // desde la interfaz no hay forma de sacarlo de ahí, así que para quien lo
    // pulsa es definitivo — y se le pregunta como tal.
    const seguro = await confirmar({
      titulo: t('action.delete'),
      mensaje: t('confirm.delete', { nombre: nombreDe(registro) }),
      confirmLabel: t('action.delete'),
      tone: 'danger',
    });
    if (!seguro) return;
    setGuardando(true);
    try {
      await api(`${modelo.clave}/${registro.id}`, { method: 'DELETE' });
      aviso2.notify({ tone: 'success', title: t('toast.deleted') });
      onGuardado();
    } catch (err) {
      setFallo(mensajeDeError(err, t));
      setGuardando(false);
    }
  }

  return (
    <aside
      class={cx(
        // Las esquinas de la DERECHA de la hoja (en <768px va fija encima y no toca ninguna).
        'sytue-side-panel sytue-sheet-corners [--sytue-sheet-bl:0px] [--sytue-sheet-tl:0px] max-md:[--sytue-sheet-tr:0px]',
        'border-border bg-surface w-full max-w-md shrink-0 overflow-auto border-l',
        // EN MÓVIL, ENCIMA Y A PANTALLA COMPLETA. Al lado de la lista, en 375 px
        // los dos quedaban aplastados: una tabla ilegible junto a un formulario
        // de campos de dos centímetros. Arranca bajo la topbar, igual que el
        // cajón de navegación, para no tapar el cromo de la Sytue Shell.
        'max-md:fixed max-md:inset-x-0 max-md:top-[var(--spacing-topbar)] max-md:bottom-0 max-md:w-auto',
        'max-md:z-[var(--z-modal)] max-md:max-w-none max-md:bg-[var(--sytue-menu-surface)]',
      )}
      aria-label={modelo.etiqueta}
    >
      <form
        ref={panel}
        onSubmit={(e) => {
          void guardar(e);
        }}
        class="flex min-h-full flex-col px-6 py-5"
      >
        <header class="mb-4 flex flex-wrap items-center gap-2">
          <div class="min-w-0 basis-full">
            <h2 class="text-text sytue-h4 truncate">
              {registro ? nombreDe(registro) : `${t('action.new')} · ${modelo.etiqueta}`}
            </h2>
            {registro?.ref ? (
              <p class="text-text-subtle font-mono text-xs">{registro.ref}</p>
            ) : null}
          </div>
          {registro && modelo.cobro ? (
            <Button
              variant="ghost"
              size="sm"
              title={t('form.downloadUbl')}
              aria-label={t('form.downloadUbl')}
              href={withBase(baseDeLaApp(), `/api/${modelo.clave}/${registro.id}/ubl`)}
            >
              <Icon name="download" size={14} class="mr-1" />
              UBL
            </Button>
          ) : null}
          {registro && modelo.clave === 'factura_venta' ? (
            <a
              class="sytue-focus-ring text-text hover:bg-bg-subtle rounded-control inline-flex items-center px-2 py-1.5"
              title={t('form.print')}
              aria-label={t('form.print')}
              href={withBase(baseDeLaApp(), `/api/${modelo.clave}/${registro.id}/imprimir`)}
              target="_blank"
              rel="noopener"
            >
              <Icon name="receipt" size={14} />
            </a>
          ) : null}
          {registro && modelo.cobro && facturae ? (
            <Button
              variant="ghost"
              size="sm"
              title={t('form.downloadFacturae')}
              aria-label={t('form.downloadFacturae')}
              href={withBase(baseDeLaApp(), `/api/${modelo.clave}/${registro.id}/facturae`)}
            >
              <Icon name="download" size={14} class="mr-1" />
              Facturae
            </Button>
          ) : null}
          {registro &&
          !soloLectura &&
          !enPapelera &&
          altas?.puedeCrear(modelo.clave === 'presupuesto' ? 'pedido_venta' : 'factura_venta') &&
          ((modelo.clave === 'presupuesto' && estadoActual === 'aceptado') ||
            (modelo.clave === 'pedido_venta' &&
              ['confirmado', 'entregado'].includes(estadoActual))) ? (
            <Button
              variant="secondary"
              size="sm"
              disabled={guardando}
              onClick={() => void convertir()}
            >
              {t(modelo.clave === 'presupuesto' ? 'sales.createOrder' : 'sales.createInvoice')}
            </Button>
          ) : null}
          {registro && modelo.cobro && facturae ? (
            <p class="text-text-muted order-last basis-full text-xs">
              {t('sales.facturaeHint')}{' '}
              <a
                class="underline"
                href="https://proveedores.face.gob.es/inicio"
                target="_blank"
                rel="noopener"
              >
                FACe
              </a>
            </p>
          ) : null}
          {enPapelera ? null : (
            <Button
              variant="ghost"
              size="sm"
              type="button"
              title={t('ai.fill')}
              aria-label={t('ai.fill')}
              onClick={() => setRellenando(true)}
            >
              <Icon name="ai" size={14} />
            </Button>
          )}
          <Button variant="ghost" size="sm" onClick={onCerrar} type="button">
            {t('action.close')}
          </Button>
        </header>

        {/* Barra de Estado y Flujo (Estilo Odoo). En móvil, UNA fila que se
            desplaza de lado (ver scrollX.ts): las transiciones y el pipeline
            envueltos se comían la ficha. */}
        {registro && modelo.estados && modelo.estados.length > 0 ? (
          <div
            data-scroll-x
            ref={barraEstadoRef}
            class={cx(
              'border-border bg-surface-sunken -mx-6 mb-4 flex flex-wrap items-center justify-between gap-3 border-y px-6 py-2.5',
              FILA_UNICA_EN_MOVIL,
              barraEstadoQueda && MASCARA_SCROLL_X,
            )}
          >
            {/* Acciones de transición rápida */}
            <div class="flex flex-wrap items-center gap-1.5">
              {(() => {
                const idxActual = modelo.estados.findIndex((e) => e.valor === estadoActual);
                const siguiente =
                  idxActual >= 0 && idxActual + 1 < modelo.estados.length
                    ? modelo.estados[idxActual + 1]
                    : null;
                const esCancelado = estadoActual === 'cancelado' || estadoActual === 'rechazado';
                const canceladoOp = modelo.estados.find((e) => e.valor === 'cancelado');

                return (
                  <>
                    {siguiente && siguiente.valor !== 'cancelado' ? (
                      <Button
                        variant="primary"
                        size="xs"
                        type="button"
                        loading={guardando}
                        onClick={() => void cambiarEstadoFicha(siguiente.valor)}
                      >
                        {siguiente.etiqueta}
                      </Button>
                    ) : null}

                    {canceladoOp && !esCancelado ? (
                      <Button
                        variant="ghost"
                        size="xs"
                        type="button"
                        loading={guardando}
                        onClick={() => void cambiarEstadoFicha('cancelado')}
                        class="text-text-muted hover:text-danger"
                      >
                        {canceladoOp.etiqueta}
                      </Button>
                    ) : null}
                  </>
                );
              })()}
            </div>

            {/* Stepper de etapas (Pipeline) */}
            <div class="flex flex-wrap items-center gap-1 text-xs">
              <span class="text-text-subtle mr-1 text-xs tracking-wider uppercase">
                {t('form.pipeline')}:
              </span>
              {modelo.estados.map((st) => {
                const activo = estadoActual === st.valor;
                return (
                  <button
                    key={st.valor}
                    type="button"
                    disabled={guardando}
                    onClick={() => void cambiarEstadoFicha(st.valor)}
                    class={cx(
                      'rounded-control cursor-pointer px-2.5 py-1 text-xs font-medium transition-colors',
                      // El texto de un relleno `bg-primary` es `--sytue-on-primary`, como en
                      // `Button`: `text-primary-text` es el azul para PAPEL, y sobre la tinta
                      // medía 1,87:1 (2,46 en oscuro). El estado activo no se leía.
                      activo
                        ? 'bg-primary font-semibold text-[var(--sytue-on-primary)] shadow-xs'
                        : 'text-text-muted bg-surface hover:text-text border-border border',
                    )}
                  >
                    {st.etiqueta}
                  </button>
                );
              })}
            </div>
          </div>
        ) : null}

        {fallo ? (
          <p class="bg-danger-bg text-danger-text mb-3 rounded-md px-3 py-2 text-sm">{fallo}</p>
        ) : null}
        {aviso ? (
          <p class="bg-success-bg text-success-text mb-3 rounded-md px-3 py-2 text-sm">{aviso}</p>
        ) : null}

        <div class="flex-1 space-y-3">
          {modelo.campos
            .filter((c) => !['tpv_huella', 'tpv_operacion'].includes(c.clave))
            .map((campo) => (
              <CampoDeFormulario
                key={campo.clave}
                campo={campo}
                valor={valores[campo.clave] ?? ''}
                etiquetas={rotulos}
                error={errores[campo.clave]}
                onCambio={(v, rotulo) => {
                  fijar(campo.clave, v);
                  if (rotulo !== undefined) setRotulos((r) => ({ ...r, [v]: rotulo }));
                }}
                t={t}
              />
            ))}
        </div>

        {asientos.length > 0 ? (
          <section class="mt-6">
            <h3 class="text-text-subtle mb-2 text-xs font-semibold tracking-wide uppercase">
              {t('nav.ledger')}
            </h3>
            <div class="space-y-2">
              {asientos.map((a) => (
                <div key={a.id}>
                  <AsientoCompacto asiento={a} locale={locale} t={t} />
                  {a.estado === 'asentado' ? (
                    <div class="mt-1 flex justify-end">
                      <Button
                        variant="ghost"
                        size="xs"
                        type="button"
                        onClick={() => void anular(a)}
                        disabled={guardando}
                      >
                        {t('ledger.void')}
                      </Button>
                    </div>
                  ) : null}
                </div>
              ))}
            </div>
          </section>
        ) : null}

        {/* Lo que cuelga del registro: qué falta por hacer y dónde está el papel.
            La factura de un gasto se sube aquí y cierra su tarea. */}
        {registro ? (
          <div class="border-border mt-6 space-y-5 border-t pt-4">
            <TareasRegistro
              modelo={modelo.clave}
              registro={registro.id}
              soloLectura={soloLectura || enPapelera}
              locale={locale}
              revision={docs}
              t={t}
            />
            <DocumentosRegistro
              modelo={modelo.clave}
              registro={registro.id}
              soloLectura={soloLectura || enPapelera}
              locale={locale}
              t={t}
              onCambio={() => {
                setDocs((n) => n + 1);
                onTareas();
              }}
            />
            {modelo.contable && !modelo.cobro ? (
              <RequisitosDeFactura requisitos={requisitos} t={t} />
            ) : null}
            {modelo.clave === 'factura_venta' && facturae ? (
              <EstadoVerifactuFactura
                factura={registro.id}
                estadoFactura={estadoActual}
                soloLectura={soloLectura || enPapelera}
                t={t}
              />
            ) : null}
            <HistorialRegistro
              modelo={modelo}
              registro={registro.id}
              soloLectura={soloLectura || enPapelera}
              locale={locale}
              etiquetas={rotulos}
              revision={`${estadoActual}:${docs}`}
              t={t}
            />
          </div>
        ) : null}

        {/* Panel de Trazabilidad y Auditoría (Estilo Odoo) */}
        {registro ? (
          <section class="border-border mt-6 border-t pt-4 text-xs">
            <div class="mb-2 flex items-center justify-between">
              <h3 class="text-text-muted text-xs font-medium tracking-wider uppercase">
                {t('form.auditTitle')}
              </h3>
              <button
                type="button"
                class="text-primary cursor-pointer text-xs hover:underline"
                onClick={() => {
                  void navigator.clipboard.writeText(registro.id);
                  aviso2.notify({ tone: 'info', title: t('form.copiedId') });
                }}
              >
                {t('form.copyId')}
              </button>
            </div>
            <div class="text-text-muted grid grid-cols-2 gap-3 text-xs">
              <div>
                <span class="text-text-subtle block">{t('form.createdAt')}:</span>
                <span class="text-text font-mono">
                  {formatearFecha(registro.creado_en, locale, true)}
                </span>
              </div>
              <div>
                <span class="text-text-subtle block">{t('form.editedAt')}:</span>
                <span class="text-text font-mono">
                  {formatearFecha(registro.editado_en, locale, true)}
                </span>
              </div>
            </div>
          </section>
        ) : null}

        <footer class="mt-6 flex items-center gap-2">
          <Button variant="primary" type="submit" loading={guardando}>
            {t('action.save')}
          </Button>
          {modelo.contable && registro ? (
            <Button
              variant="secondary"
              type="button"
              onClick={() => void asentar()}
              disabled={guardando}
            >
              {t('action.post')}
            </Button>
          ) : null}
          {registro ? (
            <>
              <Button
                variant="ghost"
                type="button"
                class="ml-auto"
                onClick={() => onDuplicar(registro)}
                disabled={guardando}
              >
                {t('action.duplicateDraft')}
              </Button>
              <Button
                variant="ghost"
                type="button"
                onClick={() => void (enPapelera ? restaurar() : borrar())}
                disabled={guardando}
              >
                {enPapelera ? t('action.restore') : t('action.delete')}
              </Button>
            </>
          ) : null}
        </footer>
      </form>
      {dialogo}
      {dialogoConfirmar}
      {rellenando ? (
        <RellenarIA
          modelo={modelo}
          t={t}
          onCerrar={() => setRellenando(false)}
          onRellenado={(data, nuevas) => {
            // Solo los campos que Sytue AI ha leído: lo que ya estaba escrito y
            // el texto no dice se queda como estaba.
            const leidos = valoresIniciales(modelo, {
              id: '',
              modelo: modelo.clave,
              etiqueta: '',
              data,
              creado_en: '',
              editado_en: '',
            });
            setValores((v) => {
              const out = { ...v };
              for (const clave of Object.keys(data)) out[clave] = leidos[clave] ?? '';
              return out;
            });
            setRotulos((r) => ({ ...r, ...nuevas }));
            setRellenando(false);
            aviso2.notify({ tone: 'success', title: t('ai.filled') });
          }}
        />
      ) : null}
    </aside>
  );
}

/**
 * Un campo. El control sale del TIPO declarado, no de código por módulo, y en
 * cuanto se puede es el nativo del navegador: `<input type="date">` trae el
 * calendario, el teclado correcto en el móvil y la accesibilidad hecha. Ninguna
 * biblioteca de selectores hace eso mejor en cinco idiomas.
 */
function CampoDeFormulario({
  prefijo = 'campo',
  campo,
  valor,
  etiquetas,
  error,
  onCambio,
  t,
}: {
  prefijo?: string;
  campo: Campo;
  valor: string;
  etiquetas: Record<string, string>;
  /** Lo que el servicio (o la comprobación previa) dice de ESTE campo. */
  error?: string | undefined;
  /** `rotulo` solo llega desde el selector de relaciones. */
  onCambio: (v: string, rotulo?: string) => void;
  t: Traducir;
}): JSX.Element {
  const id = `${prefijo}-${campo.clave}`;
  const comun =
    'sytue-focus-ring border-border bg-bg text-text w-full rounded-md border px-3 py-2 text-sm';

  const control = (): JSX.Element => {
    switch (campo.tipo) {
      case 'texto_largo':
        return (
          <textarea
            id={id}
            rows={3}
            value={valor}
            required={campo.requerido}
            onInput={(e) => onCambio((e.target as HTMLTextAreaElement).value)}
            class={comun}
          />
        );
      case 'seleccion':
        return (
          <select
            id={id}
            value={valor}
            required={campo.requerido}
            onChange={(e) => onCambio((e.target as HTMLSelectElement).value)}
            class={comun}
          >
            <option value="" />
            {campo.opciones?.map((o) => (
              <option key={o.valor} value={o.valor}>
                {o.etiqueta}
              </option>
            ))}
          </select>
        );
      case 'booleano':
        return (
          <Checkbox
            id={id}
            checked={valor === 'true'}
            onChange={(e) => onCambio(String((e.target as HTMLInputElement).checked))}
            class="size-4 rounded"
          />
        );
      case 'relacion':
        // Una relación se ELIGE, no se teclea: el valor es un identificador y
        // nadie lo acierta escribiendo. El selector busca contra el modelo
        // apuntado, así que no hay que traerse el catálogo de clientes entero
        // para poder elegir uno.
        return (
          <SelectorRelacion
            id={id}
            modelo={campo.modelo ?? ''}
            valor={valor}
            etiqueta={etiquetas[valor] ?? ''}
            requerido={campo.requerido ?? false}
            onCambio={onCambio}
            t={t}
          />
        );
      default:
        return (
          <input
            id={id}
            type={tipoDeInput(campo.tipo)}
            inputMode={campo.tipo === 'dinero' ? 'decimal' : undefined}
            step={campo.tipo === 'dinero' ? '0.01' : undefined}
            value={valor}
            required={campo.requerido}
            onInput={(e) => onCambio((e.target as HTMLInputElement).value)}
            class={comun}
          />
        );
    }
  };

  return (
    <div>
      <label for={id} class="text-text-muted mb-1 block text-xs font-medium">
        {campo.etiqueta}
        {campo.requerido ? (
          // `title` y `aria-label` sobre el asterisco: sin ellos un lector de
          // pantalla anuncia «Cliente» a secas —o «Cliente asterisco»— y quien
          // no ve el color no sabe que el campo es obligatorio hasta que el
          // formulario se niega a enviarse.
          <span
            class="text-danger-text ml-0.5"
            title={t('form.required')}
            aria-label={t('form.required')}
          >
            *
          </span>
        ) : null}
      </label>
      {control()}
      {error ? (
        // `role="alert"` para que un lector de pantalla lo anuncie al aparecer:
        // el color rojo no lo oye nadie.
        <p id={`${id}-error`} role="alert" class="text-danger-text mt-1 text-xs">
          {error}
        </p>
      ) : null}
    </div>
  );
}

function tipoDeInput(tipo: Campo['tipo']): string {
  switch (tipo) {
    case 'fecha':
      return 'date';
    case 'fechahora':
      return 'datetime-local';
    case 'correo':
      return 'email';
    case 'telefono':
      return 'tel';
    case 'url':
      return 'url';
    case 'entero':
    case 'decimal':
    case 'dinero':
      return 'number';
    default:
      return 'text';
  }
}

/** Del registro a los textos que edita el formulario. */
function valoresIniciales(modelo: Modelo, registro: Registro | null): Record<string, string> {
  const out: Record<string, string> = {};
  for (const campo of modelo.campos) {
    const v = registro?.data[campo.clave];
    if (v === undefined || v === null) {
      out[campo.clave] = '';
      continue;
    }
    if (campo.tipo === 'dinero' && typeof v === 'number') {
      out[campo.clave] = deCentimos(v);
    } else if (campo.tipo === 'fechahora' && typeof v === 'string') {
      // `datetime-local` no admite la zona: se recorta a minutos locales.
      out[campo.clave] = v.slice(0, 16);
    } else {
      out[campo.clave] = String(v);
    }
  }
  return out;
}

/**
 * De los textos del formulario al cuerpo que espera el servicio.
 *
 * Los importes salen en CÉNTIMOS enteros y los decimales como cadena: es el
 * mismo contrato que aplica el servicio, y respetarlo aquí evita que el
 * navegador sea el sitio donde se pierde el céntimo.
 */
function aCuerpo(modelo: Modelo, valores: Record<string, string>): Record<string, unknown> {
  const data: Record<string, unknown> = {};
  for (const campo of modelo.campos) {
    if (['tpv_operacion', 'tpv_huella'].includes(campo.clave)) continue;
    const bruto = (valores[campo.clave] ?? '').trim();
    if (bruto === '') continue;
    switch (campo.tipo) {
      case 'dinero': {
        const c = aCentimos(bruto);
        if (c !== null) data[campo.clave] = c;
        break;
      }
      case 'entero':
        data[campo.clave] = Number.parseInt(bruto, 10);
        break;
      case 'decimal':
        data[campo.clave] = bruto.replace(',', '.');
        break;
      case 'booleano':
        data[campo.clave] = bruto === 'true';
        break;
      case 'fechahora':
        // El servicio quiere RFC3339; `datetime-local` da hora local sin zona.
        data[campo.clave] = new Date(bruto).toISOString();
        break;
      default:
        data[campo.clave] = bruto;
    }
  }
  return data;
}

// ---------------------------------------------------------------------------

/**
 * La paleta (⌘K). Sustituye a los menús anidados: no hay que saber en qué
 * submenú vive «albaranes», basta con escribirlo.
 *
 * Busca sobre los rótulos YA TRADUCIDOS que vinieron del servicio, así que
 * funciona igual en los cinco idiomas sin diccionario propio.
 */
function Paleta({
  modelos,
  modulos,
  onElegir,
  onCerrar,
  t,
}: {
  modelos: Modelo[];
  /** Para la pista: el nombre del área, no su clave interna («consolidacion»). */
  modulos: Modulo[];
  onElegir: (m: Modelo) => void;
  onCerrar: () => void;
  t: Traducir;
}): JSX.Element {
  const [q, setQ] = useState('');
  const coincidencias = useMemo(() => {
    // Sin tildes: quien busca «fabricacion» espera encontrar «Fabricación».
    return modelos.filter((m) => contiene(m.etiqueta, q)).slice(0, 12);
  }, [modelos, q]);

  // La capa, el teclado y el foco son los de la paleta común (la misma de Sytue Cloud):
  // el velo, la superficie y el radio de un diálogo, y en el teléfono la hoja.
  return (
    <PaletaDeComandos
      open
      onClose={onCerrar}
      etiqueta={t('palette.placeholder')}
      rotuloCampo={t('palette.placeholder')}
      placeholder={t('palette.placeholder')}
      rotuloResultados={t('palette.placeholder')}
      sinResultados={() => t('relation.empty')}
      consulta={q}
      onConsulta={setQ}
      opciones={coincidencias.map((m) => ({
        id: m.clave,
        rotulo: m.etiqueta,
        pista: modulos.find((x) => x.clave === m.modulo)?.etiqueta ?? m.modulo,
        modelo: m,
      }))}
      onElegir={(o) => onElegir(o.modelo)}
    />
  );
}

/**
 * La primera pantalla: elegir qué áreas se encienden.
 *
 * ESTA ES LA DECISIÓN DE PRODUCTO que más separa a SyERP de la competencia. El
 * catálogo trae veinticuatro módulos, y enseñarlos todos el primer día es
 * exactamente por lo que un ERP se percibe como inmanejable: nadie usa los
 * veinticuatro, pero todos los ve todos los días. Aquí se encienden tres, y la
 * barra lateral tiene tres entradas.
 */
/**
 * Las áreas agrupadas por lo que son para el negocio. Treinta y tres tarjetas
 * en orden alfabético no le dicen a nadie por dónde empezar; cuatro grupos sí.
 * Lo que no esté en ninguna lista cae en operaciones, que es el cajón grande.
 */
const CATEGORIAS: { clave: 'sales' | 'operations' | 'people' | 'finance'; modulos: string[] }[] = [
  {
    clave: 'sales',
    modulos: [
      'crm',
      'ventas',
      'marketing',
      'suscripciones',
      'tpv',
      'eventos',
      'encuestas',
      'ayuda',
      'referidos',
    ],
  },
  { clave: 'finance', modulos: ['contabilidad', 'facturacion', 'gastos', 'consolidacion'] },
  {
    clave: 'people',
    modulos: [
      'contactos',
      'rrhh',
      'nominas',
      'ausencias',
      'reclutamiento',
      'evaluaciones',
      'elearning',
    ],
  },
  { clave: 'operations', modulos: [] },
];

const ROTULO_CATEGORIA = {
  sales: 'dashboard.categorySales',
  operations: 'dashboard.categoryOperations',
  people: 'dashboard.categoryPeople',
  finance: 'dashboard.categoryFinance',
} as const;

function Bienvenida({
  modulos,
  activos,
  recientes,
  locale = 'es',
  onCambiar,
  onAbrirModelo,
  onNuevaFicha,
  onVista,
  onPaleta,
  userName,
  organizacion,
  onConfigurar,
  tareas,
  verObligaciones,
  t,
}: {
  modulos: Modulo[];
  activos: string[];
  recientes: string[];
  locale?: Locale;
  onCambiar: (m: Modulo, activo: boolean) => Promise<void>;
  onAbrirModelo: (m: Modelo) => void;
  onNuevaFicha: (m: Modelo) => void;
  onVista: (v: Vista) => void;
  onPaleta: () => void;
  userName: string;
  /** Nulo = aún no se ha hecho el asistente: se ofrece arriba del todo. */
  organizacion: Organizacion | null;
  onConfigurar: () => void;
  /** Tareas propias para hoy o vencidas. */
  tareas: number;
  /** Sin lectura en Obligaciones no se enseñan sus fechas (el servicio daría 403). */
  verObligaciones: boolean;
  t: Traducir;
}): JSX.Element {
  const atajoPaleta = useAtajo('Ctrl K');
  const [resumen, setResumen] = useState<ResumenOperativo | null>(null);
  const [proximas, setProximas] = useState<Obligacion[] | null>(null);
  const [sinJustificar, setSinJustificar] = useState(0);

  // Los movimientos del banco que esperan su factura: la tarea de la semana.
  useEffect(() => {
    let montado = true;
    api<{ por_estado?: Record<string, number> }>('linea_extracto/estadisticas')
      .then((e) => montado && setSinJustificar(e.por_estado?.pendiente ?? 0))
      .catch(() => {
        // Sin el recuento, la portada no pinta el aviso: no hay nada que corregir.
      });
    return () => {
      montado = false;
    };
  }, []);

  useEffect(() => {
    let montado = true;
    api<ResumenOperativo>('resumen')
      .then((data) => {
        if (montado) setResumen(data);
      })
      .catch(() => {
        // Sin resumen la portada sigue sirviendo: se pinta lo que hay.
      });
    return () => {
      montado = false;
    };
  }, []);

  // Las próximas obligaciones, solo con perfil: las cuatro que vencen antes,
  // contando desde hoy. El año que viene se pide cuando el actual ya no da
  // más, que es lo que pasa a partir de noviembre.
  useEffect(() => {
    if (!organizacion || !verObligaciones) return;
    let montado = true;
    const anio = new Date().getFullYear();
    (async () => {
      const deEsteAnio = await api<RespuestaObligaciones>(`obligaciones${consulta({ anio })}`);
      let lista = (deEsteAnio.obligaciones ?? []).filter((o) => diasHasta(o.limite) >= 0);
      lista.sort((a, b) => a.recomendada.localeCompare(b.recomendada));
      if (lista.length < 4) {
        const siguiente = await api<RespuestaObligaciones>(
          `obligaciones${consulta({ anio: anio + 1 })}`,
        );
        lista = [...lista, ...(siguiente.obligaciones ?? [])];
      }
      if (montado) setProximas(lista.slice(0, 4));
    })().catch(() => montado && setProximas([]));
    return () => {
      montado = false;
    };
  }, [organizacion, verObligaciones]);

  const todosLosModelos = useMemo(() => modulos.flatMap((m) => m.modelos), [modulos]);
  const modeloPorClave = useMemo(
    () => new Map(todosLosModelos.map((m) => [m.clave, m])),
    [todosLosModelos],
  );
  const modeloFactura = useMemo(
    () => todosLosModelos.find((m) => m.clave === 'factura_venta' || m.clave.includes('factura')),
    [todosLosModelos],
  );
  const modelosRecientes = useMemo(
    () =>
      recientes
        .map((clave) => modeloPorClave.get(clave))
        .filter((m): m is Modelo => m !== undefined),
    [recientes, modeloPorClave],
  );
  const modulosActivos = useMemo(
    () => modulos.filter((m) => activos.includes(m.clave)),
    [modulos, activos],
  );
  const porCategoria = useMemo(() => {
    const asignados = new Set<string>();
    const grupos = CATEGORIAS.map((c) => {
      const lista = modulos.filter((m) => c.modulos.includes(m.clave));
      for (const m of lista) asignados.add(m.clave);
      return { clave: c.clave, modulos: lista };
    });
    const resto = modulos.filter((m) => !asignados.has(m.clave));
    const operaciones = grupos.find((g) => g.clave === 'operations');
    if (operaciones) operaciones.modulos = [...operaciones.modulos, ...resto];
    return grupos.filter((g) => g.modulos.length > 0);
  }, [modulos]);

  const dinero = (c: number) => formatearDinero(c, locale);
  const fecha = (iso: string) =>
    formatDate(locale, new Date(iso), { day: 'numeric', month: 'short', timeZone: 'UTC' });

  return (
    <div class="erp-dashboard-section mx-auto max-w-5xl space-y-6 px-6 py-6">
      {/* 1. Cabecera */}
      <div class="border-border flex flex-col gap-3 border-b pb-4 sm:flex-row sm:items-baseline sm:justify-between">
        <div>
          <div class="flex items-center gap-2">
            <h1 class="text-text sytue-h4 tracking-tight">
              {userName ? t('dashboard.greeting', { name: userName }) : t('nav.home')}
            </h1>
            <span class="border-border bg-bg-subtle text-text-muted inline-flex items-center rounded-full border px-2.5 py-0.5 text-xs font-semibold">
              {t('dashboard.modulesCount', {
                active: String(activos.length),
                total: String(modulos.length),
              })}
            </span>
          </div>
          <p class="text-text-muted mt-1 text-xs">{t('dashboard.subtitle')}</p>
        </div>

        <div class="flex shrink-0 items-center gap-2">
          {modeloFactura ? (
            <Button variant="primary" size="sm" onClick={() => onNuevaFicha(modeloFactura)}>
              <Icon name="plus" size={14} class="mr-1" />
              {t('dashboard.quickNewInvoice')}
            </Button>
          ) : null}
          <Button variant="secondary" size="sm" onClick={() => onVista('libro')}>
            {t('dashboard.quickLedger')}
          </Button>
          <Button
            variant="ghost"
            size="sm"
            onClick={onPaleta}
            class="text-text-muted hover:text-text hidden items-center gap-1 sm:inline-flex"
            title={t('palette.placeholder')}
          >
            <Icon name="search" size={14} />
            <kbd class="border-border bg-bg-subtle text-text-subtle ml-1 rounded px-1.5 py-0.5 font-mono text-xs">
              {atajoPaleta}
            </kbd>
          </Button>
        </div>
      </div>

      {/* 2. Esta semana: tus tareas y lo que el banco espera de ti */}
      {tareas > 0 ? (
        <Card class="border-warning flex flex-col gap-3 p-4 sm:flex-row sm:items-center sm:justify-between">
          <div class="min-w-0">
            <h2 class="text-text text-sm font-semibold">{t('dashboard.tasksTitle')}</h2>
            <p class="text-text-muted mt-1 text-sm">{t('dashboard.tasksDue', { count: tareas })}</p>
          </div>
          <Button variant="primary" size="sm" class="shrink-0" onClick={() => onVista('tareas')}>
            {t('dashboard.tasksAction')}
          </Button>
        </Card>
      ) : null}
      {sinJustificar > 0 ? (
        <Card class="border-warning flex flex-col gap-3 p-4 sm:flex-row sm:items-center sm:justify-between">
          <div class="min-w-0">
            <h2 class="text-text text-sm font-semibold">{t('dashboard.weekTitle')}</h2>
            <p class="text-text-muted mt-1 text-sm">
              {t('dashboard.bankPending', { count: sinJustificar })}
            </p>
          </div>
          <Button variant="primary" size="sm" class="shrink-0" onClick={() => onVista('banco')}>
            {t('dashboard.bankAction')}
          </Button>
        </Card>
      ) : null}

      {/* 3. Perfil: o el asistente, o las próximas obligaciones */}
      {organizacion === null ? (
        <Card class="border-primary/40 flex flex-col gap-3 p-4 sm:flex-row sm:items-center sm:justify-between">
          <div class="min-w-0">
            <h2 class="text-text text-sm font-semibold">{t('dashboard.setupTitle')}</h2>
            <p class="text-text-muted mt-1 text-sm">{t('dashboard.setupBody')}</p>
          </div>
          <Button variant="primary" size="sm" class="shrink-0" onClick={onConfigurar}>
            {t('dashboard.setupAction')}
          </Button>
        </Card>
      ) : organizacion && verObligaciones ? (
        <section aria-labelledby="heading-proximas" class="space-y-2">
          <div class="flex items-center justify-between">
            <h2 id="heading-proximas" class="sytue-overline text-text-subtle">
              {t('dashboard.upcomingTitle')}
            </h2>
            <Button variant="ghost" size="xs" onClick={() => onVista('obligaciones')}>
              {t('dashboard.seeAll')}
            </Button>
          </div>
          {proximas === null ? (
            <Spinner size={16} />
          ) : proximas.length === 0 ? (
            <p class="text-text-muted text-sm">{t('dashboard.upcomingEmpty')}</p>
          ) : (
            <ul class="border-border divide-border rounded-card divide-y border">
              {proximas.map((o) => {
                const dias = diasHasta(o.recomendada);
                const falta = faltas(o, t);
                return (
                  <li
                    key={`${o.clave}|${o.periodo}`}
                    class="flex items-center justify-between gap-3 px-4 py-2.5"
                  >
                    <div class="min-w-0">
                      <span class="text-text block truncate text-sm font-medium">
                        {o.nombre} · {rotuloDePeriodo(o.periodo, locale, t)}
                      </span>
                      <span
                        class={cx(
                          'block truncate text-xs',
                          falta.length > 0 ? 'text-warning-text' : 'text-text-subtle',
                        )}
                      >
                        {falta.length > 0
                          ? falta.map((f) => f.texto).join(' · ')
                          : o.concepto_rotulo}
                      </span>
                    </div>
                    <div class="flex shrink-0 items-center gap-2 text-xs">
                      <span class="text-text-muted tabular-nums">
                        {t('obligations.yourDate')} {fecha(o.recomendada)}
                      </span>
                      <Badge tone={dias < 0 ? 'danger' : dias <= 7 ? 'warning' : 'neutral'}>
                        {dias === 0
                          ? t('obligations.today')
                          : dias < 0
                            ? diasHasta(o.limite) === 0
                              ? t('obligations.lastDay')
                              : t('obligations.lastDays', { count: diasHasta(o.limite) })
                            : t('obligations.daysLeft', { count: dias })}
                      </Badge>
                    </div>
                  </li>
                );
              })}
            </ul>
          )}
        </section>
      ) : null}

      {/* 4. Resumen operativo */}
      <section aria-labelledby="heading-resumen" class="space-y-2">
        <h2 id="heading-resumen" class="sytue-overline text-text-subtle">
          {t('dashboard.summaryTitle')}
        </h2>
        <div class="grid grid-cols-1 gap-3 sm:grid-cols-3">
          <Card
            class={cx(
              'flex flex-col justify-between p-4 transition-colors',
              modeloFactura && 'hover:border-border-strong cursor-pointer',
            )}
            onClick={() => {
              if (modeloFactura) onAbrirModelo(modeloFactura);
            }}
          >
            <div>
              <div class="mb-1 flex items-baseline justify-between">
                <span class="text-text-muted text-xs font-medium">{t('dashboard.kpiSales')}</span>
                {resumen ? (
                  <Badge tone="success">
                    {t('dashboard.invoiceCount', { count: resumen.facturacion.total_registros })}
                  </Badge>
                ) : null}
              </div>
              <div class="text-text text-xl font-semibold tabular-nums">
                {dinero(resumen?.facturacion.total_centimos ?? 0)}
              </div>
              <p class="text-text-subtle mt-1 text-xs">{t('dashboard.kpiSalesHint')}</p>
            </div>
          </Card>

          <Card
            class="hover:border-border-strong flex cursor-pointer flex-col justify-between p-4 transition-colors"
            onClick={() => onVista('libro')}
          >
            <div>
              <div class="mb-1 flex items-baseline justify-between">
                <span class="text-text-muted text-xs font-medium">{t('nav.ledger')}</span>
                {resumen ? (
                  <Badge tone={resumen.contabilidad.cuadra ? 'success' : 'danger'}>
                    {resumen.contabilidad.cuadra
                      ? t('dashboard.ledgerBalanced')
                      : t('balance.doesNotSquare')}
                  </Badge>
                ) : null}
              </div>
              <div class="text-text text-xl font-semibold tabular-nums">
                {resumen
                  ? t('dashboard.entries', { count: resumen.contabilidad.total_asientos })
                  : t('dashboard.doubleEntry')}
              </div>
              <p class="text-text-subtle mt-1 text-xs">
                {resumen
                  ? t('dashboard.debitCredit', { amount: dinero(resumen.contabilidad.total_debe) })
                  : t('dashboard.kpiFinanceHint')}
              </p>
            </div>
          </Card>

          <Card
            class={cx(
              'flex flex-col justify-between p-4 transition-colors',
              modeloFactura && 'hover:border-border-strong cursor-pointer',
            )}
            onClick={() => {
              if (modeloFactura) onAbrirModelo(modeloFactura);
            }}
          >
            <div>
              <div class="mb-1 flex items-baseline justify-between">
                <span class="text-text-muted text-xs font-medium">
                  {t('dashboard.kpiReceivable')}
                </span>
                {resumen ? (
                  <Badge tone="info">
                    {t('dashboard.pendingInvoices', {
                      count: resumen.facturacion.facturas_pendientes,
                    })}
                  </Badge>
                ) : null}
              </div>
              <div class="text-text text-xl font-semibold tabular-nums">
                {dinero(resumen?.facturacion.pendiente_cobro_centimos ?? 0)}
              </div>
              <p class="text-text-subtle mt-1 text-xs">
                {resumen?.facturacion.proximo_vencimiento
                  ? t('dashboard.nextDue', { date: fecha(resumen.facturacion.proximo_vencimiento) })
                  : t('dashboard.kpiPeopleHint')}
              </p>
            </div>
          </Card>
        </div>
      </section>

      {/* 5. Recientes */}
      {modelosRecientes.length > 0 ? (
        <section aria-labelledby="recientes-heading" class="space-y-2">
          <h2 id="recientes-heading" class="sytue-overline text-text-subtle">
            {t('welcome.quickAccess')}
          </h2>
          <div class="grid gap-3 sm:grid-cols-2 lg:grid-cols-3">
            {modelosRecientes.map((m) => {
              const modulo = modulos.find((mod) => mod.clave === m.modulo);
              return (
                <Card
                  key={m.clave}
                  class="hover:border-border-strong flex cursor-pointer items-center justify-between gap-3 p-3.5 transition-colors"
                  onClick={() => onAbrirModelo(m)}
                >
                  <div class="min-w-0 flex-1">
                    <span class="text-text block truncate text-sm font-semibold">{m.etiqueta}</span>
                    <span class="text-text-subtle block truncate text-xs">
                      {modulo?.etiqueta ?? m.modulo}
                    </span>
                  </div>
                  <Button size="xs" variant="secondary" onClick={() => onAbrirModelo(m)}>
                    {t('dashboard.openModel')}
                  </Button>
                </Card>
              );
            })}
          </div>
        </section>
      ) : null}

      {/* 6. Áreas, por lo que son para el negocio */}
      <section aria-labelledby="areas-heading" class="space-y-4">
        <div class="flex items-center justify-between">
          <h2 id="areas-heading" class="sytue-overline text-text-subtle">
            {t('dashboard.allModules')}
          </h2>
          <Badge tone={modulosActivos.length > 0 ? 'success' : 'neutral'}>
            {t('welcome.activeCount', { count: modulosActivos.length })}
          </Badge>
        </div>

        {/* Obligaciones no se enciende ni se apaga: tras el asistente está siempre,
            porque son plazos con Hacienda y la Seguridad Social, no una preferencia. */}
        {organizacion && verObligaciones ? (
          <Card class="border-border bg-surface flex flex-col gap-3 p-4 shadow-xs sm:flex-row sm:items-center sm:justify-between">
            <div class="min-w-0">
              <div class="flex items-center gap-2">
                <h3 class="text-text text-sm font-semibold">{t('nav.obligations')}</h3>
                <Badge tone="success">{t('dashboard.alwaysOn')}</Badge>
              </div>
              <p class="text-text-muted mt-1 text-sm">{t('dashboard.alwaysOnBody')}</p>
            </div>
            <Button
              size="sm"
              variant="secondary"
              class="shrink-0"
              onClick={() => onVista('obligaciones')}
            >
              {t('action.open')}
            </Button>
          </Card>
        ) : null}

        {porCategoria.map((grupo) => (
          <div key={grupo.clave} class="space-y-2">
            <h3 class="text-text-muted text-xs font-semibold">
              {t(ROTULO_CATEGORIA[grupo.clave])}
            </h3>
            <div class="grid gap-3 sm:grid-cols-2 lg:grid-cols-3">
              {grupo.modulos.map((m) => {
                const encendido = activos.includes(m.clave);
                return (
                  <Card
                    key={m.clave}
                    class={cx(
                      'flex flex-col justify-between p-4 transition-colors',
                      encendido
                        ? 'border-border bg-surface shadow-xs'
                        : 'border-border-subtle bg-bg-subtle/50 opacity-80',
                    )}
                  >
                    <div>
                      <div class="mb-1 flex items-center justify-between gap-2">
                        <h4 class="text-text truncate text-sm font-semibold">{m.etiqueta}</h4>
                        <Badge tone={encendido ? 'success' : 'neutral'}>
                          {encendido ? t('welcome.enabled') : t('welcome.disabled')}
                        </Badge>
                      </div>
                      {m.dependencias && m.dependencias.length > 0 ? (
                        <p class="text-text-subtle mt-0.5 text-sm">
                          {t('dashboard.requires', {
                            deps: m.dependencias
                              .map(
                                (depClave) =>
                                  modulos.find((mod) => mod.clave === depClave)?.etiqueta ??
                                  depClave,
                              )
                              .join(', '),
                          })}
                        </p>
                      ) : null}
                      <div class="mt-3 flex flex-wrap gap-1.5">
                        {m.modelos.map((x) => (
                          <button
                            key={x.clave}
                            type="button"
                            onClick={() => {
                              if (!encendido) void onCambiar(m, true);
                              onAbrirModelo(x);
                            }}
                            class={cx(
                              'sytue-focus-ring rounded-control min-h-7 border px-2.5 py-1 text-sm transition-colors',
                              encendido
                                ? 'border-border bg-bg hover:bg-bg-subtle text-text hover:border-border-strong font-medium'
                                : 'border-border-subtle bg-bg/50 text-text-muted hover:text-text',
                            )}
                          >
                            {x.etiqueta}
                          </button>
                        ))}
                      </div>
                    </div>
                    <div class="border-border-subtle mt-4 flex items-center justify-between border-t pt-3">
                      <span class="text-text-subtle text-sm">
                        {t('welcome.models', { count: m.modelos.length })}
                      </span>
                      <Button
                        size="sm"
                        variant={encendido ? 'ghost' : 'secondary'}
                        onClick={() => void onCambiar(m, !encendido)}
                      >
                        {encendido ? t('welcome.disable') : t('welcome.enable')}
                      </Button>
                    </div>
                  </Card>
                );
              })}
            </div>
          </div>
        ))}
      </section>
    </div>
  );
}

/** La misma ficha y validación para crear lo que falta, sin abandonar el trabajo. */
function AltaRapida({
  abierto,
  modelo,
  semilla,
  etiquetas,
  t,
  onCerrar,
  onCreado,
}: {
  abierto: boolean;
  modelo: Modelo;
  semilla?: Registro['data'] | undefined;
  etiquetas?: Record<string, string> | undefined;
  t: Traducir;
  onCerrar: () => void;
  onCreado: (r: Registro) => void;
}): JSX.Element {
  const [valores, setValores] = useState<Record<string, string>>(() =>
    valoresIniciales(
      modelo,
      semilla
        ? {
            id: '',
            modelo: modelo.clave,
            etiqueta: '',
            creado_en: '',
            editado_en: '',
            data: semilla,
          }
        : null,
    ),
  );
  const [rotulos, setRotulos] = useState<Record<string, string>>(etiquetas ?? {});
  const [busy, setBusy] = useState(false);
  const [error, setError] = useState('');
  const [errores, setErrores] = useState<Record<string, string>>({});
  const enviandoRef = useRef(false);
  async function guardar(e: TargetedSubmitEvent<HTMLFormElement>): Promise<void> {
    e.preventDefault();
    if (enviandoRef.current) return;
    const faltan: Record<string, string> = {};
    for (const campo of modelo.campos)
      if (campo.requerido && campo.tipo === 'relacion' && !valores[campo.clave])
        faltan[campo.clave] = t('form.required');
    setErrores(faltan);
    if (Object.keys(faltan).length) return;
    enviandoRef.current = true;
    setBusy(true);
    setError('');
    try {
      const r = await api<Registro>(modelo.clave, {
        method: 'POST',
        body: JSON.stringify({
          data: {
            ...aCuerpo(modelo, valores),
            ...(modelo.clave === 'factura_compra' && typeof semilla?.externo_id === 'string'
              ? { externo_id: semilla.externo_id }
              : {}),
          },
        }),
      });
      onCreado(r);
    } catch (e) {
      setError(mensajeDeError(e, t));
      if (e instanceof ErrorApi) setErrores(e.porCampo);
    } finally {
      enviandoRef.current = false;
      setBusy(false);
    }
  }
  const formId = useId();
  return (
    <Modal
      open={abierto}
      title={`${t('action.new')} · ${modelo.etiqueta}`}
      size="md"
      placement="right"
      onClose={() => {
        if (!busy) onCerrar();
      }}
      dismissable={!busy}
      footer={
        <>
          <Button type="button" variant="ghost" disabled={busy} onClick={onCerrar}>
            {t('action.close')}
          </Button>
          <Button type="submit" form={formId} disabled={busy}>
            {busy ? <Spinner size={16} /> : null}
            {t('action.save')}
          </Button>
        </>
      }
    >
      <form id={formId} method="post" onSubmit={(e) => void guardar(e)} class="space-y-4">
        {error ? (
          <p role="alert" class="text-danger-text">
            {error}
          </p>
        ) : null}
        <fieldset disabled={busy} class="space-y-3">
          {modelo.campos
            .filter((c) => !['externo_id', 'tpv_huella', 'tpv_operacion'].includes(c.clave))
            .map((campo) => (
              <CampoDeFormulario
                key={campo.clave}
                prefijo="alta"
                campo={campo}
                valor={valores[campo.clave] ?? ''}
                etiquetas={rotulos}
                error={errores[campo.clave]}
                onCambio={(v, rotulo) => {
                  setValores((prev) => ({ ...prev, [campo.clave]: v }));
                  if (rotulo !== undefined) setRotulos((prev) => ({ ...prev, [v]: rotulo }));
                }}
                t={t}
              />
            ))}
        </fieldset>
      </form>
    </Modal>
  );
}
