/*
 * SPDX-License-Identifier: Apache-2.0
 * SPDX-FileCopyrightText: (c) 2026 Daniel Delgado Ramos
 *
 * La rejilla editable de una tabla de Bases, con el patrón «grid» de WAI-ARIA: una sola
 * celda en el orden de tabulación (foco itinerante), flechas/Inicio/Fin/RePág/AvPág
 * para moverse (`lib/rejilla.ts`), Intro o F2 o escribir para editar, Escape para
 * cancelar y Espacio para marcar una casilla. Lo que se edita se envía al servidor,
 * que vuelve a validarlo: aquí solo se evita mandar lo que ya se sabe inválido.
 *
 * Al propietario, cada cabecera le da un menú ▾ con lo que se hace con el campo
 * (renombrarlo, cambiar su fórmula o sus opciones, moverlo, borrarlo) y una última
 * cabecera «+» añade otro: el campo se arregla donde se ve, no buscando en otro menú. Las
 * columnas se reordenan arrastrando la cabecera o, con el foco en su botón ▾, con
 * Alt+← y Alt+→; las dos vías acaban en el mismo `onMoverCampo`.
 *
 * Lo copiado de una hoja de cálculo se pega (Ctrl+V) desde la celda con el foco, y las
 * filas que sobran son registros nuevos (`lib/pegar.ts`); lo mismo al soltar ese texto
 * sobre una celda. Y Ctrl+C copia la celda con el foco como texto, el mismo que se pega:
 * Fyne ya lo hacía y la web no, así que copiar de una tabla a otra iba en un solo sentido. Los ficheros soltados sobre una fila van a sus adjuntos: mientras se
 * arrastran, la rejilla se recuadra y la fila de debajo se marca.
 */
import type { JSX } from 'preact';
import { useEffect, useLayoutEffect, useRef, useState } from 'preact/hooks';
import {
  Button,
  Checkbox,
  Dropdown,
  Icon,
  IconButton,
  Input,
  MenuItem,
  MenuSeparator,
  Select,
} from '@sytue/ui';
import type { BasesField, BasesRecord, BasesSummary, BasesValue } from '@sytue/api-clients/bases';
import type { T } from '../i18n/index.js';
import { moverFoco, type Celda } from '../lib/rejilla.js';
import { vecino } from '../lib/esquema.js';
import { opcionesDeSelect } from '../lib/ficha.js';
import { esNumerico, ICONO_DE_TIPO } from '../lib/aspecto.js';
import { vacioDe } from '../lib/vacios.js';
import { llevaFicheros, planDePegado } from '../lib/pegar.js';
import { pieDeColumna, recuento } from '../lib/resumen.js';
import { admiteDefecto } from '../lib/defectos.js';
import { InsigniaOpcion } from './InsigniaOpcion.js';
import { VacioGuiado } from './VacioGuiado.js';
import {
  enlaceDeValor,
  enlacesDe,
  MODO_DE_ENTRADA,
  TIPO_DE_INPUT,
  textoDeValor,
  textoVisible,
  valorDeTexto,
} from '../lib/valores.js';

export interface OpcionEnlace {
  id: string;
  etiqueta: string;
}

/** Lo que se hace con un campo desde su cabecera. */
export type AccionCampo = 'renombrar' | 'formula' | 'opciones' | 'defecto' | 'borrar';

