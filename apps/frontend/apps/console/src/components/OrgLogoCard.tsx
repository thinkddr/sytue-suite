/**
 * OrgLogoCard — el logotipo de la organización.
 *
 * LO QUE SE ENSEÑA ES LA BARRA, no el logo. Un logo puesto grande y aislado engaña:
 * lo que hay que decidir es si se lee a 28 px sobre la barra superior, en claro
 * y en oscuro, que es donde va a vivir. Por eso la previsualización son dos tiras de
 * barra de verdad y no un cuadro con el fichero dentro.
 *
 * Y LO QUE EL SISTEMA DECIDA SE ENSEÑA ANTES DE GUARDARLO. El reencuadre es
 * automático —quita el margen muerto que casi todos los ficheros de marca traen—,
 * pero un recorte que se equivoca y que el administrador descubre después, ya
 * publicado en las once superficies, es peor que no recortar nada. Así que al elegir
 * el fichero se pide el análisis SIN guardar (`?save=0`), se enseña el resultado y se
 * guarda solo si dice que sí. No hay editor de recorte: quien no esté conforme
 * reexporta su logo, que es lo que iba a hacer de todas formas.
 */
import { useState } from 'preact/hooks';
import { Button, Card, CardBody, CardHeader, CardTitle, InlineHelp, OrgMark } from '@sytue/ui';
import type { Locale } from '@sytue/i18n';
import { appDomains, themeColors } from '@sytue/config';
import { useT, type T } from '../i18n/index.js';

interface Analisis {
  png: string;
  hash: string;
  width: number;
  height: number;
  trimmed: boolean;
  trim_capped: boolean;
  opaque_box: boolean;
  saved: boolean;
}

/** Una tira de barra superior, del tamaño real, para ver el logo donde va a vivir. */
function Tira({
  oscura,
  children,
  pie,
}: {
  oscura: boolean;
  children: preact.ComponentChildren;
  pie: string;
}) {
  const tema = themeColors[oscura ? 'dark' : 'light'];
  return (
    <div class="flex flex-col gap-1">
      <div
        class="border-border flex h-14 items-center gap-3 rounded-md border px-3"
        style={{ backgroundColor: tema['topbar-bg'], color: tema['topbar-fg'] }}
      >
        {children}
      </div>
      <span class="sytue-label">{pie}</span>
    </div>
  );
}

/** Alto al que se rasteriza un SVG. El doble de los 112 px que guarda el servidor,
 *  para que el reencuadre trabaje con margen y el resultado no se vea blando. */
const ALTO_RASTER = 224;

/**
 * Convierte un SVG a PNG con el propio navegador.
 *
 * El SVG es el formato en el que llega la mayoría de logotipos corporativos, pero
 * es también el que más superficie de ataque tiene: puede traer `<script>`,
 * `<foreignObject>` o referencias a recursos remotos. Rasterizarlo AQUÍ, y subir
 * solo el PNG, evita las dos cosas de golpe — al servidor nunca le llega un SVG, y
 * el motor de encaje sigue recibiendo un mapa de bits como cualquier otro logo.
 *
 * Es seguro porque un SVG cargado a través de `<img>` va en modo imagen: el
 * navegador no ejecuta su script ni carga sus recursos externos, por especificación.
 * Y si el fichero intentara traer algo de fuera, el lienzo quedaría contaminado y
 * `toBlob` fallaría, así que ese caso se rechaza solo.
 */
async function svgAPng(fichero: File): Promise<Blob> {
  const url = URL.createObjectURL(fichero);
  try {
    const img = new Image();
    img.src = url;
    await img.decode();
    // Un SVG sin `width`/`height` intrínsecos mide 0 en algunos navegadores y 300×150
    // en otros. Con el aspecto del viewBox se rasteriza igual en todos.
    const w = img.naturalWidth || 300;
    const h = img.naturalHeight || 150;
    const lienzo = document.createElement('canvas');
    lienzo.height = ALTO_RASTER;
    lienzo.width = Math.max(1, Math.round((w / h) * ALTO_RASTER));
    lienzo.getContext('2d')?.drawImage(img, 0, 0, lienzo.width, lienzo.height);
    return await new Promise<Blob>((ok, mal) =>
      lienzo.toBlob((b) => (b ? ok(b) : mal(new Error('canvas'))), 'image/png'),
    );
  } finally {
    URL.revokeObjectURL(url);
  }
}

