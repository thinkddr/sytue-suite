/*
 * SPDX-License-Identifier: Apache-2.0
 * SPDX-FileCopyrightText: (c) 2026 Daniel Delgado Ramos
 *
 * GaleriaPlantillas — la fila de plantillas de «Crear nuevo», bajo los accesos
 * «En blanco». La lista (nombre y descripción ya traducidos) la da el servicio
 * (GET /api/templates); elegir una crea el documento CON su contenido en el
 * servidor (POST /api/create con `template`), así que aquí no se siembra nada.
 *
 * Una sola fila que se desplaza de lado, también en escritorio: con ocho
 * plantillas en una rejilla, la portada empujaba los documentos recientes —lo
 * que la mayoría viene a buscar— fuera de la primera pantalla del móvil.
 *
 * La miniatura es un ESQUEMA de cada plantilla (dónde van el título, los
 * párrafos, la tabla o el gráfico) dibujado aquí, no una captura: no depende del
 * idioma ni pide nada al servidor. Una plantilla sin esquema propio (las de la
 * organización, que son documentos suyos) lleva el dibujo de su tipo.
 * ponytail: esquemas a mano para las ocho de Sytue; si la galería creciera, pintar
 * la primera página real del contenido.
 */
import type { JSX } from 'preact';
import { useState } from 'preact/hooks';
import type { DocKind, DocTemplate } from '@sytue/api-clients';
import { cx } from '@sytue/ui';
import type { DocsT } from '../i18n';
import { KIND_COLOR, KindGlyph } from './DocList';

/** [x, y, ancho, alto, tono] sobre un lienzo de 120×80. `a` es el acento (el
 *  color del producto: títulos y cabeceras), `t` el texto y `l` una línea fina. */
type Trazo = [number, number, number, number, 'a' | 't' | 'l'];

/** Hoja vertical centrada: la página de un documento. */
const PAGINA = { x: 34, y: 6, w: 52, h: 68 };
/** Lienzo 16:9: una diapositiva. */
const DIAPOSITIVA = { x: 12, y: 12.5, w: 96, h: 54 };
/** Rejilla de hoja de cálculo, a todo el ancho. */
const HOJA = { x: 10, y: 8, w: 100, h: 64 };

/** Párrafo de `n` renglones desde `y`, con el último más corto. */
const parrafo = (x: number, y: number, w: number, n: number): Trazo[] =>
  Array.from({ length: n }, (_, i): Trazo => [x, y + i * 3, i === n - 1 ? w * 0.7 : w, 1.6, 't']);

/** Tabla de `filas` × columnas (anchos) con su marco y divisiones. */
function tabla(x: number, y: number, anchos: number[], filas: number, alto: number): Trazo[] {
  const w = anchos.reduce((s, a) => s + a, 0);
  const out: Trazo[] = [];
  for (let f = 0; f <= filas; f++) out.push([x, y + f * alto, w, 0.6, 'l']);
  let cx0 = x;
  for (const a of [0, ...anchos]) {
    cx0 += a;
    out.push([cx0 - (a === 0 ? 0 : 0.6), y, 0.6, filas * alto, 'l']);
  }
  return out;
}

/** Cabecera de hoja: título, fila de rótulos en acento y la rejilla de debajo. */
function hoja(anchos: number[], extra: Trazo[] = []): Trazo[] {
  const fila = 6;
  const y0 = HOJA.y + 10;
  let x = HOJA.x;
  const rotulos = anchos.map((a): Trazo => {
    const t: Trazo = [x + 2, y0 + 2, Math.min(a - 4, 14), 2, 'a'];
    x += a;
    return t;
  });
  return [
    [HOJA.x + 2, HOJA.y + 3, 28, 3, 'a'],
    ...rotulos,
    ...tabla(HOJA.x, y0, anchos, 8, fila),
    ...extra,
  ];
}

