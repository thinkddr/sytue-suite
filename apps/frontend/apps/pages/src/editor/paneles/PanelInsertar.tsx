/*
 * SPDX-License-Identifier: Apache-2.0
 * SPDX-FileCopyrightText: (c) 2026 Daniel Delgado Ramos
 *
 * Panel de Inserción del editor (PAGES-PLAN §14.1).
 * Inserción de nodos individuales y secciones de plantilla completas.
 */

import type { JSX } from 'preact';
import type { Icono, Seccion, Sitio } from '@sytue/pages-render';
import { nuevoId } from '@sytue/pages-render';
import type { BloqueInsertable } from '../modelo/operaciones';
import { crearNodoPorTipo } from '../modelo/operaciones';

interface PanelInsertarProps {
  componentes: Sitio['components'];
  onInsertarNodo: (bloque: BloqueInsertable) => void;
  onInsertarSeccion: (seccion: Seccion) => void;
  onInsertarComponente: (id: string) => void;
  /** «Tabla desde Sytue Bases»: rótulo, descripción y aviso ya traducidos por quien monta. */
  bases?: { titulo: string; desc: string; aviso: string; onInsertar: () => void };
}

const BLOQUES: Array<{ tipo: BloqueInsertable; titulo: string; desc: string; icono: string }> = [
  { tipo: 'texto', titulo: 'Texto', desc: 'Párrafos, titulares y rótulos', icono: 'T' },
  { tipo: 'imagen', titulo: 'Imagen', desc: 'Fotografía con texto alternativo', icono: '🖼' },
  { tipo: 'boton', titulo: 'Botón', desc: 'Llamada a la acción con enlace', icono: '🔘' },
  { tipo: 'marco', titulo: 'Contenedor', desc: 'Caja con auto-layout (fila/columna)', icono: '📦' },
  { tipo: 'icono', titulo: 'Icono', desc: 'Símbolo vectorial del catálogo', icono: '⭐' },
  {
    tipo: 'formulario',
    titulo: 'Formulario',
    desc: 'Campos de contacto y consentimiento',
    icono: '📝',
  },
  { tipo: 'tabla', titulo: 'Tabla', desc: 'Filas y columnas de datos', icono: '📊' },
  { tipo: 'mapa', titulo: 'Mapa', desc: 'Ubicación interactiva OpenStreetMap', icono: '🗺' },
  { tipo: 'video', titulo: 'Vídeo', desc: 'Reproductor YouTube o Cast', icono: '▶' },
  {
    tipo: 'rejilla_productos',
    titulo: 'Catálogo',
    desc: 'Rejilla de productos de la tienda',
    icono: '🛍',
  },
  { tipo: 'menu', titulo: 'Menú', desc: 'Navegación entre páginas', icono: '☰' },
  {
    tipo: 'pdf',
    titulo: 'Descarga PDF',
    desc: 'Enlace para ver o descargar documento',
    icono: '📄',
  },
  { tipo: 'whatsapp', titulo: 'WhatsApp', desc: 'Botón directo a chat de WhatsApp', icono: '💬' },
  { tipo: 'redes', titulo: 'Redes', desc: 'Iconos sociales con enlaces', icono: '🌐' },
];

