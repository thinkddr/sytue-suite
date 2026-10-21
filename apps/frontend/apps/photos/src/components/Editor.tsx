/**
 * El EDITOR de Sytue Photos: una foto, cuatro deslizadores, giro y espejo, y un
 * lienzo que responde al arrastre sin pedirle nada al servidor.
 *
 * Cómo se reparte el trabajo:
 *   1. La `previa` (2048 px) se baja UNA vez y se decodifica a `ImageBitmap`.
 *   2. La geometría de la receta se aplica en Canvas 2D → un canvas «fuente».
 *      Solo cambia cuando cambia un paso de geometría.
 *   3. El color se pinta con WebGL2 sobre la fuente en cada arrastre; sin WebGL,
 *      la CPU hace el mismo cálculo (más lento, mismo resultado).
 *   4. Guardar manda la receta ENTERA; el servidor devuelve la huella y con ella
 *      se nombran las derivadas nuevas. La galería la verá al recargar.
 *
 * Los deslizadores son `<input type="range">` nativos: teclado, lector de
 * pantalla y táctil vienen de serie. Cada deslizador es UN paso de su tipo en la
 * receta (el último gana); giro y espejo AÑADEN pasos, y deshacer quita el último.
 */
import type { JSX } from 'preact';
import { driveFolderUrl, orgActual } from '@sytue/config';
import { useCallback, useEffect, useMemo, useRef, useState } from 'preact/hooks';
import {
  MASCARA_SCROLL_X,
  Button,
  Icon,
  IconButton,
  Spinner,
  cx,
  useScrollRestante,
  atajoDelSistema,
} from '@sytue/ui';
import type { Locale } from '@sytue/i18n';
import {
  foto as pedirFoto,
  guardarReceta,
  obtenerReceta,
  urlDeVarianteCon,
  type Foto,
} from '../client/photosApi.js';
import { PROPORCIONES, conProporcion } from '../editor/proporcion.js';
import type { AjustesIA, RecorteIA } from '../lib/edicionIA.js';
import {
  obtenerFondoLienzo,
  guardarFondoLienzo,
  estilosFondoLienzo,
  type FondoLienzo,
} from '../lib/fondoLienzo.js';

/** Rótulo de cada proporción; el texto es el mismo en los cinco idiomas. */
const ETIQUETA_PROPORCION: Record<string, string> = {
  '1': '1:1',
  [String(4 / 3)]: '4:3',
  [String(3 / 2)]: '3:2',
  [String(16 / 9)]: '16:9',
};
import { useT } from '../i18n/index.js';
import {
  RANGOS,
  huella,
  recetaVacia,
  type Paso,
  type Receta,
  type TipoPaso,
} from '../editor/receta.js';
import {
  MotorGL,
  aplicarColorCPU,
  aplicarGeometria,
  esDeColor,
  esDeGeometria,
} from '../editor/motor.js';

interface Props {
  locale: Locale;
  id: string;
  /** Carpeta de Drive desde la que se abrió la foto, si vino de allí. */
  volverA?: string | undefined;
}

type Deslizador = Extract<TipoPaso, 'exposicion' | 'contraste' | 'saturacion' | 'temperatura'>;
const DESLIZADORES: Deslizador[] = ['exposicion', 'contraste', 'saturacion', 'temperatura'];
const ETIQUETA = {
  exposicion: 'editor.exposure',
  contraste: 'editor.contrast',
  saturacion: 'editor.saturation',
  temperatura: 'editor.temperature',
} as const;

/** Preajustes rápidos de edición compleja atendidos por Sytue AI. */
const PREAJUSTES_IA = [
  { id: 'mejora-auto', labelKey: 'editor.aiPresetAuto' },
  { id: 'atardecer', labelKey: 'editor.aiPresetGolden' },
  { id: 'cinematografico', labelKey: 'editor.aiPresetCinematic' },
  { id: 'bn-dramatico', labelKey: 'editor.aiPresetBw' },
  { id: 'vivido', labelKey: 'editor.aiPresetVivid' },
  { id: 'vintage', labelKey: 'editor.aiPresetVintage' },
  { id: 'reencuadre', labelKey: 'editor.aiPresetCrop' },
  { id: 'noche', labelKey: 'editor.aiPresetNight' },
] as const;

/** El valor vigente de un deslizador: el ÚLTIMO paso de su tipo, o 0. */
function valorDe(r: Receta, tipo: Deslizador): number {
  for (let i = r.pasos.length - 1; i >= 0; i--) {
    if (r.pasos[i]!.tipo === tipo) return r.pasos[i]!.params?.valor ?? 0;
  }
  return 0;
}

