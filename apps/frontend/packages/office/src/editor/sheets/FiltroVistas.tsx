/*
 * SPDX-License-Identifier: Apache-2.0
 * SPDX-FileCopyrightText: (c) 2026 Daniel Delgado Ramos
 *
 * Vistas de filtro PERSONALES. Una vista es una foto con nombre de los criterios del
 * autofiltro (sheets/filtro.ts). Son de quien las crea y viven en el SERVIDOR, por persona y
 * documento (vistasFiltroSync.ts), así que viajan entre dispositivos; NUNCA en el modelo
 * compartido: activar una no cambia lo que ven los demás. «Aplicar a todos» es la única vía
 * a lo compartido, y es una orden explícita al autofiltro (con su permiso de protección),
 * no un efecto de la vista. El navegador solo guarda lo que aún no ha subido.
 *
 * ponytail: se guardan por COLUMNA ABSOLUTA: si otra persona inserta una columna, la vista
 * no se desplaza con ella.
 */
import type { JSX } from 'preact';
import { useEffect, useRef, useState } from 'preact/hooks';
import { fetchSession } from '@sytue/auth/client';
import { Button, Input, Select } from '@sytue/ui';
import { useT } from '../context';
import {
  activeView,
  filterViewsKey,
  mergeFilterViews,
  parseFilterViews,
  type FilterView,
  type FilterViewsState,
} from './filtro';
import { moverVista, type Desplazamiento } from './refshift';
import { cargarVistas, guardarVistas, type EntornoVistas } from './vistasFiltroSync';

/** Las vistas de la persona en este documento y la activa de cada hoja. */
export interface FiltroVistas {
  /** El estado entero: cambia de identidad con cada cambio (para los `useMemo`). */
  state: FilterViewsState;
  /** Vistas de una hoja. */
  of: (sheet: string) => FilterView[];
  /** Vista activa de una hoja (null = el filtro compartido). */
  activeOf: (sheet: string) => FilterView | null;
  select: (sheet: string, id: string | null) => void;
  /** Crea una vista y la deja activa. */
  add: (sheet: string, view: FilterView) => void;
  /** Sustituye una vista (mismo id). */
  update: (sheet: string, view: FilterView) => void;
  remove: (sheet: string, id: string) => void;
  /** Las vistas de la hoja siguen a sus columnas al insertar o borrar (refshift.ts). */
  mover: (sheet: string, d: Desplazamiento) => void;
}

const entorno = (): EntornoVistas => {
  let storage: Storage | null = null;
  try {
    storage = localStorage;
  } catch {
    /* sin almacenamiento: no hay buzón de salida */
  }
  return { fetch: (...a) => fetch(...a), storage };
};

const leer = (key: string): FilterViewsState => {
  try {
    return parseFilterViews(localStorage.getItem(key));
  } catch {
    return parseFilterViews(null); // sin almacenamiento: las vistas viven mientras dure la pestaña
  }
};