export function PanelInsertar({
  componentes,
  onInsertarNodo,
  onInsertarSeccion,
  onInsertarComponente,
  bases,
}: PanelInsertarProps): JSX.Element {
  const insertarHero = () => {
    const sid = nuevoId();
    const sec: Seccion = {
      id: sid,
      type: 'section',
      kind: 'hero',
      full_bleed: false,
      layout: {
        direction: 'column',
        gap: 24,
        padding: [64, 24, 64, 24],
        align: 'center',
        justify: 'center',
        wrap: false,
      },
      children: [
        {
          id: nuevoId(),
          type: 'text',
          name: 'Titular Hero',
          tag: 'h1',
          content: { es: [{ text: 'Diseña tu presencia en la web' }] },
          style: { text: { preset: 'h1', align: 'center' } },
        },
        {
          id: nuevoId(),
          type: 'text',
          name: 'Subtítulo',
          tag: 'p',
          content: {
            es: [
              {
                text: 'Crea páginas ultrarrápidas, accesibles y alojadas en centros de datos 100% de la Unión Europea.',
              },
            ],
          },
          style: { text: { preset: 'body', align: 'center' } },
        },
        {
          id: nuevoId(),
          type: 'button',
          name: 'Botón Hero',
          label: { es: 'Empezar ahora' },
          variant: 'primary',
          action: { url: '#contacto' },
          size: { width: 'hug', height: 'hug' },
        },
      ],
    };
    onInsertarSeccion(sec);
  };

  const insertarCaracteristicas = () => {
    const sid = nuevoId();
    const sec: Seccion = {
      id: sid,
      type: 'section',
      kind: 'features',
      full_bleed: false,
      layout: {
        direction: 'column',
        gap: 32,
        padding: [48, 24, 48, 24],
        align: 'center',
        justify: 'start',
        wrap: false,
      },
      children: [
        {
          id: nuevoId(),
          type: 'text',
          name: 'Título Sección',
          tag: 'h2',
          content: { es: [{ text: 'Nuestros Servicios' }] },
          style: { text: { preset: 'h2', align: 'center' } },
        },
        {
          id: nuevoId(),
          type: 'frame',
          name: 'Columnas',
          tag: 'div',
          layout: {
            direction: 'row',
            gap: 24,
            padding: [0, 0, 0, 0],
            align: 'start',
            justify: 'center',
            wrap: true,
          },
          children: [1, 2, 3].map((num) => ({
            id: nuevoId(),
            type: 'frame' as const,
            name: `Tarjeta ${num}`,
            tag: 'div' as const,
            layout: {
              direction: 'column' as const,
              gap: 12,
              padding: [24, 24, 24, 24] as [number, number, number, number],
              align: 'start' as const,
              justify: 'start' as const,
              wrap: false,
            },
            style: {
              background: '$background',
              border: { width: 1, color: '$border', style: 'solid' as const },
              radius: '$md' as const,
            },
            size: { width: 300, height: 'hug' as const },
            children: [
              {
                id: nuevoId(),
                type: 'icon' as const,
                name: (num === 1 ? 'star' : num === 2 ? 'shield' : 'globe') as Icono['name'],
              },
              {
                id: nuevoId(),
                type: 'text',
                tag: 'h3',
                content: { es: [{ text: `Característica ${num}` }] },
                style: { text: { preset: 'h3' } },
              },
              {
                id: nuevoId(),
                type: 'text',
                tag: 'p',
                content: {
                  es: [
                    {
                      text: 'Explicación clara del beneficio que aporta este servicio a tus clientes.',
                    },
                  ],
                },
                style: { text: { preset: 'body' } },
              },
            ],
          })),
        },
      ],
    };
    onInsertarSeccion(sec);
  };

  const insertarContacto = () => {
    const sid = nuevoId();
    const sec: Seccion = {
      id: sid,
      type: 'section',
      kind: 'contact',
      full_bleed: false,
      layout: {
        direction: 'column',
        gap: 24,
        padding: [48, 24, 48, 24],
        align: 'center',
        justify: 'start',
        wrap: false,
      },
      children: [
        {
          id: nuevoId(),
          type: 'text',
          name: 'Título Contacto',
          tag: 'h2',
          content: { es: [{ text: 'Ponte en contacto con nosotros' }] },
          style: { text: { preset: 'h2', align: 'center' } },
        },
        crearNodoPorTipo('formulario', 'es'),
      ],
    };
    onInsertarSeccion(sec);
  };

  return (
    <div class="flex h-full flex-col overflow-y-auto p-3">
      {/* Bloques individuales */}
      <div class="mb-4">
        <h3 class="text-text-muted mb-2 text-[11px] font-semibold tracking-wider uppercase">
          Bloques y componentes
        </h3>
        <div class="grid grid-cols-2 gap-2">
          {BLOQUES.map((b) => (
            <button
              key={b.tipo}
              type="button"
              onClick={() => onInsertarNodo(b.tipo)}
              class="border-border hover:bg-bg-subtle hover:border-primary flex flex-col items-start rounded-lg border p-2.5 text-left transition-colors"
            >
              <div class="mb-1 flex items-center gap-1.5">
                <span class="text-base" aria-hidden="true">
                  {b.icono}
                </span>
                <span class="text-text text-xs font-semibold">{b.titulo}</span>
              </div>
              <p class="text-text-muted line-clamp-2 text-[10px] leading-tight">{b.desc}</p>
            </button>
          ))}
        </div>
        {bases ? (
          <div class="mt-2">
            <button
              type="button"
              onClick={bases.onInsertar}
              class="border-border hover:bg-bg-subtle hover:border-primary flex w-full flex-col items-start rounded-lg border p-2.5 text-left transition-colors"
            >
              <span class="text-text text-xs font-semibold">{bases.titulo}</span>
              <span class="text-text-muted text-[10px] leading-tight">{bases.desc}</span>
            </button>
            {bases.aviso ? (
              <p role="alert" class="text-danger-text mt-1 text-xs">
                {bases.aviso}
              </p>
            ) : null}
          </div>
        ) : null}
      </div>

      {componentes.length > 0 && (
        <div class="mb-4">
          <h3 class="text-text-muted mb-2 text-[11px] font-semibold tracking-wider uppercase">
            Componentes guardados
          </h3>
          <div class="flex flex-col gap-2">
            {componentes.map((componente) => (
              <button
                key={componente.id}
                type="button"
                onClick={() => onInsertarComponente(componente.id)}
                class="border-border hover:bg-bg-subtle hover:border-primary flex items-center justify-between rounded-lg border p-3 text-left transition-colors"
              >
                <span class="text-text text-xs font-semibold">{componente.name}</span>
                <span class="text-primary-text text-xs font-bold">+</span>
              </button>
            ))}
          </div>
        </div>
      )}

      {/* Secciones de plantilla */}
      <div class="mb-4">
        <h3 class="text-text-muted mb-2 text-[11px] font-semibold tracking-wider uppercase">
          Secciones de página
        </h3>
        <div class="flex flex-col gap-2">
          <button
            type="button"
            onClick={insertarHero}
            class="border-border hover:bg-bg-subtle hover:border-primary flex items-center justify-between rounded-lg border p-3 text-left transition-colors"
          >
            <div>
              <span class="text-text block text-xs font-semibold">Cabecera Hero</span>
              <span class="text-text-muted text-[11px]">Titular, bajada y botón principal</span>
            </div>
            <span class="text-primary-text text-xs font-bold">+</span>
          </button>

          <button
            type="button"
            onClick={insertarCaracteristicas}
            class="border-border hover:bg-bg-subtle hover:border-primary flex items-center justify-between rounded-lg border p-3 text-left transition-colors"
          >
            <div>
              <span class="text-text block text-xs font-semibold">Rejilla de Servicios</span>
              <span class="text-text-muted text-[11px]">3 tarjetas con icono y descripción</span>
            </div>
            <span class="text-primary-text text-xs font-bold">+</span>
          </button>

          <button
            type="button"
            onClick={insertarContacto}
            class="border-border hover:bg-bg-subtle hover:border-primary flex items-center justify-between rounded-lg border p-3 text-left transition-colors"
          >
            <div>
              <span class="text-text block text-xs font-semibold">Formulario de Contacto</span>
              <span class="text-text-muted text-[11px]">
                Nombre, email, mensaje y consentimiento
              </span>
            </div>
            <span class="text-primary-text text-xs font-bold">+</span>
          </button>
        </div>
      </div>
    </div>
  );
}
