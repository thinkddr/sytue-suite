/*
 * SPDX-License-Identifier: Apache-2.0
 * SPDX-FileCopyrightText: (c) 2026 Daniel Delgado Ramos
 *
 * Aplicación principal del editor de Sytue Web (PAGES-PLAN §14.1).
 *
 * Integra:
 * - Modelo del editor sobre el IR (operaciones puras, historial deshacer/rehacer con tope 100).
 * - Autoguardado con concurrencia optimista (rebote 1.500 ms, rev, detección de conflicto 409).
 * - Lienzo aislado (iframe srcdoc sandbox same-origin sin scripts e inert).
 * - Selección, hover, 8 tiradores de redimensión con usePointerDrag.
 * - Barra superior, barra de estado inferior y atajos de teclado del editor.
 */

import { useCallback, useEffect, useMemo, useRef, useState } from 'preact/hooks';
import type { JSX } from 'preact';
import type { Locale } from '@sytue/i18n';
import type {
  DatosRender,
  Estilo,
  Idioma,
  Layout,
  Nodo,
  Pagina,
  Seccion,
  Sitio,
  Tamano,
} from '@sytue/pages-render';
import { bytesIR, validar } from '@sytue/pages-render';
import { ANCHOS_PUNTO_DE_CORTE } from './modelo/tipos';
import type { EstadoGuardado, PuntoDeCorte } from './modelo/tipos';
import {
  crearHistorialPages,
  deshacerPages,
  puedeDeshacerPages,
  puedeRehacerPages,
  rehacerPages,
  registrarCambioPages,
} from './modelo/historial';
import type { HistorialPages } from './modelo/historial';
import {
  actualizarAjustesPagina,
  actualizarContenidoNodo,
  aplicarTraducciones,
  actualizarEstiloNodo,
  actualizarIdiomas,
  actualizarLayoutNodo,
  actualizarSeo,
  actualizarTema,
  bloquearNodo,
  crearNodoPorTipo,
  crearTablaDeBases,
  crearComponente,
  crearInstanciaComponente,
  crearPagina,
  desenvolverMarco,
  duplicarNodo,
  duplicarPagina,
  eliminarNodo,
  eliminarPagina,
  encontrarNodo,
  envolverEnMarco,
  insertarNodo,
  insertarSeccionEnPagina,
  ocultarNodo,
  redimensionarNodo,
  renombrarNodo,
  reordenarHermanos,
  reordenarPaginas,
  restablecerOverrideBreakpoint,
} from './modelo/operaciones';
import type { TablaDeBases } from './modelo/operaciones';
import { Modal, usePrompt } from '@sytue/ui';
import type { BloqueInsertable } from './modelo/operaciones';
import { BarraSuperior } from './componentes/BarraSuperior';
import { BarraEstado } from './componentes/BarraEstado';
import { DialogoConflicto } from './componentes/DialogoConflicto';
import { ModoContenido } from './componentes/ModoContenido';
import { Lienzo } from './lienzo/Lienzo';
import { PanelCapas } from './paneles/PanelCapas';
import { PanelInsertar } from './paneles/PanelInsertar';
import { PanelPaginas } from './paneles/PanelPaginas';
import { PanelSitio } from './paneles/PanelSitio';
import { PanelPropiedades } from './propiedades/PanelPropiedades';
import { DisenarConIA } from '../ia/DisenarConIA';
import { TraducirConIA } from '../ia/TraducirConIA';
import { procesarAtajo } from './atajos';
import { Alerta, IconoCapas, IconoInsertar, IconoPaginas, IconoSitio, Spinner } from './iconos';
import { ruta } from '../lib/rutas.js';
import { ejecutarPublicacion, mensajeDePublicacion } from '../lib/ficha.js';
import { t } from '../i18n/index.js';

export interface EditorAppProps {
  siteId: string;
  locale: Locale;
  /** ¿Administra la organización quien edita? (`administra` de lib/roles, desde la sesión). */
  administra: boolean;
}

interface BorradorRespuesta {
  id: string;
  slug: string;
  name: string;
  template: string;
  ir: Sitio;
  rev: number;
}

interface GuardarBorradorRespuesta {
  rev: number;
  warnings: unknown[];
}

