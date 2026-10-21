/*
 * SPDX-License-Identifier: Apache-2.0
 * SPDX-FileCopyrightText: (c) 2026 Daniel Delgado Ramos
 *
 * LOS GESTOS DEL VISOR con el dedo: deslizar entre fotos, pellizcar para ampliar,
 * arrastrar lo ampliado y doble toque.
 *
 * DESLIZAR. Sin él, en un teléfono el único camino eran las flechas. La foto SIGUE AL
 * DEDO mientras se arrastra y al soltar pasa de foto o vuelve a su sitio.
 *
 * PELLIZCAR. Antes el pellizco ampliaba la PÁGINA entera —la cabecera con sus mandos
 * incluida— y había que deshacerlo a mano; ahora amplía la foto, hasta ×4, alrededor
 * de los dedos, y con ella ampliada un dedo la recorre (sin salirse: el borde de la foto
 * no entra en el hueco).
 *
 * DOBLE TOQUE. Amplía ×2,5 donde se toca, o vuelve a encajarla si ya lo estaba. Un toque
 * suelto no hace nada: con el dedo, ampliar al primer roce era lo que pasaba cada vez que
 * alguien tocaba la foto para otra cosa.
 *
 * Todo con `transform` sobre el `<img>` y sin repintar el visor: un `setState` por
 * `pointermove` rehacía la cabecera entera sesenta veces por segundo. Solo el dedo y el
 * lápiz; con el ratón, el clic en la foto es el tamaño real (y arrastrar la foto no pasa
 * de foto en ningún visor de escritorio).
 *
 * La regla de deslizar es la de `decidir`, y es la MISMA en Fyne (`decidirDeslizamiento`
 * en `apps/fyne/apps/photos/deslizar.go`): mismos umbrales, mismo resultado. El
 * pellizco no tiene pareja allí: ver la nota de deslizar.go.
 */
import { useEffect, useState } from 'preact/hooks';

/** Lo menos que tiene que recorrer el dedo, en px, para pasar de foto. */
export const RECORRIDO_MIN = 56;
/** O esta fracción del ancho, si es mayor: en una tableta 56 px es un roce. */
export const FRACCION_ANCHO = 0.18;
/** Un golpe rápido pasa aunque recorra menos: px por milisegundo. */
export const VELOCIDAD_RAPIDA = 0.5;
/** Y aun rápido, al menos esto: un temblor no es un gesto. */
export const RECORRIDO_RAPIDO = 24;
/** Horizontal de verdad: |dx| tiene que pasar de |dy| por este factor. */
export const DOMINIO_HORIZONTAL = 1.4;
/** Hacia un lado sin foto la foto va a un cuarto del dedo: se nota que no hay más. */
export const RESISTENCIA = 0.25;
/** Lo más que se amplía con los dedos, sobre la foto encajada. */
export const ESCALA_MAX = 4;
/** Lo que amplía un doble toque. */
export const ESCALA_DOBLE = 2.5;
/** Dos toques son uno doble si van a menos de esto, en ms y en px. */
export const DOBLE_MS = 300;
export const DOBLE_PX = 24;
/** Un toque que se mueve más de esto ya es un arrastre. */
const TOQUE_PX = 10;

/**
 * Qué hace un deslizamiento al soltar: −1 la anterior, 1 la siguiente, 0 nada. Hacia la
 * IZQUIERDA (dx < 0) se va a la siguiente, como pasar una página.
 */
export function decidir(dx: number, dy: number, ancho: number, ms: number): -1 | 0 | 1 {
  const ax = Math.abs(dx);
  if (ax < DOMINIO_HORIZONTAL * Math.abs(dy)) return 0;
  const lejos = ax >= Math.max(RECORRIDO_MIN, FRACCION_ANCHO * ancho);
  const rapido = ax >= RECORRIDO_RAPIDO && ax / Math.max(ms, 1) >= VELOCIDAD_RAPIDA;
  if (!lejos && !rapido) return 0;
  return dx < 0 ? 1 : -1;
}

/**
 * La traslación de la foto ampliada, sin que su borde entre en el hueco: con la foto de
 * `lado` px a escala `s` dentro de un hueco de `hueco` px, centrada, se puede mover como
 * mucho lo que sobresale por cada lado. Si no sobresale, se queda centrada.
 */
export function limitar(t: number, s: number, lado: number, hueco: number): number {
  const m = Math.max(0, (s * lado - hueco) / 2);
  return Math.min(m, Math.max(-m, t));
}

/**
 * La traslación que deja QUIETO el punto `m` de la pantalla al pasar de la escala `s0`
 * (con traslación `t0`) a `s`. Todo en px relativos al centro de la foto encajada. El
 * punto de la foto bajo `m` es `(m − t0) / s0`; tras ampliar tiene que seguir en `m`.
 */
export function anclar(m: number, s0: number, t0: number, s: number): number {
  return m - (s * (m - t0)) / s0;
}

interface Punto {
  x: number;
  y: number;
}

type Modo = 'nada' | 'deslizar' | 'mover' | 'pellizco';