export interface RejillaProps {
  t: T;
  nombreTabla: string;
  campos: BasesField[];
  registros: BasesRecord[];
  puedeEditar: boolean;
  /**
   * Si lo que se mira es un recorte (`'busqueda'` o una `'vista'` con filtros): sin filas
   * no es «aún no hay registros», sino que nada pasa el recorte, y se ofrece verlo todo.
   */
  recorte?: 'busqueda' | 'vista';
  /** Vuelve a todos los registros: quita la búsqueda y la vista filtrada. */
  onVerTodos?: () => void;
  /** Etiquetas de los registros de cada tabla enlazada, por id de tabla. */
  etiquetas: Record<string, Map<string, string>>;
  /** Cambia una celda. Devuelve el mensaje de error, o null si salió bien. */
  onCambiar: (
    r: BasesRecord,
    data: Record<string, BasesValue>,
    links?: Record<string, string[]>,
  ) => Promise<string | null>;
  /** Abre el selector de varios (selección múltiple o enlace) para una celda. */
  onElegirVarios: (r: BasesRecord, campo: BasesField) => void;
  /** Abre los ficheros de una celda de adjuntos (también quien solo lee: para descargar). */
  onAdjuntos: (r: BasesRecord, campo: BasesField) => void;
  /** Abre la ficha del registro: todos sus campos en un formulario (y borrarlo, allí). */
  onAbrir: (r: BasesRecord) => void;
  /** Abre los comentarios de un registro. Sin ella no se pinta el botón. */
  onComentar?: (r: BasesRecord) => void;
  /** Quién más está en cada celda (clave `registro:campo`): se marca y se anuncia. */
  presentes?: Map<string, string[]>;
  /** La celda que tiene el foco, para decírsela a los demás. */
  onFoco?: (r: BasesRecord, campo: BasesField) => void;
  onNuevo: () => void;
  /**
   * Crea registros con sus valores, en orden: las filas de un bloque pegado que no caben
   * en las que hay. Devuelve el mensaje de error, o null si salió bien.
   */
  onCrearVarios?: (filas: Record<string, BasesValue>[]) => Promise<string | null>;
  /**
   * Ficheros soltados sobre una fila; `col` es la columna sobre la que caen (adjuntos, o
   * un CSV que se importa: lo decide quien los recibe). Sin ella no se aceptan.
   */
  onSoltarFicheros?: (ficheros: File[], r: BasesRecord, col: number) => void;
  /**
   * Solo para quien puede cambiar el esquema (el propietario): con ella cada cabecera
   * lleva su menú. Sin ella no se pinta, y el servicio tampoco lo dejaría.
   */
  onCampo?: (accion: AccionCampo, campo: BasesField) => void;
  /**
   * Mueve `campo` al sitio de `destino` (arrastrando su cabecera o con Alt+flechas). Solo
   * para el propietario y cuando la rejilla enseña el orden de la tabla.
   */
  onMoverCampo?: (campo: BasesField, destino: BasesField) => void;
  /** Añadir un campo desde la cabecera «+» del final. Va con `onCampo`. */
  onNuevoCampo?: () => void;
  /**
   * El pie con los totales de la vista entera (`lib/resumen.ts`). Sin él no se pinta: lo
   * pide quien lo enciende, porque cuesta una pasada por toda la tabla.
   */
  resumen?: BasesSummary | null;
  /** Cuántos registros hay ANTES de los que se enseñan (carga por ventanas). */
  desde: number;
  hayAntes: boolean;
  onAntes: () => void;
  hayMas: boolean;
  onMas: () => void;
}

const IMPRIMIBLE = /^[^\s]$/u;

/** Los tipos cuya celda empieza a editarse al teclear (los demás, con Intro). */
const TECLEABLES = new Set<BasesField['kind']>([
  'text',
  'number',
  'date',
  'url',
  'email',
  'phone',
  'currency',
  'rating',
]);

