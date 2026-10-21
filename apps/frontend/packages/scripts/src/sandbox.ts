/*
 * SPDX-License-Identifier: Apache-2.0
 * SPDX-FileCopyrightText: (c) 2026 Daniel Delgado Ramos
 *
 * El sandbox real de los scripts: un iframe `sandbox="allow-scripts"` SIN
 * `allow-same-origin` (origen opaco: sin cookies, sin storage, sin acceso al DOM del
 * anfitrión) con una CSP `default-src 'none'` propia, y DENTRO un Worker que corre el
 * código del usuario. Cuanto sale de ahí viaja por `postMessage` y el anfitrión lo
 * valida (protocolo.ts). Matar el iframe mata al Worker: es la terminación forzosa.
 *
 * Por qué iframe Y Worker. El Worker creado desde la página tendría NUESTRO origen
 * (cookies, `fetch` con sesión): el iframe de origen opaco es lo que quita la autoridad.
 * El Worker dentro del iframe es lo que permite que un bucle infinito no congele ni el
 * iframe ni la pestaña, y que se pueda parar sin cooperación del código.
 *
 * Por qué `srcdoc`. El documento se compone aquí, sin pedirlo al servidor: no hay una
 * ruta nueva que proteger, y las cabeceras de la suite (`X-Frame-Options: DENY`,
 * `frame-ancestors 'none'`) no se aplican a un `srcdoc`. Hereda la CSP de la página
 * (objeto, base, formularios) y la suya la AÑADE, no la sustituye.
 *
 * CSP del iframe: `default-src 'none'` apaga fetch, XHR, WebSocket, imágenes, estilos,
 * marcos y `importScripts` de fuera; `script-src 'unsafe-inline'` solo deja correr el
 * arranque en línea (sin `unsafe-eval`: ni `eval` ni `new Function`, también en el
 * Worker, que hereda la política); `worker-src blob:` permite el Worker. Además el
 * preludio del Worker deja sin definir las APIs de red, como segunda barrera.
 *
 * Es COMÚN a los productos: el iframe, la CSP, el Worker y el reenvío son los mismos para
 * Docs y para Bases. Lo único que aporta cada uno es su superficie `sytue.v1` (qué
 * operaciones existen y cuántos argumentos llevan), declarada como DATOS: el producto no
 * escribe código que corra dentro del sandbox.
 */
import { LIMITES } from './protocolo';
import type { Sandbox } from './ejecutor';

export const CSP_SANDBOX = "default-src 'none'; script-src 'unsafe-inline'; worker-src blob:";

/**
 * Una operación de la superficie `sytue.v1` de un producto: `op` es el nombre que viaja
 * en el mensaje y también dónde cuelga en la API (`run` → `sytue.v1.run`, `read.cell` →
 * `sytue.v1.read.cell`). Admite entre `min` y `max` argumentos; los que pasan de `max` se
 * ignoran y uno opcional sin definir corta la lista. Todo argumento viaja como cadena;
 * con `json`, un objeto se serializa con `JSON.stringify` (así cruza un registro entero
 * sin ensanchar el protocolo: sigue siendo una cadena de tamaño acotado que el anfitrión
 * analiza y valida).
 */
export interface OperacionDeSuperficie {
  op: string;
  min: number;
  max: number;
  json?: boolean;
}

/** Un nombre de operación: uno o dos segmentos de letras. Va DENTRO del código generado. */
const RE_OP = /^[a-z][a-zA-Z]{0,23}(?:\.[a-z][a-zA-Z]{0,23})?$/;
/** Lo que el preludio pone siempre en `sytue.v1`: ninguna operación puede llamarse así. */
const RESERVADAS = new Set(['version', 'log']);

/**
 * El cuerpo de `sytue.v1` para una superficie, como líneas de ES5. Lanza si la
 * superficie no es válida: los nombres se interpolan en código que corre junto al del
 * usuario, así que solo valen letras, y eso se comprueba aquí y no se confía a quien llama.
 */