const ESQUEMAS: Record<string, { marco: 'pagina' | 'diapositiva' | 'hoja'; trazos: Trazo[] }> = {
  carta: {
    marco: 'pagina',
    trazos: [
      [62, 12, 18, 2, 't'],
      [66, 15.5, 14, 2, 't'],
      [64, 19, 16, 2, 't'],
      [40, 25, 20, 2, 't'],
      [40, 28.5, 16, 2, 't'],
      [66, 34, 14, 2, 't'],
      [40, 40, 26, 2.5, 'a'],
      ...parrafo(40, 46, 40, 3),
      ...parrafo(40, 56, 40, 2),
      [40, 66, 14, 2, 't'],
    ],
  },
  informe: {
    marco: 'pagina',
    trazos: [
      [40, 12, 34, 4, 'a'],
      [40, 18.5, 20, 2, 't'],
      [40, 25, 18, 2.5, 'a'],
      ...parrafo(40, 30, 40, 2),
      [40, 39, 16, 2.5, 'a'],
      ...parrafo(40, 44, 40, 3),
      [40, 56, 20, 2.5, 'a'],
      [42, 61, 30, 1.6, 't'],
      [42, 64, 26, 1.6, 't'],
    ],
  },
  acta: {
    marco: 'pagina',
    trazos: [
      [40, 12, 30, 4, 'a'],
      ...tabla(40, 19, [14, 26], 4, 4),
      [41.5, 20.5, 9, 1.4, 't'],
      [41.5, 24.5, 7, 1.4, 't'],
      [41.5, 28.5, 10, 1.4, 't'],
      [41.5, 32.5, 8, 1.4, 't'],
      [40, 40, 16, 2.5, 'a'],
      [42, 45, 30, 1.6, 't'],
      [42, 48, 26, 1.6, 't'],
      [42, 51, 28, 1.6, 't'],
      [40, 56, 14, 2.5, 'a'],
      ...tabla(40, 61, [14, 13, 13], 2, 4.5),
    ],
  },
  orden: {
    marco: 'pagina',
    // Los datos de la reunión arriba y la tabla de temas con su hora debajo.
    trazos: [
      [40, 12, 30, 4, 'a'],
      ...tabla(40, 19, [14, 26], 3, 4),
      [41.5, 20.5, 9, 1.4, 't'],
      [41.5, 24.5, 7, 1.4, 't'],
      [41.5, 28.5, 10, 1.4, 't'],
      [40, 36, 14, 2.5, 'a'],
      ...tabla(40, 41, [8, 20, 12], 4, 4.5),
      [41.5, 42.5, 5, 1.4, 'a'],
      [49.5, 42.5, 10, 1.4, 'a'],
      [69.5, 42.5, 6, 1.4, 'a'],
      [40, 63, 24, 1.6, 't'],
      [40, 66, 18, 1.6, 't'],
    ],
  },
  presupuesto: {
    marco: 'hoja',
    // Una fila de total al pie, en acento: es lo que distingue un presupuesto.
    trazos: hoja([40, 20, 20, 20], [[HOJA.x + 2, HOJA.y + 10 + 6 * 7 + 2, 12, 2, 'a']]),
  },
  gastos: {
    marco: 'hoja',
    // El total arriba a la derecha, sobre la tabla de gastos.
    trazos: hoja([18, 32, 20, 16, 14], [[HOJA.x + 56, HOJA.y + 3, 30, 3, 't']]),
  },
  planificacion: {
    marco: 'hoja',
    // La columna de estado lleva una marca por tarea.
    trazos: hoja(
      [30, 16, 13, 13, 14, 14],
      Array.from({ length: 5 }, (_, i): Trazo => [
        HOJA.x + 74,
        HOJA.y + 10 + 6 * (i + 1) + 2,
        10,
        2,
        't',
      ]),
    ),
  },
  factura: {
    marco: 'hoja',
    // Emisor y cliente a la izquierda, subtotal, IVA y total en acento abajo a la derecha.
    trazos: hoja(
      [44, 14, 20, 22],
      [
        [HOJA.x + 2, HOJA.y + 3, 14, 3, 't'],
        [HOJA.x + 72, HOJA.y + 10 + 6 * 7 + 2, 14, 2, 'a'],
        [HOJA.x + 72, HOJA.y + 10 + 6 * 6 + 2, 12, 2, 't'],
      ],
    ),
  },
  proyecto: {
    marco: 'diapositiva',
    trazos: [
      [26, 32, 68, 6, 'a'],
      [38, 42, 44, 2.5, 't'],
    ],
  },
  propuesta: {
    marco: 'diapositiva',
    // El titular a la izquierda y el bloque de la inversión a la derecha.
    trazos: [
      [18, 20, 40, 5, 'a'],
      ...parrafo(18, 30, 36, 4),
      [64, 22, 28, 30, 'l'],
      [66, 26, 16, 2.5, 'a'],
      [66, 33, 22, 1.6, 't'],
      [66, 37, 18, 1.6, 't'],
      [66, 44, 14, 3, 'a'],
    ],
  },
  trimestral: {
    marco: 'diapositiva',
    trazos: [
      [18, 18, 44, 4, 'a'],
      [22, 48, 8, 12, 'a'],
      [33, 44, 8, 16, 'a'],
      [44, 46, 8, 14, 'a'],
      [55, 40, 8, 20, 'a'],
      [20, 60.4, 46, 0.6, 'l'],
      ...parrafo(72, 30, 28, 4),
    ],
  },
  mensual: {
    marco: 'hoja',
    // Dos bloques (ingresos y gastos) con su total, y el saldo en acento al pie.
    trazos: hoja(
      [40, 20, 20, 20],
      [
        [HOJA.x + 2, HOJA.y + 10 + 6 * 3 + 2, 14, 2, 'a'],
        [HOJA.x + 2, HOJA.y + 10 + 6 * 7 + 2, 12, 2, 'a'],
      ],
    ),
  },
  inventario: {
    marco: 'hoja',
    // Muchas columnas estrechas: código, artículo, existencias, valor y reposición.
    trazos: hoja([12, 26, 14, 12, 12, 12, 12]),
  },
  semanal: {
    marco: 'hoja',
    // Una columna de horas y siete de días.
    trazos: hoja([12, 12, 12, 12, 12, 12, 14, 14]),
  },
  contactos: {
    marco: 'hoja',
    trazos: hoja([22, 22, 16, 22, 18]),
  },
  horas: {
    marco: 'hoja',
    trazos: hoja([16, 26, 30, 12, 16], [[HOJA.x + 74, HOJA.y + 10 + 6 * 7 + 2, 14, 2, 'a']]),
  },
  cobros: {
    marco: 'hoja',
    trazos: hoja(
      [14, 24, 14, 14, 12, 12, 10],
      Array.from({ length: 4 }, (_, i): Trazo => [
        HOJA.x + 90,
        HOJA.y + 10 + 6 * (i + 1) + 2,
        7,
        2,
        't',
      ]),
    ),
  },
  equipo: {
    marco: 'diapositiva',
    // Cuatro personas en rejilla: un retrato y dos líneas cada una.
    trazos: [
      [18, 18, 40, 4, 'a'],
      ...[0, 1, 2, 3].flatMap((i): Trazo[] => [
        [18 + i * 22, 32, 14, 14, 'l'],
        [18 + i * 22, 50, 14, 2, 't'],
        [18 + i * 22, 54, 10, 1.6, 't'],
      ]),
    ],
  },
  lanzamiento: {
    marco: 'diapositiva',
    // El titular y, a la derecha, una línea de hitos que sube.
    trazos: [
      [18, 20, 40, 5, 'a'],
      ...parrafo(18, 30, 34, 4),
      [62, 56, 34, 0.6, 'l'],
      [64, 46, 6, 10, 'a'],
      [74, 40, 6, 16, 'a'],
      [84, 32, 6, 24, 'a'],
    ],
  },
  formacion: {
    marco: 'diapositiva',
    // Agenda: hora a la izquierda y tema a la derecha, en cuatro filas.
    trazos: [
      [18, 18, 36, 4, 'a'],
      ...[0, 1, 2, 3].flatMap((i): Trazo[] => [
        [18, 30 + i * 8, 8, 2, 'a'],
        [30, 30 + i * 8, 40, 2, 't'],
      ]),
    ],
  },
  cierre: {
    marco: 'diapositiva',
    // Dos columnas, lo previsto y lo conseguido.
    trazos: [
      [18, 18, 44, 4, 'a'],
      [18, 30, 18, 2.5, 'a'],
      ...parrafo(18, 36, 34, 4),
      [64, 30, 18, 2.5, 'a'],
      ...parrafo(64, 36, 34, 4),
    ],
  },
  estrategia: {
    marco: 'diapositiva',
    // Metas por trimestre: barras ascendentes y una nota.
    trazos: [
      [18, 18, 44, 4, 'a'],
      [22, 54, 8, 8, 'a'],
      [33, 50, 8, 12, 'a'],
      [44, 46, 8, 16, 'a'],
      [55, 40, 8, 22, 'a'],
      [20, 62.4, 46, 0.6, 'l'],
      ...parrafo(72, 30, 28, 4),
    ],
  },
  bienvenida: {
    marco: 'diapositiva',
    // Saludo grande y tres pasos numerados debajo.
    trazos: [
      [18, 22, 56, 6, 'a'],
      ...[0, 1, 2].flatMap((i): Trazo[] => [
        [18 + i * 26, 42, 5, 5, 'a'],
        [18 + i * 26, 51, 20, 2, 't'],
        [18 + i * 26, 55, 14, 1.6, 't'],
      ]),
    ],
  },
};

