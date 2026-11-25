/*
 * SPDX-License-Identifier: Apache-2.0
 * SPDX-FileCopyrightText: (c) 2026 Daniel Delgado Ramos
 *
 * La ficha de un registro: todos sus campos en un formulario, uno debajo de otro, cada
 * uno con su control (fecha, casilla, opciones, enlaces). Rellenar una tabla celda a
 * celda obliga a ir de lado por la rejilla, y en un teléfono eso es una columna a la
 * vista y un doble toque por valor. Se abre con el botón de cada fila o de cada tarjeta
 * y, al añadir un registro, sola: añadir es rellenar. Es la ficha de Fyne
 * (`apps/fyne/apps/bases/registro.go`) y el «expandir registro» de Airtable o Notion.
 *
 * El `Modal` atrapa el foco, lo pone en el primer campo y lo devuelve al botón de la
 * fila al cerrar. Lo que se escribe es un borrador; al guardar se envía SOLO lo
 * cambiado (`lib/ficha.ts`), y el servicio vuelve a validarlo. Quien solo lee ve los
 * valores, sin un control que escriba.
 */
import type { JSX } from 'preact';
import { useId, useState } from 'preact/hooks';
import {
  Button,
  Checkbox,
  EmptyState,
  FieldGroup,
  Icon,
  IconButton,
  Input,
  Modal,
  Select,
} from '@sytue/ui';
import { contiene } from '@sytue/i18n';
import type { BasesField, BasesRecord, BasesValue } from '@sytue/api-clients/bases';
import type { T } from '../i18n/index.js';
import {
  alternar,
  borradorDe,
  cambiosDeFicha,
  editableEnFicha,
  opcionesDeSelect,
  type BorradorFicha,
} from '../lib/ficha.js';
import {
  enlacesDe,
  estrellas,
  MAX_VALORACION,
  MODO_DE_ENTRADA,
  textoVisible,
  TIPO_DE_INPUT,
} from '../lib/valores.js';
import type { OpcionEnlace } from './Rejilla.js';
import { InsigniaOpcion } from './InsigniaOpcion.js';

/** A partir de cuántas opciones una lista de varios lleva su búsqueda. */
const UMBRAL_BUSQUEDA = 8;

export interface FichaRegistroProps {
  t: T;
  campos: BasesField[];
  /** El registro tal como se abrió la ficha: es contra lo que se calcula qué cambia. */
  registro: BasesRecord;
  /** El registro de ahora (adjuntos subidos desde la ficha, cambios de otros). */
  actual: BasesRecord;
  /** El número de la fila (desde 1), que da nombre a la ficha. */
  n: number;
  /** Se acaba de añadir: el pie ofrece «Añadir registro» para rellenar seguido. */
  nuevo: boolean;
  puedeEditar: boolean;
  /** Las opciones de los campos de enlace, por id de la tabla enlazada. */
  opciones: Record<string, OpcionEnlace[]>;
  /** Las etiquetas de los registros enlazados, por id de tabla. */
  etiquetas: Record<string, Map<string, string>>;
  /** Guarda lo cambiado (puede no haber nada). Con `otro`, añade después otro registro. */
  onGuardar: (
    datos: Record<string, BasesValue>,
    enlaces: Record<string, string[]>,
    otro: boolean,
  ) => void;
  onBorrar: () => void;
  /** Copia el registro en uno nuevo (valores y enlaces). Sin ella no se pinta el botón. */
  onDuplicar?: () => void;
  /**
   * Pasa al registro de al lado en la rejilla (-1 el de arriba, 1 el de abajo), guardando
   * antes lo cambiado. Sin ella no hay flechas; sin vecino, la flecha se apaga.
   */
  onIr?: (
    paso: -1 | 1,
    datos: Record<string, BasesValue>,
    enlaces: Record<string, string[]>,
  ) => void;
  hayAnterior?: boolean;
  haySiguiente?: boolean;
  /** Abre los ficheros de un campo de adjuntos (también quien solo lee: para descargar). */
  onAdjuntos: (campo: BasesField) => void;
  /** Abre los comentarios del registro. Sin ella no se pinta el botón. */
  onComentar?: () => void;
  onCerrar: () => void;
}

