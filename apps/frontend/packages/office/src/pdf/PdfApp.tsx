/*
 * SPDX-License-Identifier: Apache-2.0
 * SPDX-FileCopyrightText: (c) 2026 Daniel Delgado Ramos
 *
 * Sytue PDF — visor y editor. Cuarto tipo de Sytue Docs.
 *
 * EL LIENZO ES SVG, NO UNA IMAGEN, y de ahí salen tres cosas que un visor de
 * mapa de bits no puede dar:
 *
 *   - el texto del documento es TEXTO del DOM: se selecciona con el ratón, se
 *     copia, lo lee un lector de pantalla y se puede buscar. Resaltar es tomar
 *     los rectángulos de la selección del navegador, que es exactamente lo que
 *     hace un visor de escritorio;
 *   - el zoom no pierde calidad ni obliga a volver a pedir la página;
 *   - la capa de anotaciones comparte sistema de coordenadas con el documento,
 *     así que una marca cae donde el usuario la puso a cualquier aumento.
 *
 * Las páginas se piden por LOTES según se va llegando a ellas: un expediente de
 * trescientas páginas no cabe en una respuesta, y pedir de una en una vuelve a
 * subir el fichero por cada una.
 */
import { useCallback, useEffect, useMemo, useRef, useState } from 'preact/hooks';
import { appUrl, driveFolderUrl } from '@sytue/config';
import { docsT, type DocsKey } from '../i18n';
// Alias: en este fichero `r` ya nombra la respuesta de varias llamadas al API.
import { r as ruta } from '../lib/rutas';
import { descargarBlob, nombreDePagina, paginaComoImagen } from './imagenes';
import type { Locale } from '@sytue/i18n';
import {
  api,
  firmas,
  pendienteDe,
  quienFirma,
  aPDF,
  textoParaLeer,
  colorARGB,
  pideContrasena,
  invalidaFirmas,
  rectEnSVG,
  type AnotacionNueva,
  type Enlace,
  type Contenido,
  type Fragmento,
  type Pagina,
  type PeticionFirma,
} from './api';
import {
  DialogosDelEditor,
  Spinner,
  confirmarEnEditor,
  enCampoDeTexto,
  preguntarEnEditor,
} from '@sytue/ui';
import { mensajeDeError } from './errores';
import { BarraHerramientas, type Herramienta } from './Barra';
import { PanelLateral, Dialogos, type Dialogo, type PanelActivo } from './Paneles';

/** Cuántas páginas se dibujan de golpe. Alineado con el tope del servicio. */
const LOTE = 4;

/** Cuántas páginas por delante se piden antes de llegar a ellas. */
const ANTICIPO = 2;

export interface PdfAppProps {
  /** Ruta del fichero en Drive. */
  path: string;
  locale: Locale;
  /** Nombre de quien anota, para firmar los comentarios. */
  autor: string;
  /** Carpeta de Drive desde la que se abrió, si vino de allí. */
  volverA?: string | undefined;
}

