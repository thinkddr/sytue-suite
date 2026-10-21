/** Rectángulo alcanzable por un panel: viewport visual menos muesca y gestos. */
export function limitesViewport(margen = 0): {
  left: number;
  top: number;
  right: number;
  bottom: number;
  width: number;
  height: number;
} {
  const raiz = document.documentElement;
  const vv = window.visualViewport;
  const estilo = getComputedStyle(raiz);
  const seguro = (lado: string): number =>
    Math.max(0, Number.parseFloat(estilo.getPropertyValue(`--sytue-safe-${lado}`)) || 0);
  const x = vv?.offsetLeft ?? 0;
  const y = vv?.offsetTop ?? 0;
  const left = x + seguro('left') + margen;
  const top = y + seguro('top') + margen;
  const right = Math.max(
    left,
    x + (vv?.width ?? (raiz.clientWidth || window.innerWidth)) - seguro('right') - margen,
  );
  const bottom = Math.max(top, y + (vv?.height ?? window.innerHeight) - seguro('bottom') - margen);
  return { left, top, right, bottom, width: right - left, height: bottom - top };
}

/** También escucha desplazamientos de paneles internos, que no burbujean. */
export function observarViewport(medir: () => void): () => void {
  const vv = window.visualViewport;
  window.addEventListener('resize', medir);
  window.addEventListener('scroll', medir, true);
  vv?.addEventListener('resize', medir);
  vv?.addEventListener('scroll', medir);
  return () => {
    window.removeEventListener('resize', medir);
    window.removeEventListener('scroll', medir, true);
    vv?.removeEventListener('resize', medir);
    vv?.removeEventListener('scroll', medir);
  };
}
