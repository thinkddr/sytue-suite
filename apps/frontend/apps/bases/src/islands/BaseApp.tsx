/*
 * SPDX-License-Identifier: Apache-2.0
 * SPDX-FileCopyrightText: (c) 2026 Daniel Delgado Ramos
 *
 * Una base abierta: sus tablas en pestañas, la rejilla editable, y —según el rol— crear
 * campos y tablas, importar un CSV, exportar y compartir. El rol que llega del servidor
 * solo decide qué se PINTA; cada operación la vuelve a autorizar el servicio.
 */
import type { JSX } from 'preact';
import { useCallback, useEffect, useMemo, useRef, useState } from 'preact/hooks';
import { Avatar, Button, Skeleton, Tabs, useConfirm, usePrompt } from '@sytue/ui';
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
import { DialogoCampo, DialogoImportar, DialogoVarios } from '../components/Dialogos.js';
import { DialogoCompartir } from '../components/DialogoCompartir.js';
import { DialogoAdjuntos } from '../components/DialogoAdjuntos.js';
import { DialogoConexiones } from '../components/DialogoConexiones.js';
import { DialogoVista } from '../components/DialogoVista.js';
import { Marco } from '../components/Marco.js';
import { PanelComentarios } from '../components/PanelComentarios.js';
import { PanelFormulario } from '../components/PanelFormulario.js';
import { PanelScripts } from '../components/PanelScripts.js';
import { Rejilla } from '../components/Rejilla.js';
import { Tablero } from '../components/Tablero.js';
import { useT } from '../i18n/index.js';
import { cliente, mensajeDeError } from '../lib/api.js';
import { registroDelEnlace } from '../lib/comentarios.js';
import { MAX_OPCIONES_ENLACE, PAGINA } from '../lib/limites.js';
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
  | { tipo: 'compartir' }
  | { tipo: 'importar' }
  | { tipo: 'vista' }
  | { tipo: 'conexiones' }
  | { tipo: 'scripts' }
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
  /** El registro cuyos comentarios están abiertos en el panel lateral. */
  const [comentando, setComentando] = useState<{ id: string; nombre: string } | null>(null);
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
      try {
        const pag = await cliente().listRecords(tablaId, {
          limit: PAGINA,
          ...(cursor ? { cursor } : {}),
          ...(vista ? { view: vista } : {}),
        });
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
        // El cursor es del orden de la vista: si la vista cambió, se vuelve a empezar.
        if (hacia !== 'inicio' && e instanceof HttpError && e.status === 422)
          return cargar(tablaId, vista);
        setError(mensajeDeError(e, t));
      }
    },
    [t],
  );

  // Al cambiar de tabla: sus vistas guardadas, y se vuelve a «todos los registros».
  useEffect(() => {
    setVistaId('');
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
  }, [activa, vistaId, cargarRegistros]);

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

  async function nuevoRegistro(): Promise<void> {
    try {
      const r = await cliente().createRecord(activa, {});
      setVentanas((v) => anadir(v, r));
      setEstado(t('grid.added'));
    } catch (e) {
      setError(mensajeDeError(e, t));
    }
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
            if (plan.nuevo && (v.vistaId || v.hayMas)) {
              if (v.vistaId) recargar(false);
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
  }, [baseId, conn]);

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

  const exportar = (formato: 'csv' | 'json') =>
    ruta(`/api/tables/${activa}/export?format=${formato}`);

  return (
    <Marco user={user} hubBaseUrl={hubBaseUrl} locale={locale} activa="bases">
      <div class="flex flex-col gap-4 p-4 md:p-6">
        <div class="flex flex-wrap items-center justify-between gap-2">
          <div class="flex min-w-0 items-center gap-3">
            <a
              href={ruta('/')}
              class="text-primary-text sytue-focus-ring rounded-sm text-sm underline"
            >
              {t('base.back')}
            </a>
            <h1 class="text-text min-w-0 truncate text-2xl font-semibold">
              {base?.name || t('nav.title')}
            </h1>
          </div>
          {puedeScripts || esPropietario ? (
            <div class="flex flex-wrap gap-2">
              {puedeScripts ? (
                <Button
                  variant="secondary"
                  size="sm"
                  onClick={() => setDialogo({ tipo: 'scripts' })}
                >
                  {t('scripts.open')}
                </Button>
              ) : null}
              {esPropietario ? (
                <>
                  <Button
                    variant="secondary"
                    size="sm"
                    onClick={() => setDialogo({ tipo: 'compartir' })}
                  >
                    {t('share.open')}
                  </Button>
                  {tabla ? (
                    <Button
                      variant="secondary"
                      size="sm"
                      onClick={() => setDialogo({ tipo: 'conexiones' })}
                    >
                      {t('conn.open')}
                    </Button>
                  ) : null}
                  <Button variant="ghost" size="sm" onClick={() => void renombrarBase()}>
                    {t('base.rename')}
                  </Button>
                  <Button variant="ghost" size="sm" onClick={() => void borrarBase()}>
                    {t('base.delete')}
                  </Button>
                </>
              ) : null}
            </div>
          ) : null}
        </div>

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
          <div class="flex flex-col items-start gap-3">
            <p class="text-text-muted text-sm">{t('base.noTables')}</p>
            {esPropietario ? (
              <div class="flex gap-2">
                <Button onClick={() => void nuevaTabla()}>{t('table.add')}</Button>
                <Button variant="secondary" onClick={() => setDialogo({ tipo: 'importar' })}>
                  {t('import.open')}
                </Button>
              </div>
            ) : null}
          </div>
        ) : (
          <>
            <Tabs
              label={t('base.tables')}
              value={activa}
              onChange={setActiva}
              items={tablas.map((x) => ({
                id: x.id,
                label: x.name,
                content:
                  x.id === activa ? (
                    <div class="flex flex-col gap-3">
                      <div class="flex flex-wrap items-center gap-2">
                        {esPropietario ? (
                          <>
                            <Button
                              variant="secondary"
                              size="sm"
                              onClick={() => setDialogo({ tipo: 'campo' })}
                            >
                              {t('field.add')}
                            </Button>
                            <Button variant="ghost" size="sm" onClick={() => void renombrarTabla()}>
                              {t('table.rename')}
                            </Button>
                            <Button variant="ghost" size="sm" onClick={() => void borrarTabla()}>
                              {t('table.delete')}
                            </Button>
                          </>
                        ) : null}
                        <a
                          href={exportar('csv')}
                          download
                          class="text-primary-text sytue-focus-ring rounded-sm px-2 text-sm underline"
                        >
                          {t('export.csv')}
                        </a>
                        <a
                          href={exportar('json')}
                          download
                          class="text-primary-text sytue-focus-ring rounded-sm px-2 text-sm underline"
                        >
                          {t('export.json')}
                        </a>
                      </div>
                      <div
                        role="group"
                        aria-label={t('view.list')}
                        class="flex flex-wrap items-center gap-2"
                      >
                        <Button
                          variant={vistaId === '' ? 'secondary' : 'ghost'}
                          size="sm"
                          aria-pressed={vistaId === ''}
                          onClick={() => setVistaId('')}
                        >
                          {t('view.all')}
                        </Button>
                        {vistas.map((v) => (
                          <Button
                            key={v.id}
                            variant={vistaId === v.id ? 'secondary' : 'ghost'}
                            size="sm"
                            aria-pressed={vistaId === v.id}
                            onClick={() => setVistaId(v.id)}
                          >
                            {v.name} · {t(`view.kind.${v.kind}`)}
                          </Button>
                        ))}
                        {puedeEditar ? (
                          <Button
                            variant="ghost"
                            size="sm"
                            onClick={() => setDialogo({ tipo: 'vista' })}
                          >
                            {t('view.add')}
                          </Button>
                        ) : null}
                        {puedeEditar && vista ? (
                          <Button variant="ghost" size="sm" onClick={() => void borrarVista(vista)}>
                            {t('view.delete')}
                          </Button>
                        ) : null}
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
                      ) : campoTablero ? (
                        <>
                          <Tablero
                            t={t}
                            campos={campos}
                            agrupar={campoTablero}
                            registros={registros}
                            puedeEditar={puedeEditar}
                            onMover={(r, valor) => cambiar(r, { [campoTablero.id]: valor })}
                            onComentar={comentar}
                            presentes={enCeldas}
                          />
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
                        <Rejilla
                          t={t}
                          nombreTabla={x.name}
                          campos={campos}
                          registros={registros}
                          puedeEditar={puedeEditar}
                          etiquetas={etiquetas}
                          onCambiar={cambiar}
                          onElegirVarios={(registro, campo) =>
                            setDialogo({ tipo: 'varios', registro, campo })
                          }
                          onAdjuntos={(registro, campo) =>
                            setDialogo({ tipo: 'adjuntos', registro: registro.id, campo })
                          }
                          onBorrar={(r) => void borrarRegistro(r)}
                          onComentar={comentar}
                          presentes={enCeldas}
                          onFoco={alEnfocarCelda}
                          onNuevo={() => void nuevoRegistro()}
                          desde={ventanas.primera * PAGINA}
                          hayAntes={hayAntes}
                          onAntes={() => void cargarRegistros(activa, vistaId, 'antes')}
                          hayMas={hayMas}
                          onMas={() => void cargarRegistros(activa, vistaId, 'mas')}
                        />
                      )}
                    </div>
                  ) : null,
              }))}
            />
            {esPropietario ? (
              <div class="flex flex-wrap gap-2">
                <Button variant="ghost" size="sm" onClick={() => void nuevaTabla()}>
                  {t('table.add')}
                </Button>
                <Button variant="ghost" size="sm" onClick={() => setDialogo({ tipo: 'importar' })}>
                  {t('import.open')}
                </Button>
              </div>
            ) : null}
          </>
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
      {dialogo?.tipo === 'importar' ? (
        <DialogoImportar
          t={t}
          error={errorDialogo}
          ocupado={ocupado}
          onImportar={(n, c) => void importar(n, c)}
          onCerrar={() => setDialogo(null)}
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
