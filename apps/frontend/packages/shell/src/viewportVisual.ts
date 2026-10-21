/**
 * Viewport visual → `--sytue-vvh`/`--sytue-vvy` + vuelta al origen cuando se
 * retira el teclado.
 *
 * En iOS el teclado virtual NO encoge el viewport de maquetación (`100dvh`
 * incluido): Safari desplaza el viewport VISUAL para enseñar el campo con foco y,
 * al cerrar el teclado, a veces lo deja donde estaba. La app se ve recortada o
 * «con zoom» hasta que la persona toca algo (informado en Mail el 2026-09-09
 * al redactar y ocultar el teclado). `interactive-widget=resizes-content` lo
 * arregla en Chrome de Android y Safari lo ignora.
 *
 * Dos cosas, las dos desde la shell para que valgan en toda la suite:
 * - `--sytue-vvh`: la altura visual en px. El Modal la usa como alto de su
 *   contenedor, así el diálogo se queda POR ENCIMA del teclado con su pie a la
 *   vista, en vez de centrarse en una pantalla que ya no se ve entera.
 * - `--sytue-vvy`: el desplazamiento vertical de ese viewport. Safari puede
 *   desplazarlo al enfocar un campo aunque no cambie su altura; un diálogo
 *   fijado en el viewport de maquetación quedaría entonces separado del teclado.
 * - Cuando se ha visto un teclado y vuelve la altura, `scrollTo(0, 0)` si el
 *   documento quedó desplazado. Un giro también aumenta la altura: no es un
 *   cierre de teclado y debe conservar el sitio de lectura.
 */
import { enCampoDeTexto } from '@sytue/ui';

export function publicarViewportVisual(): () => void {
  const vv = typeof window === 'undefined' ? null : window.visualViewport;
  if (!vv) return () => {};
  const raiz = document.documentElement;
  let ancho = vv.width;
  let alturaConTeclado: number | null = null;
  const medir = () => {
    raiz.style.setProperty('--sytue-vvh', `${Math.round(vv.height)}px`);
    raiz.style.setProperty('--sytue-vvy', `${Math.round(vv.offsetTop)}px`);
    const giroOZoom = Math.abs(vv.width - ancho) > 1 || vv.scale > 1.05;
    const recortado = window.innerHeight - vv.height > 80;
    if (giroOZoom) alturaConTeclado = null;
    else if (recortado && (alturaConTeclado !== null || enCampoDeTexto(document.activeElement)))
      alturaConTeclado = Math.min(alturaConTeclado ?? vv.height, vv.height);
    else if (!recortado) {
      if (
        alturaConTeclado !== null &&
        vv.height - alturaConTeclado > 80 &&
        (window.scrollY > 0 || vv.offsetTop > 0)
      )
        window.scrollTo(0, 0);
      alturaConTeclado = null;
    }
    ancho = vv.width;
  };
  medir();
  vv.addEventListener('resize', medir);
  vv.addEventListener('scroll', medir);
  return () => {
    vv.removeEventListener('resize', medir);
    vv.removeEventListener('scroll', medir);
    raiz.style.removeProperty('--sytue-vvh');
    raiz.style.removeProperty('--sytue-vvy');
  };
}
