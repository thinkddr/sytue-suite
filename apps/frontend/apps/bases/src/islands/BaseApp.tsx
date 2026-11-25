/*
 * SPDX-License-Identifier: Apache-2.0
 * SPDX-FileCopyrightText: (c) 2026 Daniel Delgado Ramos
 *
 * Una base abierta: sus tablas en pestañas, la rejilla editable, y —según el rol— crear
 * campos y tablas, importar un CSV, exportar y compartir. El rol que llega del servidor
 * solo decide qué se PINTA; cada operación la vuelve a autorizar el servicio.
 */
import type { JSX } from 'preact';
import { useCallback, useEffect, useId, useMemo, useRef, useState } from 'preact/hooks';
import {
  Avatar,
  Button,
  CardIcon,
  Dropdown,
  Icon,
  IconButton,
  marcarAppLista,
  MenuItem,
  MenuSeparator,
  Skeleton,
  Tabs,
  useConfirm,
  usePrompt,
  useUltimo,
} from '@sytue/ui';
import type { Locale } from '@sytue/i18n';
import type { SytueUser } from '@sytue/auth';
import { HttpError } from '@sytue/api-clients';
import type {
  BasesBase,
  BasesField,
  BasesNewField,
  BasesPresence,
  BasesRecord,
  BasesTable,
  BasesValue,
  BasesView,
  BasesViewConfig,
  BasesViewKind,
} from '@sytue/api-clients/bases';
import {
  DialogoBorrarCampo,
  DialogoCampo,
  DialogoFormulaCampo,
  DialogoImportar,
  DialogoOpcionesCampo,
  DialogoRenombrarCampo,
  DialogoVarios,
} from '../components/Dialogos.js';
import { DialogoCompartir } from '../components/DialogoCompartir.js';
import { DialogoAdjuntos } from '../components/DialogoAdjuntos.js';
import { DialogoConexiones } from '../components/DialogoConexiones.js';
import { DialogoVista } from '../components/DialogoVista.js';
import { FichaRegistro } from '../components/FichaRegistro.js';
import { BarraLectura } from '../components/BarraLectura.js';
import { Marco } from '../components/Marco.js';
import { PanelActividad, DialogoDescripcion } from '../components/PanelActividad.js';
import { PanelComentarios } from '../components/PanelComentarios.js';
import { PanelFormulario } from '../components/PanelFormulario.js';
import { PanelScripts } from '../components/PanelScripts.js';
import { Rejilla, type AccionCampo } from '../components/Rejilla.js';
import { VacioGuiado } from '../components/VacioGuiado.js';
import { esRecorte, vacioDe } from '../lib/vacios.js';
import { Tablero } from '../components/Tablero.js';
import { Galeria } from '../components/Galeria.js';
import { datosDeColumna } from '../lib/vistas.js';
import { useT } from '../i18n/index.js';
import { cliente, mensajeDeError } from '../lib/api.js';
import { tonoDeBase } from '../lib/aspecto.js';
import { registroDelEnlace } from '../lib/comentarios.js';
import { moverCampo as moverCampoEn } from '../lib/esquema.js';
import { hayCambios } from '../lib/ficha.js';
import {
  SIN_LECTURA,
  crearTurnos,
  hayLectura,
  opcionesDeLectura,
  type Lectura,
} from '../lib/lectura.js';
import { MAX_OPCIONES_ENLACE, PAGINA } from '../lib/limites.js';
import { errorDeAdjunto, MAX_BYTES_ADJUNTO } from '../lib/adjuntos.js';
import { campoDeAdjuntos, esCSV, llevaFicheros } from '../lib/pegar.js';
import { ruta } from '../lib/rutas.js';
import {
  idAleatorio,
  personasPresentes,
  planDe,
  presentesPorCelda,
  seguir,
} from '../lib/tiemporeal.js';
import {
  SIN_VENTANAS,
  anadir,
  avanzar,
  cursorAnterior,
  editar,
  empezar,
  retroceder,
  type Ventanas,
} from '../lib/ventanas.js';
import {
  etiquetaDeRegistro,
  enlacesDe,
  puedeComentar,
  puedeEscribir,
  puedeEsquema,
} from '../lib/valores.js';

interface Props {
  baseId: string;
  locale: Locale;
  user: SytueUser;
  hubBaseUrl: string;
}

type Dialogo =
  | { tipo: 'campo' }
  | { tipo: 'editarCampo'; accion: AccionCampo; campo: BasesField }
  | { tipo: 'compartir' }
  | { tipo: 'importar'; fichero?: File }
  | { tipo: 'vista' }
  | { tipo: 'editarVista'; vista: BasesView }
  | { tipo: 'conexiones' }
  | { tipo: 'scripts' }
  | { tipo: 'actividad' }
  | { tipo: 'descripcion' }
  | { tipo: 'varios'; registro: BasesRecord; campo: BasesField }
  | { tipo: 'adjuntos'; registro: string; campo: BasesField };