export function Rejilla(p: RejillaProps): JSX.Element {
  const { t, campos, registros, puedeEditar } = p;
  // Sin filas, el estado vacío dice si la tabla está vacía o si nada pasa el recorte.
  const vacio =
    registros.length === 0
      ? p.recorte
        ? vacioDe(
            'sinResultados',
            { esquema: false, escribir: puedeEditar },
            p.recorte === 'busqueda',
          )
        : vacioDe('sinRegistros', { esquema: false, escribir: puedeEditar })
      : null;
  const [foco, setFoco] = useState<Celda>({ fila: 0, col: 0 });
  const [editando, setEditando] = useState<Celda | null>(null);
  const [borrador, setBorrador] = useState('');
  const [aviso, setAviso] = useState('');
  // Lo que no entró de un bloque pegado se dice a la vista, no solo al lector de pantalla:
  // quien pega cien filas tiene que saber que faltan valores.
  const [avisoPegado, setAvisoPegado] = useState('');
  // La fila sobre la que hay ficheros arrastrándose (recuadro de «suelta aquí»).
  const [sueltaEn, setSueltaEn] = useState<number | null>(null);
  const tabla = useRef<HTMLTableElement>(null);
  const [arrastrado, setArrastrado] = useState<string | null>(null);
  const [sobre, setSobre] = useState<string | null>(null);
  // Tras mover con el teclado, el foco vuelve al botón ▾ del campo en su nuevo sitio.
  const [focoMenu, setFocoMenu] = useState<string | null>(null);
  useEffect(() => {
    if (!focoMenu) return;
    tabla.current?.querySelector<HTMLElement>(`[data-menu-campo="${focoMenu}"]`)?.focus();
    setFocoMenu(null);
  }, [campos, focoMenu]);

  function mover(c: BasesField, paso: -1 | 1): void {
    const destino = vecino(
      campos.map((x) => x.id),
      c.id,
      paso,
    );
    const d = campos.find((x) => x.id === destino);
    if (!d || !p.onMoverCampo) return;
    setFocoMenu(c.id);
    p.onMoverCampo(c, d);
    setAviso(t('field.moved', { name: c.name, n: campos.indexOf(d) + 1 }));
  }

  // Una fila borrada o una página nueva puede dejar el foco fuera: se recoloca.
  useEffect(() => {
    setFoco((f) => ({
      fila: Math.min(f.fila, Math.max(registros.length - 1, 0)),
      col: Math.min(f.col, Math.max(campos.length - 1, 0)),
    }));
  }, [registros.length, campos.length]);

  // El editor recibe foco y cursor al montarse, antes de que llegue otra tecla.
  // Un efecto pasivo puede mover el cursor después de Ctrl+A o de empezar a escribir.
  useLayoutEffect(() => {
    if (!editando) return;
    const el = document.getElementById(`ed-${editando.fila}-${editando.col}`);
    el?.focus();
    // Los campos de fecha no admiten selección de texto; los números usan type=text.
    if (el instanceof HTMLInputElement && el.type === 'text') {
      el.setSelectionRange(el.value.length, el.value.length);
    }
  }, [editando]);

  function enfocar(c: Celda): void {
    setFoco(c);
    tabla.current
      ?.querySelector<HTMLElement>(`[data-fila="${c.fila}"][data-col="${c.col}"]`)
      ?.focus();
  }

  function empezarEdicion(c: Celda, inicial?: string): void {
    const campo = campos[c.col];
    const r = registros[c.fila];
    if (campo && r && campo.kind === 'attachment') {
      p.onAdjuntos(r, campo);
      return;
    }
    if (!campo || !r || !puedeEditar) return;
    if (campo.kind === 'formula') {
      // Lo calcula el servicio: no se edita, y se dice.
      setAviso(t('grid.formulaReadOnly'));
      return;
    }
    if (campo.kind === 'checkbox') {
      void guardar(r, campo, r.data[campo.id] === true ? null : true);
      return;
    }
    if (campo.kind === 'multiselect' || campo.kind === 'link') {
      p.onElegirVarios(r, campo);
      return;
    }
    setBorrador(inicial ?? textoDeValor(campo, r.data[campo.id]));
    setEditando(c);
  }

  async function guardar(r: BasesRecord, campo: BasesField, valor: BasesValue): Promise<boolean> {
    const error = await p.onCambiar(r, { [campo.id]: valor });
    setAviso(error ?? t('grid.saved'));
    // El aviso de un pegado anterior ya no habla de lo que se ve.
    setAvisoPegado('');
    return error === null;
  }

  async function confirmar(c: Celda): Promise<boolean> {
    const campo = campos[c.col];
    const r = registros[c.fila];
    if (!campo || !r) return true;
    // Sin cambios no se valida: una opción retirada que el registro conserva se puede
    // abrir y cerrar sin que salte «no encaja».
    if (textoDeValor(campo, r.data[campo.id]) === borrador) {
      setEditando(null);
      return true;
    }
    const valor = valorDeTexto(campo, borrador);
    if (valor === undefined) {
      setAviso(t('grid.invalidValue'));
      return false;
    }
    setEditando(null);
    if (textoDeValor(campo, r.data[campo.id]) === textoDeValor(campo, valor)) return true;
    return guardar(r, campo, valor);
  }

  /** Pega un bloque copiado de una hoja a partir de la celda `desde`. */
  async function pegar(desde: Celda, texto: string): Promise<void> {
    if (!puedeEditar) return;
    const plan = planDePegado(campos, registros, desde, texto);
    if (plan.celdas.length === 0 && plan.nuevos.length === 0 && plan.saltados === 0) return;
    let error: string | null = null;
    for (const { registro, datos } of plan.celdas) {
      error = (await p.onCambiar(registro, datos)) ?? error;
    }
    if (plan.nuevos.length > 0 && p.onCrearVarios) {
      error = (await p.onCrearVarios(plan.nuevos)) ?? error;
    }
    const saltados = plan.saltados > 0 ? t('grid.pasteSkipped', { n: plan.saltados }) : '';
    setAvisoPegado(saltados);
    setAviso(error ?? (saltados || (plan.nuevos.length > 0 ? t('grid.added') : t('grid.saved'))));
  }

  /** El texto que se copia de una celda: el que se escribiría en ella al pegarlo. */
  function textoDeCelda(c: Celda): string {
    const campo = campos[c.col];
    const r = registros[c.fila];
    if (!campo || !r) return '';
    if (campo.kind === 'link') {
      const mapa = p.etiquetas[campo.link_table_id ?? ''];
      return enlacesDe(r, campo)
        .map((id) => mapa?.get(id) ?? id)
        .join(', ');
    }
    if (campo.kind === 'attachment') {
      return (r.attachments?.[campo.id] ?? []).map((a) => a.name).join(', ');
    }
    if (campo.kind === 'multiselect') {
      const v = r.data[campo.id];
      return Array.isArray(v) ? v.join(', ') : '';
    }
    return textoDeValor(campo, r.data[campo.id]);
  }

  function alCopiar(e: ClipboardEvent, c: Celda): void {
    // En edición, lo seleccionado del editor se copia como en cualquier campo.
    if (editando || !e.clipboardData) return;
    e.preventDefault();
    e.clipboardData.setData('text/plain', textoDeCelda(c));
  }

  function alPegar(e: ClipboardEvent, c: Celda): void {
    // En edición, el texto va al editor como en cualquier campo.
    if (editando || !puedeEditar) return;
    const texto = e.clipboardData?.getData('text/plain') ?? '';
    if (!texto) return;
    e.preventDefault();
    void pegar(c, texto);
  }

  /** Lo que se arrastra sobre una celda: ficheros (adjuntos) o texto (se pega). */
  function aceptaSoltar(e: DragEvent): 'ficheros' | 'texto' | null {
    // Una cabecera arrastrada para reordenar también lleva texto: no se pega.
    if (!puedeEditar || arrastrado || editando) return null;
    if (llevaFicheros(e.dataTransfer)) return p.onSoltarFicheros ? 'ficheros' : null;
    return e.dataTransfer?.types.includes('text/plain') ? 'texto' : null;
  }

  function alSoltar(e: DragEvent, c: Celda): void {
    const que = aceptaSoltar(e);
    setSueltaEn(null);
    if (!que) return;
    e.preventDefault();
    e.stopPropagation();
    const r = registros[c.fila];
    if (que === 'ficheros') {
      const ficheros = Array.from(e.dataTransfer?.files ?? []);
      if (r && ficheros.length > 0) p.onSoltarFicheros?.(ficheros, r, c.col);
      return;
    }
    const texto = e.dataTransfer?.getData('text/plain') ?? '';
    if (texto) {
      enfocar(c);
      void pegar(c, texto);
    }
  }

  function alPulsarCelda(e: KeyboardEvent, c: Celda): void {
    if (editando) return;
    const destino = moverFoco(c, e.key, e.ctrlKey || e.metaKey, registros.length, campos.length);
    if (destino) {
      e.preventDefault();
      enfocar(destino);
      return;
    }
    if (e.key === 'Enter' || e.key === 'F2') {
      e.preventDefault();
      empezarEdicion(c);
    } else if (e.key === ' ' && campos[c.col]?.kind === 'checkbox') {
      e.preventDefault();
      empezarEdicion(c);
    } else if (e.key === 'Delete' || e.key === 'Backspace') {
      const campo = campos[c.col];
      const r = registros[c.fila];
      if (
        campo &&
        r &&
        puedeEditar &&
        campo.kind !== 'link' &&
        campo.kind !== 'formula' &&
        campo.kind !== 'attachment'
      ) {
        e.preventDefault();
        void guardar(r, campo, null);
      }
    } else if (
      IMPRIMIBLE.test(e.key) &&
      !e.ctrlKey &&
      !e.metaKey &&
      !e.altKey &&
      campos[c.col] &&
      TECLEABLES.has(campos[c.col]!.kind)
    ) {
      e.preventDefault();
      empezarEdicion(c, e.key);
    }
  }

  async function alPulsarEditor(e: KeyboardEvent, c: Celda): Promise<void> {
    // El blur del editor llega ANTES del render que lo desmonta: se marca en el propio
    // elemento que ya está resuelto, o un Escape se confirmaría al perder el foco.
    // Y la tecla no sube a la celda: tras confirmar, la celda ya no está «en edición» y
    // volvería a abrir el editor con el mismo Intro.
    e.stopPropagation();
    const editor = e.currentTarget as HTMLElement;
    if (e.key === 'Escape') {
      e.preventDefault();
      editor.dataset.resuelto = '1';
      setEditando(null);
      enfocar(c);
    } else if (e.key === 'Enter' || (e.key === 'Tab' && !e.shiftKey)) {
      e.preventDefault();
      editor.dataset.resuelto = '1';
      if (await confirmar(c)) {
        const sig =
          e.key === 'Enter'
            ? moverFoco(c, 'ArrowDown', false, registros.length, campos.length)
            : moverFoco(c, 'ArrowRight', false, registros.length, campos.length);
        enfocar(sig ?? c);
      } else {
        delete editor.dataset.resuelto; // sigue abierto: el valor no encajaba
      }
    }
  }

  function contenido(c: Celda, campo: BasesField, r: BasesRecord): JSX.Element | string {
    if (editando && editando.fila === c.fila && editando.col === c.col) {
      const comun = {
        id: `ed-${c.fila}-${c.col}`,
        'aria-label': campo.name,
        onKeyDown: (e: KeyboardEvent) => void alPulsarEditor(e, c),
        onBlur: (e: FocusEvent) => {
          if (!(e.currentTarget as HTMLElement).dataset.resuelto) void confirmar(c);
        },
      };
      if (campo.kind === 'select') {
        return (
          <Select
            size="sm"
            {...comun}
            value={borrador}
            onChange={(e) => setBorrador(e.currentTarget.value)}
          >
            <option value="" />
            {opcionesDeSelect(
              campo,
              typeof r.data[campo.id] === 'string' ? (r.data[campo.id] as string) : '',
            ).map((o) => (
              <option key={o} value={o}>
                {o}
              </option>
            ))}
          </Select>
        );
      }
      return (
        <Input
          size="sm"
          {...comun}
          type={TIPO_DE_INPUT[campo.kind] ?? 'text'}
          inputMode={MODO_DE_ENTRADA[campo.kind]}
          value={borrador}
          onInput={(e) => setBorrador(e.currentTarget.value)}
        />
      );
    }
    if (campo.kind === 'checkbox') {
      return (
        <Checkbox
          tabIndex={-1}
          checked={r.data[campo.id] === true}
          aria-label={campo.name}
          disabled={!puedeEditar}
          class="border-border-strong accent-primary pointer-events-none h-4 w-4"
          onChange={() => undefined}
        />
      );
    }
    if (campo.kind === 'attachment') {
      const nombres = (r.attachments?.[campo.id] ?? []).map((a) => a.name).join(', ');
      return <span class="block max-w-72 break-words whitespace-normal">{nombres}</span>;
    }
    if (campo.kind === 'link') {
      const mapa = p.etiquetas[campo.link_table_id ?? ''];
      const nombres = enlacesDe(r, campo)
        .map((id) => mapa?.get(id) ?? id.slice(0, 8))
        .join(', ');
      return <span class="block max-w-72 break-words whitespace-normal">{nombres}</span>;
    }
    if (campo.kind === 'select' || campo.kind === 'multiselect') {
      const valor = r.data[campo.id];
      const opciones = Array.isArray(valor)
        ? valor
        : typeof valor === 'string' && valor
          ? [valor]
          : [];
      return (
        <span class="flex max-w-72 flex-wrap gap-1">
          {opciones.map((opcion) => (
            <InsigniaOpcion key={opcion} t={t} campo={campo} opcion={opcion} />
          ))}
        </span>
      );
    }
    const texto = textoVisible(campo, r.data[campo.id]);
    const href = enlaceDeValor(campo, r.data[campo.id]);
    if (href) {
      // tabIndex -1: la celda es la parada de tabulación de la rejilla; el enlace se abre
      // con el ratón o desde la ficha.
      return (
        <a
          href={href}
          tabIndex={-1}
          {...(campo.kind === 'url' ? { target: '_blank', rel: 'noopener noreferrer' } : {})}
          class="text-primary block max-w-72 break-all whitespace-normal underline-offset-2 hover:underline"
          onClick={(e) => e.stopPropagation()}
        >
          {texto}
        </a>
      );
    }
    const valoracion = r.data[campo.id];
    if (campo.kind === 'rating' && typeof valoracion === 'number') {
      // role="img": un aria-label en un <span> sin rol no se anuncia, y las estrellas
      // sueltas se leerían una a una. Sin valor no hay nada que anunciar.
      return (
        <span
          role="img"
          class="text-warning-text tracking-wider"
          aria-label={t('field.ratingValue', { n: String(valoracion) })}
        >
          {texto}
        </span>
      );
    }
    return <span class="block max-w-72 break-words whitespace-normal">{texto}</span>;
  }

  if (campos.length === 0) {
    // Sin campos no hay rejilla que pintar: solo el paso siguiente, si es suyo darlo.
    const onNuevoCampo = p.onNuevoCampo;
    return (
      <VacioGuiado
        t={t}
        vacio={vacioDe('sinCampos', { esquema: !!onNuevoCampo, escribir: puedeEditar })}
        acciones={onNuevoCampo ? { anadirCampo: onNuevoCampo } : {}}
      />
    );
  }

  const menuCampo = (c: BasesField): JSX.Element | null => {
    const onCampo = p.onCampo;
    if (!onCampo) return null;
    const etiqueta = t('field.actions', { name: c.name });
    return (
      <Dropdown
        class="shrink-0"
        align={esNumerico(c) ? 'end' : 'start'}
        label={etiqueta}
        trigger={(props) => (
          <IconButton
            {...props}
            size="sm"
            label={etiqueta}
            showTitle
            data-menu-campo={c.id}
            {...(p.onMoverCampo
              ? {
                  'aria-keyshortcuts': 'Alt+ArrowLeft Alt+ArrowRight',
                  onKeyDown: (e: KeyboardEvent) => {
                    if (!e.altKey || (e.key !== 'ArrowLeft' && e.key !== 'ArrowRight')) return;
                    e.preventDefault();
                    // En una página de derecha a izquierda, «izquierda» es avanzar.
                    const rtl = getComputedStyle(e.currentTarget as Element).direction === 'rtl';
                    mover(c, (e.key === 'ArrowLeft') !== rtl ? -1 : 1);
                  },
                }
              : {})}
          >
            <Icon name="chevronDown" size={14} />
          </IconButton>
        )}
      >
        {(cerrar) => (
          <>
            <MenuItem
              size="sm"
              icon={<Icon name="pen" size={14} />}
              onClick={() => {
                cerrar();
                onCampo('renombrar', c);
              }}
            >
              {t('field.rename')}
            </MenuItem>
            {c.kind === 'formula' ? (
              <MenuItem
                size="sm"
                icon={<Icon name="code" size={14} />}
                onClick={() => {
                  cerrar();
                  onCampo('formula', c);
                }}
              >
                {t('field.editFormula')}
              </MenuItem>
            ) : null}
            {c.kind === 'select' || c.kind === 'multiselect' ? (
              <MenuItem
                size="sm"
                icon={<Icon name="list" size={14} />}
                onClick={() => {
                  cerrar();
                  onCampo('opciones', c);
                }}
              >
                {t('field.editChoices')}
              </MenuItem>
            ) : null}
            {admiteDefecto(c) ? (
              <MenuItem
                size="sm"
                icon={<Icon name="tag" size={14} />}
                onClick={() => {
                  cerrar();
                  onCampo('defecto', c);
                }}
              >
                {t('field.default')}
              </MenuItem>
            ) : null}
            {p.onMoverCampo ? (
              <>
                <MenuSeparator />
                <MenuItem
                  size="sm"
                  icon={<Icon name="chevronLeft" size={14} />}
                  disabled={campos[0]?.id === c.id}
                  onClick={() => {
                    cerrar();
                    mover(c, -1);
                  }}
                >
                  {t('field.moveLeft')}
                </MenuItem>
                <MenuItem
                  size="sm"
                  icon={<Icon name="chevronRight" size={14} />}
                  disabled={campos[campos.length - 1]?.id === c.id}
                  onClick={() => {
                    cerrar();
                    mover(c, 1);
                  }}
                >
                  {t('field.moveRight')}
                </MenuItem>
              </>
            ) : null}
            <MenuSeparator />
            <MenuItem
              size="sm"
              danger
              icon={<Icon name="trash" size={14} />}
              onClick={() => {
                cerrar();
                onCampo('borrar', c);
              }}
            >
              {t('field.delete')}
            </MenuItem>
          </>
        )}
      </Dropdown>
    );
  };

  return (
    <div class="flex min-h-0 flex-col gap-2">
      <p class="text-text-muted text-xs" id="ayuda-rejilla">
        {puedeEditar ? t('grid.help') : t('grid.readOnly')}
      </p>
      {p.hayAntes ? (
        <div>
          <Button variant="ghost" size="sm" onClick={p.onAntes}>
            {t('grid.loadPrevious')}
          </Button>
        </div>
      ) : null}
      {avisoPegado ? (
        <p class="text-warning-text text-sm" data-aviso-pegado>
          {avisoPegado}
        </p>
      ) : null}
      <div
        class={`bases-grid-scroll border-border bg-surface max-w-full overflow-auto rounded-lg border ${sueltaEn !== null ? 'ring-primary ring-2' : ''}`}
        data-scroll-x
        data-soltando={sueltaEn !== null ? '' : undefined}
        onDragLeave={(e: DragEvent) => {
          // Solo al salir de la rejilla, no al pasar de una celda a otra.
          const a = e.relatedTarget as Node | null;
          if (!a || !(e.currentTarget as HTMLElement).contains(a)) setSueltaEn(null);
        }}
      >
        <table
          ref={tabla}
          role="grid"
          aria-label={t('grid.label', { table: p.nombreTabla })}
          aria-describedby="ayuda-rejilla"
          aria-rowcount={p.hayMas || p.hayAntes ? -1 : registros.length + 1 + (p.resumen ? 1 : 0)}
          aria-colcount={campos.length + (p.onNuevoCampo ? 2 : 1)}
          aria-readonly={!puedeEditar}
          class="bases-grid w-full min-w-max border-separate border-spacing-0 text-sm"
        >
          <thead class="bg-bg-subtle">
            <tr>
              <th scope="col" class="border-border w-12 border-b px-2 py-3 text-start">
                <span class="sr-only">{t('grid.rowCol')}</span>
              </th>
              {campos.map((c) => (
                <th
                  key={c.id}
                  scope="col"
                  class={`border-border text-text border-b px-3 py-3 font-semibold whitespace-nowrap ${esNumerico(c) ? 'text-end' : 'text-start'} ${p.onMoverCampo ? 'cursor-grab' : ''} ${arrastrado === c.id ? 'opacity-50' : ''} ${sobre === c.id && arrastrado !== c.id ? 'bg-selected' : ''}`}
                  {...(p.onMoverCampo
                    ? {
                        draggable: true,
                        'data-campo': c.id,
                        onDragStart: (e: DragEvent) => {
                          e.dataTransfer?.setData('text/plain', c.id);
                          if (e.dataTransfer) e.dataTransfer.effectAllowed = 'move';
                          setArrastrado(c.id);
                        },
                        onDragOver: (e: DragEvent) => {
                          if (!arrastrado) return;
                          e.preventDefault();
                          setSobre(c.id);
                        },
                        onDragLeave: () => setSobre((s) => (s === c.id ? null : s)),
                        onDrop: (e: DragEvent) => {
                          e.preventDefault();
                          const de = campos.find((x) => x.id === arrastrado);
                          setArrastrado(null);
                          setSobre(null);
                          if (de && de.id !== c.id) {
                            p.onMoverCampo?.(de, c);
                            setAviso(t('field.moved', { name: de.name, n: campos.indexOf(c) + 1 }));
                          }
                        },
                        onDragEnd: () => {
                          setArrastrado(null);
                          setSobre(null);
                        },
                      }
                    : {})}
                >
                  <span class={`flex items-center gap-2 ${esNumerico(c) ? 'justify-end' : ''}`}>
                    <Icon name={ICONO_DE_TIPO[c.kind]} size={15} class="text-text-muted shrink-0" />
                    <span class="max-w-72 truncate" title={c.name}>
                      {c.name}
                    </span>
                    <span class="sr-only"> · {t(`field.kind.${c.kind}`)}</span>
                    {menuCampo(c)}
                  </span>
                </th>
              ))}
              {p.onNuevoCampo ? (
                <th scope="col" class="border-border w-12 border-b px-2 py-2 text-start">
                  <IconButton size="sm" label={t('field.add')} showTitle onClick={p.onNuevoCampo}>
                    <Icon name="plus" size={14} />
                  </IconButton>
                </th>
              ) : null}
            </tr>
          </thead>
          <tbody>
            {registros.map((r, fila) => (
              <tr key={r.id} data-suelta={sueltaEn === fila ? '' : undefined}>
                <th
                  scope="row"
                  class="border-border text-text-muted w-12 border-b px-2 py-2 text-start font-normal"
                >
                  <span class="inline-flex items-center gap-1">
                    <span aria-hidden="true" class="min-w-4 text-end text-xs tabular-nums">
                      {p.desde + fila + 1}
                    </span>
                    <span class="sr-only">{t('grid.row', { n: p.desde + fila + 1 })}</span>
                    <Button
                      variant="ghost"
                      size="sm"
                      iconOnly
                      aria-label={t('grid.openRecord', { n: p.desde + fila + 1 })}
                      title={t('grid.openRecord', { n: p.desde + fila + 1 })}
                      leadingIcon={<Icon name="maximizar" size={14} />}
                      onClick={() => p.onAbrir(r)}
                    />
                    {p.onComentar ? (
                      <Button
                        variant="ghost"
                        size="sm"
                        iconOnly
                        aria-label={t('comments.open', {
                          name: t('grid.row', { n: p.desde + fila + 1 }),
                        })}
                        leadingIcon={<Icon name="comment" size={14} />}
                        onClick={() => p.onComentar?.(r)}
                      />
                    ) : null}
                  </span>
                </th>
                {campos.map((campo, col) => {
                  const c = { fila, col };
                  const activa = foco.fila === fila && foco.col === col;
                  const otros = p.presentes?.get(`${r.id}:${campo.id}`);
                  return (
                    <td
                      key={campo.id}
                      tabIndex={activa ? 0 : -1}
                      data-fila={fila}
                      data-col={col}
                      class={`sytue-focus-ring-inset border-border text-text min-w-36 border-b px-3 py-2 align-middle ${esNumerico(campo) ? 'text-end tabular-nums' : ''} ${otros ? 'ring-primary relative ring-2 ring-inset' : ''} ${sueltaEn === fila ? 'bg-selected' : ''}`}
                      onFocus={() => {
                        setFoco(c);
                        p.onFoco?.(r, campo);
                      }}
                      onKeyDown={(e) => alPulsarCelda(e, c)}
                      onPaste={(e) => alPegar(e, c)}
                      onCopy={(e) => alCopiar(e, c)}
                      onDragOver={(e: DragEvent) => {
                        const que = aceptaSoltar(e);
                        if (!que) return;
                        e.preventDefault();
                        if (e.dataTransfer) e.dataTransfer.dropEffect = 'copy';
                        setSueltaEn(que === 'ficheros' ? fila : null);
                      }}
                      onDrop={(e: DragEvent) => alSoltar(e, c)}
                      onDblClick={() => empezarEdicion(c)}
                    >
                      {contenido(c, campo, r)}
                      {otros ? (
                        <span class="bg-selected text-primary-text ml-2 inline-block max-w-32 truncate rounded px-1 align-middle text-xs">
                          <span aria-hidden="true">{otros.join(', ')}</span>
                          <span class="sr-only">{t('rt.inCell', { names: otros.join(', ') })}</span>
                        </span>
                      ) : null}
                    </td>
                  );
                })}
                {p.onNuevoCampo ? <td class="border-border border-b" aria-hidden="true" /> : null}
              </tr>
            ))}
          </tbody>
          {p.resumen ? (
            <tfoot class="bg-bg-subtle" data-pie-resumen>
              <tr>
                <th
                  scope="row"
                  class="text-text-muted px-2 py-2 text-start text-xs font-medium whitespace-nowrap tabular-nums"
                >
                  {recuento(p.resumen, t)}
                </th>
                {campos.map((campo) => {
                  const pie = pieDeColumna(campo, p.resumen?.fields[campo.id], t);
                  return (
                    <td
                      key={campo.id}
                      class={`text-text-muted px-3 py-2 text-xs whitespace-nowrap tabular-nums ${esNumerico(campo) ? 'text-end' : ''}`}
                      {...(pie?.detalle ? { title: pie.detalle } : {})}
                    >
                      {pie ? (
                        <>
                          {pie.texto}
                          {pie.detalle ? <span class="sr-only"> · {pie.detalle}</span> : null}
                        </>
                      ) : null}
                    </td>
                  );
                })}
                {p.onNuevoCampo ? <td aria-hidden="true" /> : null}
              </tr>
            </tfoot>
          ) : null}
        </table>
      </div>
      {vacio ? (
        <VacioGuiado
          t={t}
          compact
          vacio={vacio}
          acciones={{
            anadirRegistro: p.onNuevo,
            ...(p.onVerTodos ? { verTodos: p.onVerTodos } : {}),
          }}
        />
      ) : null}
      <div class="flex flex-wrap items-center gap-2">
        {/* Con la tabla vacía, «Añadir registro» ya es la acción del estado vacío. */}
        {puedeEditar && !(vacio && !p.recorte) ? (
          <Button
            variant="secondary"
            size="sm"
            leadingIcon={<Icon name="plus" size={14} />}
            onClick={p.onNuevo}
          >
            {t('grid.addRecord')}
          </Button>
        ) : null}
        {p.hayMas ? (
          <Button variant="ghost" size="sm" onClick={p.onMas}>
            {t('grid.loadMore')}
          </Button>
        ) : null}
      </div>
      <p class="sr-only" role="status" aria-live="polite">
        {aviso}
      </p>
    </div>
  );
}