export function FichaRegistro(p: FichaRegistroProps): JSX.Element {
  const { t, campos, puedeEditar } = p;
  const base = useId();
  const idDe = (c: BasesField) => `${base}-${c.id}`;
  // El borrador nace una vez: un cambio que llega en vivo no borra lo ya escrito.
  const [borrador, setBorrador] = useState<BorradorFicha>(() => borradorDe(campos, p.registro));
  const [invalidos, setInvalidos] = useState<string[]>([]);
  const [busquedas, setBusquedas] = useState<Record<string, string>>({});

  function poner(c: BasesField, v: BorradorFicha[string]): void {
    setBorrador((b) => ({ ...b, [c.id]: v }));
    if (invalidos.includes(c.id)) setInvalidos((xs) => xs.filter((x) => x !== c.id));
  }

  /** Lo cambiado, o null si algo no encaja (y entonces el foco va a lo primero que falla). */
  function validos(): ReturnType<typeof cambiosDeFicha> | null {
    const c = cambiosDeFicha(campos, p.registro, borrador);
    if (c.invalidos.length > 0) {
      setInvalidos(c.invalidos);
      // El foco va al primero que no encaja: es lo que hay que corregir.
      const primero = campos.find((x) => c.invalidos.includes(x.id));
      if (primero) document.getElementById(idDe(primero))?.focus();
      return null;
    }
    return c;
  }

  function guardar(otro: boolean): void {
    const c = validos();
    if (c) p.onGuardar(c.datos, c.enlaces, otro);
  }

  /**
   * Al registro de al lado sin cerrar la ficha: revisar una tabla registro a registro era
   * cerrar, bajar una fila y volver a abrir. Quien solo lee no guarda nada; quien escribe
   * se lleva guardado lo cambiado, como con «Guardar».
   */
  function ir(paso: -1 | 1): void {
    if (!p.onIr) return;
    if (!puedeEditar) {
      p.onIr(paso, {}, {});
      return;
    }
    const c = validos();
    if (c) p.onIr(paso, c.datos, c.enlaces);
  }

  /** Lo que se enseña de un campo que aquí no se cambia. */
  function lectura(c: BasesField): string {
    if (c.kind === 'checkbox')
      return p.actual.data[c.id] === true ? t('view.checked') : t('view.unchecked');
    if (c.kind === 'link') {
      const mapa = p.etiquetas[c.link_table_id ?? ''];
      return enlacesDe(p.actual, c)
        .map((id) => mapa?.get(id) ?? id.slice(0, 8))
        .join(', ');
    }
    return textoVisible(c, p.actual.data[c.id]);
  }

  function grupoVarios(c: BasesField, marcados: string[]): JSX.Element {
    const opciones: OpcionEnlace[] =
      c.kind === 'link'
        ? (p.opciones[c.link_table_id ?? ''] ?? [])
        : (c.choices ?? []).map((o) => ({ id: o, etiqueta: o }));
    const orden = opciones.map((o) => o.id);
    // Lo marcado que no está entre las opciones (un valor que ya no es opción, un enlace
    // más allá de los que se cargan) se enseña también: se ve y se puede desmarcar.
    const mapa = p.etiquetas[c.link_table_id ?? ''];
    const todas = [
      ...opciones,
      ...marcados
        .filter((id) => !orden.includes(id))
        .map((id) => ({
          id,
          etiqueta: mapa?.get(id) ?? (c.kind === 'link' ? id.slice(0, 8) : id),
        })),
    ];
    const q = busquedas[c.id] ?? '';
    const vistas = todas.filter((o) => contiene(o.etiqueta, q.trim()));
    return (
      <fieldset key={c.id} class="flex min-w-0 flex-col gap-1.5">
        <legend class="text-text mb-1.5 text-sm font-semibold">{c.name}</legend>
        {todas.length === 0 ? (
          <EmptyState compact title={t('pick.none')} />
        ) : (
          <>
            {todas.length > UMBRAL_BUSQUEDA ? (
              <Input
                type="search"
                size="sm"
                aria-label={t('record.search', { name: c.name })}
                placeholder={t('record.search', { name: c.name })}
                value={q}
                onInput={(e) => {
                  const v = e.currentTarget.value;
                  setBusquedas((b) => ({ ...b, [c.id]: v }));
                }}
              />
            ) : null}
            <ul class="flex max-h-56 flex-col gap-1 overflow-y-auto">
              {vistas.map((o) => (
                <li key={o.id}>
                  <Checkbox
                    label={o.etiqueta}
                    checked={marcados.includes(o.id)}
                    onChange={(e) =>
                      poner(c, alternar(marcados, o.id, e.currentTarget.checked, orden))
                    }
                  />
                </li>
              ))}
            </ul>
            {vistas.length === 0 ? (
              <p class="text-text-muted text-sm" role="status">
                {t('record.noMatches')}
              </p>
            ) : null}
          </>
        )}
      </fieldset>
    );
  }

  function grupoAdjuntos(c: BasesField): JSX.Element {
    const ficheros = p.actual.attachments?.[c.id] ?? [];
    return (
      <div key={c.id} class="flex flex-col gap-1.5">
        <span class="text-text text-sm font-semibold">{c.name}</span>
        <p class="text-text text-sm break-words">
          {ficheros.length > 0 ? ficheros.map((a) => a.name).join(', ') : t('attach.none')}
        </p>
        <div>
          <Button
            variant="secondary"
            size="sm"
            aria-label={`${t('record.files')} · ${c.name}`}
            leadingIcon={<Icon name="download" size={14} />}
            onClick={() => p.onAdjuntos(c)}
          >
            {t('record.files')}
          </Button>
        </div>
      </div>
    );
  }

  function grupo(c: BasesField): JSX.Element {
    if (c.kind === 'attachment') return grupoAdjuntos(c);
    if (!puedeEditar || !editableEnFicha(c)) {
      const texto = lectura(c);
      const valor = p.actual.data[c.id];
      const opciones =
        c.kind === 'select' || c.kind === 'multiselect'
          ? Array.isArray(valor)
            ? valor
            : typeof valor === 'string' && valor
              ? [valor]
              : []
          : null;
      return (
        <div key={c.id} class="flex flex-col gap-1.5">
          <span class="text-text text-sm font-semibold">{c.name}</span>
          {opciones && opciones.length > 0 ? (
            <span class="flex flex-wrap gap-1">
              {opciones.map((o) => (
                <InsigniaOpcion key={o} t={t} campo={c} opcion={o} />
              ))}
            </span>
          ) : (
            <p class="text-text text-sm break-words whitespace-pre-wrap">{texto || '—'}</p>
          )}
          {c.kind === 'formula' && puedeEditar ? (
            <p class="text-text-muted text-xs">{t('grid.formulaReadOnly')}</p>
          ) : null}
        </div>
      );
    }
    const b = borrador[c.id];
    if (c.kind === 'checkbox') {
      return (
        <div key={c.id}>
          <Checkbox
            id={idDe(c)}
            label={c.name}
            checked={b === true}
            onChange={(e) => poner(c, e.currentTarget.checked)}
          />
        </div>
      );
    }
    if (c.kind === 'multiselect' || c.kind === 'link')
      return grupoVarios(c, Array.isArray(b) ? b : []);
    const texto = typeof b === 'string' ? b : '';
    const invalido = invalidos.includes(c.id);
    if (c.kind === 'rating') {
      return (
        <FieldGroup key={c.id} label={c.name} htmlFor={idDe(c)}>
          <Select id={idDe(c)} value={texto} onChange={(e) => poner(c, e.currentTarget.value)}>
            <option value="">—</option>
            {Array.from({ length: MAX_VALORACION }, (_, i) => i + 1).map((n) => (
              <option
                key={n}
                value={String(n)}
                aria-label={t('field.ratingValue', { n: String(n) })}
              >
                {estrellas(n)}
              </option>
            ))}
          </Select>
        </FieldGroup>
      );
    }
    if (c.kind === 'select') {
      return (
        <FieldGroup key={c.id} label={c.name} htmlFor={idDe(c)}>
          <Select id={idDe(c)} value={texto} onChange={(e) => poner(c, e.currentTarget.value)}>
            <option value="">—</option>
            {opcionesDeSelect(
              c,
              typeof p.registro.data[c.id] === 'string' ? (p.registro.data[c.id] as string) : '',
            ).map((o) => (
              <option key={o} value={o}>
                {o}
              </option>
            ))}
          </Select>
        </FieldGroup>
      );
    }
    return (
      <FieldGroup
        key={c.id}
        label={c.name}
        htmlFor={idDe(c)}
        {...(invalido ? { error: t('grid.invalidValue') } : {})}
      >
        <Input
          id={idDe(c)}
          type={TIPO_DE_INPUT[c.kind] ?? 'text'}
          inputMode={MODO_DE_ENTRADA[c.kind]}
          invalid={invalido}
          aria-describedby={invalido ? `${idDe(c)}-error` : undefined}
          value={texto}
          onInput={(e) => poner(c, e.currentTarget.value)}
          onKeyDown={(e) => {
            // Como en la ficha de Fyne: Intro en un campo guarda.
            if (e.key === 'Enter' && !e.isComposing) {
              e.preventDefault();
              guardar(false);
            }
          }}
        />
      </FieldGroup>
    );
  }

  const pie = !puedeEditar ? (
    <Button onClick={p.onCerrar}>{t('common.close')}</Button>
  ) : (
    <>
      {p.nuevo ? (
        <Button
          variant="ghost"
          leadingIcon={<Icon name="plus" size={14} />}
          onClick={() => guardar(true)}
        >
          {t('grid.addRecord')}
        </Button>
      ) : (
        <Button variant="ghost" onClick={p.onCerrar}>
          {t('common.cancel')}
        </Button>
      )}
      <Button onClick={() => guardar(false)}>{t('common.save')}</Button>
    </>
  );

  return (
    <Modal open onClose={p.onCerrar} title={t('grid.row', { n: p.n })} size="md" footer={pie}>
      <div class="flex flex-col gap-4">
        {p.onIr ? (
          <nav class="-mt-2 flex justify-end gap-1" aria-label={t('record.navigation')}>
            <IconButton
              size="sm"
              label={t('record.previous')}
              showTitle
              disabled={!p.hayAnterior}
              onClick={() => ir(-1)}
            >
              <Icon name="chevronUp" size={16} />
            </IconButton>
            <IconButton
              size="sm"
              label={t('record.next')}
              showTitle
              disabled={!p.haySiguiente}
              onClick={() => ir(1)}
            >
              <Icon name="chevronDown" size={16} />
            </IconButton>
          </nav>
        ) : null}
        {campos.map(grupo)}
        {p.onComentar || (puedeEditar && !p.nuevo) ? (
          // Borrar va al final del cuerpo y no en el pie: ahí, junto a «Guardar», un
          // toque de más se lleva el registro.
          <div class="border-border flex flex-wrap items-center gap-2 border-t pt-4">
            {p.onComentar ? (
              <Button
                variant="secondary"
                size="sm"
                leadingIcon={<Icon name="comment" size={14} />}
                onClick={p.onComentar}
              >
                {t('record.comments')}
              </Button>
            ) : null}
            {puedeEditar && !p.nuevo && p.onDuplicar ? (
              <Button
                variant="secondary"
                size="sm"
                leadingIcon={<Icon name="copies" size={14} />}
                title={t('record.duplicateHelp')}
                onClick={p.onDuplicar}
              >
                {t('record.duplicate')}
              </Button>
            ) : null}
            {puedeEditar && !p.nuevo ? (
              <Button
                variant="ghost"
                size="sm"
                leadingIcon={<Icon name="trash" size={14} />}
                onClick={p.onBorrar}
              >
                {t('grid.deleteRecord', { n: p.n })}
              </Button>
            ) : null}
          </div>
        ) : null}
      </div>
    </Modal>
  );
}
