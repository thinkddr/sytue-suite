/**
 * El MOTOR DE PÍXELES de Sytue Photos, en el navegador.
 *
 * Aplica una receta a una imagen: la geometría (recorte, giro, espejo) con
 * Canvas 2D y el color (exposición, contraste, saturación, temperatura) con un
 * shader WebGL2 en UNA pasada — y el mismo cálculo en CPU sobre `ImageData`
 * cuando WebGL no está, que es el respaldo y también lo que ejecutan los tests.
 *
 * LAS FÓRMULAS SON EL CONTRATO, no el shader. Están escritas una vez en
 * `ajustarColor` (CPU) y transcritas al GLSL literalmente; si la exportación
 * del servidor las implementa un día en Go, es de aquí de donde se copian. Se
 * eligieron por ser las que se explican en una línea y dan resultados
 * previsibles, no las «más bonitas»:
 *
 *   exposición   ×2^valor          (un paso de diafragma dobla la luz)
 *   contraste    (c−0,5)·(1+v)+0,5 (gira alrededor del gris medio)
 *   saturación   mezcla con la luminancia Rec.709
 *   temperatura  R·(1+0,15v), B·(1−0,15v)  (frío→cálido, suave)
 *
 * Se aplican EN EL ORDEN DE LA RECETA, que es el que decide el resultado.
 * Todo en 0..1 y se recorta al final, no entre pasos: recortar entre pasos
 * destruye información que el paso siguiente podía haber recuperado.
 */
import type { Paso, Receta } from './receta.js';

export type RGB = [number, number, number];

/** Luminancia Rec.709: lo que la vista percibe como «cuánta luz hay». */
export function luminancia([r, g, b]: RGB): number {
  return 0.2126 * r + 0.7152 * g + 0.0722 * b;
}

const clamp01 = (x: number): number => (x < 0 ? 0 : x > 1 ? 1 : x);

/** Aplica un paso de COLOR a un píxel en 0..1. Los de geometría se ignoran. */
export function ajustarColor(rgb: RGB, p: Paso): RGB {
  const v = p.params?.valor ?? 0;
  switch (p.tipo) {
    case 'exposicion': {
      const k = Math.pow(2, v);
      return [rgb[0] * k, rgb[1] * k, rgb[2] * k];
    }
    case 'contraste': {
      const k = 1 + v;
      return rgb.map((c) => (c - 0.5) * k + 0.5) as RGB;
    }
    case 'saturacion': {
      const l = luminancia(rgb);
      const k = 1 + v;
      return rgb.map((c) => l + (c - l) * k) as RGB;
    }
    case 'temperatura':
      return [rgb[0] * (1 + 0.15 * v), rgb[1], rgb[2] * (1 - 0.15 * v)];
    default:
      return rgb;
  }
}

/** Todos los pasos de color de la receta sobre un píxel, y el recorte final. */
export function aplicarColor(rgb: RGB, r: Receta): RGB {
  let c = rgb;
  for (const p of r.pasos) c = ajustarColor(c, p);
  return [clamp01(c[0]), clamp01(c[1]), clamp01(c[2])];
}

/** Respaldo en CPU: el mismo cálculo sobre un `ImageData`, en su sitio. */
export function aplicarColorCPU(img: ImageData, r: Receta): void {
  const d = img.data;
  const pasos = r.pasos.filter((p) => p.params?.valor !== undefined || p.tipo === 'saturacion');
  if (!pasos.some(esDeColor)) return;
  for (let i = 0; i < d.length; i += 4) {
    const [R, G, B] = aplicarColor([d[i]! / 255, d[i + 1]! / 255, d[i + 2]! / 255], r);
    d[i] = Math.round(R * 255);
    d[i + 1] = Math.round(G * 255);
    d[i + 2] = Math.round(B * 255);
  }
}

export const esDeColor = (p: Paso): boolean =>
  p.tipo === 'exposicion' ||
  p.tipo === 'contraste' ||
  p.tipo === 'saturacion' ||
  p.tipo === 'temperatura';

export const esDeGeometria = (p: Paso): boolean =>
  p.tipo === 'recorte' || p.tipo === 'giro' || p.tipo === 'espejo';

