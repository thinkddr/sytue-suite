/*
 * SPDX-License-Identifier: Apache-2.0
 * SPDX-FileCopyrightText: (c) 2026 Daniel Delgado Ramos
 *
 * Panel de Propiedades del editor (PAGES-PLAN §14.1, P25).
 * Orden fijo: CONTENIDO -> ESTILO -> DISEÑO.
 * Rótulos en lenguaje llano, iconos de alineación y soporte para overrides por punto de corte.
 */

import { useState } from 'preact/hooks';
import type { JSX } from 'preact';
import type { Estilo, Layout, Nodo, Seccion, Sitio, Tamano } from '@sytue/pages-render';
import { ESTILOS_TEXTO } from '@sytue/pages-render';
import { Checkbox } from '@sytue/ui';
import type { PuntoDeCorte } from '../modelo/tipos';
import { IconoAutoLayoutColumna, IconoAutoLayoutFila } from '../iconos';

interface PanelPropiedadesProps {
  nodo: Nodo | Seccion | null;
  ir: Sitio;
  puntoDeCorte: PuntoDeCorte;
  onActualizarContenido: (parche: Record<string, unknown>) => void;
  onActualizarEstilo: (parche: Partial<Estilo>) => void;
  onActualizarLayout: (parche: Partial<Layout>) => void;
  onActualizarTamano: (parche: Partial<Tamano>) => void;
  onRestablecerOverride: () => void;
}