function lineasDeSuperficie(superficie: readonly OperacionDeSuperficie[]): string[] {
  const raiz: string[] = [];
  const grupos = new Map<string, string[]>();
  const vistos = new Set<string>();
  for (const s of superficie) {
    const partes = s.op.split('.');
    if (
      !RE_OP.test(s.op) ||
      vistos.has(s.op) ||
      RESERVADAS.has(partes[0]!) ||
      !Number.isInteger(s.min) ||
      !Number.isInteger(s.max) ||
      s.min < 0 ||
      s.max < s.min ||
      s.max > LIMITES.maxArgs
    )
      throw new Error(`Operación de superficie no válida: ${s.op}`);
    vistos.add(s.op);
    const fn = `op(${JSON.stringify(s.op)},${s.min},${s.max},${s.json === true ? 'true' : 'false'})`;
    if (partes.length === 1) raiz.push(` ${partes[0]}:${fn},`);
    else grupos.set(partes[0]!, [...(grupos.get(partes[0]!) ?? []), `  ${partes[1]}:${fn}`]);
  }
  for (const g of grupos.keys())
    if (vistos.has(g)) throw new Error(`Operación de superficie no válida: ${g}`);
  const anidados = [...grupos].flatMap(([g, fns]) => [
    ` ${g}:Object.freeze({`,
    fns.join(',\n'),
    ' }),',
  ]);
  return [...raiz, ...anidados];
}

/**
 * Preludio del Worker: define `sytue.v1` con la superficie del producto y corta la
 * autoridad ambiental. Se ejecuta ANTES del código del usuario. ES5 deliberado y sin
 * backticks ni «</»: viaja dentro de una cadena y de un `<script>`.
 */
export function preludioWorker(superficie: readonly OperacionDeSuperficie[]): string {
  return [
    '(function(){',
    '"use strict";',
    'var post=self.postMessage.bind(self);',
    'var pend=new Map();var seq=0;',
    'var cierra=["fetch","XMLHttpRequest","WebSocket","EventSource","importScripts","indexedDB","caches","BroadcastChannel","Worker","SharedWorker","WebTransport","RTCPeerConnection","FileReaderSync","Notification"];',
    'cierra.forEach(function(n){try{Object.defineProperty(self,n,{value:undefined,writable:false,configurable:false});}catch(e){}});',
    'self.addEventListener("message",function(e){var m=e.data;',
    ' if(!m||m.t!=="ret"||typeof m.seq!=="number")return;',
    ' var p=pend.get(m.seq);if(!p)return;pend.delete(m.seq);',
    ' if(m.ok)p.res(m.value);else{var er=new Error(String(m.message));er.name="SytueError";er.code=String(m.code);p.rej(er);}',
    '});',
    'function llamar(op,args){return new Promise(function(res,rej){var s=++seq;pend.set(s,{res:res,rej:rej});post({t:"call",seq:s,op:op,args:args});});}',
    'function txt(x){if(typeof x==="string")return x;try{var j=JSON.stringify(x);return j===undefined?String(x):j;}catch(e){return String(x);}}',
    'function arg(x,json){return json&&typeof x==="object"&&x!==null?JSON.stringify(x):String(x);}',
    'function op(n,min,max,json){return function(){var a=[];for(var i=0;i<max;i++){if(i>=min&&arguments[i]===undefined)break;a.push(arg(arguments[i],json));}return llamar(n,a);};}',
    'var v1=Object.freeze({',
    ' version:1,',
    ...lineasDeSuperficie(superficie),
    ` log:function(){post({t:"log",text:Array.prototype.map.call(arguments,txt).join(" ").slice(0,${LIMITES.maxLinea})});}`,
    '});',
    'Object.defineProperty(self,"sytue",{value:Object.freeze({v1:v1}),writable:false,configurable:false});',
    '})();',
  ].join('\n');
}

/** Cierre del Worker: el código del usuario va envuelto en una función asíncrona. */
const ENVOLTORIO_ANTES = '\n;(async function(){\n"use strict";\n';
const ENVOLTORIO_DESPUES =
  '\n})().then(function(){self.postMessage({t:"done"});},function(e){self.postMessage({t:"fail",name:String(e&&e.name||"Error").slice(0,64),message:String(e&&e.message||e).slice(0,500)});});';