interface Opciones {
  objetivo: { current: HTMLElement | null };
  /** La foto que se enseña: al cambiar, lo ampliado se olvida. */
  clave: string;
  onPrev?: (() => void) | undefined;
  onNext?: (() => void) | undefined;
  activo: boolean;
}

/** Lo que pasa por encima de un mando (las flechas, «Ver en la ficha») es del mando. */
const delMando = (e: Event): boolean =>
  e.target instanceof Element && e.target.closest('button, a') !== null;

/**
 * Los gestos en curso y los manejadores del escenario. Una clase —y no estado suelto en
 * el hook— porque todo lo que guarda cambia DURANTE el gesto, fuera del render: el
 * compilador de React no deja mutar refs ni opciones desde un hook, y un objeto propio
 * con sus métodos es lo que se muta sin pelearse con él.
 */
class Gestos {
  op: Opciones = { objetivo: { current: null }, clave: '', activo: false };
  private dedos = new Map<number, Punto>();
  /** La transformación de la foto: escala y traslación respecto de su sitio encajada. */
  private s = 1;
  private tx = 0;
  private ty = 0;
  private modo: Modo = 'nada';
  private t0 = 0;
  private p0: Punto = { x: 0, y: 0 };
  private s0 = 1;
  private tx0 = 0;
  private ty0 = 0;
  private d0 = 1;
  private m0: Punto = { x: 0, y: 0 };
  private horizontal = false;
  private movido = false;
  private toque = { t: 0, x: 0, y: 0 };
  /** Cuándo acabó el último gesto: el `click` de detrás no es un clic en la foto. */
  private ultimoGesto = 0;

  poner(op: Opciones): void {
    this.op = op;
  }

  /** Otra foto: encajada, sin animar. */
  olvidar(): void {
    this.s = 1;
    this.tx = 0;
    this.ty = 0;
    this.pintar(false);
  }

  private pintar(animar: boolean, dx = 0): void {
    const el = this.op.objetivo.current;
    if (!el) return;
    const quieto = matchMedia('(prefers-reduced-motion: reduce)').matches;
    el.style.transition = animar && !quieto ? 'transform 150ms var(--ease-standard, ease-out)' : '';
    el.style.transform =
      this.s === 1 && this.tx === 0 && this.ty === 0 && dx === 0
        ? ''
        : `translate3d(${this.tx + dx}px,${this.ty}px,0) scale(${this.s})`;
  }

  private encajar(animar: boolean): void {
    this.s = 1;
    this.tx = 0;
    this.ty = 0;
    this.pintar(animar);
  }

  /** El centro del hueco y lo que miden el hueco y la foto encajada. */
  private medidas(hueco: HTMLElement) {
    const r = hueco.getBoundingClientRect();
    const el = this.op.objetivo.current;
    return {
      c: { x: r.left + r.width / 2, y: r.top + r.height / 2 },
      W: r.width,
      H: r.height,
      w: el?.offsetWidth ?? r.width,
      h: el?.offsetHeight ?? r.height,
    };
  }

  private limitarTodo(hueco: HTMLElement): void {
    const { W, H, w, h } = this.medidas(hueco);
    this.tx = limitar(this.tx, this.s, w, W);
    this.ty = limitar(this.ty, this.s, h, H);
  }

  private empezar(modo: Modo, hueco: HTMLElement): void {
    const ps = [...this.dedos.values()];
    this.modo = modo;
    this.t0 = performance.now();
    this.p0 = ps[0] ?? { x: 0, y: 0 };
    this.s0 = this.s;
    this.tx0 = this.tx;
    this.ty0 = this.ty;
    this.horizontal = false;
    this.movido = false;
    const [a, b] = ps;
    if (modo === 'pellizco' && a && b) {
      const { c } = this.medidas(hueco);
      this.d0 = Math.max(Math.hypot(a.x - b.x, a.y - b.y), 1);
      this.m0 = { x: (a.x + b.x) / 2 - c.x, y: (a.y + b.y) / 2 - c.y };
    }
  }

  /** Doble toque: ×2,5 donde se toca, o encajada si ya estaba ampliada. */
  private doble(p: Punto, hueco: HTMLElement): void {
    if (this.s > 1) {
      this.encajar(true);
      return;
    }
    const { c } = this.medidas(hueco);
    this.s = ESCALA_DOBLE;
    this.tx = anclar(p.x - c.x, 1, 0, ESCALA_DOBLE);
    this.ty = anclar(p.y - c.y, 1, 0, ESCALA_DOBLE);
    this.limitarTodo(hueco);
    this.pintar(true);
  }