export function PanelPropiedades({
  nodo,
  ir,
  puntoDeCorte,
  onActualizarContenido,
  onActualizarEstilo,
  onActualizarLayout,
  onActualizarTamano,
  onRestablecerOverride,
}: PanelPropiedadesProps): JSX.Element {
  const [seccionPlegada, setSeccionPlegada] = useState<{
    contenido: boolean;
    estilo: boolean;
    diseno: boolean;
  }>({
    contenido: false,
    estilo: false,
    diseno: false,
  });

  if (!nodo) {
    return (
      <div class="text-text-muted flex h-full flex-col items-center justify-center p-6 text-center">
        <p class="text-xs">
          Selecciona un elemento en el lienzo o en las capas para ver sus propiedades.
        </p>
      </div>
    );
  }

  const defLang = ir.languages.default;
  const esContenedor = nodo.type === 'frame' || nodo.type === 'section' || nodo.type === 'form';
  const nodoLayout = esContenedor ? nodo.layout : undefined;
  const layout = (puntoDeCorte !== 'desktop' && nodo.breakpoints?.[puntoDeCorte]?.layout
    ? { ...nodoLayout, ...nodo.breakpoints[puntoDeCorte]?.layout }
    : nodoLayout) ?? {
    direction: 'column',
    gap: 16,
    padding: [0, 0, 0, 0],
    align: 'start',
    justify: 'start',
    wrap: false,
  };

  const estilo = nodo.style ?? {};
  const tamano = (puntoDeCorte !== 'desktop' && nodo.breakpoints?.[puntoDeCorte]?.size
    ? { ...nodo.size, ...nodo.breakpoints[puntoDeCorte]?.size }
    : nodo.size) ?? {
    width: 'hug',
    height: 'hug',
  };

  const tieneOverride =
    puntoDeCorte !== 'desktop' && nodo.breakpoints?.[puntoDeCorte] !== undefined;

  const alternarSeccion = (sec: 'contenido' | 'estilo' | 'diseno') => {
    setSeccionPlegada((prev) => ({ ...prev, [sec]: !prev[sec] }));
  };

  return (
    <div class="flex h-full flex-col overflow-y-auto p-3 text-xs">
      {/* Cabecera del nodo seleccionado */}
      <div class="border-border mb-3 border-b pb-2">
        <div class="flex items-center justify-between">
          <span class="text-text-muted text-[10px] font-bold tracking-wider uppercase">
            {nodo.type}
          </span>
          {tieneOverride && (
            <div class="flex items-center gap-1.5">
              <span class="rounded bg-amber-500/20 px-1.5 py-0.5 text-[10px] font-semibold text-amber-600">
                Override: {puntoDeCorte}
              </span>
              <button
                type="button"
                onClick={onRestablecerOverride}
                class="text-primary-text text-[10px] font-medium hover:text-[var(--sytue-primary-hover)]"
              >
                Restablecer
              </button>
            </div>
          )}
        </div>
        <h3 class="text-text truncate text-sm font-semibold">
          {nodo.name || (nodo.type === 'section' ? `Sección ${nodo.kind || ''}` : nodo.type)}
        </h3>
      </div>

      {/* 1. CONTENIDO */}
      <div class="border-border bg-surface mb-3 rounded-lg border">
        <button
          type="button"
          onClick={() => alternarSeccion('contenido')}
          class="text-text flex w-full items-center justify-between p-2.5 font-semibold"
        >
          <span>CONTENIDO</span>
          <span class="text-text-muted text-xs">{seccionPlegada.contenido ? '+' : '−'}</span>
        </button>

        {!seccionPlegada.contenido && (
          <div class="border-border flex flex-col gap-2.5 border-t p-2.5">
            {/* Texto */}
            {nodo.type === 'text' && (
              <label class="text-text-muted flex flex-col gap-1 text-[11px]">
                <span>Texto ({defLang})</span>
                <textarea
                  rows={3}
                  value={nodo.content?.[defLang]?.[0]?.text || ''}
                  onInput={(e) =>
                    onActualizarContenido({ text: (e.target as HTMLTextAreaElement).value })
                  }
                  class="border-border bg-bg text-text w-full rounded border p-1.5 text-xs"
                />
              </label>
            )}

            {/* Imagen */}
            {nodo.type === 'image' && (
              <>
                <label class="text-text-muted flex flex-col gap-1 text-[11px]">
                  <span>Imagen (asset o biblioteca)</span>
                  <input
                    type="text"
                    value={
                      'asset' in nodo.src
                        ? nodo.src.asset
                        : 'library' in nodo.src
                          ? nodo.src.library
                          : nodo.src.placeholder
                    }
                    onInput={(e) =>
                      onActualizarContenido({
                        src: { library: (e.target as HTMLInputElement).value },
                      })
                    }
                    class="border-border bg-bg text-text h-7 w-full rounded border px-2 text-xs"
                  />
                </label>
                <label class="text-text-muted flex flex-col gap-1 text-[11px]">
                  <span>Texto alternativo (alt)</span>
                  <input
                    type="text"
                    value={nodo.alt?.[defLang] || ''}
                    onInput={(e) =>
                      onActualizarContenido({
                        alt: { ...nodo.alt, [defLang]: (e.target as HTMLInputElement).value },
                      })
                    }
                    class="border-border bg-bg text-text h-7 w-full rounded border px-2 text-xs"
                  />
                </label>
                <label class="text-text-muted flex flex-col gap-1 text-[11px]">
                  <span>Ajuste de imagen</span>
                  <select
                    value={nodo.fit || 'cover'}
                    onChange={(e) =>
                      onActualizarContenido({ fit: (e.target as HTMLSelectElement).value })
                    }
                    class="border-border bg-bg text-text h-7 w-full rounded border px-2 text-xs"
                  >
                    <option value="cover">Cubrir espacio (cover)</option>
                    <option value="contain">Contener completa (contain)</option>
                  </select>
                </label>
              </>
            )}

            {/* Botón */}
            {nodo.type === 'button' && (
              <>
                <label class="text-text-muted flex flex-col gap-1 text-[11px]">
                  <span>Rótulo ({defLang})</span>
                  <input
                    type="text"
                    value={nodo.label?.[defLang] || ''}
                    onInput={(e) =>
                      onActualizarContenido({
                        label: { ...nodo.label, [defLang]: (e.target as HTMLInputElement).value },
                      })
                    }
                    class="border-border bg-bg text-text h-7 w-full rounded border px-2 text-xs"
                  />
                </label>
                <label class="text-text-muted flex flex-col gap-1 text-[11px]">
                  <span>Enlace / Destino</span>
                  <input
                    type="text"
                    value={
                      'url' in nodo.action
                        ? nodo.action.url
                        : 'page' in nodo.action
                          ? nodo.action.page
                          : ''
                    }
                    onInput={(e) =>
                      onActualizarContenido({
                        action: { url: (e.target as HTMLInputElement).value },
                      })
                    }
                    placeholder="https://... o #seccion"
                    class="border-border bg-bg text-text h-7 w-full rounded border px-2 text-xs"
                  />
                </label>
                <label class="text-text-muted flex flex-col gap-1 text-[11px]">
                  <span>Variante visual</span>
                  <select
                    value={nodo.variant || 'primary'}
                    onChange={(e) =>
                      onActualizarContenido({ variant: (e.target as HTMLSelectElement).value })
                    }
                    class="border-border bg-bg text-text h-7 w-full rounded border px-2 text-xs"
                  >
                    <option value="primary">Primario (destacado)</option>
                    <option value="secondary">Secundario</option>
                    <option value="ghost">Transparente</option>
                  </select>
                </label>
              </>
            )}

            {/* Formulario */}
            {nodo.type === 'form' && (
              <label class="text-text-muted flex flex-col gap-1 text-[11px]">
                <span>Botón de envío ({defLang})</span>
                <input
                  type="text"
                  value={nodo.submit?.[defLang] || ''}
                  onInput={(e) =>
                    onActualizarContenido({
                      submit: {
                        ...nodo.submit,
                        [defLang]: (e.target as HTMLInputElement).value,
                      },
                    })
                  }
                  class="border-border bg-bg text-text h-7 w-full rounded border px-2 text-xs"
                />
              </label>
            )}

            {/* Incrustado / Video / Mapa */}
            {nodo.type === 'embed' && (
              <label class="text-text-muted flex flex-col gap-1 text-[11px]">
                <span>Referencia / ID</span>
                <input
                  type="text"
                  value={nodo.ref || ''}
                  onInput={(e) =>
                    onActualizarContenido({ ref: (e.target as HTMLInputElement).value })
                  }
                  class="border-border bg-bg text-text h-7 w-full rounded border px-2 font-mono text-xs"
                />
              </label>
            )}
          </div>
        )}
      </div>

      {/* 2. ESTILO */}
      <div class="border-border bg-surface mb-3 rounded-lg border">
        <button
          type="button"
          onClick={() => alternarSeccion('estilo')}
          class="text-text flex w-full items-center justify-between p-2.5 font-semibold"
        >
          <span>ESTILO</span>
          <span class="text-text-muted text-xs">{seccionPlegada.estilo ? '+' : '−'}</span>
        </button>

        {!seccionPlegada.estilo && (
          <div class="border-border flex flex-col gap-2.5 border-t p-2.5">
            {/* Tipografía (si tiene texto) */}
            {nodo.type === 'text' && (
              <label class="text-text-muted flex flex-col gap-1 text-[11px]">
                <span>Estilo de texto</span>
                <select
                  value={estilo.text?.preset || 'body'}
                  onChange={(e) =>
                    onActualizarEstilo({
                      text: {
                        ...estilo.text,
                        preset: (e.target as HTMLSelectElement)
                          .value as (typeof ESTILOS_TEXTO)[number],
                      },
                    })
                  }
                  class="border-border bg-bg text-text h-7 w-full rounded border px-2 text-xs"
                >
                  {ESTILOS_TEXTO.map((p) => (
                    <option key={p} value={p}>
                      {p.toUpperCase()}
                    </option>
                  ))}
                </select>
              </label>
            )}

            {/* Color del texto */}
            <label class="text-text-muted flex flex-col gap-1 text-[11px]">
              <span>Color del texto</span>
              <select
                value={estilo.color || '$text'}
                onChange={(e) =>
                  onActualizarEstilo({ color: (e.target as HTMLSelectElement).value })
                }
                class="border-border bg-bg text-text h-7 w-full rounded border px-2 text-xs"
              >
                <option value="$text">Texto principal ($text)</option>
                <option value="$primary">Primario ($primary)</option>
                <option value="$on_primary">Sobre primario ($on_primary)</option>
                <option value="$muted">Secundario ($muted)</option>
              </select>
            </label>

            {/* Fondo */}
            {esContenedor && (
              <label class="text-text-muted flex flex-col gap-1 text-[11px]">
                <span>Fondo</span>
                <select
                  value={typeof estilo.background === 'string' ? estilo.background : ''}
                  onChange={(e) => {
                    const val = (e.target as HTMLSelectElement).value;
                    const nuevo = { ...estilo };
                    if (val) nuevo.background = val;
                    else delete nuevo.background;
                    onActualizarEstilo(nuevo);
                  }}
                  class="border-border bg-bg text-text h-7 w-full rounded border px-2 text-xs"
                >
                  <option value="">Transparente</option>
                  <option value="$background">Fondo del sitio ($background)</option>
                  <option value="$primary">Color primario ($primary)</option>
                  <option value="$muted">Atenuado ($muted)</option>
                </select>
              </label>
            )}

            {/* Borde y radio */}
            <div class="grid grid-cols-2 gap-2">
              <label class="text-text-muted flex flex-col gap-1 text-[11px]">
                <span>Radio</span>
                <select
                  value={estilo.radius || ''}
                  onChange={(e) => {
                    const val = (e.target as HTMLSelectElement).value;
                    const nuevo = { ...estilo };
                    if (val === '$sm' || val === '$md' || val === '$lg') {
                      nuevo.radius = val;
                    } else {
                      delete nuevo.radius;
                    }
                    onActualizarEstilo(nuevo);
                  }}
                  class="border-border bg-bg text-text h-7 w-full rounded border px-2 text-xs"
                >
                  <option value="">Ninguno (0)</option>
                  <option value="$sm">Pequeño ($sm)</option>
                  <option value="$md">Medio ($md)</option>
                  <option value="$lg">Grande ($lg)</option>
                </select>
              </label>
              <label class="text-text-muted flex flex-col gap-1 text-[11px]">
                <span>Sombra</span>
                <select
                  value={estilo.shadow || 'none'}
                  onChange={(e) =>
                    onActualizarEstilo({
                      shadow: (e.target as HTMLSelectElement).value as 'none' | 'sm' | 'md' | 'lg',
                    })
                  }
                  class="border-border bg-bg text-text h-7 w-full rounded border px-2 text-xs"
                >
                  <option value="none">Sin sombra</option>
                  <option value="sm">Suave (sm)</option>
                  <option value="md">Media (md)</option>
                  <option value="lg">Elevada (lg)</option>
                </select>
              </label>
            </div>
          </div>
        )}
      </div>

      {/* 3. DISEÑO (AUTO-LAYOUT Y TAMAÑO) */}
      <div class="border-border bg-surface mb-3 rounded-lg border">
        <button
          type="button"
          onClick={() => alternarSeccion('diseno')}
          class="text-text flex w-full items-center justify-between p-2.5 font-semibold"
        >
          <span>DISEÑO</span>
          <span class="text-text-muted text-xs">{seccionPlegada.diseno ? '+' : '−'}</span>
        </button>

        {!seccionPlegada.diseno && (
          <div class="border-border flex flex-col gap-2.5 border-t p-2.5">
            {/* Tamaño ancho y alto */}
            <div>
              <span class="text-text-muted mb-1 block text-[11px]">Ancho</span>
              <div class="grid grid-cols-3 gap-1">
                {(['hug', 'fill', 'fixed'] as const).map((modo) => {
                  const esActivo =
                    modo === 'fixed' ? typeof tamano.width === 'number' : tamano.width === modo;
                  return (
                    <button
                      key={modo}
                      type="button"
                      onClick={() => {
                        if (modo === 'fixed') onActualizarTamano({ width: 300 });
                        else onActualizarTamano({ width: modo });
                      }}
                      class={`rounded py-1 text-center text-[10px] font-medium transition-colors ${
                        esActivo
                          ? 'bg-primary font-bold text-[var(--sytue-on-primary)]'
                          : 'border-border bg-bg-subtle text-text border'
                      }`}
                    >
                      {modo === 'hug' ? 'Ajustar' : modo === 'fill' ? 'Llenar' : 'Fijo (px)'}
                    </button>
                  );
                })}
              </div>
              {typeof tamano.width === 'number' && (
                <input
                  type="number"
                  min={20}
                  max={2000}
                  value={tamano.width}
                  onInput={(e) =>
                    onActualizarTamano({ width: Number((e.target as HTMLInputElement).value) })
                  }
                  class="border-border bg-bg text-text mt-1.5 h-7 w-full rounded border px-2 text-xs"
                />
              )}
            </div>

            {/* Auto-Layout (si es contenedor) */}
            {esContenedor && (
              <>
                <div>
                  <span class="text-text-muted mb-1 block text-[11px]">Disposición</span>
                  <div class="grid grid-cols-2 gap-1.5">
                    <button
                      type="button"
                      onClick={() => onActualizarLayout({ direction: 'row' })}
                      class={`flex items-center justify-center gap-1.5 rounded border py-1.5 text-xs font-medium ${
                        layout.direction === 'row'
                          ? 'border-primary bg-selected text-primary-text font-bold'
                          : 'border-border bg-bg-subtle text-text'
                      }`}
                    >
                      <IconoAutoLayoutFila size={14} />
                      <span>En fila</span>
                    </button>
                    <button
                      type="button"
                      onClick={() => onActualizarLayout({ direction: 'column' })}
                      class={`flex items-center justify-center gap-1.5 rounded border py-1.5 text-xs font-medium ${
                        layout.direction === 'column'
                          ? 'border-primary bg-selected text-primary-text font-bold'
                          : 'border-border bg-bg-subtle text-text'
                      }`}
                    >
                      <IconoAutoLayoutColumna size={14} />
                      <span>En columna</span>
                    </button>
                  </div>
                </div>

                {/* Separación (gap) */}
                <div>
                  <div class="mb-1 flex items-center justify-between">
                    <span class="text-text-muted text-[11px]">Separación (gap)</span>
                    <span class="text-text-muted font-mono text-[10px]">{layout.gap} px</span>
                  </div>
                  <input
                    type="range"
                    min={0}
                    max={96}
                    step={4}
                    value={layout.gap}
                    onInput={(e) =>
                      onActualizarLayout({ gap: Number((e.target as HTMLInputElement).value) })
                    }
                    class="w-full"
                  />
                </div>

                {/* Margen interior (padding) */}
                <div>
                  <span class="text-text-muted mb-1 block text-[11px]">
                    Margen interior (padding)
                  </span>
                  <div class="grid grid-cols-4 gap-1 text-center">
                    {(['Arriba', 'Der', 'Abajo', 'Izq'] as const).map((lado, idx) => {
                      const paddingArr = layout.padding ?? [0, 0, 0, 0];
                      return (
                        <div key={lado}>
                          <span class="text-text-muted mb-0.5 block text-[9px]">{lado}</span>
                          <input
                            type="number"
                            min={0}
                            max={200}
                            value={paddingArr[idx]}
                            onInput={(e) => {
                              const val = Number((e.target as HTMLInputElement).value);
                              const nuevoPadding = [...paddingArr] as [
                                number,
                                number,
                                number,
                                number,
                              ];
                              nuevoPadding[idx] = val;
                              onActualizarLayout({ padding: nuevoPadding });
                            }}
                            class="border-border bg-bg text-text h-6 w-full rounded border px-1 text-center text-xs"
                          />
                        </div>
                      );
                    })}
                  </div>
                </div>

                {/* Alineación y Justificación */}
                <div class="grid grid-cols-2 gap-2">
                  <label class="text-text-muted flex flex-col gap-1 text-[11px]">
                    <span>Alineación</span>
                    <select
                      value={layout.align}
                      onChange={(e) =>
                        onActualizarLayout({
                          align: (e.target as HTMLSelectElement).value as Layout['align'],
                        })
                      }
                      class="border-border bg-bg text-text h-7 w-full rounded border px-1 text-xs"
                    >
                      <option value="start">Inicio</option>
                      <option value="center">Centro</option>
                      <option value="end">Fin</option>
                    </select>
                  </label>
                  <label class="text-text-muted flex flex-col gap-1 text-[11px]">
                    <span>Distribución</span>
                    <select
                      value={layout.justify}
                      onChange={(e) =>
                        onActualizarLayout({
                          justify: (e.target as HTMLSelectElement).value as Layout['justify'],
                        })
                      }
                      class="border-border bg-bg text-text h-7 w-full rounded border px-1 text-xs"
                    >
                      <option value="start">Inicio</option>
                      <option value="center">Centro</option>
                      <option value="end">Fin</option>
                      <option value="space-between">Repartir</option>
                    </select>
                  </label>
                </div>

                {/* Salto de línea */}
                {layout.direction === 'row' && (
                  <label class="text-text flex cursor-pointer items-center gap-2 pt-1 text-xs">
                    <Checkbox
                      checked={layout.wrap}
                      onChange={(e) =>
                        onActualizarLayout({ wrap: (e.target as HTMLInputElement).checked })
                      }
                    />
                    <span>Permitir salto de línea (wrap)</span>
                  </label>
                )}
              </>
            )}
          </div>
        )}
      </div>
    </div>
  );
}