export function useFiltroVistas(docId: string): FiltroVistas {
  const [sub, setSub] = useState<string | undefined>(undefined);
  useEffect(() => {
    let vivo = true;
    void fetchSession().then((u) => {
      if (vivo && u) setSub(u.id);
    });
    return () => {
      vivo = false;
    };
  }, []);
  const key = filterViewsKey(sub, docId);
  // Lo local pendiente se ve al instante; al llegar el servidor se une con ello.
  const [st, setSt] = useState<FilterViewsState>(() => leer(key));
  const tocadoRef = useRef(false);
  const ultimoRef = useRef(st);
  useEffect(() => {
    ultimoRef.current = st;
  });
  const temporizadorRef = useRef<ReturnType<typeof setTimeout> | undefined>(undefined);
  useEffect(() => {
    // Sin sesión resuelta la clave es la anónima: se espera al `sub` para no mezclar cuentas.
    if (!sub) return;
    tocadoRef.current = false;
    setSt(leer(key));
    let vivo = true;
    void cargarVistas(entorno(), docId, key).then((remoto) => {
      if (!vivo || !remoto) return;
      if (!tocadoRef.current) {
        setSt(remoto);
        return;
      }
      // Ya se editó mientras llegaba el servidor: lo editado manda y se le suma lo remoto.
      const unido = mergeFilterViews(ultimoRef.current, remoto);
      setSt(unido);
      void guardarVistas(entorno(), docId, key, unido);
    });
    return () => {
      vivo = false;
    };
  }, [key, sub, docId]);
  useEffect(() => () => clearTimeout(temporizadorRef.current), []);

  const guardar = (next: FilterViewsState): void => {
    tocadoRef.current = true;
    setSt(next);
    // El buzón local se escribe ya (cerrar la pestaña no pierde nada) y el servidor, con
    // un pequeño retardo para no mandar una petición por cada pulsación.
    try {
      localStorage.setItem(key, JSON.stringify(next));
    } catch {
      /* cuota llena o almacenamiento bloqueado */
    }
    clearTimeout(temporizadorRef.current);
    temporizadorRef.current = setTimeout(
      () => void guardarVistas(entorno(), docId, key, next),
      300,
    );
  };
  const lista = (sheet: string): FilterView[] => st.views[sheet] ?? [];
  const sin = (m: Record<string, string>, sheet: string): Record<string, string> => {
    const resto = { ...m };
    delete resto[sheet];
    return resto;
  };

  return {
    state: st,
    of: lista,
    activeOf: (sheet) => activeView(st, sheet),
    select: (sheet, id) =>
      guardar({ ...st, active: id ? { ...st.active, [sheet]: id } : sin(st.active, sheet) }),
    add: (sheet, view) =>
      guardar({
        views: { ...st.views, [sheet]: [...lista(sheet), view] },
        active: { ...st.active, [sheet]: view.id },
      }),
    update: (sheet, view) =>
      guardar({
        ...st,
        views: { ...st.views, [sheet]: lista(sheet).map((v) => (v.id === view.id ? view : v)) },
      }),
    mover: (sheet, d) => {
      if (lista(sheet).length > 0)
        guardar({
          ...st,
          views: { ...st.views, [sheet]: lista(sheet).map((v) => moverVista(v, d)) },
        });
    },
    remove: (sheet, id) =>
      guardar({
        views: { ...st.views, [sheet]: lista(sheet).filter((v) => v.id !== id) },
        active: st.active[sheet] === id ? sin(st.active, sheet) : st.active,
      }),
  };
}

interface Props {
  views: readonly FilterView[];
  activeId: string | null;
  onSelect: (id: string | null) => void;
  /** Guarda los criterios actuales con este nombre. */
  onSave: (name: string) => void;
  onDelete: () => void;
  /** Vuelca la vista activa en el autofiltro compartido; null si no se puede (sin permiso). */
  onPromote: (() => void) | null;
}

/** Selector de vista y «Guardar como vista» en la cabecera del desplegable del autofiltro. */
export function FiltroVistasBarra({
  views,
  activeId,
  onSelect,
  onSave,
  onDelete,
  onPromote,
}: Props): JSX.Element {
  const t = useT();
  const [naming, setNaming] = useState(false);
  const [name, setName] = useState('');
  const guardar = (): void => {
    if (name.trim() === '') return;
    onSave(name);
    setName('');
    setNaming(false);
  };
  return (
    <div class="border-border flex w-64 flex-col gap-1.5 border-b px-2 pt-1 pb-1.5">
      <div class="sytue-overline text-text-subtle">{t('filtro.view')}</div>
      <Select
        size="sm"
        value={activeId ?? ''}
        aria-label={t('filtro.view')}
        onChange={(e) => onSelect(e.currentTarget.value || null)}
      >
        <option value="">{t('filtro.view.shared')}</option>
        {views.map((v) => (
          <option key={v.id} value={v.id}>
            {v.name}
          </option>
        ))}
      </Select>
      {activeId !== null && <div class="text-text-muted text-2xs">{t('filtro.view.hint')}</div>}
      {naming ? (
        <div class="flex gap-1.5">
          <Input
            size="sm"
            value={name}
            maxLength={60}
            aria-label={t('common.name')}
            placeholder={t('common.name')}
            onInput={(e) => setName(e.currentTarget.value)}
            onKeyDown={(e) => {
              if (e.key === 'Enter') guardar();
              if (e.key === 'Escape') setNaming(false);
            }}
          />
          <Button variant="secondary" size="xs" disabled={name.trim() === ''} onClick={guardar}>
            {t('action.save')}
          </Button>
        </div>
      ) : (
        <div class="flex flex-wrap gap-1.5">
          <Button variant="ghost" size="xs" onClick={() => setNaming(true)}>
            {t('filtro.view.save')}
          </Button>
          {activeId !== null && onPromote && (
            <Button variant="ghost" size="xs" onClick={onPromote}>
              {t('filtro.view.promote')}
            </Button>
          )}
          {activeId !== null && (
            <Button variant="ghost" size="xs" onClick={onDelete}>
              {t('filtro.view.delete')}
            </Button>
          )}
        </div>
      )}
    </div>
  );
}