  private soltar(e: PointerEvent, cancelado: boolean): void {
    if (!this.dedos.has(e.pointerId)) return;
    const hueco = e.currentTarget as HTMLElement;
    this.dedos.delete(e.pointerId);
    this.ultimoGesto = performance.now();
    if (this.modo === 'pellizco') {
      // Casi sin ampliar, encajada; con un dedo aún puesto, se sigue moviendo lo ampliado
      // desde donde quedó.
      if (this.s < 1.02) this.encajar(true);
      if (this.dedos.size === 1) this.empezar(this.s > 1 ? 'mover' : 'nada', hueco);
      else this.modo = 'nada';
      return;
    }
    if (this.dedos.size > 0) return;
    const p = { x: e.clientX, y: e.clientY };
    if (!cancelado && !this.movido && performance.now() - this.t0 < 250) {
      const u = this.toque;
      if (performance.now() - u.t < DOBLE_MS && Math.hypot(p.x - u.x, p.y - u.y) < DOBLE_PX) {
        this.toque = { t: 0, x: 0, y: 0 };
        this.doble(p, hueco);
      } else {
        this.toque = { t: performance.now(), x: p.x, y: p.y };
      }
    }
    if (this.modo === 'deslizar' && this.horizontal) {
      const paso = cancelado
        ? 0
        : decidir(p.x - this.p0.x, p.y - this.p0.y, hueco.clientWidth, performance.now() - this.t0);
      const ir = paso === 1 ? this.op.onNext : paso === -1 ? this.op.onPrev : undefined;
      // La foto nueva llega en su sitio: la vieja no tiene que volver de ningún lado.
      this.pintar(!ir);
      ir?.();
    }
    this.modo = 'nada';
  }

  readonly manejadores = {
    onPointerDown: (e: PointerEvent): void => {
      if (!this.op.activo || e.pointerType === 'mouse' || delMando(e)) return;
      const hueco = e.currentTarget as HTMLElement;
      this.dedos.set(e.pointerId, { x: e.clientX, y: e.clientY });
      try {
        hueco.setPointerCapture(e.pointerId);
      } catch {
        // Un puntero que ya no existe: no hay nada que capturar.
      }
      if (this.dedos.size === 2) {
        this.pintar(false); // un deslizamiento a medias vuelve a su sitio
        this.empezar('pellizco', hueco);
      } else if (this.dedos.size === 1) {
        this.empezar(this.s > 1 ? 'mover' : 'deslizar', hueco);
      }
    },
    onPointerMove: (e: PointerEvent): void => {
      if (!this.dedos.has(e.pointerId)) return;
      const hueco = e.currentTarget as HTMLElement;
      this.dedos.set(e.pointerId, { x: e.clientX, y: e.clientY });
      const dx = e.clientX - this.p0.x;
      const dy = e.clientY - this.p0.y;
      if (Math.hypot(dx, dy) > TOQUE_PX) this.movido = true;
      if (this.modo === 'pellizco') {
        const [a, b] = [...this.dedos.values()];
        if (!a || !b) return;
        const { c } = this.medidas(hueco);
        const s = Math.min(
          ESCALA_MAX,
          Math.max(1, (this.s0 * Math.hypot(a.x - b.x, a.y - b.y)) / this.d0),
        );
        const m = { x: (a.x + b.x) / 2 - c.x, y: (a.y + b.y) / 2 - c.y };
        this.s = s;
        this.tx = anclar(this.m0.x, this.s0, this.tx0, s) + (m.x - this.m0.x);
        this.ty = anclar(this.m0.y, this.s0, this.ty0, s) + (m.y - this.m0.y);
        this.limitarTodo(hueco);
        this.pintar(false);
      } else if (this.modo === 'mover') {
        this.tx = this.tx0 + dx;
        this.ty = this.ty0 + dy;
        this.limitarTodo(hueco);
        this.pintar(false);
      } else if (this.modo === 'deslizar') {
        if (
          !this.horizontal &&
          Math.abs(dx) > 8 &&
          Math.abs(dx) >= DOMINIO_HORIZONTAL * Math.abs(dy)
        ) {
          this.horizontal = true;
        }
        if (!this.horizontal) return;
        const sinFoto = (dx > 0 && !this.op.onPrev) || (dx < 0 && !this.op.onNext);
        this.pintar(false, sinFoto ? dx * RESISTENCIA : dx);
      }
    },
    onPointerUp: (e: PointerEvent): void => this.soltar(e, false),
    onPointerCancel: (e: PointerEvent): void => this.soltar(e, true),
    /**
     * Con el dedo, el `click` que llega a la foto no la pone a tamaño real: para eso está
     * el doble toque, y un toque suelto no hace nada.
     */
    onClickCapture: (e: MouseEvent): void => {
      if (delMando(e)) return;
      const tipo = (e as PointerEvent).pointerType;
      if (tipo === 'touch' || tipo === 'pen' || performance.now() - this.ultimoGesto < 350) {
        e.stopPropagation();
        e.preventDefault();
      }
    },
  };
}

/**
 * Los manejadores del escenario. `objetivo` es la foto; `clave`, la foto que se enseña.
 * `activo` en falso —el tamaño real— deja el dedo al desplazamiento del navegador.
 */
export function useGestosVisor(opciones: Opciones) {
  const [g] = useState(() => new Gestos());
  useEffect(() => g.poner(opciones));
  useEffect(() => g.olvidar(), [g, opciones.clave]);
  return g.manejadores;
}