export function BaseApp({ baseId, locale, user, hubBaseUrl }: Props): JSX.Element {
  const t = useT(locale);
  const { confirmar, dialogo: dialogoConfirmar } = useConfirm();
  const { preguntar, dialogo: dialogoPreguntar } = usePrompt();

  const [base, setBase] = useState<BasesBase | null>(null);
  const [tablas, setTablas] = useState<BasesTable[]>([]);
  const [activa, setActiva] = useState('');
  const [campos, setCampos] = useState<BasesField[]>([]);
  // La rejilla carga por ventanas (`lib/ventanas.ts`): nunca la tabla entera.
  const [ventanas, setVentanas] = useState<Ventanas>(SIN_VENTANAS);
  const ventanasRef = useRef(ventanas);
  useEffect(() => {
    ventanasRef.current = ventanas;
  }, [ventanas]);
  const registros = useMemo(() => ventanas.paginas.flat(), [ventanas]);
  const hayMas = ventanas.siguiente !== '';
  const hayAntes = ventanas.primera > 0;
  const setRegistros = (fn: (rs: BasesRecord[]) => BasesRecord[]): void =>
    setVentanas((v) => editar(v, fn));
  const [rolTabla, setRolTabla] = useState<BasesTable['role']>(undefined);
  const [etiquetas, setEtiquetas] = useState<Record<string, Map<string, string>>>({});
  const [opciones, setOpciones] = useState<Record<string, { id: string; etiqueta: string }[]>>({});
  const [cargando, setCargando] = useState(true);
  const [error, setError] = useState('');
  const [errorDialogo, setErrorDialogo] = useState('');
  const [ocupado, setOcupado] = useState(false);
  const [dialogo, setDialogo] = useState<Dialogo | null>(null);
  const [estado, setEstado] = useState('');
  /** Hay fórmulas pendientes de recalcular en la tabla que se mira. */
  const [recalculando, setRecalculando] = useState(false);
  const [vistas, setVistas] = useState<BasesView[]>([]);
  /** La vista guardada que se está mirando; '' = todos los registros, en rejilla. */
  const [vistaId, setVistaId] = useState('');
  // Búsqueda y orden de un momento, encima de la vista; se leen por ref para que todas
  // las recargas (tiempo real, «cargar más») pidan lo mismo que se está viendo.
  const [lectura, setLectura] = useState<Lectura>(SIN_LECTURA);
  const lecturaRef = useUltimo(lectura);
  const turnos = useMemo(crearTurnos, []);
  const selectorVistaId = useId();
  /** El registro cuyos comentarios están abiertos en el panel lateral. */
  const [comentando, setComentando] = useState<{ id: string; nombre: string } | null>(null);
  /**
   * La ficha abierta: el registro TAL COMO SE ABRIÓ (lo que se guarda se compara con él,
   * no con el de ahora) y si se acaba de añadir.
   */
  const [ficha, setFicha] = useState<{ registro: BasesRecord; nuevo: boolean } | null>(null);
  /** Tiempo real: quién más está, si el flujo está caído y si se ha perdido el acceso. */
  const [presentes, setPresentes] = useState<BasesPresence[]>([]);
  const [sinConexion, setSinConexion] = useState(false);
  const [sinAcceso, setSinAcceso] = useState(false);
  /** Sube cuando el flujo dice que han cambiado los comentarios del registro abierto. */
  const [versionComentarios, setVersionComentarios] = useState(0);
  /** Esta pestaña, y las operaciones de celda que ha lanzado (su eco no se vuelve a pedir). */
  const [conn] = useState(idAleatorio);
  const propiasRef = useRef(new Set<string>());
  const celdaConFocoRef = useRef<{ registro: string; campo: string } | null>(null);

  const tabla = tablas.find((x) => x.id === activa);
  const rolBase = base?.role || undefined;
  const esPropietario = puedeEsquema(rolBase);
  const puedeEditar = puedeEscribir(rolTabla ?? rolBase);
  // Los scripts son de quien tiene un rol en la base y es de la organización: una persona
  // invitada de fuera (`host_org`) o con acceso solo a alguna tabla no los usa.
  const puedeScripts = Boolean(rolBase) && !base?.host_org;

  const cargarEstructura = useCallback(async () => {
    try {
      const [b, ts] = await Promise.all([cliente().getBase(baseId), cliente().listTables(baseId)]);
      setBase(b);
      setTablas(ts);
      // Un aviso de comentario trae la tabla y el registro: se abre ahí, con su hilo.
      const enlace = registroDelEnlace(window.location.search);
      const pedida = enlace && ts.some((x) => x.id === enlace.tabla) ? enlace : null;
      setActiva((a) => pedida?.tabla ?? (ts.some((x) => x.id === a) ? a : (ts[0]?.id ?? '')));
      if (pedida) setComentando((c) => c ?? { id: pedida.registro, nombre: '' });
      setError('');
    } catch (e) {
      // Quien solo accede a alguna tabla no puede leer la base (404): se queda con las tablas.
      if (e instanceof HttpError && e.status === 404) {
        try {
          const ts = await cliente().listTables(baseId);
          setTablas(ts);
          setActiva((a) => (ts.some((x) => x.id === a) ? a : (ts[0]?.id ?? '')));
          setBase({
            id: baseId,
            name: '',
            description: '',
            owner_sub: '',
            role: '',
            created_at: '',
            updated_at: '',
          });
          return;
        } catch {
          /* cae al mensaje de abajo */
        }
      }
      setError(mensajeDeError(e, t));
    }
  }, [baseId, t]);

  useEffect(() => {
    void cargarEstructura().finally(() => setCargando(false));
  }, [cargarEstructura]);

  // `sytue:app-ready`: la base con sus tablas y campos (o su fallo) ya está pintada, como
  // la lista de bases (BasesApp). Sin la marca, la bóveda esperaba 5 s para abrirse.
  useEffect(() => {
    if (!cargando) marcarAppLista();
  }, [cargando]);

  /** Carga la primera ventana ('inicio'), la que sigue ('mas') o la anterior ('antes'). */
  const cargarRegistros = useCallback(
    async function cargar(
      tablaId: string,
      vista = '',
      hacia: 'inicio' | 'mas' | 'antes' = 'inicio',
    ): Promise<void> {
      const v = hacia === 'inicio' ? SIN_VENTANAS : ventanasRef.current;
      const cursor = hacia === 'mas' ? v.siguiente : hacia === 'antes' ? cursorAnterior(v) : '';
      if (cursor === null) return;
      const turno = hacia === 'inicio' ? turnos.nuevo() : turnos.actual();
      try {
        const pag = await cliente().listRecords(tablaId, {
          limit: PAGINA,
          ...(cursor ? { cursor } : {}),
          ...(vista ? { view: vista } : {}),
          ...opcionesDeLectura(lecturaRef.current),
        });
        if (!turnos.vigente(turno)) return;
        setCampos(pag.fields);
        setRolTabla(pag.role);
        setRecalculando(pag.recalculating === true);
        const nuevas =
          hacia === 'mas' ? avanzar(v, pag) : hacia === 'antes' ? retroceder(v, pag) : empezar(pag);
        setVentanas(nuevas);
        setError('');
        if (hacia !== 'inicio') {
          const desde = nuevas.primera * PAGINA;
          setEstado(
            t('grid.window', { from: desde + 1, to: desde + nuevas.paginas.flat().length }),
          );
          return;
        }
        // Etiquetas y opciones de los campos de enlace: los registros de la tabla destino.
        for (const c of pag.fields.filter((f) => f.kind === 'link' && f.link_table_id)) {
          const dest = c.link_table_id!;
          const otra = await cliente().listRecords(dest, { limit: MAX_OPCIONES_ENLACE });
          const lista = otra.records.map((r) => ({
            id: r.id,
            etiqueta: etiquetaDeRegistro(otra.fields, r),
          }));
          setOpciones((o) => ({ ...o, [dest]: lista }));
          setEtiquetas((m) => ({ ...m, [dest]: new Map(lista.map((x) => [x.id, x.etiqueta])) }));
        }
      } catch (e) {
        if (!turnos.vigente(turno)) return; // el fallo de una lectura ya dejada atrás
        // El cursor es del orden de la vista: si la vista cambió, se vuelve a empezar.
        if (hacia !== 'inicio' && e instanceof HttpError && e.status === 422)
          return cargar(tablaId, vista);
        setError(mensajeDeError(e, t));
      }
    },
    [t, turnos, lecturaRef],
  );

  // Al cambiar de tabla: sus vistas guardadas, y se vuelve a «todos los registros».
  useEffect(() => {
    setFicha(null);
    setVistaId('');
    setLectura(SIN_LECTURA);
    setVistas([]);
    if (!activa) return;
    cliente()
      .listViews(activa)
      .then(setVistas)
      .catch(() => setVistas([]));
  }, [activa]);

  useEffect(() => {
    if (!activa) {
      setCampos([]);
      setVentanas(SIN_VENTANAS);
      return;
    }
    setVentanas(SIN_VENTANAS);
    void cargarRegistros(activa, vistaId);
  }, [activa, vistaId, lectura, cargarRegistros]);

  /**
   * Escribe UNA celda (o los enlaces de un campo) como operación con identificador: no
   * pide la versión del registro, así que dos personas en celdas distintas no se
   * estorban; en la misma gana quien escribe después y se le avisa de que ha pisado algo.
   */
  async function cambiar(
    r: BasesRecord,
    data: Record<string, BasesValue>,
    links?: Record<string, string[]>,
  ): Promise<string | null> {
    try {
      let pisado = false;
      const escribir = async (
        campo: string,
        celda: { value?: BasesValue; links?: string[]; expected?: BasesValue },
      ): Promise<void> => {
        const op = idAleatorio();
        propiasRef.current.add(op);
        const res = await cliente().writeCell(r.id, campo, celda, op);
        pisado = pisado || res.conflict;
        setRegistros((rs) => rs.map((x) => (x.id === r.id ? res.record : x)));
      };
      for (const [campo, valor] of Object.entries(data))
        await escribir(campo, { value: valor, expected: r.data[campo] ?? null });
      for (const [campo, ids] of Object.entries(links ?? {}))
        await escribir(campo, { links: ids, expected: r.links[campo] ?? [] });
      if (pisado) setEstado(t('rt.overwrote'));
      return null;
    } catch (e) {
      return mensajeDeError(e, t);
    }
  }

  /** Añade un registro con `datos` de partida (los de la columna del tablero, si viene de ahí). */
  async function nuevoRegistro(datos: Record<string, BasesValue> = {}): Promise<void> {
    try {
      const r = await cliente().createRecord(activa, datos);
      setVentanas((v) => anadir(v, r));
      setEstado(t('grid.added'));
      // Añadir es rellenar: la ficha del registro nuevo se abre sola.
      setFicha({ registro: r, nuevo: true });
    } catch (e) {
      setError(mensajeDeError(e, t));
    }
  }

  /**
   * Crea, en orden, los registros de las filas pegadas que no cabían en las que hay. No
   * abre la ficha: quien pega un bloque ya ha rellenado los valores.
   */
  async function crearVarios(filas: Record<string, BasesValue>[]): Promise<string | null> {
    try {
      for (const datos of filas) {
        const r = await cliente().createRecord(activa, datos);
        setVentanas((v) => anadir(v, r));
      }
      setEstado(t('grid.added'));
      return null;
    } catch (e) {
      return mensajeDeError(e, t);
    }
  }

  /**
   * Ficheros soltados sobre la base: un CSV abre «Importar» con él ya elegido (si se
   * puede cambiar el esquema); lo demás, soltado sobre una fila, son adjuntos de esa fila
   * en la columna de adjuntos sobre la que cae o en la primera de la tabla.
   */
  function soltarFicheros(ficheros: File[], r?: BasesRecord, col = -1): void {
    const csv = ficheros.find((f) => esCSV(f.name));
    if (csv && esPropietario) {
      setErrorDialogo('');
      setDialogo({ tipo: 'importar', fichero: csv });
      return;
    }
    const campo = r && puedeEditar ? campoDeAdjuntos(campos, col) : null;
    if (r && campo) void subirAdjuntos(r, campo, ficheros);
  }

  /** Sube los ficheros, uno tras otro, al campo de adjuntos de un registro. */
  async function subirAdjuntos(r: BasesRecord, campo: BasesField, ficheros: File[]): Promise<void> {
    setError('');
    // Se avisa ANTES de mandar nada: uno que pasa del tope no se sube ni deja a medias
    // los demás.
    if (ficheros.some((f) => f.size > MAX_BYTES_ADJUNTO)) {
      setError(t('attach.tooLarge'));
      return;
    }
    setEstado(t('attach.uploading'));
    for (const f of ficheros) {
      try {
        const nuevo = await cliente().uploadAttachment(r.id, campo.id, f);
        setRegistros((rs) =>
          rs.map((x) =>
            x.id === r.id
              ? {
                  ...x,
                  attachments: {
                    ...x.attachments,
                    [campo.id]: [...(x.attachments?.[campo.id] ?? []), nuevo],
                  },
                }
              : x,
          ),
        );
      } catch (e) {
        setError(errorDeAdjunto(e, t));
        setEstado('');
        return;
      }
    }
    setEstado(t('grid.saved'));
  }

  async function borrarRegistro(r: BasesRecord): Promise<void> {
    if (
      !(await confirmar({
        mensaje: t('grid.deleteConfirm'),
        confirmLabel: t('common.delete'),
        tone: 'danger',
      }))
    )
      return;
    try {
      await cliente().deleteRecord(r.id);
      setRegistros((rs) => rs.filter((x) => x.id !== r.id));
    } catch (e) {
      if (e instanceof HttpError && e.code === 'referenced' && esPropietario) {
        if (
          await confirmar({
            mensaje: t('grid.forceConfirm'),
            confirmLabel: t('common.delete'),
            tone: 'danger',
          })
        ) {
          try {
            await cliente().deleteRecord(r.id, true);
            setRegistros((rs) => rs.filter((x) => x.id !== r.id));
            return;
          } catch (e2) {
            setError(mensajeDeError(e2, t));
            return;
          }
        }
        return;
      }
      setError(mensajeDeError(e, t));
    }
  }

  /**
   * Lo que guarda la ficha. Se cierra ANTES de escribir, como en Fyne: el foco vuelve a
   * la fila y lo escrito viaja celda a celda con `cambiar`. Con `otro`, se añade después
   * un registro y se abre su ficha, para rellenar una tabla seguida.
   */
  async function guardarFicha(
    datos: Record<string, BasesValue>,
    enlaces: Record<string, string[]>,
    otro: boolean,
  ): Promise<void> {
    if (!ficha) return;
    const { registro } = ficha;
    setFicha(null);
    if (hayCambios({ datos, enlaces, invalidos: [] })) {
      const err = await cambiar(registro, datos, enlaces);
      if (err) {
        setError(err);
        return;
      }
    }
    if (otro) await nuevoRegistro();
  }

  async function guardarVarios(ids: string[]): Promise<void> {
    if (dialogo?.tipo !== 'varios') return;
    const { registro, campo } = dialogo;
    setDialogo(null);
    const err =
      campo.kind === 'link'
        ? await cambiar(registro, {}, { [campo.id]: ids })
        : await cambiar(registro, { [campo.id]: ids.length > 0 ? ids : null });
    if (err) setError(err);
  }

  async function crearCampo(c: BasesNewField): Promise<void> {
    setErrorDialogo('');
    try {
      await cliente().createField(activa, c);
      setDialogo(null);
      await cargarEstructura();
      await cargarRegistros(activa, vistaId);
    } catch (e) {
      setErrorDialogo(mensajeDeError(e, t));
    }
  }

  function abrirAccionCampo(accion: AccionCampo, campo: BasesField): void {
    setErrorDialogo('');
    setDialogo({ tipo: 'editarCampo', accion, campo });
  }

  /**
   * Renombrar, cambiar la fórmula o borrar un campo. El error del servicio se queda en
   * el diálogo, que sigue abierto; si sale bien se recargan la estructura (los nombres y
   * las fórmulas) y los registros (los valores recalculados o ya sin ese campo).
   */
  async function cambiarCampo(op: () => Promise<void>): Promise<void> {
    setOcupado(true);
    setErrorDialogo('');
    try {
      await op();
      setDialogo(null);
      await cargarEstructura();
      await cargarRegistros(activa, vistaId);
    } catch (e) {
      setErrorDialogo(mensajeDeError(e, t));
    } finally {
      setOcupado(false);
    }
  }

  /**
   * Mueve un campo al sitio de otro. Se pinta ya (la columna no espera a la red) y se
   * manda el orden de TODOS los campos de la tabla, no solo de los que se ven; si el
   * servicio lo rechaza (otra persona cambió los campos entretanto), se recarga.
   */
  async function moverCampo(campo: BasesField, destino: BasesField): Promise<void> {
    const todos = tablas.find((x) => x.id === activa)?.fields ?? campos;
    const orden = moverCampoEn(
      todos.map((c) => c.id),
      campo.id,
      destino.id,
    );
    const pos = new Map(orden.map((id, i) => [id, i]));
    const ordenar = (l: BasesField[]): BasesField[] =>
      [...l].sort((a, b) => (pos.get(a.id) ?? 0) - (pos.get(b.id) ?? 0));
    setCampos(ordenar);
    setTablas((ts) => ts.map((x) => (x.id === activa ? { ...x, fields: ordenar(x.fields) } : x)));
    try {
      await cliente().reorderFields(activa, orden);
    } catch (e) {
      setEstado(mensajeDeError(e, t));
      await cargarEstructura();
      await cargarRegistros(activa, vistaId);
    }
  }

  async function crearVista(
    nombre: string,
    tipo: BasesViewKind,
    cfg: BasesViewConfig,
  ): Promise<void> {
    setErrorDialogo('');
    try {
      const v = await cliente().createView(activa, { name: nombre, kind: tipo, config: cfg });
      setDialogo(null);
      setVistas(await cliente().listViews(activa));
      setVistaId(v.id);
    } catch (e) {
      setErrorDialogo(mensajeDeError(e, t));
    }
  }

  async function editarVista(v: BasesView, nombre: string, cfg: BasesViewConfig): Promise<void> {
    setErrorDialogo('');
    try {
      const nueva = await cliente().updateView(v.id, { name: nombre, config: cfg });
      setDialogo(null);
      setVistas((vs) => vs.map((x) => (x.id === nueva.id ? nueva : x)));
      // El id no cambia, así que el efecto de la vista no salta: los registros se piden
      // otra vez a mano, con los filtros y el orden nuevos.
      if (vistaId === nueva.id) void cargarRegistros(activa, nueva.id);
    } catch (e) {
      setErrorDialogo(mensajeDeError(e, t));
    }
  }

  async function borrarVista(v: BasesView): Promise<void> {
    if (
      !(await confirmar({
        mensaje: t('view.deleteConfirm', { name: v.name }),
        confirmLabel: t('common.delete'),
        tone: 'danger',
      }))
    )
      return;
    try {
      await cliente().deleteView(v.id);
      setVistaId('');
      setVistas((vs) => vs.filter((x) => x.id !== v.id));
    } catch (e) {
      setError(mensajeDeError(e, t));
    }
  }

  async function nuevaTabla(): Promise<void> {
    const nombre = (
      await preguntar({ mensaje: t('table.namePrompt'), confirmLabel: t('table.create') })
    )?.trim();
    if (!nombre) return;
    try {
      const x = await cliente().createTable(baseId, nombre);
      await cargarEstructura();
      setActiva(x.id);
    } catch (e) {
      setError(mensajeDeError(e, t));
    }
  }

  async function renombrarTabla(): Promise<void> {
    if (!tabla) return;
    const nombre = (await preguntar({ mensaje: t('table.namePrompt'), valor: tabla.name }))?.trim();
    if (!nombre || nombre === tabla.name) return;
    try {
      await cliente().renameTable(tabla.id, nombre);
      await cargarEstructura();
    } catch (e) {
      setError(mensajeDeError(e, t));
    }
  }

  async function borrarTabla(): Promise<void> {
    if (!tabla) return;
    if (
      !(await confirmar({
        mensaje: t('table.deleteConfirm', { name: tabla.name }),
        confirmLabel: t('common.delete'),
        tone: 'danger',
      }))
    )
      return;
    try {
      await cliente().deleteTable(tabla.id);
      await cargarEstructura();
    } catch (e) {
      setError(mensajeDeError(e, t));
    }
  }

  async function renombrarBase(): Promise<void> {
    if (!base) return;
    const nombre = (await preguntar({ mensaje: t('base.namePrompt'), valor: base.name }))?.trim();
    if (!nombre || nombre === base.name) return;
    try {
      await cliente().updateBase(baseId, nombre, base.description);
      await cargarEstructura();
    } catch (e) {
      setError(mensajeDeError(e, t));
    }
  }

  /** La descripción se guarda con el nombre de ahora: el servicio cambia los dos. */
  async function guardarDescripcion(descripcion: string): Promise<void> {
    if (!base) return;
    setOcupado(true);
    setErrorDialogo('');
    try {
      await cliente().updateBase(baseId, base.name, descripcion);
      setDialogo(null);
      await cargarEstructura();
    } catch (e) {
      setErrorDialogo(mensajeDeError(e, t));
    } finally {
      setOcupado(false);
    }
  }

  async function borrarBase(): Promise<void> {
    if (!base) return;
    if (
      !(await confirmar({
        mensaje: t('base.deleteConfirm', { name: base.name }),
        confirmLabel: t('common.delete'),
        tone: 'danger',
      }))
    )
      return;
    try {
      await cliente().deleteBase(baseId);
      window.location.assign(ruta('/'));
    } catch (e) {
      setError(mensajeDeError(e, t));
    }
  }

  async function importar(nombre: string, csv: string): Promise<void> {
    setOcupado(true);
    setErrorDialogo('');
    try {
      const x = await cliente().importCsv(baseId, nombre, csv);
      setDialogo(null);
      await cargarEstructura();
      setActiva(x.id);
    } catch (e) {
      setErrorDialogo(mensajeDeError(e, t));
    } finally {
      setOcupado(false);
    }
  }

  const opcionesVarias = useMemo(() => {
    if (dialogo?.tipo !== 'varios') return [];
    const { campo } = dialogo;
    return campo.kind === 'link'
      ? (opciones[campo.link_table_id ?? ''] ?? [])
      : (campo.choices ?? []).map((o) => ({ id: o, etiqueta: o }));
  }, [dialogo, opciones]);

  const vista = vistas.find((v) => v.id === vistaId);
  const campoTablero =
    vista?.kind === 'kanban'
      ? campos.find((c) => c.id === vista.config.group_by && c.kind === 'select')
      : undefined;

  // --- Tiempo real: el flujo de cambios y la presencia de esta base ---
  const enVivoRef = useRef({
    activa,
    vistaId,
    hayMas,
    comentando,
    cargarEstructura,
    cargarRegistros,
  });
  useEffect(() => {
    enVivoRef.current = { activa, vistaId, hayMas, comentando, cargarEstructura, cargarRegistros };
  });

  useEffect(() => {
    let recarga: ReturnType<typeof setTimeout> | null = null;
    // Varias cosas seguidas (una importación, un borrado en cascada) recargan UNA vez.
    const recargar = (estructura: boolean): void => {
      if (recarga) clearTimeout(recarga);
      recarga = setTimeout(() => {
        const v = enVivoRef.current;
        if (estructura) void v.cargarEstructura();
        if (v.activa) void v.cargarRegistros(v.activa, v.vistaId);
      }, 400);
    };
    let primera = true;
    const cerrar = seguir(ruta(`/api/bases/${baseId}/events`), conn, {
      alAbrir: () => {
        setSinConexion(false);
        // Lo que cambiara entre la primera carga y la apertura del flujo no lo cuenta
        // nadie: se vuelve a pedir una vez. Al reconectar ya lo cubre el replay.
        if (primera) recargar(false);
        primera = false;
      },
      alCaer: () => setSinConexion(true),
      alPresencia: setPresentes,
      alResincronizar: () => recargar(true),
      alPerderAcceso: () => {
        setSinAcceso(true);
        setPresentes([]);
      },
      alCambiar: (c) => {
        const v = enVivoRef.current;
        const plan = planDe(c, { tabla: v.activa, propias: propiasRef.current });
        switch (plan.tipo) {
          case 'estructura':
            recargar(true);
            break;
          case 'quitar':
            setVentanas((w) => editar(w, (rs) => rs.filter((x) => x.id !== plan.id)));
            break;
          case 'comentarios':
            if (v.comentando?.id === plan.registro) setVersionComentarios((n) => n + 1);
            break;
          case 'registro':
            // Con una vista (filtro u orden) un alta puede no tocar aquí o ir a otro sitio: se recarga.
            if (plan.nuevo && (v.vistaId || v.hayMas || hayLectura(lecturaRef.current))) {
              if (v.vistaId || hayLectura(lecturaRef.current)) recargar(false);
              break;
            }
            cliente()
              .getRecord(plan.id)
              .then((r) =>
                setVentanas((w) =>
                  w.paginas.some((pg) => pg.some((x) => x.id === r.id))
                    ? editar(w, (rs) => rs.map((x) => (x.id === r.id ? r : x)))
                    : plan.nuevo
                      ? anadir(w, r)
                      : w,
                ),
              )
              .catch(() => undefined);
            break;
          default:
        }
      },
    });
    // Latido: que los demás sepan que esta pestaña sigue aquí, y en qué celda.
    const latir = (): void => {
      const f = celdaConFocoRef.current;
      const v = enVivoRef.current;
      void cliente()
        .heartbeat(baseId, {
          conn,
          ...(v.activa ? { table_id: v.activa } : {}),
          ...(f && v.activa ? { record_id: f.registro, field_id: f.campo } : {}),
        })
        .catch(() => undefined);
    };
    const reloj = setInterval(latir, 10_000);
    const alEnfocar = (): void => latir();
    window.addEventListener('bases:foco', alEnfocar);
    return () => {
      cerrar();
      clearInterval(reloj);
      if (recarga) clearTimeout(recarga);
      window.removeEventListener('bases:foco', alEnfocar);
    };
  }, [baseId, conn, lecturaRef]);

  // La celda con el foco se dice a los demás sin esperar al latido (con un respiro, para
  // no mandar una petición por cada flecha).
  const avisoDeFocoRef = useRef<ReturnType<typeof setTimeout> | null>(null);
  const alEnfocarCelda = (r: BasesRecord, campo: BasesField): void => {
    celdaConFocoRef.current = { registro: r.id, campo: campo.id };
    if (avisoDeFocoRef.current) clearTimeout(avisoDeFocoRef.current);
    avisoDeFocoRef.current = setTimeout(() => window.dispatchEvent(new Event('bases:foco')), 300);
  };

  const enCeldas = useMemo(
    () => presentesPorCelda(presentes, activa, conn),
    [presentes, activa, conn],
  );
  const personas = useMemo(() => personasPresentes(presentes, conn), [presentes, conn]);

  const comentar = (r: BasesRecord) =>
    setComentando({ id: r.id, nombre: etiquetaDeRegistro(campos, r) });
  const abrirFicha = (r: BasesRecord) => setFicha({ registro: r, nuevo: false });
  /** El registro de la ficha como está ahora (adjuntos recién subidos, cambios de otros). */
  const fichaActual = ficha
    ? (registros.find((r) => r.id === ficha.registro.id) ?? ficha.registro)
    : null;
  const filaFicha = ficha
    ? ventanas.primera * PAGINA +
      (registros.findIndex((r) => r.id === ficha.registro.id) + 1 || registros.length + 1)
    : 0;

  const exportar = (formato: 'csv' | 'json') =>
    ruta(`/api/tables/${activa}/export?format=${formato}`);
  const nombreBase = base?.name || t('nav.title');
  const accionesBase = `${t('actions.more')} · ${nombreBase}`;
  const accionesTabla = `${t('actions.more')} · ${tabla?.name ?? t('base.tables')}`;
  const nombreVista = vista
    ? `${vista.name} · ${t(`view.kind.${vista.kind}`)}`
    : `${t('view.kind.grid')} · ${t('view.all')}`;
  // «Ver todos los registros» desde un recorte vacío: fuera la búsqueda y, si la vista
  // filtra, a «todos». El orden elegido se queda: no esconde nada.
  const verTodos = (): void => {
    setLectura((l) => ({ ...l, q: '' }));
    if ((vista?.config.filters ?? []).length > 0) setVistaId('');
  };
  const elegirVista = (id: string, cerrar: () => void): void => {
    cerrar();
    setVistaId(id);
    document.getElementById(selectorVistaId)?.focus({ preventScroll: true });
  };

  return (
    <Marco user={user} hubBaseUrl={hubBaseUrl} locale={locale} activa="bases">
      {/* Un CSV soltado en cualquier sitio de la base abre «Importar»; lo soltado sobre
          una fila de la rejilla lo recoge ella antes (adjuntos). Sin esto, el navegador
          abriría el fichero y se perdería la página. */}
      <div
        class="flex min-w-0 flex-col gap-2 p-4 sm:gap-3 md:p-6"
        onDragOver={(e: DragEvent) => {
          // Se para SIEMPRE que lleve ficheros, también para quien no puede importar:
          // soltarlos donde no se aceptan no debe sacarle de la página.
          if (!llevaFicheros(e.dataTransfer)) return;
          e.preventDefault();
          if (e.dataTransfer) e.dataTransfer.dropEffect = esPropietario ? 'copy' : 'none';
        }}
        onDrop={(e: DragEvent) => {
          if (!llevaFicheros(e.dataTransfer)) return;
          e.preventDefault();
          if (esPropietario) soltarFicheros(Array.from(e.dataTransfer?.files ?? []));
        }}
      >
        <div class="border-border-subtle flex min-w-0 items-center justify-between gap-2 border-b pb-3">
          <div class="flex min-w-0 flex-1 items-center gap-2">
            <IconButton href={ruta('/')} label={t('base.back')}>
              <Icon name="chevronLeft" size={18} />
            </IconButton>
            <CardIcon tone={tonoDeBase(baseId)} class="hidden h-9 w-9 shrink-0 rounded-lg sm:flex">
              <Icon name="database" size={20} />
            </CardIcon>
            <h1
              class="text-text line-clamp-2 min-w-0 text-lg font-semibold sm:truncate sm:text-2xl"
              title={nombreBase}
            >
              {nombreBase}
            </h1>
          </div>
          {puedeScripts || esPropietario ? (
            <div class="ml-auto flex shrink-0 items-center gap-2">
              {esPropietario ? (
                <Button
                  variant="secondary"
                  size="sm"
                  aria-label={t('share.open')}
                  title={t('share.open')}
                  leadingIcon={<Icon name="users" size={15} />}
                  onClick={() => setDialogo({ tipo: 'compartir' })}
                >
                  <span class="hidden sm:inline">{t('share.open')}</span>
                </Button>
              ) : null}
              <Dropdown
                label={accionesBase}
                trigger={(props) => (
                  <IconButton {...props} label={accionesBase}>
                    <Icon name="ellipsis" size={18} />
                  </IconButton>
                )}
              >
                {(cerrar) => (
                  <>
                    {puedeScripts ? (
                      <MenuItem
                        icon={<Icon name="code" size={16} />}
                        onClick={() => {
                          cerrar();
                          setDialogo({ tipo: 'scripts' });
                        }}
                      >
                        {t('scripts.open')}
                      </MenuItem>
                    ) : null}
                    {esPropietario ? (
                      <>
                        {tabla ? (
                          <MenuItem
                            onClick={() => {
                              cerrar();
                              setDialogo({ tipo: 'conexiones' });
                            }}
                          >
                            {t('conn.open')}
                          </MenuItem>
                        ) : null}
                        <MenuSeparator />
                        <MenuItem
                          onClick={() => {
                            cerrar();
                            void renombrarBase();
                          }}
                        >
                          {t('base.rename')}
                        </MenuItem>
                        <MenuItem
                          onClick={() => {
                            cerrar();
                            setErrorDialogo('');
                            setDialogo({ tipo: 'descripcion' });
                          }}
                        >
                          {t('base.editDescription')}
                        </MenuItem>
                        <MenuItem
                          icon={<Icon name="clock" size={16} />}
                          onClick={() => {
                            cerrar();
                            setDialogo({ tipo: 'actividad' });
                          }}
                        >
                          {t('activity.open')}
                        </MenuItem>
                        <MenuItem
                          danger
                          onClick={() => {
                            cerrar();
                            void borrarBase();
                          }}
                        >
                          {t('base.delete')}
                        </MenuItem>
                      </>
                    ) : null}
                  </>
                )}
              </Dropdown>
            </div>
          ) : null}
        </div>
        {base?.description ? (
          <p class="text-text-muted line-clamp-3 max-w-3xl text-sm whitespace-pre-line">
            {base.description}
          </p>
        ) : null}

        {error ? (
          <p role="alert" class="text-danger-text text-sm">
            {error}
          </p>
        ) : null}
        {sinAcceso ? (
          <p role="alert" class="text-danger-text text-sm">
            {t('rt.lostAccess')}
          </p>
        ) : sinConexion ? (
          <p role="status" class="text-text-muted text-sm">
            {t('rt.offline')}
          </p>
        ) : null}
        {personas.length > 0 ? (
          <ul aria-label={t('rt.present')} class="flex flex-wrap items-center gap-1">
            {personas.map((nombre) => (
              <li key={nombre}>
                <Avatar name={nombre} size="xs" />
                <span class="sr-only">{nombre}</span>
              </li>
            ))}
          </ul>
        ) : null}

        {cargando ? (
          <Skeleton height="12rem" />
        ) : tablas.length === 0 ? (
          <VacioGuiado
            t={t}
            vacio={vacioDe('sinTablas', { esquema: esPropietario, escribir: puedeEditar })}
            acciones={{
              crearTabla: () => void nuevaTabla(),
              importarCsv: () => setDialogo({ tipo: 'importar' }),
            }}
          />
        ) : (
          <Tabs
            class="min-w-0"
            label={t('base.tables')}
            value={activa}
            onChange={setActiva}
            items={tablas.map((x) => ({
              id: x.id,
              label: x.name,
              icon: <Icon name="table" size={16} />,
              content:
                x.id === activa ? (
                  <div class="flex min-w-0 flex-col gap-3">
                    <div class="flex min-w-0 items-center gap-2">
                      <Dropdown
                        class="min-w-0 flex-1 sm:max-w-72"
                        align="start"
                        label={t('view.list')}
                        trigger={(props) => (
                          <Button
                            {...props}
                            id={selectorVistaId}
                            variant="secondary"
                            size="sm"
                            class="w-full min-w-0 justify-between"
                            aria-label={`${t('view.list')}: ${nombreVista}`}
                            title={nombreVista}
                            trailingIcon={<Icon name="chevronDown" size={14} />}
                          >
                            <span class="min-w-0 truncate">
                              {vista?.name || t('view.kind.grid')}
                            </span>
                          </Button>
                        )}
                      >
                        {(cerrar) => (
                          <>
                            <MenuItem
                              aria-current={vistaId === '' ? 'true' : undefined}
                              onClick={() => elegirVista('', cerrar)}
                            >
                              {t('view.all')}
                            </MenuItem>
                            {vistas.map((v) => (
                              <MenuItem
                                key={v.id}
                                aria-current={vistaId === v.id ? 'true' : undefined}
                                onClick={() => elegirVista(v.id, cerrar)}
                              >
                                {v.name} · {t(`view.kind.${v.kind}`)}
                              </MenuItem>
                            ))}
                            {puedeEditar ? (
                              <>
                                <MenuSeparator />
                                <MenuItem
                                  onClick={() => {
                                    cerrar();
                                    setDialogo({ tipo: 'vista' });
                                  }}
                                >
                                  {t('view.add')}
                                </MenuItem>
                                {vista ? (
                                  <MenuItem
                                    onClick={() => {
                                      cerrar();
                                      setErrorDialogo('');
                                      setDialogo({ tipo: 'editarVista', vista });
                                    }}
                                  >
                                    {t('view.edit')}
                                  </MenuItem>
                                ) : null}
                                {vista ? (
                                  <MenuItem
                                    danger
                                    onClick={() => {
                                      cerrar();
                                      void borrarVista(vista);
                                    }}
                                  >
                                    {t('view.delete')}
                                  </MenuItem>
                                ) : null}
                              </>
                            ) : null}
                          </>
                        )}
                      </Dropdown>
                      {puedeEditar && vista?.kind !== 'form' ? (
                        <Button
                          variant="primary"
                          size="sm"
                          class="max-w-[50%] min-w-0 shrink-0 sm:max-w-none"
                          disabled={ocupado || sinAcceso}
                          aria-label={t('grid.addRecord')}
                          title={t('grid.addRecord')}
                          leadingIcon={<Icon name="plus" size={14} />}
                          onClick={() => void nuevoRegistro()}
                        >
                          <span class="min-w-0 truncate">{t('grid.addRecord')}</span>
                        </Button>
                      ) : null}
                      <Dropdown
                        class="shrink-0"
                        label={accionesTabla}
                        trigger={(props) => (
                          <IconButton {...props} label={accionesTabla}>
                            <Icon name="ellipsis" size={18} />
                          </IconButton>
                        )}
                      >
                        {(cerrar) => (
                          <>
                            {esPropietario ? (
                              <>
                                <MenuItem
                                  icon={<Icon name="plus" size={16} />}
                                  onClick={() => {
                                    cerrar();
                                    setDialogo({ tipo: 'campo' });
                                  }}
                                >
                                  {t('field.add')}
                                </MenuItem>
                                <MenuItem
                                  onClick={() => {
                                    cerrar();
                                    void renombrarTabla();
                                  }}
                                >
                                  {t('table.rename')}
                                </MenuItem>
                                <MenuItem
                                  danger
                                  onClick={() => {
                                    cerrar();
                                    void borrarTabla();
                                  }}
                                >
                                  {t('table.delete')}
                                </MenuItem>
                                <MenuSeparator />
                              </>
                            ) : null}
                            <MenuItem
                              href={exportar('csv')}
                              download
                              icon={<Icon name="download" size={16} />}
                              onClick={cerrar}
                            >
                              {t('export.csv')}
                            </MenuItem>
                            <MenuItem
                              href={exportar('json')}
                              download
                              icon={<Icon name="download" size={16} />}
                              onClick={cerrar}
                            >
                              {t('export.json')}
                            </MenuItem>
                            {esPropietario ? (
                              <>
                                <MenuSeparator />
                                <MenuItem
                                  onClick={() => {
                                    cerrar();
                                    void nuevaTabla();
                                  }}
                                >
                                  {t('table.add')}
                                </MenuItem>
                                <MenuItem
                                  onClick={() => {
                                    cerrar();
                                    setDialogo({ tipo: 'importar' });
                                  }}
                                >
                                  {t('import.open')}
                                </MenuItem>
                              </>
                            ) : null}
                          </>
                        )}
                      </Dropdown>
                    </div>
                    {vista?.kind === 'form' ? (
                      <div class="flex flex-col gap-3">
                        <p class="text-text-muted text-sm">{t('form.preview')}</p>
                        <ul class="text-text list-disc pl-5 text-sm">
                          {campos
                            .filter((c) =>
                              (vista.config.fields ?? []).length > 0
                                ? vista.config.fields!.includes(c.id)
                                : c.kind !== 'link' &&
                                  c.kind !== 'formula' &&
                                  c.kind !== 'attachment',
                            )
                            .map((c) => (
                              <li key={c.id}>
                                {c.name} ({t(`field.kind.${c.kind}`)})
                              </li>
                            ))}
                        </ul>
                        {esPropietario ? (
                          <PanelFormulario t={t} locale={locale} vista={vista.id} />
                        ) : null}
                      </div>
                    ) : campoTablero || vista?.kind === 'gallery' ? (
                      <>
                        {/* La búsqueda y el orden siguen al cambiar de vista: el tablero
                            y la galería también llevan la barra, o filtrarían sin enseñarlo. */}
                        <BarraLectura
                          t={t}
                          campos={campos}
                          lectura={lectura}
                          onCambiar={setLectura}
                        />
                        {campoTablero ? (
                          <Tablero
                            t={t}
                            campos={campos}
                            agrupar={campoTablero}
                            registros={registros}
                            puedeEditar={puedeEditar}
                            onMover={(r, valor) => cambiar(r, { [campoTablero.id]: valor })}
                            onAbrir={abrirFicha}
                            onAnadir={(valor) =>
                              void nuevoRegistro(datosDeColumna(campoTablero, valor))
                            }
                            onComentar={comentar}
                            presentes={enCeldas}
                          />
                        ) : (
                          <Galeria
                            t={t}
                            campos={campos}
                            registros={registros}
                            puedeEditar={puedeEditar}
                            onAbrir={abrirFicha}
                            onComentar={comentar}
                            presentes={enCeldas}
                          />
                        )}
                        <div class="flex flex-wrap gap-2">
                          {hayAntes ? (
                            <Button
                              variant="ghost"
                              size="sm"
                              onClick={() => void cargarRegistros(activa, vistaId, 'antes')}
                            >
                              {t('grid.loadPrevious')}
                            </Button>
                          ) : null}
                          {hayMas ? (
                            <Button
                              variant="ghost"
                              size="sm"
                              onClick={() => void cargarRegistros(activa, vistaId, 'mas')}
                            >
                              {t('grid.loadMore')}
                            </Button>
                          ) : null}
                        </div>
                      </>
                    ) : (
                      <>
                        <BarraLectura
                          t={t}
                          campos={campos}
                          lectura={lectura}
                          onCambiar={setLectura}
                        />
                        <Rejilla
                          t={t}
                          nombreTabla={x.name}
                          campos={campos}
                          registros={registros}
                          puedeEditar={puedeEditar}
                          {...(lectura.q.trim()
                            ? { recorte: 'busqueda' as const }
                            : esRecorte(lectura, vista)
                              ? { recorte: 'vista' as const }
                              : {})}
                          onVerTodos={verTodos}
                          etiquetas={etiquetas}
                          onCambiar={cambiar}
                          onElegirVarios={(registro, campo) =>
                            setDialogo({ tipo: 'varios', registro, campo })
                          }
                          onAdjuntos={(registro, campo) =>
                            setDialogo({ tipo: 'adjuntos', registro: registro.id, campo })
                          }
                          onAbrir={abrirFicha}
                          onComentar={comentar}
                          presentes={enCeldas}
                          onFoco={alEnfocarCelda}
                          onNuevo={() => void nuevoRegistro()}
                          onCrearVarios={crearVarios}
                          onSoltarFicheros={soltarFicheros}
                          {...(esPropietario
                            ? {
                                onCampo: abrirAccionCampo,
                                ...((vista?.config.fields ?? []).length === 0
                                  ? {
                                      onMoverCampo: (c: BasesField, d: BasesField) =>
                                        void moverCampo(c, d),
                                    }
                                  : {}),
                                onNuevoCampo: () => {
                                  setErrorDialogo('');
                                  setDialogo({ tipo: 'campo' });
                                },
                              }
                            : {})}
                          desde={ventanas.primera * PAGINA}
                          hayAntes={hayAntes}
                          onAntes={() => void cargarRegistros(activa, vistaId, 'antes')}
                          hayMas={hayMas}
                          onMas={() => void cargarRegistros(activa, vistaId, 'mas')}
                        />
                      </>
                    )}
                  </div>
                ) : null,
            }))}
          />
        )}
        {recalculando ? (
          <p class="text-text-muted text-sm" role="status">
            {t('grid.recalculating')}
          </p>
        ) : null}
        <p class="sr-only" role="status" aria-live="polite">
          {estado}
        </p>
      </div>

      {dialogo?.tipo === 'campo' ? (
        <DialogoCampo
          t={t}
          otras={tablas.filter((x) => x.id !== activa)}
          campos={campos}
          tablas={tablas}
          error={errorDialogo}
          onCrear={(c) => void crearCampo(c)}
          onCerrar={() => setDialogo(null)}
        />
      ) : null}
      {dialogo?.tipo === 'editarCampo' && dialogo.accion === 'renombrar' ? (
        <DialogoRenombrarCampo
          t={t}
          campo={dialogo.campo}
          error={errorDialogo}
          ocupado={ocupado}
          onGuardar={(nombre) =>
            void cambiarCampo(() => cliente().renameField(dialogo.campo.id, nombre))
          }
          onCerrar={() => setDialogo(null)}
        />
      ) : null}
      {dialogo?.tipo === 'editarCampo' && dialogo.accion === 'formula' ? (
        <DialogoFormulaCampo
          t={t}
          campo={dialogo.campo}
          campos={campos}
          tablas={tablas}
          error={errorDialogo}
          ocupado={ocupado}
          onGuardar={(formula) =>
            void cambiarCampo(() => cliente().setFieldFormula(dialogo.campo.id, formula))
          }
          onCerrar={() => setDialogo(null)}
        />
      ) : null}
      {dialogo?.tipo === 'editarCampo' && dialogo.accion === 'opciones' ? (
        <DialogoOpcionesCampo
          t={t}
          campo={dialogo.campo}
          error={errorDialogo}
          ocupado={ocupado}
          onGuardar={(opciones) =>
            void cambiarCampo(() => cliente().setFieldChoices(dialogo.campo.id, opciones))
          }
          onCerrar={() => setDialogo(null)}
        />
      ) : null}
      {dialogo?.tipo === 'editarCampo' && dialogo.accion === 'borrar' ? (
        <DialogoBorrarCampo
          t={t}
          campo={dialogo.campo}
          tablas={tablas}
          error={errorDialogo}
          ocupado={ocupado}
          onBorrar={() => void cambiarCampo(() => cliente().deleteField(dialogo.campo.id))}
          onCerrar={() => setDialogo(null)}
        />
      ) : null}
      {dialogo?.tipo === 'compartir' ? (
        <DialogoCompartir
          t={t}
          baseId={baseId}
          nombreBase={base?.name ?? ''}
          tenant={user.tenant ?? ''}
          onCerrar={() => setDialogo(null)}
          onTransferida={() => window.location.reload()}
        />
      ) : null}
      {dialogo?.tipo === 'vista' ? (
        <DialogoVista
          t={t}
          campos={campos}
          error={errorDialogo}
          onCrear={(n, k, c) => void crearVista(n, k, c)}
          onCerrar={() => setDialogo(null)}
        />
      ) : null}
      {dialogo?.tipo === 'editarVista' ? (
        <DialogoVista
          t={t}
          campos={campos}
          error={errorDialogo}
          vista={dialogo.vista}
          onCrear={(n, _k, c) => void editarVista(dialogo.vista, n, c)}
          onCerrar={() => setDialogo(null)}
        />
      ) : null}
      {dialogo?.tipo === 'conexiones' && tabla ? (
        <DialogoConexiones
          t={t}
          locale={locale}
          baseId={baseId}
          tabla={tabla}
          vistas={vistas.filter((v) => v.kind !== 'form')}
          onCerrar={() => setDialogo(null)}
        />
      ) : null}
      {dialogo?.tipo === 'scripts' ? (
        <PanelScripts
          t={t}
          locale={locale}
          baseId={baseId}
          sub={user.id}
          tablas={tablas}
          onCambio={() => {
            if (activa) void cargarRegistros(activa, vistaId);
          }}
          onCerrar={() => setDialogo(null)}
        />
      ) : null}
      {dialogo?.tipo === 'actividad' ? (
        <PanelActividad
          t={t}
          locale={locale}
          baseId={baseId}
          nombre={nombreBase}
          yo={user.id}
          tablas={tablas}
          onCerrar={() => setDialogo(null)}
        />
      ) : null}
      {dialogo?.tipo === 'descripcion' && base ? (
        <DialogoDescripcion
          t={t}
          valor={base.description}
          ocupado={ocupado}
          error={errorDialogo}
          onGuardar={(d) => void guardarDescripcion(d)}
          onCerrar={() => setDialogo(null)}
        />
      ) : null}
      {dialogo?.tipo === 'importar' ? (
        <DialogoImportar
          t={t}
          {...(dialogo.fichero ? { fichero: dialogo.fichero } : {})}
          error={errorDialogo}
          ocupado={ocupado}
          onImportar={(n, c) => void importar(n, c)}
          onCerrar={() => setDialogo(null)}
        />
      ) : null}
      {ficha && fichaActual ? (
        <FichaRegistro
          key={ficha.registro.id}
          t={t}
          campos={campos}
          registro={ficha.registro}
          actual={fichaActual}
          n={filaFicha}
          nuevo={ficha.nuevo}
          puedeEditar={puedeEditar && !sinAcceso}
          opciones={opciones}
          etiquetas={etiquetas}
          onGuardar={(datos, enlaces, otro) => void guardarFicha(datos, enlaces, otro)}
          onBorrar={() => {
            const r = ficha.registro;
            setFicha(null);
            void borrarRegistro(r);
          }}
          onAdjuntos={(campo) =>
            setDialogo({ tipo: 'adjuntos', registro: ficha.registro.id, campo })
          }
          onComentar={() => comentar(fichaActual)}
          onCerrar={() => setFicha(null)}
        />
      ) : null}
      {dialogo?.tipo === 'adjuntos' && registros.some((r) => r.id === dialogo.registro) ? (
        <DialogoAdjuntos
          t={t}
          locale={locale}
          registro={registros.find((r) => r.id === dialogo.registro)!}
          campo={dialogo.campo}
          puedeEditar={puedeEditar}
          onCambio={(adjuntos) =>
            setRegistros((rs) =>
              rs.map((x) =>
                x.id === dialogo.registro
                  ? { ...x, attachments: { ...x.attachments, [dialogo.campo.id]: adjuntos } }
                  : x,
              ),
            )
          }
          onCerrar={() => setDialogo(null)}
        />
      ) : null}
      {dialogo?.tipo === 'varios' ? (
        <DialogoVarios
          t={t}
          titulo={dialogo.campo.name}
          opciones={opcionesVarias}
          elegidos={
            dialogo.campo.kind === 'link'
              ? enlacesDe(dialogo.registro, dialogo.campo)
              : ((dialogo.registro.data[dialogo.campo.id] as string[] | undefined) ?? [])
          }
          onGuardar={(ids) => void guardarVarios(ids)}
          onCerrar={() => setDialogo(null)}
        />
      ) : null}
      {comentando ? (
        <PanelComentarios
          key={`${comentando.id}:${versionComentarios}`}
          t={t}
          locale={locale}
          registro={comentando.id}
          nombre={
            comentando.nombre ||
            (registros.find((r) => r.id === comentando.id)
              ? etiquetaDeRegistro(
                  campos,
                  registros.find((r) => r.id === comentando.id)!,
                )
              : t('nav.title'))
          }
          puedeComentar={puedeComentar(rolTabla ?? rolBase)}
          onCerrar={() => setComentando(null)}
        />
      ) : null}
      {dialogoConfirmar}
      {dialogoPreguntar}
    </Marco>
  );
}