export function EditorApp({ siteId, locale, administra }: EditorAppProps): JSX.Element {
  const [ir, setIr] = useState<Sitio | null>(null);
  const [siteName, setSiteName] = useState<string>('');
  const [datosRender, setDatosRender] = useState<DatosRender | null>(null);
  const [rev, setRev] = useState<number>(0);
  const [historial, setHistorial] = useState<HistorialPages | null>(null);
  const [estadoGuardado, setEstadoGuardado] = useState<EstadoGuardado>('guardado');
  const [paginaActivaId, setPaginaActivaId] = useState<string>('');
  const [puntoDeCorte, setPuntoDeCorte] = useState<PuntoDeCorte>('desktop');
  const [idiomaActivo, setIdiomaActivo] = useState<Idioma>(
    (locale === 'en' ? 'en' : 'es') as Idioma,
  );
  const [zoom, setZoom] = useState<number>(1.0);
  const [nodoSeleccionadoId, setNodoSeleccionadoId] = useState<string | null>(null);
  const [pestanaActiva, setPestanaActiva] = useState<'capas' | 'insertar' | 'paginas' | 'sitio'>(
    'capas',
  );
  const [conflictoVisible, setConflictoVisible] = useState<boolean>(false);
  const [asistenteAbierto, setAsistenteAbierto] = useState<'diseno' | 'traduccion' | null>(null);
  const [cargando, setCargando] = useState<boolean>(true);
  const [errorCarga, setErrorCarga] = useState<string | null>(null);
  const [previsualizando, setPrevisualizando] = useState(false);
  const [resultadoVista, setResultadoVista] = useState<
    { ok: true; url: string } | { ok: false; texto: string } | null
  >(null);

  const irRef = useRef<Sitio | null>(null);
  const revRef = useRef<number>(0);
  const timeoutGuardadoRef = useRef<Timer | null>(null);
  const guardadoEnCursoRef = useRef(false);
  const guardadoPromesaRef = useRef<Promise<boolean> | null>(null);
  const modeloGuardadoRef = useRef<Sitio | null>(null);

  const idiomaActual = idiomaActivo;

  // Carga inicial del borrador y datos de renderizado
  const cargarDatos = useCallback(async () => {
    setCargando(true);
    setErrorCarga(null);
    try {
      const [resDraft, resRender] = await Promise.all([
        fetch(ruta(`/api/sites/${encodeURIComponent(siteId)}/draft`)),
        fetch(ruta(`/api/sites/${encodeURIComponent(siteId)}/render-data`)),
      ]);

      if (!resDraft.ok) {
        throw new Error(`Error cargando borrador: ${resDraft.statusText}`);
      }

      const draftData = (await resDraft.json()) as BorradorRespuesta;
      if (!resRender.ok) {
        throw new Error(`No se pudo cargar la configuración del sitio (${resRender.status}).`);
      }
      const renderData = (await resRender.json()) as DatosRender;

      const sitioIR: Sitio = draftData.ir;
      irRef.current = sitioIR;
      modeloGuardadoRef.current = sitioIR;
      revRef.current = draftData.rev || 0;
      setIr(sitioIR);
      setSiteName(draftData.name || '');
      setRev(draftData.rev || 0);
      setDatosRender(renderData);
      setHistorial(crearHistorialPages());
      setPaginaActivaId(sitioIR.pages[0]?.id || 'inicio');
      setIdiomaActivo(sitioIR.languages.default);
      setEstadoGuardado('guardado');
    } catch (err: unknown) {
      setErrorCarga(err instanceof Error ? err.message : 'Error desconocido al cargar');
    } finally {
      setCargando(false);
    }
  }, [siteId]);

  useEffect(() => {
    void cargarDatos();
  }, [cargarDatos]);

  // Actualiza el IR anotando en el historial
  const aplicarCambioIR = useCallback((nuevoIR: Sitio) => {
    const actual = irRef.current;
    irRef.current = nuevoIR;
    setIr(nuevoIR);
    setHistorial((prev: HistorialPages | null) => {
      if (!prev) return crearHistorialPages();
      return actual ? registrarCambioPages(prev, actual) : prev;
    });
    setEstadoGuardado('pendiente');
  }, []);

  // Guardado en backend con concurrencia optimista (PUT /api/sites/:id/draft).
  // El autosave y la preparación de IA comparten el mismo PUT y su confirmación.
  const ejecutarGuardado = useCallback(
    (irAGuardar: Sitio, revActual: number): Promise<boolean> => {
      if (guardadoPromesaRef.current) return guardadoPromesaRef.current;
      if (!navigator.onLine) {
        setEstadoGuardado('offline');
        return Promise.resolve(false);
      }
      guardadoEnCursoRef.current = true;
      setEstadoGuardado('guardando');
      const promesa = Promise.resolve()
        .then(async () => {
          try {
            const res = await fetch(ruta(`/api/sites/${encodeURIComponent(siteId)}/draft`), {
              method: 'PUT',
              headers: { 'Content-Type': 'application/json' },
              body: JSON.stringify({ rev: revActual, ir: irAGuardar }),
            });
            if (res.status === 409) {
              setEstadoGuardado('conflicto');
              setConflictoVisible(true);
              return false;
            }
            if (!res.ok) throw new Error(`Error en guardado: ${res.statusText}`);
            const data = (await res.json()) as GuardarBorradorRespuesta;
            revRef.current = data.rev;
            modeloGuardadoRef.current = irAGuardar;
            setRev(data.rev);
            setEstadoGuardado(irRef.current === irAGuardar ? 'guardado' : 'pendiente');
            return true;
          } catch {
            setEstadoGuardado('error');
            return false;
          }
        })
        .finally(() => {
          guardadoEnCursoRef.current = false;
          guardadoPromesaRef.current = null;
        });
      guardadoPromesaRef.current = promesa;
      return promesa;
    },
    [siteId],
  );

  const prepararBorrador = useCallback(async (): Promise<boolean> => {
    if (timeoutGuardadoRef.current) clearTimeout(timeoutGuardadoRef.current);
    // Si se editó durante un PUT, esperar su revisión y guardar también el modelo nuevo.
    while (irRef.current && modeloGuardadoRef.current !== irRef.current) {
      if (!(await ejecutarGuardado(irRef.current, revRef.current))) return false;
    }
    return !!irRef.current && navigator.onLine;
  }, [ejecutarGuardado]);

  // Rebote de 1.500 ms tras cambios en el IR
  // La nueva revisión vuelve a programar el pendiente que llegó durante el PUT anterior.
  useEffect(() => {
    if (estadoGuardado !== 'pendiente' || !ir || guardadoEnCursoRef.current) return;

    if (timeoutGuardadoRef.current) {
      clearTimeout(timeoutGuardadoRef.current);
    }

    timeoutGuardadoRef.current = setTimeout(() => {
      if (irRef.current) {
        void ejecutarGuardado(irRef.current, revRef.current);
      }
    }, 1500);

    return () => {
      if (timeoutGuardadoRef.current) {
        clearTimeout(timeoutGuardadoRef.current);
      }
    };
  }, [ir, rev, estadoGuardado, ejecutarGuardado]);

  // Aviso de beforeunload si hay cambios sin guardar
  useEffect(() => {
    const handleBeforeUnload = (e: BeforeUnloadEvent) => {
      if (
        estadoGuardado === 'pendiente' ||
        estadoGuardado === 'guardando' ||
        estadoGuardado === 'error'
      ) {
        e.preventDefault();
      }
    };

    window.addEventListener('beforeunload', handleBeforeUnload);
    return () => window.removeEventListener('beforeunload', handleBeforeUnload);
  }, [estadoGuardado]);

  // Reconexión de red
  useEffect(() => {
    const handleOnline = () => {
      if (estadoGuardado === 'offline' || estadoGuardado === 'pendiente') {
        if (irRef.current) {
          void ejecutarGuardado(irRef.current, revRef.current);
        }
      }
    };
    const handleOffline = () => {
      setEstadoGuardado('offline');
    };

    window.addEventListener('online', handleOnline);
    window.addEventListener('offline', handleOffline);
    return () => {
      window.removeEventListener('online', handleOnline);
      window.removeEventListener('offline', handleOffline);
    };
  }, [estadoGuardado, ejecutarGuardado]);

  // Deshacer / Rehacer
  const handleDeshacer = useCallback(() => {
    if (!historial || !ir || !puedeDeshacerPages(historial)) return;
    const res = deshacerPages(historial, ir);
    if (res) {
      setHistorial(res.historial);
      irRef.current = res.modelo;
      setIr(res.modelo);
      setEstadoGuardado('pendiente');
    }
  }, [historial, ir]);

  const handleRehacer = useCallback(() => {
    if (!historial || !ir || !puedeRehacerPages(historial)) return;
    const res = rehacerPages(historial, ir);
    if (res) {
      setHistorial(res.historial);
      irRef.current = res.modelo;
      setIr(res.modelo);
      setEstadoGuardado('pendiente');
    }
  }, [historial, ir]);

  // Acciones sobre el nodo seleccionado
  const handleDuplicar = useCallback(() => {
    if (!ir || !nodoSeleccionadoId) return;
    const resultado = duplicarNodo(ir, nodoSeleccionadoId);
    if (resultado) {
      aplicarCambioIR(resultado.ir);
      setNodoSeleccionadoId(resultado.nuevoId);
    }
  }, [ir, nodoSeleccionadoId, aplicarCambioIR]);

  const handleEliminar = useCallback(() => {
    if (!ir || !nodoSeleccionadoId) return;
    const nuevoIR = eliminarNodo(ir, nodoSeleccionadoId);
    aplicarCambioIR(nuevoIR);
    setNodoSeleccionadoId(null);
  }, [ir, nodoSeleccionadoId, aplicarCambioIR]);

  const handleEnvolverMarco = useCallback(() => {
    if (!ir || !nodoSeleccionadoId) return;
    const resultado = envolverEnMarco(ir, [nodoSeleccionadoId]);
    if (resultado) {
      aplicarCambioIR(resultado.ir);
      setNodoSeleccionadoId(resultado.marcoId);
    }
  }, [ir, nodoSeleccionadoId, aplicarCambioIR]);

  const handleDesenvolverMarco = useCallback(() => {
    if (!ir || !nodoSeleccionadoId) return;
    const nuevoIR = desenvolverMarco(ir, nodoSeleccionadoId);
    aplicarCambioIR(nuevoIR);
  }, [ir, nodoSeleccionadoId, aplicarCambioIR]);

  const handleRedimensionar = useCallback(
    (id: string, nuevoTamano: Tamano) => {
      if (!ir) return;
      const nuevoIR = redimensionarNodo(ir, id, nuevoTamano, puntoDeCorte);
      aplicarCambioIR(nuevoIR);
    },
    [ir, puntoDeCorte, aplicarCambioIR],
  );

  // Handlers de capas, inserción, páginas, temas y propiedades
  const handleRenombrarNodo = useCallback(
    (id: string, nuevoNombre: string) => {
      if (!ir) return;
      aplicarCambioIR(renombrarNodo(ir, id, nuevoNombre));
    },
    [ir, aplicarCambioIR],
  );

  const handleBloquearNodo = useCallback(
    (id: string) => {
      if (!ir) return;
      aplicarCambioIR(bloquearNodo(ir, id));
    },
    [ir, aplicarCambioIR],
  );

  const handleOcultarNodo = useCallback(
    (id: string) => {
      if (!ir) return;
      aplicarCambioIR(ocultarNodo(ir, id, undefined, puntoDeCorte));
    },
    [ir, puntoDeCorte, aplicarCambioIR],
  );

  const handleReordenarHermanos = useCallback(
    (id: string, direccion: 'arriba' | 'abajo') => {
      if (!ir) return;
      aplicarCambioIR(reordenarHermanos(ir, id, direccion));
    },
    [ir, aplicarCambioIR],
  );

  const insertarEnSeleccion = useCallback(
    (nuevoNodo: Nodo) => {
      if (!ir) return;
      if (nodoSeleccionadoId) {
        const ref = encontrarNodo(ir, nodoSeleccionadoId);
        if (ref && 'children' in ref.nodo) {
          aplicarCambioIR(insertarNodo(ir, ref.nodo.id, ref.nodo.children.length, nuevoNodo));
          setNodoSeleccionadoId(nuevoNodo.id);
          return;
        }
      }
      const pag = ir.pages.find((p) => p.id === paginaActivaId) || ir.pages[0];
      if (pag && pag.sections.length > 0) {
        const ultimaSec = pag.sections[pag.sections.length - 1]!;
        aplicarCambioIR(insertarNodo(ir, ultimaSec.id, ultimaSec.children.length, nuevoNodo));
        setNodoSeleccionadoId(nuevoNodo.id);
      }
    },
    [ir, nodoSeleccionadoId, paginaActivaId, aplicarCambioIR],
  );

  const handleInsertarNodo = useCallback(
    (tipo: BloqueInsertable) => insertarEnSeleccion(crearNodoPorTipo(tipo, idiomaActual)),
    [insertarEnSeleccion, idiomaActual],
  );

  // «Tabla desde Sytue Bases»: el token de una conexión concedida a Sytue Web se pide, se
  // manda UNA vez al servicio (que lee la vista y no lo guarda) y lo que vuelve entra en el
  // borrador como una tabla corriente.
  const { preguntar, dialogo: dialogoBases } = usePrompt();
  const [avisoBases, setAvisoBases] = useState('');
  const handleInsertarTablaDeBases = useCallback(async () => {
    setAvisoBases('');
    const token = (
      await preguntar({
        titulo: t(locale, 'editor.bases.title'),
        mensaje: t(locale, 'editor.bases.prompt'),
        confirmLabel: t(locale, 'editor.bases.confirm'),
        secreto: true,
      })
    )?.trim();
    if (!token) return;
    try {
      const res = await fetch(ruta(`/api/sites/${encodeURIComponent(siteId)}/bases/table`), {
        method: 'POST',
        headers: { 'content-type': 'application/json' },
        body: JSON.stringify({ token }),
      });
      if (!res.ok) {
        const codigo = ((await res.json().catch(() => null)) as { code?: string } | null)?.code;
        setAvisoBases(
          t(
            locale,
            res.status === 403
              ? 'editor.bases.forbidden'
              : codigo === 'bases_connection_invalid'
                ? 'editor.bases.invalid'
                : codigo === 'bases_view_too_large'
                  ? 'editor.bases.large'
                  : 'editor.bases.generic',
          ),
        );
        return;
      }
      insertarEnSeleccion(crearTablaDeBases((await res.json()) as TablaDeBases, idiomaActual));
    } catch {
      setAvisoBases(t(locale, 'editor.bases.generic'));
    }
  }, [preguntar, locale, siteId, insertarEnSeleccion, idiomaActual]);

  const handleInsertarComponente = useCallback(
    (componenteId: string) => {
      if (!ir) return;
      const referencia = nodoSeleccionadoId ? encontrarNodo(ir, nodoSeleccionadoId)?.nodo : null;
      const secciones = ir.pages.find((pagina) => pagina.id === paginaActivaId)?.sections;
      const contenedor =
        referencia?.type === 'frame' ? referencia : secciones?.[secciones.length - 1];
      if (!contenedor) return;
      const instancia = crearInstanciaComponente(componenteId);
      aplicarCambioIR(insertarNodo(ir, contenedor.id, contenedor.children.length, instancia));
      setNodoSeleccionadoId(instancia.id);
    },
    [ir, nodoSeleccionadoId, paginaActivaId, aplicarCambioIR],
  );

  const handleInsertarSeccion = useCallback(
    (sec: Seccion) => {
      if (!ir) return;
      aplicarCambioIR(insertarSeccionEnPagina(ir, paginaActivaId, sec));
      setNodoSeleccionadoId(sec.id);
    },
    [ir, paginaActivaId, aplicarCambioIR],
  );

  const handleCrearPagina = useCallback(
    (titulo: string) => {
      if (!ir) return;
      const res = crearPagina(ir, titulo);
      aplicarCambioIR(res.ir);
      setPaginaActivaId(res.paginaId);
      setNodoSeleccionadoId(null);
    },
    [ir, aplicarCambioIR],
  );

  const handleDuplicarPagina = useCallback(
    (id: string) => {
      if (!ir) return;
      const res = duplicarPagina(ir, id);
      if (res) {
        aplicarCambioIR(res.ir);
        setPaginaActivaId(res.paginaId);
      }
    },
    [ir, aplicarCambioIR],
  );

  const handleEliminarPagina = useCallback(
    (id: string) => {
      if (!ir) return;
      const nuevoIR = eliminarPagina(ir, id);
      aplicarCambioIR(nuevoIR);
      if (paginaActivaId === id) {
        setPaginaActivaId(nuevoIR.pages[0]?.id || 'inicio');
      }
      setNodoSeleccionadoId(null);
    },
    [ir, paginaActivaId, aplicarCambioIR],
  );

  const handleReordenarPaginas = useCallback(
    (ids: string[]) => {
      if (!ir) return;
      aplicarCambioIR(reordenarPaginas(ir, ids));
    },
    [ir, aplicarCambioIR],
  );

  const handleActualizarAjustesPagina = useCallback(
    (id: string, ajustes: Partial<Pagina>) => {
      if (!ir) return;
      aplicarCambioIR(actualizarAjustesPagina(ir, id, ajustes));
    },
    [ir, aplicarCambioIR],
  );

  const handleActualizarTema = useCallback(
    (tema: Partial<Sitio['theme']>) => {
      if (!ir) return;
      aplicarCambioIR(actualizarTema(ir, tema));
    },
    [ir, aplicarCambioIR],
  );

  const handleActualizarIdiomas = useCallback(
    (languages: Sitio['languages']) => {
      if (!ir) return;
      aplicarCambioIR(actualizarIdiomas(ir, languages));
    },
    [ir, aplicarCambioIR],
  );

  const handleActualizarSeo = useCallback(
    (seo: Partial<Sitio['seo']>) => {
      if (!ir) return;
      aplicarCambioIR(actualizarSeo(ir, seo));
    },
    [ir, aplicarCambioIR],
  );

  const handleActualizarContenido = useCallback(
    (parche: Record<string, unknown>) => {
      if (!ir || !nodoSeleccionadoId) return;
      aplicarCambioIR(actualizarContenidoNodo(ir, nodoSeleccionadoId, parche, idiomaActual));
    },
    [ir, nodoSeleccionadoId, idiomaActual, aplicarCambioIR],
  );

  const handleActualizarContenidoMovil = useCallback(
    (id: string, parche: Record<string, unknown>) => {
      if (!ir) return;
      aplicarCambioIR(actualizarContenidoNodo(ir, id, parche, idiomaActual));
    },
    [ir, idiomaActual, aplicarCambioIR],
  );

  const handleActualizarEstilo = useCallback(
    (parche: Partial<Estilo>) => {
      if (!ir || !nodoSeleccionadoId) return;
      aplicarCambioIR(actualizarEstiloNodo(ir, nodoSeleccionadoId, parche, puntoDeCorte));
    },
    [ir, nodoSeleccionadoId, puntoDeCorte, aplicarCambioIR],
  );

  const handleActualizarLayout = useCallback(
    (parche: Partial<Layout>) => {
      if (!ir || !nodoSeleccionadoId) return;
      aplicarCambioIR(actualizarLayoutNodo(ir, nodoSeleccionadoId, parche, puntoDeCorte));
    },
    [ir, nodoSeleccionadoId, puntoDeCorte, aplicarCambioIR],
  );

  const handleActualizarTamano = useCallback(
    (parche: Partial<Tamano>) => {
      if (!ir || !nodoSeleccionadoId) return;
      const ref = encontrarNodo(ir, nodoSeleccionadoId);
      if (!ref) return;
      const actual =
        puntoDeCorte !== 'desktop' && ref.nodo.breakpoints?.[puntoDeCorte]?.size
          ? { ...ref.nodo.size, ...ref.nodo.breakpoints[puntoDeCorte]?.size }
          : (ref.nodo.size ?? { width: 'hug', height: 'hug' });
      const nuevoTamano = {
        width: parche.width !== undefined ? parche.width : (actual.width ?? 'hug'),
        height: parche.height !== undefined ? parche.height : (actual.height ?? 'hug'),
      };
      aplicarCambioIR(redimensionarNodo(ir, nodoSeleccionadoId, nuevoTamano, puntoDeCorte));
    },
    [ir, nodoSeleccionadoId, puntoDeCorte, aplicarCambioIR],
  );

  const handleRestablecerOverride = useCallback(() => {
    if (!ir || !nodoSeleccionadoId || puntoDeCorte === 'desktop') return;
    aplicarCambioIR(restablecerOverrideBreakpoint(ir, nodoSeleccionadoId, puntoDeCorte));
  }, [ir, nodoSeleccionadoId, puntoDeCorte, aplicarCambioIR]);

  // Controles de zoom
  const handleZoomMas = useCallback(() => {
    setZoom((z) => Math.min(Math.round((z + 0.1) * 10) / 10, 2.0));
  }, []);

  const handleZoomMenos = useCallback(() => {
    setZoom((z) => Math.max(Math.round((z - 0.1) * 10) / 10, 0.25));
  }, []);

  const handleZoom100 = useCallback(() => {
    setZoom(1.0);
  }, []);

  const handleZoomAjustar = useCallback(() => {
    const anchoBp = ANCHOS_PUNTO_DE_CORTE[puntoDeCorte];
    const anchoDisponible = window.innerWidth - 80;
    const nuevoZoom = Math.min(Math.max(anchoDisponible / anchoBp, 0.25), 1.0);
    setZoom(Math.round(nuevoZoom * 100) / 100);
  }, [puntoDeCorte]);

  // Atajos de teclado globales del editor
  useEffect(() => {
    const handleKeyDown = (e: KeyboardEvent) => {
      procesarAtajo(e, {
        onDeshacer: handleDeshacer,
        onRehacer: handleRehacer,
        onDuplicar: handleDuplicar,
        onEliminar: handleEliminar,
        onEnvolverMarco: handleEnvolverMarco,
        onDesenvolverMarco: handleDesenvolverMarco,
        onZoomMas: handleZoomMas,
        onZoomMenos: handleZoomMenos,
        onZoom100: handleZoom100,
        onZoomAjustar: handleZoomAjustar,
        onEscape: () => setNodoSeleccionadoId(null),
      });
    };

    window.addEventListener('keydown', handleKeyDown);
    return () => window.removeEventListener('keydown', handleKeyDown);
  }, [
    handleDeshacer,
    handleRehacer,
    handleDuplicar,
    handleEliminar,
    handleEnvolverMarco,
    handleDesenvolverMarco,
    handleZoomMas,
    handleZoomMenos,
    handleZoom100,
    handleZoomAjustar,
  ]);

  // Resolución de conflicto 409
  const handleRecargarConflicto = () => {
    setConflictoVisible(false);
    void cargarDatos();
  };

  const handleSobrescribirConflicto = async () => {
    setConflictoVisible(false);
    try {
      const res = await fetch(ruta(`/api/sites/${encodeURIComponent(siteId)}/draft`));
      if (!res.ok) throw new Error('Error al obtener la revisión actual');
      const data = (await res.json()) as BorradorRespuesta;
      const revServidor = data.rev;
      revRef.current = revServidor;
      setRev(revServidor);
      if (irRef.current) {
        await ejecutarGuardado(irRef.current, revServidor);
      }
    } catch {
      setEstadoGuardado('error');
    }
  };

  useEffect(() => {
    // Una vista ya creada representa el borrador guardado de ese momento.
    setResultadoVista(null);
  }, [ir]);

  const handleVistaPrevia = async () => {
    if (
      estadoGuardado !== 'guardado' ||
      previsualizando ||
      guardadoEnCursoRef.current ||
      irRef.current !== ir
    )
      return;
    const modeloVista = irRef.current;
    const revisionVista = revRef.current;
    setPrevisualizando(true);
    setResultadoVista(null);
    const intento = await ejecutarPublicacion(siteId, 'preview');
    setPrevisualizando(false);
    // La respuesta también puede llegar después de editar: no presentar una vista obsoleta.
    if (irRef.current !== modeloVista || revRef.current !== revisionVista) return;
    if (intento.ok && intento.url) {
      setResultadoVista({ ok: true, url: intento.url });
    } else {
      setResultadoVista({
        ok: false,
        texto: t(locale, intento.ok ? 'publish.err_generic' : mensajeDePublicacion(intento.fallo)),
      });
    }
  };

  // Cálculo de tamaño del IR y avisos de validación
  const { tamanoBytes, avisosCount } = useMemo(() => {
    if (!ir) return { tamanoBytes: 0, avisosCount: 0 };
    const bytes = bytesIR(ir);
    const probs = validar(ir, { modo: 'borrador' });
    const avisos = probs.filter((p) => p.severity === 'warning').length;
    return { tamanoBytes: bytes, avisosCount: avisos };
  }, [ir]);

  // Nodo seleccionado
  const nodoSeleccionado: Nodo | Seccion | null = useMemo(() => {
    if (!ir || !nodoSeleccionadoId) return null;
    const ref = encontrarNodo(ir, nodoSeleccionadoId);
    return ref ? ref.nodo : null;
  }, [ir, nodoSeleccionadoId]);

  if (cargando) {
    return (
      <div class="sytue-safe-fullscreen bg-bg-subtle text-text-muted flex h-[100dvh] w-full flex-col items-center justify-center">
        <Spinner size={28} class="text-primary-text mb-3" />
        <p class="text-sm font-medium">Cargando editor de Sytue Web...</p>
      </div>
    );
  }

  if (errorCarga || !ir || !datosRender) {
    return (
      <div class="sytue-safe-fullscreen bg-bg-subtle text-text flex h-[100dvh] w-full flex-col items-center justify-center p-6">
        <div class="border-border bg-surface max-w-md rounded-xl border p-6 text-center shadow-sm">
          <Alerta size={35} class="mx-auto mb-3 text-red-600" />
          <h2 class="text-text text-base font-semibold">Error al abrir el editor</h2>
          <p class="text-text-muted mt-2 text-sm">
            {errorCarga || 'No se pudo cargar la configuración del sitio.'}
          </p>
          <button
            type="button"
            onClick={() => {
              void cargarDatos();
            }}
            class="bg-primary rounded-control mt-5 px-4 py-2 text-sm font-medium text-[var(--sytue-on-primary)] hover:bg-[var(--sytue-primary-hover)]"
          >
            Reintentar
          </button>
        </div>
      </div>
    );
  }

  return (
    <div class="sytue-safe-fullscreen bg-bg text-text flex h-[100dvh] w-full flex-col overflow-hidden">
      {/* Barra superior */}
      <BarraSuperior
        locale={locale}
        siteId={siteId}
        siteName={siteName}
        ir={ir}
        paginaActivaId={paginaActivaId}
        puntoDeCorte={puntoDeCorte}
        idiomaActivo={idiomaActivo}
        zoom={zoom}
        estadoGuardado={estadoGuardado}
        puedeDeshacer={historial ? puedeDeshacerPages(historial) : false}
        puedeRehacer={historial ? puedeRehacerPages(historial) : false}
        onCambiarPagina={(id) => {
          setPaginaActivaId(id);
          setNodoSeleccionadoId(null);
        }}
        onCambiarPuntoDeCorte={setPuntoDeCorte}
        onCambiarIdioma={setIdiomaActivo}
        onAbrirAsistente={setAsistenteAbierto}
        onDeshacer={handleDeshacer}
        onRehacer={handleRehacer}
        onZoomMas={handleZoomMas}
        onZoomMenos={handleZoomMenos}
        onZoomAjustar={handleZoomAjustar}
        onZoom100={handleZoom100}
        previsualizando={previsualizando}
        onVistaPrevia={() => {
          void handleVistaPrevia();
        }}
      />

      {resultadoVista ? (
        <div
          role={resultadoVista.ok ? 'status' : 'alert'}
          class={`border-border flex items-center gap-3 border-b px-4 py-3 text-sm ${
            resultadoVista.ok ? 'bg-success-bg text-success-text' : 'bg-danger-bg text-danger-text'
          }`}
        >
          {resultadoVista.ok ? (
            <>
              <span>{t(locale, 'publish.done_preview')}</span>
              <a
                href={resultadoVista.url}
                target="_blank"
                rel="noopener noreferrer"
                class="font-semibold underline"
              >
                {t(locale, 'publish.open_preview')}
              </a>
            </>
          ) : (
            resultadoVista.texto
          )}
        </div>
      ) : null}

      {/* Cuerpo principal con barra lateral izquierda, lienzo y barra de propiedades */}
      <main class="relative hidden flex-1 overflow-hidden lg:flex" aria-label="Editor visual">
        {/* Barra lateral izquierda */}
        <aside class="border-border bg-surface flex w-72 shrink-0 flex-col border-r">
          {/* Selector de pestañas */}
          <div class="border-border bg-surface flex border-b">
            <button
              type="button"
              onClick={() => setPestanaActiva('capas')}
              class={`flex flex-1 items-center justify-center gap-1.5 border-b-2 py-2 text-xs font-medium transition-colors ${
                pestanaActiva === 'capas'
                  ? 'bg-surface border-primary text-primary-text font-bold'
                  : 'text-text-muted hover:text-text border-transparent'
              }`}
            >
              <IconoCapas size={14} />
              <span>Capas</span>
            </button>
            <button
              type="button"
              onClick={() => setPestanaActiva('insertar')}
              class={`flex flex-1 items-center justify-center gap-1.5 border-b-2 py-2 text-xs font-medium transition-colors ${
                pestanaActiva === 'insertar'
                  ? 'bg-surface border-primary text-primary-text font-bold'
                  : 'text-text-muted hover:text-text border-transparent'
              }`}
            >
              <IconoInsertar size={14} />
              <span>Insertar</span>
            </button>
            <button
              type="button"
              onClick={() => setPestanaActiva('paginas')}
              class={`flex flex-1 items-center justify-center gap-1.5 border-b-2 py-2 text-xs font-medium transition-colors ${
                pestanaActiva === 'paginas'
                  ? 'bg-surface border-primary text-primary-text font-bold'
                  : 'text-text-muted hover:text-text border-transparent'
              }`}
            >
              <IconoPaginas size={14} />
              <span>Páginas</span>
            </button>
            <button
              type="button"
              onClick={() => setPestanaActiva('sitio')}
              class={`flex flex-1 items-center justify-center gap-1.5 border-b-2 py-2 text-xs font-medium transition-colors ${
                pestanaActiva === 'sitio'
                  ? 'bg-surface border-primary text-primary-text font-bold'
                  : 'text-text-muted hover:text-text border-transparent'
              }`}
            >
              <IconoSitio size={14} />
              <span>Sitio</span>
            </button>
          </div>

          {/* Contenido de la pestaña activa */}
          <div class="flex-1 overflow-hidden">
            {pestanaActiva === 'capas' && (
              <PanelCapas
                ir={ir}
                paginaActivaId={paginaActivaId}
                nodoSeleccionadoId={nodoSeleccionadoId}
                puntoDeCorte={puntoDeCorte}
                onSeleccionarNodo={(id) => setNodoSeleccionadoId(id)}
                onRenombrarNodo={handleRenombrarNodo}
                onBloquearNodo={handleBloquearNodo}
                onOcultarNodo={handleOcultarNodo}
                onEliminarNodo={handleEliminar}
                onReordenarHermanos={handleReordenarHermanos}
              />
            )}
            {pestanaActiva === 'insertar' && (
              <PanelInsertar
                componentes={ir.components}
                onInsertarNodo={handleInsertarNodo}
                onInsertarSeccion={handleInsertarSeccion}
                onInsertarComponente={handleInsertarComponente}
                bases={{
                  titulo: t(locale, 'editor.bases.insert'),
                  desc: t(locale, 'editor.bases.desc'),
                  aviso: avisoBases,
                  onInsertar: () => void handleInsertarTablaDeBases(),
                }}
              />
            )}
            {pestanaActiva === 'paginas' && (
              <PanelPaginas
                ir={ir}
                paginaActivaId={paginaActivaId}
                onCambiarPagina={(id) => {
                  setPaginaActivaId(id);
                  setNodoSeleccionadoId(null);
                }}
                onCrearPagina={handleCrearPagina}
                onDuplicarPagina={handleDuplicarPagina}
                onEliminarPagina={handleEliminarPagina}
                onReordenarPaginas={handleReordenarPaginas}
                onActualizarAjustesPagina={handleActualizarAjustesPagina}
              />
            )}
            {pestanaActiva === 'sitio' && (
              <PanelSitio
                ir={ir}
                datosRender={datosRender}
                onActualizarTema={handleActualizarTema}
                onActualizarIdiomas={handleActualizarIdiomas}
                onActualizarSeo={handleActualizarSeo}
              />
            )}
          </div>
        </aside>

        {/* Lienzo en el centro */}
        <div class="relative flex flex-1 overflow-hidden">
          <Lienzo
            siteId={siteId}
            ir={ir}
            datosRender={datosRender}
            paginaActivaId={paginaActivaId}
            idioma={idiomaActual}
            puntoDeCorte={puntoDeCorte}
            zoom={zoom}
            nodoSeleccionadoId={nodoSeleccionadoId}
            onSeleccionar={(id) => setNodoSeleccionadoId(id)}
            onRedimensionar={handleRedimensionar}
            onZoomChange={setZoom}
          />
        </div>

        {/* Barra lateral derecha: Propiedades */}
        <aside class="border-border bg-surface flex w-72 shrink-0 flex-col border-l">
          {nodoSeleccionado?.type === 'frame' && (
            <button
              type="button"
              onClick={() =>
                aplicarCambioIR(
                  crearComponente(ir, nodoSeleccionado.id, nodoSeleccionado.name || 'Componente'),
                )
              }
              class="border-border hover:bg-bg-subtle sytue-focus-ring m-3 rounded border px-3 py-2 text-xs font-medium"
            >
              Guardar como componente
            </button>
          )}
          <PanelPropiedades
            nodo={nodoSeleccionado}
            ir={ir}
            puntoDeCorte={puntoDeCorte}
            onActualizarContenido={handleActualizarContenido}
            onActualizarEstilo={handleActualizarEstilo}
            onActualizarLayout={handleActualizarLayout}
            onActualizarTamano={handleActualizarTamano}
            onRestablecerOverride={handleRestablecerOverride}
          />
        </aside>
      </main>

      <main class="flex flex-1 overflow-hidden lg:hidden" aria-label="Editor en modo contenido">
        <ModoContenido
          ir={ir}
          paginaActivaId={paginaActivaId}
          idioma={idiomaActual}
          onActualizar={handleActualizarContenidoMovil}
        />
      </main>

      {/* Barra de estado inferior */}
      <BarraEstado
        nodoSeleccionado={nodoSeleccionado}
        tamanoBytes={tamanoBytes}
        avisosCount={avisosCount}
      />

      {/* Diálogo modal de conflicto de concurrencia */}
      <DialogoConflicto
        abierto={conflictoVisible}
        locale={locale}
        onRecargar={handleRecargarConflicto}
        onSobrescribir={() => {
          void handleSobrescribirConflicto();
        }}
        onCerrar={() => setConflictoVisible(false)}
      />

      {/* El `Modal` de la suite y no una capa propia: la de antes no atrapaba el foco,
          no cerraba con Escape y anunciaba su título en castellano en cualquier idioma. */}
      <Modal
        open={asistenteAbierto !== null}
        onClose={() => setAsistenteAbierto(null)}
        title={t(
          locale,
          asistenteAbierto === 'traduccion' ? 'ai.translate.title' : 'ai.design.title',
        )}
        size="lg"
      >
        {asistenteAbierto === 'diseno' ? (
          <DisenarConIA
            siteId={siteId}
            idioma={idiomaActual}
            locale={locale}
            administra={administra}
            prepararBorrador={prepararBorrador}
            onCerrar={() => setAsistenteAbierto(null)}
            onAplicar={(secciones) => {
              const actual = irRef.current;
              if (!actual) return;
              aplicarCambioIR(
                secciones.reduce<Sitio>(
                  (actual, seccion) =>
                    insertarSeccionEnPagina(actual, paginaActivaId, seccion as Seccion),
                  actual,
                ),
              );
              setAsistenteAbierto(null);
            }}
          />
        ) : (
          <TraducirConIA
            siteId={siteId}
            pageId={paginaActivaId}
            locale={locale}
            administra={administra}
            prepararBorrador={prepararBorrador}
            idioma={ir.languages.default}
            idiomas={[ir.languages.default, ...ir.languages.others]}
            onCerrar={() => setAsistenteAbierto(null)}
            onAplicar={(resultado, sobrescribir, destinos) => {
              const actual = irRef.current;
              if (!actual) return;
              aplicarCambioIR(aplicarTraducciones(actual, resultado, sobrescribir, destinos));
              setAsistenteAbierto(null);
            }}
          />
        )}
      </Modal>
      {dialogoBases}
    </div>
  );
}

export default EditorApp;