function Inner({ slug, logoHash, t }: { slug: string; logoHash: string; t: T }) {
  const [analisis, setAnalisis] = useState<Analisis | null>(null);
  const [hash, setHash] = useState(logoHash);
  const [error, setError] = useState('');
  const [ocupado, setOcupado] = useState(false);

  async function pedir(fichero: Blob, guardar: boolean) {
    setOcupado(true);
    setError('');
    try {
      const res = await fetch(`/api/admin/org/logo?save=${guardar ? 1 : 0}`, {
        method: 'PUT',
        body: fichero,
      });
      const cuerpo = (await res.json().catch(() => ({}))) as Analisis & { message?: string };
      if (!res.ok) {
        setError(cuerpo.message ?? t('logo.error.generic'));
        return;
      }
      setAnalisis(cuerpo);
      if (guardar && cuerpo.saved) {
        // El hash que devuelve el servicio, no un timestamp: es el mismo con el que
        // se va a pedir el logo en la barra, así que la previsualización y lo que
        // verá el resto de la plantilla apuntan al mismo sitio.
        setHash(cuerpo.hash);
        setAnalisis(null);
      }
    } catch {
      setError(t('logo.error.generic'));
    } finally {
      setOcupado(false);
    }
  }

  /** Rasteriza el SVG y lo sube como PNG; si el fichero no se deja dibujar, lo dice. */
  async function subirSvg(fichero: File) {
    setOcupado(true);
    setError('');
    try {
      const png = await svgAPng(fichero);
      await pedir(png, false);
    } catch {
      setError(t('logo.error.svg'));
      setOcupado(false);
    }
  }

  async function quitar() {
    setOcupado(true);
    setError('');
    try {
      // Sin mirar la respuesta, un 403 vaciaba la tarjeta y el logo seguía puesto
      // hasta que alguien recargaba: éxito fingido.
      const res = await fetch('/api/admin/org/logo', { method: 'DELETE' });
      if (!res.ok) {
        setError(t('logo.error.delete'));
        return;
      }
      setHash('');
      setAnalisis(null);
    } catch {
      setError(t('logo.error.delete'));
    } finally {
      setOcupado(false);
    }
  }

  // Mientras hay un análisis sin confirmar se enseña ESE, no el guardado: es lo que
  // se está decidiendo.
  const previo = analisis ? `data:image/png;base64,${analisis.png}` : '';
  // Con el host de Sytue ID: quien sirve el logo es el IdP, no la consola. Una
  // ruta relativa aquí pediría el PNG a admin.sytue.eu y no lo encontraría.
  const actual = hash
    ? `${appDomains.id}/branding/${encodeURIComponent(slug)}/logo.png?v=${hash}`
    : '';
  const src = previo || actual;

  return (
    <Card>
      <CardHeader>
        <CardTitle>{t('logo.title')}</CardTitle>
      </CardHeader>
      <CardBody class="flex flex-col gap-4">
        <p class="text-text-muted text-sm">{t('logo.description')}</p>

        <div class="grid gap-3 sm:grid-cols-2">
          <Tira oscura pie={t('logo.previewDark')}>
            {src ? (
              <img src={src} alt="" class="h-7 w-auto max-w-[160px] object-contain" />
            ) : (
              <OrgMark slug={slug} size={28} />
            )}
            <span class="text-sm font-semibold">{t('logo.previewApp')}</span>
          </Tira>
          <Tira oscura={false} pie={t('logo.previewLight')}>
            {src ? (
              <img src={src} alt="" class="h-7 w-auto max-w-[160px] object-contain" />
            ) : (
              <OrgMark slug={slug} size={28} />
            )}
            <span class="text-sm font-semibold">{t('logo.previewApp')}</span>
          </Tira>
        </div>

        {analisis?.trimmed ? (
          <InlineHelp tone="info" title={t('logo.trimmed')}>
            {t('logo.trimmedHint')}
          </InlineHelp>
        ) : null}
        {analisis?.trim_capped ? (
          <InlineHelp tone="warning" title={t('logo.trimCapped')}>
            {t('logo.trimCappedHint')}
          </InlineHelp>
        ) : null}
        {analisis?.opaque_box ? (
          <InlineHelp tone="warning" title={t('logo.opaqueBox')}>
            {t('logo.opaqueBoxHint')}
          </InlineHelp>
        ) : null}
        {error ? (
          <InlineHelp tone="danger" title={t('logo.error.title')}>
            {error}
          </InlineHelp>
        ) : null}

        <div class="flex flex-wrap items-center gap-2">
          {/* El input va oculto tras un <label>: un `<input type=file>` a pelo no se
              puede estilar y su rótulo lo pone el navegador, en su idioma y no en el
              nuestro. Sigue siendo alcanzable con teclado, que es lo que importa. */}
          <label class="sytue-focus-ring-within border-border-strong text-text hover:bg-bg-subtle inline-flex h-9 cursor-pointer items-center rounded-md border px-3 text-sm font-semibold">
            {t('logo.choose')}
            <input
              type="file"
              accept="image/png,image/jpeg,image/gif,image/svg+xml"
              class="sr-only"
              disabled={ocupado}
              onChange={(e) => {
                const f = e.currentTarget.files?.[0];
                // El SVG se convierte a PNG aquí y sube ya rasterizado: al servicio
                // nunca le llega un SVG. Ver svgAPng.
                if (f) void (f.type === 'image/svg+xml' ? subirSvg(f) : pedir(f, false));
                e.currentTarget.value = '';
              }}
            />
          </label>
          {analisis ? (
            <Button
              variant="primary"
              size="sm"
              loading={ocupado}
              onClick={() => {
                const bin = Uint8Array.from(atob(analisis.png), (c) => c.charCodeAt(0));
                void pedir(new Blob([bin], { type: 'image/png' }), true);
              }}
            >
              {t('logo.save')}
            </Button>
          ) : null}
          {hash && !analisis ? (
            <Button size="sm" variant="ghost" disabled={ocupado} onClick={() => void quitar()}>
              {t('logo.reset')}
            </Button>
          ) : null}
        </div>
        <p class="text-text-muted text-xs leading-relaxed">{t('logo.formats')}</p>
      </CardBody>
    </Card>
  );
}

export default function OrgLogoCard({
  locale,
  slug,
  logoHash = '',
}: {
  locale: Locale;
  slug: string;
  logoHash?: string;
}) {
  return <Inner slug={slug} logoHash={logoHash} t={useT(locale)} />;
}