const MARCOS = { pagina: PAGINA, diapositiva: DIAPOSITIVA, hoja: HOJA };

/** Esquema de una plantilla; el dibujo de su tipo si no tiene uno propio. */
export function Miniatura({ clave, kind }: { clave: string; kind: DocKind }): JSX.Element {
  const esquema = ESQUEMAS[clave];
  if (!esquema) return <KindGlyph kind={kind} size={32} />;
  const m = MARCOS[esquema.marco];
  return (
    <svg viewBox="0 0 120 80" class="h-full w-full" aria-hidden="true">
      <rect
        x={m.x}
        y={m.y}
        width={m.w}
        height={m.h}
        rx="1.5"
        class="fill-surface stroke-border-strong"
        stroke-width="0.6"
      />
      {esquema.trazos.map(([x, y, w, h, tono], i) => (
        <rect
          key={i}
          x={x}
          y={y}
          width={w}
          height={h}
          rx={tono === 'l' ? 0 : 0.6}
          class={
            tono === 'a' ? undefined : tono === 't' ? 'fill-text-subtle' : 'fill-border-strong'
          }
          fill={tono === 'a' ? 'currentColor' : undefined}
        />
      ))}
    </svg>
  );
}

export interface GaleriaPlantillasProps {
  t: DocsT;
  plantillas: DocTemplate[];
  /** Las de la organización (clave «org:<id>»): se ofrecen en su propia fila, con la
   *  versión a elegir si hay más de una. */
  deOrganizacion?: DocTemplate[];
  /** Clave de la que se está creando, para su «Creando…». */
  creando: string | null;
  deshabilitada: boolean;
  /** `version` solo llega con las de la organización y solo si no es la vigente. */
  onElegir: (plantilla: DocTemplate, version?: string) => void;
  /** Los temas con que se puede crear (marca de la organización y catálogo común) y el
   *  elegido; sin `temas` no se pinta el selector. */
  temas?: { id: string; label: string }[];
  tema?: string;
  onTema?: (id: string) => void;
}