/** Arranque del iframe: reenvía entre el anfitrión y el Worker con tope de ritmo y tamaño. */
export function arranqueIframe(preludio: string): string {
  return [
    '(function(){',
    '"use strict";',
    'var host=window.parent;var worker=null;var iniciado=false;var marcas=[];',
    'var PRE=' + JSON.stringify(preludio) + ';',
    'var ANTES=' + JSON.stringify(ENVOLTORIO_ANTES) + ';',
    'var DESPUES=' + JSON.stringify(ENVOLTORIO_DESPUES) + ';',
    'function aHost(m){host.postMessage(m,"*");}',
    'function parar(){if(worker){worker.terminate();worker=null;}}',
    'function desbordado(m){',
    ' var t=Date.now();marcas.push(t);while(marcas.length&&t-marcas[0]>1000)marcas.shift();',
    ` if(marcas.length>${LIMITES.maxPorSegundo})return true;`,
    ` try{return JSON.stringify(m).length>${LIMITES.maxMensaje};}catch(e){return true;}`,
    '}',
    'function arrancar(codigo){',
    ' var src=PRE+ANTES+codigo+DESPUES;',
    ' var url=URL.createObjectURL(new Blob([src],{type:"text/javascript"}));',
    ' worker=new Worker(url);',
    ' worker.onmessage=function(e){',
    '  URL.revokeObjectURL(url);var m=e.data;',
    '  if(desbordado(m)){parar();aHost({t:"fail",name:"LimitError",message:"mensajes"});return;}',
    '  aHost(m);',
    ' };',
    ' worker.onerror=function(e){URL.revokeObjectURL(url);e.preventDefault();parar();aHost({t:"fail",name:"SyntaxError",message:String(e.message||"error").slice(0,500)});};',
    '}',
    'window.addEventListener("message",function(e){',
    ' if(e.source!==host)return;',
    ' var m=e.data;if(!m||typeof m!=="object")return;',
    ' if(m.t==="init"&&!iniciado&&typeof m.code==="string"){iniciado=true;arrancar(m.code);}',
    ' else if(m.t==="ret"&&worker&&typeof m.seq==="number"){',
    '  worker.postMessage(m.ok?{t:"ret",seq:m.seq,ok:true,value:m.value}:{t:"ret",seq:m.seq,ok:false,code:String(m.code),message:String(m.message)});',
    ' }',
    '});',
    'aHost({t:"ready"});',
    '})();',
  ].join('\n');
}

/** El documento del iframe, completo, para la superficie de un producto. */
export function sandboxHtml(superficie: readonly OperacionDeSuperficie[]): string {
  return (
    '<!doctype html><html><head><meta charset="utf-8">' +
    `<meta http-equiv="Content-Security-Policy" content="${CSP_SANDBOX}">` +
    '</head><body><script>' +
    arranqueIframe(preludioWorker(superficie)) +
    '</' +
    'script></body></html>'
  );
}

/**
 * Crea el sandbox en el documento dado, con el documento que da `sandboxHtml` para la
 * superficie del producto. El `sandbox` del iframe es EXACTAMENTE
 * `allow-scripts`: añadir `allow-same-origin` devolvería al código del usuario nuestro
 * origen, y hay un test que vigila que no se haga.
 */
export function crearSandboxIframe(html: string, doc: Document = document): Sandbox {
  const iframe = doc.createElement('iframe');
  iframe.setAttribute('sandbox', 'allow-scripts');
  iframe.setAttribute('referrerpolicy', 'no-referrer');
  iframe.setAttribute('allow', '');
  iframe.setAttribute('aria-hidden', 'true');
  iframe.tabIndex = -1;
  iframe.style.cssText = 'position:absolute;width:0;height:0;border:0;visibility:hidden';
  iframe.srcdoc = html;
  let receptor: ((datos: unknown) => void) | undefined;
  const alMensaje = (ev: MessageEvent): void => {
    // Solo lo que sale de ESTE iframe, y su origen es opaco («null»).
    if (ev.source !== iframe.contentWindow || ev.origin !== 'null') return;
    receptor?.(ev.data);
  };
  const ventana = doc.defaultView ?? window;
  ventana.addEventListener('message', alMensaje);
  doc.body.appendChild(iframe);
  return {
    recibir: (fn) => {
      receptor = fn;
    },
    enviar: (mensaje) => {
      iframe.contentWindow?.postMessage(mensaje, '*');
    },
    destruir: () => {
      receptor = undefined;
      ventana.removeEventListener('message', alMensaje);
      iframe.remove();
    },
  };
}