export function PdfApp({ path, locale, autor, volverA }: PdfAppProps) {
  const t = useMemo(() => docsT(locale), [locale]);

  const [contenido, setContenido] = useState<Contenido | null>(null);
  const [svgs, setSvgs] = useState<Record<number, string>>({});
  const [textos, setTextos] = useState<(Fragmento[] | null)[] | null>(null);
  const [clave, setClave] = useState('');
  const [pideClave, setPideClave] = useState(false);
  const [error, setError] = useState('');
  const [aviso, setAviso] = useState('');
  const [cargando, setCargando] = useState(true);
  const [trabajando, setTrabajando] = useState(false);

  const [zoom, setZoom] = useState(1);
  // Dos hojas por fila. No se guarda en la URL: es una preferencia de lectura de
  // este rato, no del documento.
  const [dosPaginas, setDosPaginas] = useState(false);
  const [ajuste, setAjuste] = useState<'ancho' | 'pagina' | 'libre'>('ancho');
  const [paginaActual, setPaginaActual] = useState(1);
  const [herramienta, setHerramienta] = useState<Herramienta>('mano');
  const [color, setColor] = useState('#f9b923');
  const [grosor, setGrosor] = useState(2);
  const [dialogo, setDialogo] = useState<Dialogo>(null);
  const [panel, setPanel] = useState<PanelActivo>('miniaturas');
  // Debajo de `lg` el panel no cabe al lado del documento y se abre encima. Sin
  // esto, buscar, los comentarios y los campos del formulario no existían en un
  // móvil: la barra lateral se ocultaba y no había forma de sacarla.
  const [panelAbierto, setPanelAbierto] = useState(false);
  /** Zonas marcadas para censurar, aún sin aplicar. En coordenadas del PDF. */
  const [zonas, setZonas] = useState<ZonaCensura[]>([]);
  // Campos marcados y todavía sin crear. Como las zonas de censura, van en DOS
  // pasos: aquí se ven sobre el documento y solo al aplicar se escriben.
  const [camposNuevos, setCamposNuevos] = useState<CampoMarcado[]>([]);
  const [tipoCampo, setTipoCampo] = useState<TipoCampoNuevo>('texto');
  /** Ruta de la copia que dejó la última operación, para poder abrirla. */
  const [copia, setCopia] = useState('');
  /** La petición de firma pendiente que alguien me ha hecho sobre ESTE PDF. */
  const [peticion, setPeticion] = useState<PeticionFirma | undefined>(undefined);
  // «Bloquear» pide confirmación en la propia franja (ADR 0059).
  const [bloqueando, setBloqueando] = useState(false);
  /** El firmado que no se llegó a entregar: se ofrece reintentar. */
  const [sinEntregar, setSinEntregar] = useState('');

  const scrollRef = useRef<HTMLDivElement>(null);
  const anchoDisponible = useRef(0);
  const pidiendo = useRef(new Set<number>());

  /* --- Carga inicial ----------------------------------------------------- */

  const cargar = useCallback(
    async (conClave: string) => {
      setCargando(true);
      setError('');
      try {
        const c = await api.contenido(path, conClave || undefined);
        setContenido(c);
        setPideClave(false);
        setClave(conClave);
      } catch (e) {
        if (pideContrasena(e)) {
          setPideClave(true);
        } else {
          setError(mensajeDeError(e, t, 'pdf.error.open'));
        }
      } finally {
        setCargando(false);
      }
    },
    [path, t],
  );

  useEffect(() => {
    void cargar('');
  }, [cargar]);

  // Si alguien me pide firmar este PDF, la franja lo dice. Un fallo aquí no
  // estorba a abrir el documento: sin la lista, simplemente no hay franja.
  useEffect(() => {
    firmas
      .listar()
      .then((ps) => setPeticion(pendienteDe(ps, path)))
      .catch(() => setPeticion(undefined));
  }, [path]);

  /** Entrega el firmado a quien lo pidió. */
  const entregar = useCallback(
    async (p: PeticionFirma, firmado: string) => {
      const nombre = quienFirma(p, 'received');
      try {
        await firmas.entregar(p.id, firmado);
        setPeticion(undefined);
        setSinEntregar('');
        setAviso(t('pdf.signatureRequest.delivered', { name: nombre }));
      } catch (e) {
        setSinEntregar(firmado);
        setError(
          `${t('pdf.signatureRequest.notDelivered', { name: nombre })} ${mensajeDeError(e, t, 'pdf.error.apply')}`,
        );
      }
    },
    [t],
  );

  /* --- Dibujo por lotes -------------------------------------------------- */

  const pedirLote = useCallback(
    async (desde: number) => {
      if (!contenido) return;
      const hasta = Math.min(desde + LOTE - 1, contenido.total);
      const faltan: number[] = [];
      for (let n = desde; n <= hasta; n++) {
        if (svgs[n] === undefined && !pidiendo.current.has(n)) faltan.push(n);
      }
      if (faltan.length === 0) return;
      faltan.forEach((n) => pidiendo.current.add(n));
      try {
        const r = await api.render(path, `${faltan[0]}-${faltan[faltan.length - 1]}`, 1, clave);
        setSvgs((prev) => {
          const out = { ...prev };
          for (const [n, svg] of Object.entries(r.paginas)) out[Number(n)] = svg;
          // Una página que el motor no supo dibujar no se reintenta sin fin: se
          // marca con cadena vacía y la interfaz enseña el hueco.
          for (const n of faltan) if (out[n] === undefined) out[n] = '';
          return out;
        });
      } catch (e) {
        setError(mensajeDeError(e, t, 'pdf.error.render'));
      } finally {
        faltan.forEach((n) => pidiendo.current.delete(n));
      }
    },
    [contenido, path, clave, svgs, t],
  );

  useEffect(() => {
    if (contenido) void pedirLote(1);
  }, [contenido, pedirLote]);

  /** Al desplazarse: qué página se está mirando y qué hay que ir pidiendo. */
  const alDesplazar = useCallback(() => {
    const cont = scrollRef.current;
    if (!cont || !contenido) return;
    const centro = cont.scrollTop + cont.clientHeight / 2;
    const hojas = cont.querySelectorAll<HTMLElement>('[data-pagina]');
    for (const hoja of hojas) {
      if (hoja.offsetTop <= centro && hoja.offsetTop + hoja.offsetHeight >= centro) {
        const n = Number(hoja.dataset.pagina);
        setPaginaActual(n);
        void pedirLote(Math.max(1, n - 1));
        void pedirLote(Math.min(contenido.total, n + ANTICIPO));
        return;
      }
    }
  }, [contenido, pedirLote]);

  /* --- Zoom -------------------------------------------------------------- */

  useEffect(() => {
    const cont = scrollRef.current;
    if (!cont || !contenido || contenido.paginas.length === 0) return;
    const medir = () => {
      anchoDisponible.current = cont.clientWidth - 48;
      if (ajuste === 'libre') return;
      const p = contenido.paginas[0]!;
      // Con dos hojas por fila cada una dispone de la mitad menos el hueco: sin
      // esto, «ajustar al ancho» dejaría la segunda columna fuera de la pantalla.
      const util = dosPaginas ? (anchoDisponible.current - 24) / 2 : anchoDisponible.current;
      const porAncho = util / p.ancho;
      const porAlto = (cont.clientHeight - 48) / p.alto;
      setZoom(ajuste === 'ancho' ? porAncho : Math.min(porAncho, porAlto));
    };
    medir();
    const obs = new ResizeObserver(medir);
    obs.observe(cont);
    return () => obs.disconnect();
  }, [contenido, ajuste, dosPaginas]);

  const cambiarZoom = (delta: number) => {
    setAjuste('libre');
    setZoom((z) => Math.max(0.15, Math.min(8, z + delta)));
  };

  /**
   * Lleva a una página. ÚNICA puerta: la usan el número de la barra, los
   * botones de anterior/siguiente, las miniaturas, el índice y los enlaces
   * internos del documento, así que lo que se compruebe aquí vale para todos.
   *
   * Se acota AQUÍ y no en cada llamante porque el campo de la barra deja
   * escribir cualquier cosa —vaciarlo da `NaN`, y «999» de un documento de
   * doce— y entonces no había hoja que buscar: el visor no se movía y tampoco
   * decía nada, que se lee como que la navegación está rota.
   */
  const irAPagina = (n: number) => {
    const cont = scrollRef.current;
    if (!cont || !contenido) return;
    const destino = Math.min(Math.max(Math.trunc(n) || 1, 1), contenido.total);
    const hoja = cont.querySelector<HTMLElement>(`[data-pagina="${destino}"]`);
    // `offsetTop` es relativo al ancestro POSICIONADO, y el contenedor lo es
    // (`relative`, más abajo): sin eso medía desde el <body> y se colaban la
    // topbar y la barra de herramientas, así que ir a una página aterrizaba
    // unos 120 px más abajo y el número de página iba adelantado.
    // EL `smooth` SE CONSULTA, no se da por hecho. La regla global de
    // `prefers-reduced-motion` de theme.css pone `scroll-behavior: auto`, pero eso
    // NO alcanza a un `behavior` pedido explícitamente desde JavaScript: el valor
    // de la llamada gana al de la hoja. Así que quien tiene activado el ajuste
    // —por vértigo o migraña— recibía igual el desplazamiento animado cada vez que
    // salta de página, que en un documento largo es un barrido entero.
    const sinMovimiento = window.matchMedia('(prefers-reduced-motion: reduce)').matches;
    if (hoja) {
      cont.scrollTo({ top: hoja.offsetTop - 12, behavior: sinMovimiento ? 'auto' : 'smooth' });
    }
    setPaginaActual(destino);
    void pedirLote(destino);
  };

  /* --- Aplicar cambios --------------------------------------------------- */

  /** Devuelve si el cambio llegó a aplicarse: quien pierda algo al hacerlo
   *  —las marcas de censura— necesita saberlo para no tirarlo tras un fallo. */
  const aplicar = useCallback(
    async (cuerpoPedido: Record<string, unknown>, mensaje: string): Promise<boolean> => {
      let cuerpo = cuerpoPedido;
      for (;;) {
        setTrabajando(true);
        setError('');
        try {
          const r = await api.aplicar({ path, clave: clave || undefined, ...cuerpo });
          const deCopia = r.copia ? t('pdf.saved.copy', { name: nombreDe(r.path) }) : mensaje;
          // Reconocer cuenta lo que hizo, y hay que decirlo: de un expediente de
          // veinte páginas se pueden reconocer doce y saltar ocho por no tener
          // imagen, y sin este aviso las dos cosas se anuncian igual —«guardado»—
          // y nadie va a revisar las que faltan.
          const partes: string[] = [];
          if (typeof r.ocrPaginas === 'number') {
            partes.push(t('pdf.saved.ocr', { count: r.ocrPaginas }));
            if (r.ocrSinImagen) partes.push(t('pdf.saved.ocr.skipped', { count: r.ocrSinImagen }));
          }
          // Un PDF/A que deja de serlo, o que baja de nivel, se dice: quien lo
          // archiva cree que sigue a salvo.
          if (r.pdfa === 'perdido') partes.push(t('pdf.saved.pdfa.lost'));
          if (r.pdfa === 'rebajado') partes.push(t('pdf.saved.pdfa.downgraded'));
          partes.push(deCopia);
          setAviso(partes.join(' · '));
          // La copia se guarda con otro nombre y en Drive: sin la ruta aquí, el
          // aviso decía que existía y no había forma de llegar a ella salvo salir
          // a buscarla a mano, que es justo lo que nadie quiere hacer.
          setCopia(r.copia ? r.path : '');
          // Firmar con una petición pendiente de ESTE PDF la entrega sola: es lo que
          // se había pedido, y un segundo paso a mano es el que se olvida.
          const firmando = cuerpo.op === 'firmar' || cuerpo.op === 'firmar-certificado';
          if (firmando && r.copia && peticion) await entregar(peticion, r.path);
          // El fichero cambió: se vuelve a leer todo. Reusar lo dibujado enseñaría
          // el documento ANTERIOR con la barra lateral del nuevo, que es peor que
          // esperar un segundo.
          if (!r.copia) {
            setSvgs({});
            setTextos(null);
            await cargar(clave);
          }
          return true;
        } catch (e) {
          // Firmado y sin poder anexarse: se pregunta, y solo con un sí se repite
          // permitiendo que las firmas se pierdan. Nunca en silencio.
          if (
            invalidaFirmas(e) &&
            !(cuerpo.campos as Record<string, unknown> | undefined)?.invalidar_firmas
          ) {
            setTrabajando(false);
            const seguir = await confirmarEnEditor({
              mensaje: t('pdf.signatures.invalidate.confirm'),
              confirmLabel: t('pdf.signatures.invalidate.apply'),
              tone: 'danger',
            });
            if (!seguir) return false;
            const campos = (cuerpo.campos as Record<string, unknown> | undefined) ?? {};
            cuerpo = { ...cuerpo, campos: { ...campos, invalidar_firmas: '1' } };
            continue;
          }
          setError(mensajeDeError(e, t, 'pdf.error.apply'));
          return false;
        } finally {
          setTrabajando(false);
        }
      }
    },
    [path, clave, cargar, t, peticion, entregar],
  );

  const anotar = useCallback(
    (anots: AnotacionNueva[]) => {
      if (anots.length === 0) return;
      void aplicar(
        { op: 'anotar', datos: anots.map((a) => ({ ...a, autor })) },
        t('pdf.saved.annotated'),
      );
    },
    [aplicar, autor, t],
  );

  /* --- Censurar ----------------------------------------------------------- */

  /**
   * Censurar va en DOS pasos —marcar y aplicar— y no en uno, igual que en un
   * editor de escritorio: lo que se borra no vuelve, así que primero se ve todo
   * lo marcado sobre el documento y solo después se destruye.
   */
  const censurar = useCallback(async () => {
    if (zonas.length === 0) return;
    const ok = await confirmarEnEditor({
      mensaje: t('pdf.redact.confirm'),
      confirmLabel: t('pdf.redact.apply'),
      tone: 'danger',
    });
    if (!ok) return;
    const hecho = await aplicar(
      {
        op: 'redactar',
        datos: zonas.map((z) => ({
          pagina: z.pagina,
          x1: z.rect[0],
          y1: z.rect[1],
          x2: z.rect[2],
          y2: z.rect[3],
        })),
        sufijo: t('pdf.suffix.redacted'),
      },
      '',
    );
    // Las marcas solo se sueltan si el servidor las aplicó: borrarlas tras un
    // fallo obligaría a volver a marcarlo todo sin haber conseguido nada.
    if (hecho) setZonas([]);
  }, [zonas, aplicar, t]);

  /* --- Crear campos de formulario ----------------------------------------- */

  /**
   * Marca un campo y pregunta cómo se llama.
   *
   * El nombre se pide AQUÍ y no después porque es lo que ve quien rellena y lo
   * que sale al exportar las respuestas: `texto1` obliga a adivinar. Se puede
   * dejar vacío —entonces el motor lo numera— para marcar veinte casillas
   * seguidas sin contestar veinte veces.
   */
  const marcarCampo = useCallback(
    async (pagina: number, rect: [number, number, number, number]) => {
      const nombre = await preguntarEnEditor({
        mensaje: t('pdf.field.name.ask'),
        permitirVacio: true,
      });
      if (nombre === null) return; // cancelar no marca nada
      setCamposNuevos((prev) => [
        ...prev,
        { pagina, rect, tipo: tipoCampo, nombre: nombre.trim() },
      ]);
    },
    [tipoCampo, t],
  );

  /**
   * Se lleva un trozo de la página como imagen.
   *
   * No toca el documento: es para sacar una tabla o un detalle de un plano y
   * pegarlo en un correo. Va al PORTAPAPELES, que es donde se quiere, y solo si
   * eso no se puede se descarga —así el caso normal es un pegar y no un fichero
   * más en Descargas—.
   *
   * Se rasteriza desde el SVG que ya está en pantalla: ni una petición más.
   */
  const recortar = useCallback(
    async (numero: number, rect: [number, number, number, number]) => {
      const total = contenido?.total;
      const pagina = contenido?.paginas.find((p) => p.numero === numero);
      const svg = svgs[numero];
      if (!pagina || total === undefined || svg === undefined) return;
      const { x, y, ancho, alto } = rectEnSVG(pagina, rect);
      try {
        const blob = await paginaComoImagen(svg, pagina.ancho, pagina.alto, {
          formato: 'png',
          recorte: [x, y, ancho, alto],
        });
        // `write` no existe en todos los navegadores y el permiso puede estar
        // denegado: el fallo NO puede perder el recorte, por eso se descarga.
        try {
          await navigator.clipboard.write([new ClipboardItem({ 'image/png': blob })]);
          setAviso(t('pdf.snapshot.copied'));
        } catch {
          descargarBlob(blob, nombreDePagina(t('pdf.snapshot.file'), numero, total, 'png'));
          setAviso(t('pdf.snapshot.downloaded'));
        }
      } catch {
        setAviso(t('pdf.snapshot.failed'));
      }
    },
    [contenido, svgs, t],
  );

  /** Escribe los campos marcados. Deja copia: el original de un impreso que
   *  alguien va a rellenar sigue siendo el que vale. */
  const crearCampos = useCallback(async () => {
    if (camposNuevos.length === 0) return;
    const hecho = await aplicar(
      {
        op: 'crear-campos',
        datos: camposNuevos.map((c, i) => ({
          tipo: c.tipo,
          // Sin nombre, uno por orden: el motor los desambigua igualmente, pero
          // un `texto3` en la exportación no le dice nada a nadie.
          nombre: c.nombre || `${c.tipo}${i + 1}`,
          pagina: c.pagina,
          rect: c.rect,
        })),
        sufijo: t('pdf.suffix.form'),
      },
      t('pdf.saved.fieldsCreated'),
    );
    if (hecho) setCamposNuevos([]);
  }, [camposNuevos, aplicar, t]);

  /* --- Teclado ------------------------------------------------------------ */

  // Los atajos que cualquiera trae aprendidos de un visor de escritorio. Ctrl+F
  // es el que más importa: el buscador del NAVEGADOR solo encuentra lo que hay
  // dibujado, y aquí solo están dibujadas las páginas por las que se ha pasado,
  // así que sin esto decía «no encontrado» sobre un documento que sí lo tenía.
  useEffect(() => {
    const alPulsar = (e: KeyboardEvent) => {
      const meta = e.ctrlKey || e.metaKey;
      if (meta && (e.key === 'f' || e.key === 'F')) {
        e.preventDefault();
        setPanel('buscar');
        setPanelAbierto(true);
        return;
      }
      if (meta && (e.key === '+' || e.key === '=' || e.key === '-')) {
        e.preventDefault();
        cambiarZoom(e.key === '-' ? -0.15 : 0.15);
        return;
      }
      if (meta && e.key === '0') {
        e.preventDefault();
        setAjuste('ancho');
        return;
      }
      if (meta || e.altKey || enCampoDeTexto(e.target)) return;
      if (e.key === 'Escape') {
        // Escape suelta la herramienta ANTES que cerrar el panel: quien acaba de
        // marcar algo sin querer quiere volver a la mano, no perder la vista.
        if (herramienta !== 'mano') setHerramienta('mano');
        else setPanelAbierto(false);
        return;
      }
      // Pasar página con el teclado, como en cualquier visor de escritorio.
      // Aquí no existía: sin ratón, la única forma de llegar a la página 40 era
      // escribir el número en la barra, y desplazarse a mano no pedía los lotes
      // por delante. El desplazamiento por defecto se cancela a propósito —salta
      // media pantalla, no una hoja— que es justo lo que se espera aquí.
      if (!contenido) return;
      const salto = e.key === 'PageDown' ? 1 : e.key === 'PageUp' ? -1 : 0;
      if (salto !== 0) {
        e.preventDefault();
        irAPagina(paginaActual + salto);
      } else if (e.key === 'Home' || e.key === 'End') {
        e.preventDefault();
        irAPagina(e.key === 'Home' ? 1 : contenido.total);
      }
    };
    document.addEventListener('keydown', alPulsar);
    return () => document.removeEventListener('keydown', alPulsar);
    // `paginaActual` y `contenido` entran en las dependencias porque el manejador
    // los LEE: sin ellas se quedaba con los del primer render y «página
    // siguiente» siempre llevaba a la dos.
    // `irAPagina` no es un `useCallback`: se recrea en cada render, así que
    // meterla aquí volvería a registrar el listener del teclado en TODOS. Lo que
    // esa función lee —`contenido` y la ref del scroll— ya está cubierto.
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [herramienta, paginaActual, contenido]);

  /* --- Buscar ------------------------------------------------------------ */

  const cargarTexto = useCallback(async () => {
    if (textos) return textos;
    try {
      const r = await api.texto(path, clave || undefined);
      setTextos(r.paginas);
      return r.paginas;
    } catch {
      return null;
    }
  }, [textos, path, clave]);

  /* --- Leer en voz alta -------------------------------------------------- */

  const [leyendo, setLeyendo] = useState(false);

  // Al cerrar la pestaña o cambiar de documento, la voz TIENE que callarse: el
  // motor del navegador es global, así que seguiría leyendo sobre otra página.
  useEffect(
    () => () => {
      if (typeof speechSynthesis !== 'undefined') speechSynthesis.cancel();
    },
    [],
  );

  const leerEnVozAlta = useCallback(() => {
    // No está en todos los navegadores; si no está, el botón no hace nada raro.
    if (typeof speechSynthesis === 'undefined' || typeof SpeechSynthesisUtterance === 'undefined') {
      setAviso(t('pdf.speak.unsupported'));
      return;
    }
    if (leyendo) {
      speechSynthesis.cancel();
      setLeyendo(false);
      return;
    }
    setLeyendo(true);
    void cargarTexto().then((paginas) => {
      // Desde la página que se está mirando hasta el final: empezar por la 1
      // obligaría a escuchar todo lo ya leído para llegar a donde vas.
      const desde = (paginas ?? []).slice(paginaActual - 1);
      const texto = desde
        .map((frags) => textoParaLeer(frags ?? []))
        .filter((x) => x !== '')
        .join('. ');
      if (texto === '') {
        setLeyendo(false);
        // Un escaneo sin OCR no tiene texto que leer, y callarse sin decir nada
        // se interpreta como que la función está rota.
        setAviso(t('pdf.speak.noText'));
        return;
      }
      const frase = new SpeechSynthesisUtterance(texto);
      // El idioma de la interfaz: sin esto, una voz inglesa lee el español como
      // si fuera inglés y no se entiende nada.
      frase.lang = locale;
      frase.onend = () => setLeyendo(false);
      frase.onerror = () => setLeyendo(false);
      speechSynthesis.speak(frase);
    });
  }, [leyendo, cargarTexto, paginaActual, locale, t]);

  /* --- Render ------------------------------------------------------------ */

  if (pideClave) {
    return <PideContrasena t={t} onEnviar={(c) => void cargar(c)} error={error} />;
  }
  if (cargando && !contenido) {
    return (
      <div class="text-text-muted flex h-full items-center justify-center gap-3 text-sm">
        <Spinner size={16} label={t('pdf.loading')} />
        {t('pdf.loading')}
      </div>
    );
  }
  if (!contenido) {
    return (
      <div class="flex h-full flex-col items-center justify-center gap-3 p-6 text-center">
        <p class="text-danger-text max-w-sm text-sm">{error || t('pdf.error.open')}</p>
        {/* Casi todo lo que trae aquí es pasajero —la red, el servicio—, así que
            reintentar tiene que estar a un clic y no obligar a recargar. */}
        <div class="flex items-center gap-3">
          <button
            type="button"
            onClick={() => void cargar(clave)}
            disabled={cargando}
            class="bg-primary sytue-focus-ring rounded-control px-3 py-1.5 text-sm font-medium text-[var(--sytue-on-primary)] hover:bg-[var(--sytue-primary-hover)] disabled:opacity-40"
          >
            {t('pdf.retry')}
          </button>
          {/* Al Hub, igual que el «<» de la barra: iba a `ruta('/')`, la lista
              genérica, y encima el rótulo decía «Volver a Sytue Docus» desde un
              error de CertiPDF. */}
          <a
            class="text-accent-text text-sm underline"
            href={volverA ? driveFolderUrl(volverA) : appUrl('one')}
          >
            {volverA ? t('pdf.backToDrive') : t('pdf.back')}
          </a>
        </div>
      </div>
    );
  }

  return (
    <div data-suelo-tactil class="sytue-sheet-corners bg-bg-subtle flex h-full min-h-0 flex-col">
      {/* Lo que cambia sin que nadie lo pida: en qué página se está al desplazarse y que se
          trabaja en algo. Una región viva SIEMPRE presente —una que aparece con su texto ya
          dentro no se anuncia—; los errores y los avisos tienen la suya más abajo. */}
      <div class="sr-only" role="status" aria-live="polite">
        {trabajando ? t('pdf.working') : t('pdf.page.n', { n: paginaActual })}
      </div>
      <BarraHerramientas
        t={t}
        volverA={volverA}
        herramienta={herramienta}
        onHerramienta={setHerramienta}
        color={color}
        onColor={setColor}
        grosor={grosor}
        onGrosor={setGrosor}
        dosPaginas={dosPaginas}
        onDosPaginas={setDosPaginas}
        leyendo={leyendo}
        onLeer={leerEnVozAlta}
        zoom={zoom}
        ajuste={ajuste}
        onZoom={cambiarZoom}
        onAjuste={(a) => setAjuste(a)}
        pagina={paginaActual}
        total={contenido.total}
        onIrA={irAPagina}
        onDialogo={setDialogo}
        permisos={contenido.permisos}
        trabajando={trabajando}
        panelAbierto={panelAbierto}
        onPanelAbierto={setPanelAbierto}
      />

      {(error || aviso) && (
        // Un fallo interrumpe («alert») y un «guardado» no («status»): anunciar
        // el error como estado deja a quien usa un lector de pantalla operando
        // sobre un documento que no se guardó sin enterarse hasta mucho después.
        <div
          role={error ? 'alert' : 'status'}
          class={`flex flex-wrap items-center justify-between gap-3 px-4 py-2 text-sm ${
            error ? 'bg-danger-bg text-danger-text' : 'bg-success-bg text-success-text'
          }`}
        >
          <span>{error || aviso}</span>
          <span class="flex items-center gap-3">
            {copia && (
              <a
                class="sytue-focus-ring rounded px-2 py-0.5 text-xs font-medium underline"
                href={ruta(`/pdf?path=${encodeURIComponent(copia)}`)}
              >
                {t('pdf.saved.open')}
              </a>
            )}
            <button
              type="button"
              class="sytue-focus-ring rounded px-2 py-0.5 text-xs underline"
              onClick={() => {
                setError('');
                setAviso('');
                setCopia('');
              }}
            >
              {t('pdf.dismiss')}
            </button>
          </span>
        </div>
      )}

      {peticion && (
        <div
          role="status"
          class="bg-info-bg text-info-text flex flex-wrap items-center justify-between gap-3 px-4 py-2 text-sm"
        >
          <span class="min-w-0">
            {t('pdf.signatureRequest.banner', { name: quienFirma(peticion, 'received') })}
            {peticion.message && (
              <span class="text-text-muted block text-xs">{peticion.message}</span>
            )}
            {peticion.external && (
              <span class="block text-xs">
                {t('pdf.signatureRequest.externalNote', { name: quienFirma(peticion, 'received') })}
              </span>
            )}
          </span>
          <span class="flex flex-wrap items-center gap-2">
            {peticion.external && bloqueando && (
              <>
                <span class="text-xs">
                  {t('pdf.signatureRequest.blockConfirm', {
                    name: quienFirma(peticion, 'received'),
                  })}
                </span>
                <button
                  type="button"
                  class="sytue-focus-ring rounded px-2 py-1 text-xs font-medium underline"
                  onClick={() => {
                    const nombre = quienFirma(peticion, 'received');
                    void firmas
                      .bloquear(peticion.id)
                      .then(() => {
                        setPeticion(undefined);
                        setAviso(t('pdf.signatureRequest.blocked', { name: nombre }));
                      })
                      .catch((e: unknown) => setError(mensajeDeError(e, t, 'pdf.error.apply')))
                      .finally(() => setBloqueando(false));
                  }}
                >
                  {t('pdf.signatureRequest.block')}
                </button>
                <button
                  type="button"
                  class="sytue-focus-ring rounded px-2 py-1 text-xs underline"
                  onClick={() => setBloqueando(false)}
                >
                  {t('pdf.cancel')}
                </button>
              </>
            )}
            {peticion.external && !bloqueando && (
              <button
                type="button"
                class="sytue-focus-ring rounded px-2 py-1 text-xs underline"
                aria-expanded={false}
                onClick={() => setBloqueando(true)}
              >
                {t('pdf.signatureRequest.block')}
              </button>
            )}
            {sinEntregar && (
              <button
                type="button"
                class="sytue-focus-ring rounded px-2 py-1 text-xs underline"
                onClick={() => void entregar(peticion, sinEntregar)}
              >
                {t('pdf.signatureRequest.retry')}
              </button>
            )}
            <button
              type="button"
              class="sytue-focus-ring rounded px-2 py-1 text-xs underline"
              onClick={() =>
                void firmas
                  .rechazar(peticion.id)
                  .then(() => {
                    setPeticion(undefined);
                    setAviso(t('pdf.signatureRequest.declined'));
                  })
                  .catch((e: unknown) => setError(mensajeDeError(e, t, 'pdf.error.apply')))
              }
            >
              {t('pdf.signatureRequest.decline')}
            </button>
            <button
              type="button"
              class="bg-primary sytue-focus-ring rounded-control px-3 py-1 text-xs font-medium text-[var(--sytue-on-primary)]"
              onClick={() => setDialogo('firmar')}
            >
              {t('pdf.signatureRequest.sign')}
            </button>
          </span>
        </div>
      )}

      {camposNuevos.length > 0 && (
        <div
          role="status"
          class="bg-bg-subtle text-text flex flex-wrap items-center justify-between gap-3 px-4 py-2 text-sm"
        >
          <span>{t('pdf.field.pending', { count: camposNuevos.length })}</span>
          <span class="flex items-center gap-2">
            <label class="flex items-center gap-1.5 text-xs">
              {t('pdf.field.type')}
              <select
                class="border-border bg-surface text-text sytue-focus-ring rounded-md border px-2 py-1 text-xs"
                value={tipoCampo}
                onChange={(e) => setTipoCampo(e.currentTarget.value as TipoCampoNuevo)}
              >
                {TIPOS_CAMPO.map((o) => (
                  <option key={o.valor} value={o.valor}>
                    {t(o.clave)}
                  </option>
                ))}
              </select>
            </label>
            <button
              type="button"
              class="sytue-focus-ring rounded px-2 py-1 text-xs underline"
              onClick={() => setCamposNuevos([])}
            >
              {t('pdf.field.clear')}
            </button>
            <button
              type="button"
              disabled={trabajando}
              class="bg-primary sytue-focus-ring rounded-control px-3 py-1 text-xs font-medium text-[var(--sytue-on-primary)] hover:bg-[var(--sytue-primary-hover)] disabled:opacity-40"
              onClick={() => void crearCampos()}
            >
              {t('pdf.field.apply')}
            </button>
          </span>
        </div>
      )}

      {zonas.length > 0 && (
        <div
          role="status"
          class="bg-warning-bg text-warning-text flex flex-wrap items-center justify-between gap-3 px-4 py-2 text-sm"
        >
          <span>{t('pdf.redact.pending', { count: zonas.length })}</span>
          <span class="flex items-center gap-2">
            <button
              type="button"
              class="sytue-focus-ring rounded px-2 py-1 text-xs underline"
              onClick={() => setZonas([])}
            >
              {t('pdf.redact.clear')}
            </button>
            <button
              type="button"
              disabled={trabajando}
              class="bg-danger-bg text-danger-text border-danger-text/30 sytue-focus-ring rounded-control border px-3 py-1 text-xs font-medium disabled:opacity-40"
              onClick={() => void censurar()}
            >
              {t('pdf.redact.apply')}
            </button>
          </span>
        </div>
      )}

      <div class="flex min-h-0 flex-1">
        <PanelLateral
          t={t}
          panel={panel}
          onPanel={setPanel}
          contenido={contenido}
          svgs={svgs}
          paginaActual={paginaActual}
          onIrA={(n) => {
            irAPagina(n);
            // En un móvil el panel tapa el documento: llevar a una página y
            // dejarlo abierto encima es enseñar la página que no se ve.
            setPanelAbierto(false);
          }}
          onCargarTexto={cargarTexto}
          onAplicar={aplicar}
          trabajando={trabajando}
          path={path}
          clave={clave}
          abierto={panelAbierto}
          onCerrar={() => setPanelAbierto(false)}
        />

        <div
          ref={scrollRef}
          onScroll={alDesplazar}
          // `relative` no es decoración: hace que este contenedor sea el
          // `offsetParent` de las hojas, que es lo que `irAPagina` y
          // `alDesplazar` dan por hecho al comparar `offsetTop` con
          // `scrollTop`. Sin él, el ancestro posicionado era el <body> y las
          // dos cuentas iban desplazadas por la altura del cromo (medido: 148
          // px en vez de 24 para la primera página).
          class="sytue-focus-ring-inner relative flex-1 overflow-auto px-6 py-6"
          data-herramienta={herramienta}
          // Una región con scroll se enfoca por sí misma (WCAG 2.1.1): el teclado desplaza el
          // documento con las flechas, y `scrollable-region-focusable` lo exige a la vista.
          role="region"
          aria-label={t('pdf.panel.pages')}
          // eslint-disable-next-line jsx-a11y/no-noninteractive-tabindex
          tabIndex={0}
        >
          <div
            // Las hojas del documento: lo que dibujan sobre ellas es contenido, no cromo.
            data-sin-tactil
            class={`mx-auto grid w-fit justify-center gap-6 ${
              dosPaginas ? 'grid-cols-2 items-start' : 'grid-cols-1 items-center'
            }`}
          >
            {contenido.paginas.map((p) => (
              <Hoja
                key={p.numero}
                pagina={p}
                svg={svgs[p.numero]}
                zoom={zoom}
                herramienta={herramienta}
                color={color}
                grosor={grosor}
                anotaciones={(contenido.anotaciones ?? []).filter((a) => a.pagina === p.numero - 1)}
                zonas={zonas.filter((z) => z.pagina === p.numero - 1).map((z) => z.rect)}
                campos={camposNuevos.filter((c) => c.pagina === p.numero - 1).map((c) => c.rect)}
                enlaces={(contenido.enlaces ?? []).filter((e) => e.pagina === p.numero - 1)}
                onIrA={irAPagina}
                onAnotar={anotar}
                onCensurar={(rect) => setZonas((prev) => [...prev, { pagina: p.numero - 1, rect }])}
                onCampo={(rect) => void marcarCampo(p.numero - 1, rect)}
                onRecorte={(n, r) => void recortar(n, r)}
                t={t}
              />
            ))}
          </div>
        </div>
      </div>

      <Dialogos
        dialogo={dialogo}
        onCerrar={() => setDialogo(null)}
        t={t}
        contenido={contenido}
        path={path}
        onAplicar={aplicar}
        trabajando={trabajando}
        svgs={svgs}
      />

      {/* Lo que usan las anotaciones para pedir su texto: sin esto montado,
          `preguntarEnEditor` se cae al diálogo del navegador. */}
      <DialogosDelEditor />
    </div>
  );
}

/* ------------------------------------------------------------------------ */

/** Zona marcada para censurar, en coordenadas del PDF. */
interface ZonaCensura {
  pagina: number;
  rect: [number, number, number, number];
}

/** Los tipos de campo que se pueden dibujar. Firma incluida: un impreso que se
 *  va a firmar necesita el recuadro puesto por quien lo prepara, no por quien
 *  lo firma. */
type TipoCampoNuevo = 'texto' | 'casilla' | 'desplegable' | 'firma';

const TIPOS_CAMPO: { valor: TipoCampoNuevo; clave: DocsKey }[] = [
  { valor: 'texto', clave: 'pdf.field.kind.text' },
  { valor: 'casilla', clave: 'pdf.field.kind.check' },
  { valor: 'desplegable', clave: 'pdf.field.kind.select' },
  { valor: 'firma', clave: 'pdf.field.kind.signature' },
];

interface CampoMarcado {
  pagina: number;
  rect: [number, number, number, number];
  tipo: TipoCampoNuevo;
  /** Lo que escribió quien lo marcó; vacío = se numera al crearlo. */
  nombre: string;
}

/** Una página: el documento dibujado y, encima, la capa donde se anota. */
function Hoja({
  pagina,
  svg,
  zoom,
  herramienta,
  color,
  grosor,
  anotaciones,
  zonas,
  campos,
  enlaces,
  onIrA,
  onAnotar,
  onCensurar,
  onCampo,
  onRecorte,
  t,
}: {
  pagina: Pagina;
  svg: string | undefined;
  zoom: number;
  herramienta: Herramienta;
  color: string;
  grosor: number;
  anotaciones: { rect: [number, number, number, number]; tipo: string; texto?: string }[];
  zonas: [number, number, number, number][];
  campos: [number, number, number, number][];
  enlaces: Enlace[];
  onIrA: (n: number) => void;
  onAnotar: (a: AnotacionNueva[]) => void;
  onCensurar: (rect: [number, number, number, number]) => void;
  onCampo: (rect: [number, number, number, number]) => void;
  onRecorte: (pagina: number, rect: [number, number, number, number]) => void;
  t: ReturnType<typeof docsT>;
}) {
  const hojaRef = useRef<HTMLDivElement>(null);
  const [trazo, setTrazo] = useState<[number, number][]>([]);
  const [arrastre, setArrastre] = useState<{
    x0: number;
    y0: number;
    x1: number;
    y1: number;
  } | null>(null);
  const dibujandoRef = useRef(false);

  const ancho = pagina.ancho * zoom;
  const alto = pagina.alto * zoom;

  /** Coordenadas del ratón → coordenadas del SVG de la página. */
  const enSVG = (ev: PointerEvent): [number, number] => {
    const caja = hojaRef.current?.getBoundingClientRect();
    if (!caja) return [0, 0];
    return [(ev.clientX - caja.left) / zoom, (ev.clientY - caja.top) / zoom];
  };

  const esTrazo = herramienta === 'lapiz' || herramienta === 'borrador';
  const esLinea = herramienta === 'linea' || herramienta === 'flecha' || herramienta === 'medida';
  const marcaTexto =
    herramienta === 'resaltar' || herramienta === 'subrayar' || herramienta === 'tachar';
  // Las herramientas de texto NO dibujan: marcan lo SELECCIONADO, y para
  // seleccionar hay que llegar al texto, que está debajo de esta capa. Con la
  // capa interceptando el puntero el arrastre no seleccionaba nada y resaltar,
  // subrayar y tachar no hacían absolutamente nada.
  const capaInteractiva = herramienta !== 'mano' && !marcaTexto;

  // RECHAZO DE LA PALMA. Es LA diferencia entre poder anotar un PDF con lápiz en
  // un iPad y no poder: al escribir, la mano descansa sobre la pantalla y genera
  // un puntero `touch` que dibuja una raya atravesando la página. Ningún visor
  // serio deja que eso pase.
  //
  // La regla: en cuanto se ha visto un lápiz, el dedo deja de dibujar. No se
  // mira solo el evento en curso porque la palma suele tocar ANTES que la punta
  // —se apoya la mano y luego se escribe—, así que hace falta recordarlo. El
  // dedo sigue sirviendo para desplazar y para hacer zoom, que es lo que se
  // espera de él cuando hay un lápiz en la mano.
  const huboLapizRef = useRef(false);

  /**
   * Presiones del trazo en curso (0..1). Se acumulan aparte de los puntos
   * porque el grosor de una anotación /Ink es UNO para todo el trazo: la
   * especificación pone el ancho en /BS /W, no por vértice.
   *
   * ponytail: presión media por trazo, no por punto. Se nota al apretar —que es
   * lo que se espera de un lápiz— sin partir el trazo en segmentos. Si hace
   * falta el trazo que engorda y adelgaza DENTRO de una línea, hay que emitir la
   * apariencia como relleno y no como polilínea, y eso es otra pieza.
   */
  const presionesRef = useRef<number[]>([]);

  /** Si este puntero debe dibujar. El ratón siempre; el dedo solo sin lápiz. */
  const puedeDibujar = (ev: PointerEvent): boolean => {
    if (ev.pointerType === 'pen') {
      huboLapizRef.current = true;
      return true;
    }
    return !(ev.pointerType === 'touch' && huboLapizRef.current);
  };

  const onDown = (ev: PointerEvent) => {
    if (!capaInteractiva || !puedeDibujar(ev)) return;
    (ev.currentTarget as Element).setPointerCapture(ev.pointerId);
    dibujandoRef.current = true;
    const [x, y] = enSVG(ev);
    presionesRef.current = [];
    if (esTrazo) {
      anotarPresion(ev);
      setTrazo([[x, y]]);
    } else setArrastre({ x0: x, y0: y, x1: x, y1: y });
  };

  const onMove = (ev: PointerEvent) => {
    if (!dibujandoRef.current || !puedeDibujar(ev)) return;
    if (!esTrazo) {
      const [x, y] = enSVG(ev);
      setArrastre((a) => (a ? { ...a, x1: x, y1: y } : a));
      return;
    }
    // Un lápiz muestrea muy por encima de la frecuencia de refresco, y el
    // navegador ENTREGA UN SOLO `pointermove` con los intermedios guardados.
    // Sin pedirlos, un trazo rápido sale como una poligonal de cuatro tramos en
    // vez de una curva. `getCoalescedEvents` no existe en todos los motores, de
    // ahí la comprobación.
    const puntos = typeof ev.getCoalescedEvents === 'function' ? ev.getCoalescedEvents() : [ev];
    const usados = puntos.length > 0 ? puntos : [ev];
    usados.forEach(anotarPresion);
    setTrazo((p) => [...p, ...usados.map(enSVG)]);
  };

  /**
   * Guarda la presión SOLO del lápiz. Un ratón y un dedo declaran 0,5 fijo —lo
   * manda la especificación de eventos de puntero para un dispositivo sin
   * sensor—, así que promediarlos con las de un lápiz haría que el grosor
   * cambiase según con qué se hubiera empezado el trazo.
   */
  const anotarPresion = (ev: { pointerType?: string; pressure?: number }): void => {
    if (ev.pointerType !== 'pen') return;
    const p = ev.pressure ?? 0;
    if (p > 0) presionesRef.current.push(p);
  };

  /**
   * Grosor del trazo que se acaba de soltar. Con lápiz, la presión media mueve
   * el grosor elegido entre la mitad y el doble; sin lápiz, se respeta tal cual.
   *
   * El 0,5 de referencia es la presión «normal» de la especificación: apretar
   * por encima engorda, rozar adelgaza, y quien no aprieta nada obtiene
   * exactamente el grosor que eligió en la barra.
   */
  const grosorDelTrazo = (): number => {
    const n = presionesRef.current.length;
    if (n === 0) return grosor;
    const media = presionesRef.current.reduce((a, b) => a + b, 0) / n;
    const factor = Math.min(2, Math.max(0.5, media / 0.5));
    return Math.round(grosor * factor * 10) / 10;
  };

  const onUp = () => {
    if (!dibujandoRef.current) return;
    dibujandoRef.current = false;
    const rgb = colorARGB(color);

    if (esTrazo && trazo.length > 1) {
      const puntos = trazo.map(([x, y]) => aPDF(pagina, x, y));
      onAnotar([
        {
          pagina: pagina.numero - 1,
          tipo: 'tinta',
          trazos: [puntos],
          color: rgb,
          opacidad: 1,
          grosor: grosorDelTrazo(),
        },
      ]);
    } else if (arrastre) {
      const [ax, ay] = aPDF(pagina, arrastre.x0, arrastre.y0);
      const [bx, by] = aPDF(pagina, arrastre.x1, arrastre.y1);
      const rect: [number, number, number, number] = [
        Math.min(ax, bx),
        Math.min(ay, by),
        Math.max(ax, bx),
        Math.max(ay, by),
      ];
      // La línea se decide ANTES que la caja: una línea en diagonal también
      // tiene ancho y alto, y mirando primero el tamaño de la caja las tres
      // herramientas de línea acababan dibujando un rectángulo — que además no
      // es lo que la previsualización enseñaba mientras se arrastraba.
      const largo = Math.hypot(bx - ax, by - ay);
      if (esLinea) {
        // Un clic sin arrastrar no es una línea: sin este mínimo, rozar la
        // página dejaba una anotación de longitud cero imposible de ver y de
        // quitar salvo desde el panel de comentarios.
        if (largo >= 2) {
          onAnotar([
            {
              pagina: pagina.numero - 1,
              tipo: herramienta,
              puntos: [
                [ax, ay],
                [bx, by],
              ],
              color: rgb,
              opacidad: 1,
              grosor,
              // Una medición sin la medida escrita no mide nada. El PDF cuenta
              // en puntos (1/72") y nadie piensa en puntos: se rotula en mm.
              ...(herramienta === 'medida'
                ? { etiqueta: t('pdf.measure.mm', { v: (largo * (25.4 / 72)).toFixed(1) }) }
                : {}),
            },
          ]);
        }
      } else if (herramienta === 'censura') {
        if (rect[2] - rect[0] > 2 && rect[3] - rect[1] > 2) onCensurar(rect);
      } else if (herramienta === 'campo') {
        if (rect[2] - rect[0] > 2 && rect[3] - rect[1] > 2) onCampo(rect);
      } else if (herramienta === 'recorte') {
        if (rect[2] - rect[0] > 2 && rect[3] - rect[1] > 2) onRecorte(pagina.numero, rect);
      } else if (rect[2] - rect[0] > 2 && rect[3] - rect[1] > 2) {
        void anotacionDeCaja(herramienta, pagina.numero - 1, rect, rgb, grosor, t).then(onAnotar);
      }
    }
    setTrazo([]);
    setArrastre(null);
  };

  /** Resaltar lo SELECCIONADO: los rectángulos los da el propio navegador. */
  const resaltarSeleccion = () => {
    const sel = window.getSelection();
    const caja = hojaRef.current?.getBoundingClientRect();
    if (!sel || sel.isCollapsed || !caja) return;
    const cuadros: [number, number, number, number][] = [];
    for (let i = 0; i < sel.rangeCount; i++) {
      for (const r of Array.from(sel.getRangeAt(i).getClientRects())) {
        if (r.width < 1 || r.height < 1) continue;
        if (r.bottom < caja.top || r.top > caja.bottom) continue;
        const [ax, ay] = aPDF(pagina, (r.left - caja.left) / zoom, (r.top - caja.top) / zoom);
        const [bx, by] = aPDF(pagina, (r.right - caja.left) / zoom, (r.bottom - caja.top) / zoom);
        cuadros.push([Math.min(ax, bx), Math.min(ay, by), Math.max(ax, bx), Math.max(ay, by)]);
      }
    }
    if (cuadros.length === 0) return;
    onAnotar([
      {
        pagina: pagina.numero - 1,
        tipo:
          herramienta === 'tachar'
            ? 'tachado'
            : herramienta === 'subrayar'
              ? 'subrayado'
              : 'resaltado',
        cuadros,
        color: colorARGB(color),
        opacidad: herramienta === 'resaltar' ? 0.4 : 1,
        grosor,
      },
    ]);
    sel.removeAllRanges();
  };

  // La marca de texto se engancha con listeners nativos y no con props de JSX
  // por dos motivos, y el segundo es el que importa: `keyup` cubre la selección
  // CON TECLADO (mayúsculas + flechas), que es la única forma de resaltar sin
  // ratón. Con solo `onMouseUp` la herramienta no existiría para quien navega
  // con el teclado.
  useEffect(() => {
    const hoja = hojaRef.current;
    if (!hoja || !marcaTexto) return;
    const alSoltar = () => resaltarSeleccion();
    hoja.addEventListener('mouseup', alSoltar);
    hoja.addEventListener('keyup', alSoltar);
    return () => {
      hoja.removeEventListener('mouseup', alSoltar);
      hoja.removeEventListener('keyup', alSoltar);
    };
  });

  return (
    <div class="flex flex-col items-center gap-1.5" data-pagina={pagina.numero}>
      <div
        ref={hojaRef}
        class="border-border relative border bg-white shadow-sm"
        style={{ width: `${ancho}px`, height: `${alto}px` }}
      >
        {svg === undefined ? (
          <div class="text-text-subtle absolute inset-0 flex items-center justify-center text-xs">
            {t('pdf.page.loading')}
          </div>
        ) : svg === '' ? (
          <div class="text-text-subtle absolute inset-0 flex items-center justify-center px-6 text-center text-xs">
            {t('pdf.page.failed')}
          </div>
        ) : (
          // El SVG entra en el DOM y no en un <img> a propósito: dentro del
          // documento su texto se puede seleccionar y copiar, y las fuentes
          // incrustadas cargan. En un <img> nada de eso ocurre.
          <div
            class="absolute inset-0 [&>svg]:h-full [&>svg]:w-full"
            // eslint-disable-next-line react/no-danger
            dangerouslySetInnerHTML={{ __html: svg }}
          />
        )}

        {/* Los enlaces del documento, encima del dibujo y DEBAJO de la capa de
            anotaciones: son elementos HTML de verdad —un <a> y un <button>— para
            que el teclado los alcance con el tabulador y el lector de pantalla
            los anuncie, cosa que un <rect> con onClick no da. Solo existen con la
            mano activa: con una herramienta de dibujo en curso, un enlace bajo el
            trazo se lo comería. */}
        {!capaInteractiva &&
          enlaces.map((e, i) => {
            const c = rectEnSVG(pagina, e.rect);
            const caja = {
              left: `${(c.x / pagina.ancho) * 100}%`,
              top: `${(c.y / pagina.alto) * 100}%`,
              width: `${(c.ancho / pagina.ancho) * 100}%`,
              height: `${(c.alto / pagina.alto) * 100}%`,
            };
            const clase =
              'absolute rounded-[2px] hover:bg-accent/10 focus-visible:bg-accent/10 sytue-focus-ring';
            return e.url ? (
              <a
                key={`e${i}`}
                href={e.url}
                target="_blank"
                // El documento lo aporta quien llama y puede venir de cualquier
                // parte: sin `noopener` la página abierta puede reescribir esta
                // por `window.opener`, que es la variante clásica del engaño.
                rel="noopener noreferrer nofollow"
                style={caja}
                class={clase}
                title={e.url}
              >
                <span class="sr-only">{t('pdf.link.external', { url: e.url })}</span>
              </a>
            ) : (
              <button
                key={`e${i}`}
                type="button"
                onClick={() => onIrA(e.destino + 1)}
                style={caja}
                class={clase}
                title={t('pdf.link.internal', { n: String(e.destino + 1) })}
              >
                <span class="sr-only">{t('pdf.link.internal', { n: String(e.destino + 1) })}</span>
              </button>
            );
          })}

        {/* Capa de anotaciones: mismo sistema de coordenadas que la página. */}
        <svg
          class={`absolute inset-0 h-full w-full ${
            capaInteractiva ? 'cursor-crosshair' : 'pointer-events-none'
          }`}
          // `touch-action: none` mientras se dibuja, y solo entonces: sin esto,
          // el primer milímetro de un trazo con el dedo o con lápiz lo entiende
          // el navegador como desplazar la página, cancela el puntero y el trazo
          // se pierde. Dibujar en una tableta era imposible.
          style={capaInteractiva ? { touchAction: 'none' } : undefined}
          viewBox={`0 0 ${pagina.ancho} ${pagina.alto}`}
          onPointerDown={onDown}
          onPointerMove={onMove}
          onPointerUp={onUp}
          onPointerCancel={onUp}
          aria-hidden="true"
        >
          {/* Lo marcado para censurar, tal y como quedará: negro y opaco. Se ve
              antes de aplicar porque después no hay vuelta atrás. */}
          {zonas.map((z, i) => {
            const c = rectEnSVG(pagina, z);
            return (
              <rect
                key={`z${i}`}
                x={c.x}
                y={c.y}
                width={c.ancho}
                height={c.alto}
                fill="#111111"
                stroke="var(--color-danger-text)"
                stroke-width={1}
              />
            );
          })}
          {/* Los campos marcados, con el aspecto que van a tener: quien prepara
              un impreso necesita ver dónde caen ANTES de escribirlos. */}
          {campos.map((r, i) => {
            const c = rectEnSVG(pagina, r);
            return (
              <rect
                key={`c${i}`}
                x={c.x}
                y={c.y}
                width={c.ancho}
                height={c.alto}
                fill="var(--color-accent)"
                fill-opacity={0.14}
                stroke="var(--color-accent)"
                stroke-width={1}
              />
            );
          })}
          {anotaciones.map((a, i) => {
            const c = rectEnSVG(pagina, a.rect);
            return (
              <rect
                key={i}
                x={c.x}
                y={c.y}
                width={c.ancho}
                height={c.alto}
                fill="none"
                stroke="var(--color-accent)"
                stroke-width={0.6}
                stroke-dasharray="3 2"
                opacity={0.45}
              >
                <title>{a.texto || a.tipo}</title>
              </rect>
            );
          })}
          {trazo.length > 1 && (
            <polyline
              points={trazo.map(([x, y]) => `${x},${y}`).join(' ')}
              fill="none"
              stroke={color}
              stroke-width={grosor}
              stroke-linecap="round"
              stroke-linejoin="round"
            />
          )}
          {arrastre && (
            <PrevisualizacionCaja
              arrastre={arrastre}
              herramienta={herramienta}
              color={color}
              grosor={grosor}
            />
          )}
        </svg>
      </div>
      <span class="text-text-subtle text-xs tabular-nums">{pagina.numero}</span>
    </div>
  );
}

function PrevisualizacionCaja({
  arrastre,
  herramienta,
  color,
  grosor,
}: {
  arrastre: { x0: number; y0: number; x1: number; y1: number };
  herramienta: Herramienta;
  color: string;
  grosor: number;
}) {
  const x = Math.min(arrastre.x0, arrastre.x1);
  const y = Math.min(arrastre.y0, arrastre.y1);
  const w = Math.abs(arrastre.x1 - arrastre.x0);
  const h = Math.abs(arrastre.y1 - arrastre.y0);
  if (herramienta === 'linea' || herramienta === 'flecha' || herramienta === 'medida') {
    return (
      <line
        x1={arrastre.x0}
        y1={arrastre.y0}
        x2={arrastre.x1}
        y2={arrastre.y1}
        stroke={color}
        stroke-width={grosor}
        stroke-linecap="round"
      />
    );
  }
  if (herramienta === 'elipse') {
    return (
      <ellipse
        cx={x + w / 2}
        cy={y + h / 2}
        rx={w / 2}
        ry={h / 2}
        fill="none"
        stroke={color}
        stroke-width={grosor}
      />
    );
  }
  return (
    <rect
      x={x}
      y={y}
      width={w}
      height={h}
      fill={herramienta === 'censura' ? '#111111' : 'none'}
      stroke={herramienta === 'censura' ? '#111111' : color}
      stroke-width={grosor}
    />
  );
}

/**
 * Traduce la herramienta de caja a la anotación que le corresponde.
 *
 * Asíncrona porque tres de las herramientas piden un texto, y pedirlo con
 * `window.prompt` pintaba los botones en el idioma del sistema operativo en vez
 * del de la aplicación. Esto no es un componente y no tiene dónde montar un
 * diálogo, así que usa el registrador imperativo de `@sytue/ui`: el árbol monta
 * `DialogosDelEditor` una vez y aquí se pide la promesa.
 */
async function anotacionDeCaja(
  herramienta: Herramienta,
  pagina: number,
  rect: [number, number, number, number],
  color: [number, number, number],
  grosor: number,
  t: ReturnType<typeof docsT>,
): Promise<AnotacionNueva[]> {
  const base = { pagina, rect, color, opacidad: 1, grosor };
  switch (herramienta) {
    case 'nota': {
      const texto = (await preguntarEnEditor({ mensaje: t('pdf.prompt.note') })) ?? '';
      if (!texto.trim()) return [];
      return [{ ...base, tipo: 'nota', texto }];
    }
    case 'texto': {
      const texto = (await preguntarEnEditor({ mensaje: t('pdf.prompt.text') })) ?? '';
      if (!texto.trim()) return [];
      return [{ ...base, tipo: 'texto', texto, tamano: 11 }];
    }
    case 'sello': {
      const etiqueta =
        (await preguntarEnEditor({
          mensaje: t('pdf.prompt.stamp'),
          valor: t('pdf.stamp.default'),
        })) ?? '';
      if (!etiqueta.trim()) return [];
      return [{ ...base, tipo: 'sello', etiqueta }];
    }
    case 'elipse':
      return [{ ...base, tipo: 'elipse' }];
    default:
      return [{ ...base, tipo: 'rectangulo' }];
  }
}

function PideContrasena({
  t,
  onEnviar,
  error,
}: {
  t: ReturnType<typeof docsT>;
  onEnviar: (clave: string) => void;
  error: string;
}) {
  const [valor, setValor] = useState('');
  const campoRef = useRef<HTMLInputElement>(null);
  // El foco se pone al montar en vez de con `autoFocus`: el atributo lo mueve
  // ANTES de que la página termine de anunciarse, y un lector de pantalla se
  // salta el título y la explicación de por qué se pide la contraseña.
  useEffect(() => campoRef.current?.focus(), []);

  return (
    <form
      class="mx-auto flex h-full max-w-sm flex-col items-center justify-center gap-4 px-6"
      onSubmit={(e) => {
        e.preventDefault();
        onEnviar(valor);
      }}
    >
      <h1 class="text-text text-lg font-semibold">{t('pdf.password.title')}</h1>
      <p class="text-text-muted text-center text-sm">{t('pdf.password.help')}</p>
      <input
        type="password"
        ref={campoRef}
        value={valor}
        onInput={(e) => setValor((e.target as HTMLInputElement).value)}
        class="border-border bg-surface text-text sytue-focus-ring w-full rounded-md border px-3 py-2 text-sm"
        placeholder={t('pdf.password.placeholder')}
        aria-label={t('pdf.password.placeholder')}
      />
      {error && <p class="text-danger-text text-sm">{error}</p>}
      <button
        type="submit"
        class="bg-primary sytue-focus-ring rounded-control w-full px-3 py-2 text-sm font-medium text-[var(--sytue-on-primary)] hover:bg-[var(--sytue-primary-hover)]"
      >
        {t('pdf.password.open')}
      </button>
    </form>
  );
}

function nombreDe(path: string): string {
  const partes = path.split('/').filter(Boolean);
  return partes[partes.length - 1] ?? path;
}