/**
 * La GEOMETRÍA se resuelve en un solo Canvas 2D, paso a paso y en orden:
 * recortar y luego girar no da lo mismo que girar y luego recortar. Cada paso
 * pinta el canvas anterior en uno nuevo con la transformación aplicada; son a
 * lo sumo tres copias de la previa (2048 px), que es barato.
 */
export function aplicarGeometria(
  origen: CanvasImageSource,
  ancho: number,
  alto: number,
  r: Receta,
): HTMLCanvasElement {
  let actual = lienzo(ancho, alto);
  actual.getContext('2d')!.drawImage(origen, 0, 0, ancho, alto);
  for (const p of r.pasos) {
    if (!esDeGeometria(p)) continue;
    const w = actual.width;
    const h = actual.height;
    if (p.tipo === 'recorte') {
      const { x = 0, y = 0, ancho: fw = 1, alto: fh = 1 } = p.params ?? {};
      const sx = Math.round(x * w);
      const sy = Math.round(y * h);
      const sw = Math.max(1, Math.round(fw * w));
      const sh = Math.max(1, Math.round(fh * h));
      const c = lienzo(sw, sh);
      c.getContext('2d')!.drawImage(actual, sx, sy, sw, sh, 0, 0, sw, sh);
      actual = c;
    } else if (p.tipo === 'giro') {
      const g = (((p.params?.grados ?? 0) % 360) + 360) % 360;
      const lados = g === 90 || g === 270;
      const c = lienzo(lados ? h : w, lados ? w : h);
      const ctx = c.getContext('2d')!;
      ctx.translate(c.width / 2, c.height / 2);
      ctx.rotate((g * Math.PI) / 180);
      ctx.drawImage(actual, -w / 2, -h / 2);
      actual = c;
    } else if (p.tipo === 'espejo') {
      const c = lienzo(w, h);
      const ctx = c.getContext('2d')!;
      if (p.eje === 'h') {
        ctx.translate(w, 0);
        ctx.scale(-1, 1);
      } else {
        ctx.translate(0, h);
        ctx.scale(1, -1);
      }
      ctx.drawImage(actual, 0, 0);
      actual = c;
    }
  }
  return actual;
}

function lienzo(w: number, h: number): HTMLCanvasElement {
  const c = document.createElement('canvas');
  c.width = Math.max(1, w);
  c.height = Math.max(1, h);
  return c;
}

// ── WebGL2: el mismo cálculo, en la GPU ────────────────────────────────────
//
// Una sola pasada con TODOS los pasos de color desplegados como uniformes. La
// receta admite hasta 256 pasos pero los de color son cuatro tipos; se pasan
// como una lista de (tipo, valor) y el shader los recorre en orden. Así un
// arrastre de un deslizador es un `uniform` y un `draw`, no una recompilación.

const VS = `#version 300 es
in vec2 a;out vec2 v;void main(){v=vec2(a.x,1.0-a.y);gl_Position=vec4(a*2.0-1.0,0.0,1.0);}`;

const FS = `#version 300 es
precision highp float;
in vec2 v;out vec4 o;uniform sampler2D img;
uniform int n;uniform int tipo[32];uniform float valor[32];
float lum(vec3 c){return dot(c,vec3(0.2126,0.7152,0.0722));}
void main(){
  vec4 tex=texture(img,v);
  vec3 c=tex.rgb;
  for(int i=0;i<32;i++){ if(i>=n) break; float x=valor[i];
    if(tipo[i]==0) c*=pow(2.0,x);
    else if(tipo[i]==1) c=(c-0.5)*(1.0+x)+0.5;
    else if(tipo[i]==2){ float l=lum(c); c=l+(c-l)*(1.0+x); }
    else if(tipo[i]==3){ c.r*=1.0+0.15*x; c.b*=1.0-0.15*x; }
  }
  o=vec4(clamp(c,0.0,1.0),tex.a);
}`;

const CODIGO: Record<string, number> = {
  exposicion: 0,
  contraste: 1,
  saturacion: 2,
  temperatura: 3,
};