/** Sustituye (o añade) el paso de ese deslizador. A 0 se quita: no ensucia la receta. */
function conValor(r: Receta, tipo: Deslizador, valor: number): Receta {
  const pasos = r.pasos.filter((p) => p.tipo !== tipo);
  if (valor !== 0) pasos.push({ tipo, params: { valor } });
  return { ...r, pasos };
}

export function Editor({ locale, id, volverA }: Props): JSX.Element {
  const t = useT(locale);
  const [foto, setFoto] = useState<Foto | null>(null);
  const [receta, setReceta] = useState<Receta>(recetaVacia());
  const [guardada, setGuardada] = useState<Receta>(recetaVacia());
  const [estado, setEstado] = useState<'cargando' | 'lista' | 'guardando'>('cargando');
  const [aviso, setAviso] = useState('');
  const bitmap = useRef<ImageBitmap | null>(null);
  const fuente = useRef<HTMLCanvasElement | null>(null);
  const lienzo = useRef<HTMLCanvasElement | null>(null);
  const gl = useRef<MotorGL | null>(null);
  const [motor, setMotor] = useState<'gpu' | 'cpu' | null>(null);
  const [pestana, setPestana] = useState<'ajustes' | 'ia'>('ajustes');
  // «Comparar con el original»: el lienzo enseña la foto con su geometría y SIN los
  // ajustes de color, que es lo que se compara en un revelado. Se queda puesto hasta
  // volver a pulsarlo (o la «\»), y no se pierde nada: la receta no cambia.
  const [comparar, setComparar] = useState(false);
  const [aiPrompt, setAiPrompt] = useState('');
  const [aiCargando, setAiCargando] = useState(false);
  const [aiError, setAiError] = useState('');
  const [aiResultado, setAiResultado] = useState<{
    explicacion: string;
    ajustes: AjustesIA;
    recorte?: RecorteIA;
  } | null>(null);
  // La cabecera es una fila que NO envuelve; en un teléfono estrecho se desplaza
  // de lado antes que quedarse sin el botón de guardar (ver scrollX.ts).
  const cabecera = useScrollRestante<HTMLElement>();
  // Relación de aspecto del lienzo TRAS la geometría (recorte/giro incluidos).
  // Se necesita como CSS `aspect-ratio` del contenedor: sin ella, un lienzo
  // dentro de una caja centrada (`items-center`) que no lo estira no tiene
  // una altura DEFINIDA de la que `max-height:100%` pueda partir —el límite
  // se ignora (una altura en % contra una caja de altura automática no cuenta,
  // es la propia especificación de CSS) y el lienzo se pinta a su tamaño
  // natural aunque no quepa. Medido en un iPhone: el lienzo se salía casi
  // 150px por debajo de su propio contenedor y tapaba los controles.
  const [proporcionLienzo, setProporcionLienzo] = useState<[number, number] | null>(null);
  const [fondo, setFondo] = useState<FondoLienzo>(obtenerFondoLienzo);
  const cambiarFondo = useCallback((f: FondoLienzo) => {
    setFondo(f);
    guardarFondoLienzo(f);
  }, []);
  const estiloFondo = useMemo(() => estilosFondoLienzo(fondo), [fondo]);

  const ejecutarEdicionIA = useCallback(
    async (accion?: string, instruccionTexto?: string) => {
      setAiCargando(true);
      setAiError('');
      try {
        const res = await fetch('./api/editar-ia', {
          method: 'POST',
          headers: { 'content-type': 'application/json' },
          body: JSON.stringify({
            id,
            accion: accion || undefined,
            instruccion: instruccionTexto || aiPrompt.trim() || undefined,
          }),
        });
        const data = (await res.json().catch(() => ({}))) as {
          ok?: boolean;
          explicacion?: string;
          ajustes?: AjustesIA;
          recorte?: RecorteIA;
        };
        if (!res.ok || !data.ok || !data.explicacion || !data.ajustes) {
          setAiError(t('editor.aiFailed'));
          return;
        }

        setAiResultado({
          explicacion: data.explicacion,
          ajustes: data.ajustes,
          ...(data.recorte ? { recorte: data.recorte } : {}),
        });

        // Aplicar los ajustes calculados por Sytue AI a la receta
        setReceta((prevReceta) => {
          let nueva = prevReceta;
          if (data.recorte) {
            nueva = {
              ...nueva,
              pasos: [
                ...nueva.pasos,
                {
                  tipo: 'recorte',
                  params: {
                    x: data.recorte.x,
                    y: data.recorte.y,
                    ancho: data.recorte.ancho,
                    alto: data.recorte.alto,
                  },
                },
              ],
            };
          }
          for (const tipo of DESLIZADORES) {
            if (data.ajustes && typeof data.ajustes[tipo] === 'number') {
              nueva = conValor(nueva, tipo, data.ajustes[tipo]);
            }
          }
          return nueva;
        });

        setAviso(t('editor.aiResultTitle'));
      } catch {
        setAiError(t('editor.aiFailed'));
      } finally {
        setAiCargando(false);
      }
    },
    [id, aiPrompt, t],
  );

  // 1. Foto, receta guardada y previa, en paralelo.
  useEffect(() => {
    let viva = true;
    (async () => {
      // LA PREVIA DEL ORIGINAL, y no la de la receta vigente.
      //
      // La derivada lleva la receta HORNEADA: el servidor se la aplicó al
      // generarla. Cargar esa y volver a aplicarle la receta aquí enseñaba
      // cada ajuste dos veces —una foto girada aparecía del revés, una subida
      // de exposición quemada— en cuanto se abría algo ya editado.
      //
      // El original se pide por el nombre que le corresponde: la huella de la
      // receta VACÍA, que se calcula aquí con el mismo algoritmo que Go.
      const original = await huella(recetaVacia());
      const [f, r, blob] = await Promise.all([
        pedirFoto(id),
        obtenerReceta(id),
        fetch(urlDeVarianteCon(id, 'previa', original), { credentials: 'include' }).then((res) => {
          if (!res.ok) throw new Error(`previa ${res.status}`);
          return res.blob();
        }),
      ]);
      const bm = await createImageBitmap(blob);
      if (!viva) return;
      bitmap.current = bm;
      setFoto(f);
      setReceta(r.recipe);
      setGuardada(r.recipe);
      setEstado('lista');
    })().catch(() => viva && setAviso(t('editor.saveFailed')));
    return () => {
      viva = false;
      bitmap.current?.close();
    };
    // `t` queda fuera a propósito: esto carga la foto, y meterlo en las
    // dependencias volvería a descargarla entera al cambiar de idioma. Solo se
    // usa para el texto de un aviso de error.
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [id]);

  // 2. La fuente (geometría) se rehace solo cuando cambian los pasos de geometría.
  const claveGeometria = useMemo(
    () => JSON.stringify(receta.pasos.filter(esDeGeometria)),
    [receta],
  );
  useEffect(() => {
    const bm = bitmap.current;
    if (!bm || estado === 'cargando') return;
    fuente.current = aplicarGeometria(bm, bm.width, bm.height, receta);
    gl.current?.cargar(fuente.current);
    setProporcionLienzo([fuente.current.width, fuente.current.height]);
    // `receta` entera queda fuera de las dependencias A PROPÓSITO: la clave ya
    // resume los pasos de GEOMETRÍA, que son los únicos que `aplicarGeometria`
    // mira. Añadirla haría rehacer la geometría —y recargar la textura— en cada
    // movimiento de un deslizador de color, que es justo lo que este efecto
    // existe para evitar. Si algún día la geometría pasa a depender de otro
    // paso, lo que hay que ampliar es `claveGeometria`, no esta lista.
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [claveGeometria, estado]);

  // 3. El color, en cada cambio de receta; WebGL si hay, CPU si no.
  useEffect(() => {
    const vista = comparar ? { ...receta, pasos: receta.pasos.filter(esDeGeometria) } : receta;
    const src = fuente.current;
    const c = lienzo.current;
    if (!src || !c || estado === 'cargando') return;
    if (motor === null) {
      // ponytail: un solo intento de WebGL por montaje; si falla, CPU para siempre.
      const m = MotorGL.crear(c);
      gl.current = m;
      setMotor(m ? 'gpu' : 'cpu');
      m?.cargar(src);
      return;
    }
    if (gl.current) {
      gl.current.pintar(vista, src.width, src.height);
    } else {
      c.width = src.width;
      c.height = src.height;
      const ctx = c.getContext('2d')!;
      ctx.drawImage(src, 0, 0);
      const img = ctx.getImageData(0, 0, c.width, c.height);
      aplicarColorCPU(img, vista);
      ctx.putImageData(img, 0, 0);
    }
  }, [receta, motor, claveGeometria, estado, comparar]);

  const anyadir = useCallback(
    (p: Paso) => setReceta((r) => ({ ...r, pasos: [...r.pasos, p] })),
    [],
  );
  const deshacer = useCallback(() => setReceta((r) => ({ ...r, pasos: r.pasos.slice(0, -1) })), []);
  const sucia = JSON.stringify(receta) !== JSON.stringify(guardada);

  const guardar = useCallback(async () => {
    setEstado('guardando');
    try {
      await guardarReceta(id, receta);
      setGuardada(receta);
      setAviso(t('editor.saved'));
    } catch {
      setAviso(t('editor.saveFailed'));
    } finally {
      setEstado('lista');
    }
  }, [id, receta, t]);

  // ── Recorte por arrastre. El marco se guarda en FRACCIONES del lienzo
  // (0..1), que es lo que la receta espera y lo que sobrevive a cualquier
  // tamaño de pantalla.
  const [recortando, setRecortando] = useState(false);
  const [marco, setMarco] = useState<{ x: number; y: number; w: number; h: number } | null>(null);
  // Proporción fija del recorte (null = libre). Se conserva entre recortes.
  const [proporcion, setProporcion] = useState<number | null>(null);
  const origen = useRef<{ x: number; y: number } | null>(null);
  const medidas = useRef<{ ancho: number; alto: number }>({ ancho: 0, alto: 0 });
  const fraccion = (e: PointerEvent) => {
    const r = lienzo.current!.getBoundingClientRect();
    medidas.current = { ancho: r.width, alto: r.height };
    return {
      x: Math.min(1, Math.max(0, (e.clientX - r.left) / r.width)),
      y: Math.min(1, Math.max(0, (e.clientY - r.top) / r.height)),
    };
  };
  const empezarRecorte = (e: PointerEvent) => {
    if (!recortando) return;
    (e.currentTarget as HTMLElement).setPointerCapture(e.pointerId);
    origen.current = fraccion(e);
    setMarco({ ...origen.current, w: 0, h: 0 });
  };
  const moverRecorte = (e: PointerEvent) => {
    if (!recortando || !origen.current) return;
    const p = fraccion(e);
    const bruto = {
      x: Math.min(origen.current.x, p.x),
      y: Math.min(origen.current.y, p.y),
      w: Math.abs(p.x - origen.current.x),
      h: Math.abs(p.y - origen.current.y),
    };
    setMarco(
      proporcion === null
        ? bruto
        : conProporcion(bruto, proporcion, medidas.current, origen.current),
    );
  };
  const soltarRecorte = () => {
    origen.current = null;
  };
  const aplicarRecorte = () => {
    if (marco && marco.w > 0.01 && marco.h > 0.01) {
      anyadir({
        tipo: 'recorte',
        params: { x: marco.x, y: marco.y, ancho: marco.w, alto: marco.h },
      });
    }
    setMarco(null);
    setRecortando(false);
  };
  const cancelarRecorte = () => {
    setMarco(null);
    setRecortando(false);
  };

  // Atajos de teclado: los de siempre, sin robar nada a los campos de texto.
  useEffect(() => {
    const onKey = (e: KeyboardEvent) => {
      const tag = (e.target as HTMLElement | null)?.tagName;
      if (tag === 'INPUT' || tag === 'TEXTAREA' || tag === 'SELECT') return;
      const mod = e.ctrlKey || e.metaKey;
      if (mod && e.key.toLowerCase() === 'z') {
        e.preventDefault();
        deshacer();
      } else if (mod && e.key.toLowerCase() === 's') {
        e.preventDefault();
        if (sucia && estado === 'lista') void guardar();
      } else if (!mod && e.key.toLowerCase() === 'r') {
        anyadir({ tipo: 'giro', params: { grados: 90 } });
      } else if (!mod && e.key.toLowerCase() === 'c') {
        setRecortando((v) => !v);
        setMarco(null);
      } else if (!mod && e.key === '\\') {
        setComparar((v) => !v);
      } else if (e.key === 'Escape' && recortando) {
        cancelarRecorte();
      }
    };
    window.addEventListener('keydown', onKey);
    return () => window.removeEventListener('keydown', onKey);
  }, [deshacer, guardar, anyadir, sucia, estado, recortando]);

  return (
    <div class="sytue-safe-fullscreen bg-bg text-text flex h-dvh min-h-0 flex-col">
      {/* La fila nunca envuelve y lo fijo no se encoge: si no cabe —nombre largo
            y cinco mandos en un teléfono— se desplaza de lado, con la máscara
            avisando de que queda contenido. Sin scroll, «Guardar» se salía de la
            pantalla y no había forma de alcanzarlo. */}
      <header
        data-scroll-x
        ref={cabecera.ref}
        class={cx(
          'border-border flex items-center gap-2 overflow-x-auto border-b px-3 py-2 [&>*]:shrink-0',
          cabecera.queda && MASCARA_SCROLL_X,
        )}
      >
        {/* La salida devuelve A DONDE SE ENTRÓ. Quien abre una foto desde su
            carpeta de Drive no venía de la galería, y mandarlo allí le obliga a
            rehacer a mano el camino hasta la carpeta. Con la org de la URL actual
            (ADR 0056) la vuelta sale canónica, sin el 302 del middleware de
            Drive; sin org —p. ej. en SSR— la añade ese salto igualmente. */}
        {/* El `Button` de la casa y no un enlace de texto: medía 18 px de alto, por
            debajo de los 24 de WCAG 2.5.8 y de los 44 del dedo. */}
        <Button
          href={volverA ? driveFolderUrl(volverA, orgActual(import.meta.env.BASE_URL)) : './'}
          variant="ghost"
          size="sm"
          leadingIcon={<Icon name="chevronLeft" />}
        >
          {volverA ? t('editor.backToDrive') : t('editor.back')}
        </Button>
        <span class="truncate text-sm font-medium">{foto?.name ?? ''}</span>
        {/* Solo los pasos: «GPU»/«CPU» era un dato del motor, no de la foto, y en una
            cabecera de edición es ruido. Quien lo necesita lo tiene en el `title`. */}
        <span
          class="text-text-muted ml-auto text-xs tabular-nums"
          title={motor === 'gpu' ? 'WebGL2' : motor === 'cpu' ? 'ImageData' : undefined}
        >
          {t('editor.steps', { count: receta.pasos.length })}
        </span>
        <IconButton
          label={t('editor.compare')}
          pressed={comparar}
          aria-keyshortcuts={'\\'}
          onClick={() => setComparar((v) => !v)}
          disabled={!receta.pasos.some(esDeColor)}
        >
          <Icon name="ojo" />
        </IconButton>
        <IconButton
          label={t('editor.undo')}
          onClick={deshacer}
          disabled={receta.pasos.length === 0}
        >
          <Icon name="undo" />
        </IconButton>
        <Button
          variant="primary"
          size="sm"
          onClick={() => void guardar()}
          loading={estado === 'guardando'}
          disabled={!sucia}
        >
          {t('editor.save')}
        </Button>
      </header>

      <div class="flex min-h-0 flex-1 flex-col md:flex-row">
        {/* El lienzo ocupa lo que quede; la imagen se ajusta dentro sin deformarse. */}
        <div class="bg-bg-subtle relative flex min-h-0 flex-1 items-center justify-center p-3 max-md:min-h-[40dvh]">
          {estado === 'cargando' ? <Spinner label={t('editor.loading')} /> : null}
          {/* El contenedor mide EXACTAMENTE lo que el lienzo (inline-block +
              relative), así la capa de recorte y el lienzo comparten geometría. */}
          <div
            class={`relative inline-block max-h-full max-w-full rounded-xs ${recortando ? 'cursor-crosshair select-none' : ''}`}
            style={{
              display: estado === 'cargando' ? 'none' : 'inline-block',
              // Sin esto, un `inline-block` que se centra (no se estira) no tiene
              // una altura DEFINIDA hasta saber la de su contenido —el lienzo—, y el
              // lienzo a su vez necesita la del contenedor para su `max-height:100%`:
              // circular, y CSS lo resuelve ignorando ese límite (ver el comentario
              // de `proporcionLienzo`). Fijar aquí la proporción real rompe el
              // círculo: el contenedor ya sabe su tamaño por el ancho disponible, sin
              // esperar al lienzo.
              ...(proporcionLienzo
                ? { aspectRatio: `${proporcionLienzo[0]} / ${proporcionLienzo[1]}` }
                : {}),
              ...estiloFondo.style,
            }}
            onPointerDown={empezarRecorte}
            onPointerMove={moverRecorte}
            onPointerUp={soltarRecorte}
            onPointerCancel={soltarRecorte}
          >
            <canvas
              ref={lienzo}
              class="block max-h-full max-w-full object-contain shadow-sm"
              style={estiloFondo.style}
            />
            {recortando && marco ? (
              <div
                aria-hidden="true"
                class="pointer-events-none absolute border-2 border-white shadow-[0_0_0_9999px_rgba(0,0,0,0.55)]"
                style={{
                  left: `${marco.x * 100}%`,
                  top: `${marco.y * 100}%`,
                  width: `${marco.w * 100}%`,
                  height: `${marco.h * 100}%`,
                }}
              />
            ) : null}
          </div>
        </div>

        <aside
          class={
            'border-border flex w-full flex-col gap-4 border-t p-4 max-md:shrink ' +
            'max-md:min-h-0 max-md:overflow-y-auto md:w-80 md:shrink-0 md:border-t-0 md:border-l'
          }
        >
          {/* Selector de pestañas: Ajustes manuales y Sytue AI */}
          <div class="border-border -mx-4 -mt-4 mb-1 flex border-b">
            <button
              type="button"
              onClick={() => setPestana('ajustes')}
              class={cx(
                'sytue-focus-ring flex-1 border-b-2 py-2.5 text-center text-xs font-medium transition-colors',
                pestana === 'ajustes'
                  ? 'border-primary text-text font-semibold'
                  : 'text-text-muted hover:bg-bg-subtle hover:text-text border-transparent',
              )}
            >
              {t('editor.tabAdjustments')}
            </button>
            <button
              type="button"
              onClick={() => setPestana('ia')}
              class={cx(
                'sytue-focus-ring flex flex-1 items-center justify-center gap-1.5 border-b-2 py-2.5 text-center text-xs font-medium transition-colors',
                pestana === 'ia'
                  ? 'border-primary text-text font-semibold'
                  : 'text-text-muted hover:bg-bg-subtle hover:text-text border-transparent',
              )}
            >
              <Icon name="ai" size={12} />
              <span>{t('editor.tabAi')}</span>
            </button>
          </div>

          {pestana === 'ajustes' ? (
            <>
              {DESLIZADORES.map((tipo) => {
                const [min, max] = RANGOS[tipo].valor!;
                const v = valorDe(receta, tipo);
                // Bipolar: el tramo lleno va del 0 al valor. Con el relleno desde la
                // izquierda de un `range` nativo, a 0 la barra salía medio llena y no
                // decía hacia qué lado iba el ajuste.
                const cero = ((0 - min) / (max - min)) * 100;
                const aqui = ((v - min) / (max - min)) * 100;
                return (
                  // El rótulo va con `htmlFor` y no envolviendo al deslizador: con el texto
                  // dos niveles por debajo, el linter (y algún lector de pantalla) no lo
                  // asociaba al control.
                  <div key={tipo} class="flex flex-col gap-1 text-sm">
                    <span class="flex justify-between">
                      <label htmlFor={`ajuste-${tipo}`}>{t(ETIQUETA[tipo])}</label>
                      <output class="text-text-muted tabular-nums">
                        {v > 0 ? '+' : ''}
                        {v.toFixed(2)}
                      </output>
                    </span>
                    <input
                      id={`ajuste-${tipo}`}
                      type="range"
                      min={min}
                      max={max}
                      step={0.01}
                      value={v}
                      onInput={(e) =>
                        setReceta((r) =>
                          conValor(r, tipo, Number((e.target as HTMLInputElement).value)),
                        )
                      }
                      onDblClick={() => setReceta((r) => conValor(r, tipo, 0))}
                      class="deslizador-ajuste w-full"
                      style={{
                        '--desde': `${Math.min(cero, aqui)}%`,
                        '--hasta': `${Math.max(cero, aqui)}%`,
                      }}
                    />
                  </div>
                );
              })}

              {recortando ? (
                <div
                  class="bg-bg-subtle flex flex-col gap-2 rounded-sm p-2 text-sm"
                  role="group"
                  aria-label={t('editor.crop')}
                >
                  <p class="text-text-muted text-xs">{t('editor.cropHint')}</p>
                  {/* Proporciones fijas: lo que pide un avatar (1:1) o una portada
                      (16:9). El cálculo va en píxeles, no en fracciones del lienzo. */}
                  <div class="flex flex-wrap gap-1" role="group" aria-label={t('editor.cropRatio')}>
                    {PROPORCIONES.map((p) => (
                      <button
                        key={String(p)}
                        type="button"
                        aria-pressed={proporcion === p}
                        onClick={() => {
                          setProporcion(p);
                          if (p !== null && marco && origen.current === null) {
                            setMarco(
                              conProporcion(marco, p, medidas.current, { x: marco.x, y: marco.y }),
                            );
                          }
                        }}
                        class={cx(
                          'sytue-focus-ring rounded-sm border px-2 py-0.5 text-xs transition-colors duration-150',
                          proporcion === p
                            ? 'border-primary bg-selected text-primary-text'
                            : 'border-border text-text-muted hover:bg-bg-subtle',
                        )}
                      >
                        {p === null ? t('editor.cropFree') : ETIQUETA_PROPORCION[String(p)]}
                      </button>
                    ))}
                  </div>
                  <div class="flex gap-2">
                    <Button
                      variant="primary"
                      size="sm"
                      onClick={aplicarRecorte}
                      disabled={!marco || marco.w < 0.01}
                    >
                      {t('editor.cropApply')}
                    </Button>
                    <Button variant="ghost" size="sm" onClick={cancelarRecorte}>
                      {t('editor.cropCancel')}
                    </Button>
                  </div>
                </div>
              ) : null}

              <div class="flex flex-wrap gap-2">
                <IconButton
                  label={t('editor.crop')}
                  onClick={() => setRecortando((v) => !v)}
                  aria-pressed={recortando}
                  class={recortando ? 'bg-bg-subtle ring-primary ring-2' : undefined}
                >
                  <Icon name="recorte" />
                </IconButton>
                <IconButton
                  label={t('editor.rotate')}
                  onClick={() => anyadir({ tipo: 'giro', params: { grados: 90 } })}
                >
                  <Icon name="rotate" />
                </IconButton>
                <IconButton
                  label={t('editor.flipH')}
                  onClick={() => anyadir({ tipo: 'espejo', eje: 'h' })}
                >
                  <Icon name="flipH" />
                </IconButton>
                <IconButton
                  label={t('editor.flipV')}
                  onClick={() => anyadir({ tipo: 'espejo', eje: 'v' })}
                >
                  <Icon name="flipV" />
                </IconButton>
                <Button
                  variant="ghost"
                  size="sm"
                  onClick={() => setReceta(recetaVacia())}
                  disabled={receta.pasos.length === 0}
                >
                  {t('editor.reset')}
                </Button>
              </div>

              {/* Selector de fondo del lienzo */}
              <div class="border-border flex flex-col gap-1.5 border-t pt-3 text-xs font-medium">
                <span class="text-text-muted flex items-center gap-1">
                  <Icon name="grid" size={13} />
                  <span>{t('editor.canvasBackground')}</span>
                </span>
                <div
                  class="grid grid-cols-2 gap-1.5"
                  role="group"
                  aria-label={t('editor.canvasBackground')}
                >
                  {(['crema', 'blanco', 'oscuro', 'damero'] as const).map((opcion) => (
                    <button
                      key={opcion}
                      type="button"
                      aria-pressed={fondo === opcion}
                      onClick={() => cambiarFondo(opcion)}
                      class={cx(
                        'sytue-focus-ring rounded-control flex items-center gap-2 border px-2.5 py-1.5 text-xs transition-colors',
                        fondo === opcion
                          ? 'border-primary bg-selected text-primary-text font-semibold'
                          : 'border-border text-text hover:bg-bg-subtle',
                      )}
                    >
                      <span
                        class={cx(
                          'h-3.5 w-3.5 shrink-0 rounded-full border',
                          opcion === 'crema' && 'border-[#d8d3c6] bg-[#faf9f6]',
                          opcion === 'blanco' && 'border-gray-300 bg-white',
                          opcion === 'oscuro' && 'border-gray-700 bg-[#131619]',
                          opcion === 'damero' &&
                            'border-gray-300 bg-white bg-[repeating-conic-gradient(#cbd5e1_0%_25%,transparent_0%_50%)] bg-[length:6px_6px]',
                        )}
                      />
                      <span class="truncate">{t(`editor.bg_${opcion}`)}</span>
                    </button>
                  ))}
                </div>
              </div>
            </>
          ) : (
            <div class="flex flex-col gap-4">
              <div>
                <h3 class="text-text flex items-center gap-1.5 text-sm font-semibold">
                  <Icon name="ai" size={14} />
                  <span>{t('editor.aiTitle')}</span>
                </h3>
                <p class="text-text-muted mt-0.5 text-xs">{t('editor.aiDescription')}</p>
              </div>

              {/* Formulario de prompt libre */}
              <div class="flex flex-col gap-2">
                <textarea
                  value={aiPrompt}
                  onInput={(e) => setAiPrompt((e.target as HTMLTextAreaElement).value)}
                  placeholder={t('editor.aiPromptPlaceholder')}
                  rows={3}
                  disabled={aiCargando}
                  class="sytue-focus-ring border-border bg-surface text-text placeholder:text-text-muted w-full resize-none rounded-sm border p-2 text-xs"
                />
                <Button
                  variant="primary"
                  size="sm"
                  onClick={() => void ejecutarEdicionIA(undefined, aiPrompt)}
                  loading={aiCargando}
                  disabled={!aiPrompt.trim() || aiCargando}
                >
                  <Icon name="ai" size={12} />
                  <span>{t('editor.aiApply')}</span>
                </Button>
              </div>

              {/* Acciones y transformaciones sugeridas */}
              <div class="flex flex-col gap-1.5">
                <span class="text-text-muted text-xs font-medium tracking-wider uppercase">
                  {t('editor.aiPresets')}
                </span>
                <div class="grid grid-cols-2 gap-1.5">
                  {PREAJUSTES_IA.map((p) => (
                    <button
                      key={p.id}
                      type="button"
                      disabled={aiCargando}
                      onClick={() => void ejecutarEdicionIA(p.id)}
                      class={cx(
                        'sytue-focus-ring border-border bg-surface text-text flex items-center justify-start rounded-sm border px-2.5 py-2 text-left text-xs transition-colors',
                        'hover:bg-bg-subtle disabled:pointer-events-none disabled:opacity-40',
                      )}
                    >
                      <span class="truncate">{t(p.labelKey)}</span>
                    </button>
                  ))}
                </div>
              </div>

              {/* Spinner durante procesamiento */}
              {aiCargando ? (
                <div class="bg-bg-subtle text-text-muted flex items-center justify-center gap-2 rounded-sm p-3 text-xs">
                  <Spinner label={t('editor.aiProcessing')} />
                  <span>{t('editor.aiProcessing')}</span>
                </div>
              ) : null}

              {/* Mensaje de error si falla */}
              {aiError ? (
                // La pareja de error de la suite (`danger-text` sobre `danger-bg`): el rojo
                // de relleno sobre su velo al 10 % medía 4,42:1 en claro y 2,96 en oscuro.
                <p role="alert" class="text-danger-text bg-danger-bg rounded-sm p-2 text-xs">
                  {aiError}
                </p>
              ) : null}

              {/* Tarjeta de resultado con explicación de la IA */}
              {aiResultado ? (
                <div class="bg-bg-subtle border-border flex flex-col gap-2 rounded-sm border p-3 text-xs">
                  <div class="text-text flex items-center justify-between font-medium">
                    <span class="flex items-center gap-1">
                      <Icon name="ai" size={12} />
                      <span>{t('editor.aiResultTitle')}</span>
                    </span>
                    <button
                      type="button"
                      onClick={() => setPestana('ajustes')}
                      class="sytue-focus-ring text-primary hover:underline"
                    >
                      {t('editor.aiViewManual')} →
                    </button>
                  </div>
                  <p class="text-text-muted leading-relaxed italic">«{aiResultado.explicacion}»</p>
                  <div class="flex flex-wrap gap-1 pt-1">
                    {aiResultado.ajustes.exposicion !== undefined &&
                    aiResultado.ajustes.exposicion !== 0 ? (
                      <span class="bg-surface border-border rounded border px-1.5 py-0.5 text-[11px] tabular-nums">
                        Exp:{' '}
                        {aiResultado.ajustes.exposicion > 0
                          ? `+${aiResultado.ajustes.exposicion}`
                          : aiResultado.ajustes.exposicion}
                      </span>
                    ) : null}
                    {aiResultado.ajustes.contraste !== undefined &&
                    aiResultado.ajustes.contraste !== 0 ? (
                      <span class="bg-surface border-border rounded border px-1.5 py-0.5 text-[11px] tabular-nums">
                        Cont:{' '}
                        {aiResultado.ajustes.contraste > 0
                          ? `+${aiResultado.ajustes.contraste}`
                          : aiResultado.ajustes.contraste}
                      </span>
                    ) : null}
                    {aiResultado.ajustes.saturacion !== undefined &&
                    aiResultado.ajustes.saturacion !== 0 ? (
                      <span class="bg-surface border-border rounded border px-1.5 py-0.5 text-[11px] tabular-nums">
                        Sat:{' '}
                        {aiResultado.ajustes.saturacion > 0
                          ? `+${aiResultado.ajustes.saturacion}`
                          : aiResultado.ajustes.saturacion}
                      </span>
                    ) : null}
                    {aiResultado.ajustes.temperatura !== undefined &&
                    aiResultado.ajustes.temperatura !== 0 ? (
                      <span class="bg-surface border-border rounded border px-1.5 py-0.5 text-[11px] tabular-nums">
                        Temp:{' '}
                        {aiResultado.ajustes.temperatura > 0
                          ? `+${aiResultado.ajustes.temperatura}`
                          : aiResultado.ajustes.temperatura}
                      </span>
                    ) : null}
                    {aiResultado.recorte ? (
                      <span class="bg-surface border-border rounded border px-1.5 py-0.5 text-[11px]">
                        {t('editor.crop')}
                      </span>
                    ) : null}
                  </div>
                </div>
              ) : null}
            </div>
          )}

          <p role="status" aria-live="polite" class="text-text-muted min-h-5 text-xs">
            {aviso}
          </p>
          <p class="text-text-muted mt-auto text-xs">{atajoDelSistema(t('editor.shortcuts'))}</p>
        </aside>
      </div>
    </div>
  );
}