interface TarjetaProps {
  t: DocsT;
  p: DocTemplate;
  creando: string | null;
  deshabilitada: boolean;
  onElegir: (plantilla: DocTemplate, version?: string) => void;
}

function Tarjeta({ t, p, creando, deshabilitada, onElegir }: TarjetaProps): JSX.Element {
  // La vigente es la primera: sin tocar el selector, el servicio ya copia esa.
  const [version, setVersion] = useState('');
  const versiones = p.versions ?? [];
  return (
    <li class="w-40 shrink-0 snap-start">
      <div class="flex h-full flex-col">
        <button
          type="button"
          onClick={() => onElegir(p, version && version !== versiones[0]?.id ? version : undefined)}
          disabled={deshabilitada}
          aria-busy={creando === p.key}
          class={cx(
            'border-border bg-surface hover:border-border-strong hover:bg-bg-subtle sytue-focus-ring-inset flex w-full flex-1 flex-col overflow-hidden rounded-lg border text-left transition duration-150 disabled:pointer-events-none',
            deshabilitada && creando !== p.key && 'opacity-60',
          )}
        >
          <span
            class="border-border bg-bg-subtle sytue-glifo-tinta flex h-24 w-full items-center justify-center border-b"
            style={{ color: KIND_COLOR[p.kind] ?? KIND_COLOR.document }}
            aria-hidden="true"
          >
            <Miniatura clave={p.key} kind={p.kind} />
          </span>
          <span class="flex min-w-0 flex-col gap-0.5 p-2.5">
            <span class="text-text truncate text-sm font-medium">{p.name}</span>
            <span class="text-text-muted line-clamp-2 text-xs">
              {creando === p.key ? t('list.new.creating') : p.description}
            </span>
          </span>
        </button>
        {versiones.length > 1 && (
          <select
            aria-label={t('list.templates.version', { name: p.name })}
            value={version || versiones[0]?.id}
            disabled={deshabilitada}
            onChange={(e) => setVersion(e.currentTarget.value)}
            class="border-border bg-surface text-text-muted sytue-focus-ring mt-1 w-full rounded-md border px-1.5 py-1 text-xs"
          >
            {versiones.map((v, i) => (
              <option key={v.id} value={v.id}>
                {i === 0
                  ? t('list.templates.versionCurrent', { no: v.no })
                  : t('list.templates.versionNo', { no: v.no })}
              </option>
            ))}
          </select>
        )}
      </div>
    </li>
  );
}