/** Motor de color en GPU. `null` si WebGL2 no está: entonces se usa la CPU. */
export class MotorGL {
  private gl: WebGL2RenderingContext;
  private prog: WebGLProgram;
  private tex: WebGLTexture;
  private uN: WebGLUniformLocation;
  private uTipo: WebGLUniformLocation;
  private uValor: WebGLUniformLocation;

  static crear(canvas: HTMLCanvasElement): MotorGL | null {
    const gl = canvas.getContext('webgl2', {
      premultipliedAlpha: false,
      preserveDrawingBuffer: true,
      alpha: true,
    });
    if (!gl) return null;
    try {
      return new MotorGL(gl);
    } catch {
      return null;
    }
  }

  private constructor(gl: WebGL2RenderingContext) {
    this.gl = gl;
    const sh = (t: number, src: string): WebGLShader => {
      const s = gl.createShader(t)!;
      gl.shaderSource(s, src);
      gl.compileShader(s);
      if (!gl.getShaderParameter(s, gl.COMPILE_STATUS))
        throw new Error(gl.getShaderInfoLog(s) ?? 'shader');
      return s;
    };
    const prog = gl.createProgram();
    gl.attachShader(prog, sh(gl.VERTEX_SHADER, VS));
    gl.attachShader(prog, sh(gl.FRAGMENT_SHADER, FS));
    gl.linkProgram(prog);
    if (!gl.getProgramParameter(prog, gl.LINK_STATUS))
      throw new Error(gl.getProgramInfoLog(prog) ?? 'link');
    this.prog = prog;
    gl.useProgram(prog);
    const buf = gl.createBuffer();
    gl.bindBuffer(gl.ARRAY_BUFFER, buf);
    gl.bufferData(gl.ARRAY_BUFFER, new Float32Array([0, 0, 1, 0, 0, 1, 1, 1]), gl.STATIC_DRAW);
    const a = gl.getAttribLocation(prog, 'a');
    gl.enableVertexAttribArray(a);
    gl.vertexAttribPointer(a, 2, gl.FLOAT, false, 0, 0);
    this.tex = gl.createTexture()!;
    gl.bindTexture(gl.TEXTURE_2D, this.tex);
    gl.texParameteri(gl.TEXTURE_2D, gl.TEXTURE_MIN_FILTER, gl.LINEAR);
    gl.texParameteri(gl.TEXTURE_2D, gl.TEXTURE_MAG_FILTER, gl.LINEAR);
    gl.texParameteri(gl.TEXTURE_2D, gl.TEXTURE_WRAP_S, gl.CLAMP_TO_EDGE);
    gl.texParameteri(gl.TEXTURE_2D, gl.TEXTURE_WRAP_T, gl.CLAMP_TO_EDGE);
    this.uN = gl.getUniformLocation(prog, 'n')!;
    this.uTipo = gl.getUniformLocation(prog, 'tipo')!;
    this.uValor = gl.getUniformLocation(prog, 'valor')!;
  }

  /** Sube la imagen ya con la geometría aplicada. Se hace una vez por fuente. */
  cargar(fuente: TexImageSource): void {
    const gl = this.gl;
    gl.bindTexture(gl.TEXTURE_2D, this.tex);
    gl.pixelStorei(gl.UNPACK_FLIP_Y_WEBGL, false);
    gl.texImage2D(gl.TEXTURE_2D, 0, gl.RGBA, gl.RGBA, gl.UNSIGNED_BYTE, fuente);
  }

  /** Pinta la receta de color sobre la fuente cargada. Es lo que se llama por arrastre. */
  pintar(r: Receta, ancho: number, alto: number): void {
    const gl = this.gl;
    const pasos = r.pasos.filter(esDeColor).slice(0, 32);
    gl.canvas.width = ancho;
    gl.canvas.height = alto;
    gl.viewport(0, 0, ancho, alto);
    gl.useProgram(this.prog);
    gl.uniform1i(this.uN, pasos.length);
    gl.uniform1iv(
      this.uTipo,
      new Int32Array(32).map((_, i) => CODIGO[pasos[i]?.tipo ?? ''] ?? 0),
    );
    gl.uniform1fv(
      this.uValor,
      new Float32Array(32).map((_, i) => pasos[i]?.params?.valor ?? 0),
    );
    gl.drawArrays(gl.TRIANGLE_STRIP, 0, 4);
  }
}