function Fila({
  id,
  titulo,
  lista,
  ...resto
}: Omit<TarjetaProps, 'p'> & { id: string; titulo: string; lista: DocTemplate[] }): JSX.Element {
  return (
    <div class="mt-5">
      <h3 id={id} class="sytue-overline text-text-subtle mb-2">
        {titulo}
      </h3>
      {/* La fila se desliza de lado a propósito, y `data-scroll-x` es como se
          declara: sin él, el barrido de desbordamiento (test/e2e/mobile-audit.mjs)
          la contaba como contenido que se sale, en cada pantalla y cada día. */}
      <ul
        aria-labelledby={id}
        data-scroll-x
        class="-mx-1 flex snap-x gap-3 overflow-x-auto px-1 pt-0.5 pb-2"
      >
        {lista.map((p) => (
          <Tarjeta key={p.key} p={p} {...resto} />
        ))}
      </ul>
    </div>
  );
}

export function GaleriaPlantillas({
  t,
  plantillas,
  deOrganizacion = [],
  creando,
  deshabilitada,
  onElegir,
  temas = [],
  tema = '',
  onTema,
}: GaleriaPlantillasProps): JSX.Element | null {
  if (plantillas.length === 0 && deOrganizacion.length === 0) return null;
  const comun = { t, creando, deshabilitada, onElegir };
  return (
    <>
      {temas.length > 0 && onTema && (
        <label class="text-text-muted mt-5 flex items-center gap-2 text-xs">
          {t('list.themes.label')}
          <select
            value={tema}
            disabled={deshabilitada}
            onChange={(e) => onTema(e.currentTarget.value)}
            class="border-border bg-surface text-text sytue-focus-ring rounded-md border px-1.5 py-1 text-xs"
          >
            <option value="">{t('list.themes.none')}</option>
            {temas.map((x) => (
              <option key={x.id} value={x.id}>
                {x.label}
              </option>
            ))}
          </select>
        </label>
      )}
      {deOrganizacion.length > 0 && (
        <Fila
          id="org-templates-heading"
          titulo={t('list.templates.org')}
          lista={deOrganizacion}
          {...comun}
        />
      )}
      {plantillas.length > 0 && (
        <Fila
          id="templates-heading"
          titulo={t('list.templates.heading')}
          lista={plantillas}
          {...comun}
        />
      )}
    </>
  );
}
