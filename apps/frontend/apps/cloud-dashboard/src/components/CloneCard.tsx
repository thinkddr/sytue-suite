/**
 * CloneCard — crear otro proyecto igual que este.
 *
 * Es lo que todo el mundo acaba haciendo a mano para tener un staging: repetir a
 * ojo el repositorio, la rama, los comandos de compilación, el tamaño, el puerto,
 * la sonda… y las veinte variables de entorno. Ahí es donde se cuela el error que
 * cuesta una tarde encontrar.
 *
 * No vive en la zona de peligro porque no toca nada de este proyecto: crea uno
 * nuevo. Lo que sí dice, antes de pulsar, es qué NO se lleva —dominios, bases de
 * datos y demás servicios— para que nadie descubra después que su staging no
 * tiene base de datos.
 *
 * Solo @sytue/ui. §14.
 */
import { useState } from 'preact/hooks';
import {
  Card,
  CardBody,
  CardHeader,
  CardTitle,
  Button,
  FieldGroup,
  Input,
  Icon,
  InlineHelp,
  ToastProvider,
  useToast,
} from '@sytue/ui';
import type { Locale } from '@sytue/i18n';
import { useT } from '../i18n/index.js';
import { r } from '../lib/rutas.js';
import { postBff, notifyDemoIfNeeded } from '../lib/bff-client.js';

interface Props {
  project: string;
  live: boolean;
  /** Idioma ya negociado en el servidor (llega por props, nunca se detecta aquí). */
  locale: Locale;
}

export default function CloneCard(props: Props) {
  return (
    <ToastProvider>
      <Inner {...props} />
    </ToastProvider>
  );
}

function Inner({ project, live, locale }: Props) {
  const t = useT(locale);
  const toast = useToast();
  // Un destino evidente y editable: casi siempre se quiere «-staging», y quien
  // quiera otra cosa lo cambia sin tener que inventarlo desde cero.
  const [slug, setSlug] = useState(`${project}-staging`);
  const [busy, setBusy] = useState(false);
  const [error, setError] = useState<string | null>(null);

  async function clone(e: Event) {
    e.preventDefault();
    const destino = slug.trim();
    // Mismo patrón que el slug del alta: minúsculas, números y guiones. Se valida
    // aquí para no gastar un viaje, y el control-plane lo vuelve a validar.
    if (!/^[a-z0-9]([a-z0-9-]*[a-z0-9])?$/.test(destino)) {
      setError(t('clone.error.slug'));
      return;
    }
    setError(null);
    setBusy(true);
    try {
      const res = await postBff<{ ok: boolean; live: boolean; slug: string; copied: number }>(
        '/api/projects/clone',
        { project, slug: destino },
      );
      notifyDemoIfNeeded(t, toast, res.live);
      toast.notify({
        tone: 'success',
        title: t('clone.done', { slug: destino }),
        // La descripción se OMITE cuando no hay nada que contar, en vez de pasarla
        // como `undefined`: con `exactOptionalPropertyTypes` una propiedad opcional
        // no acepta ese valor explícito, y el typecheck cae con ts(2379).
        ...(res.live ? { description: t('clone.done.desc', { count: res.copied }) } : {}),
      });
      // Al proyecto nuevo directamente: el siguiente paso es siempre desplegarlo,
      // y buscarlo en la lista para eso sobra.
      if (res.live) window.location.href = r(`/projects/${destino}`);
    } catch (err) {
      const status = /bff_(\d+)/.exec(err instanceof Error ? err.message : '')?.[1];
      setError(
        status === '409'
          ? t('clone.error.taken')
          : status === '402'
            ? t('clone.error.quota')
            : t('clone.error.generic'),
      );
    } finally {
      setBusy(false);
    }
  }

  return (
    <Card class="mt-6">
      <CardHeader>
        <CardTitle as="h2">
          <span class="flex items-center gap-2">
            <Icon name="grid" size={18} />
            {t('clone.title')}
          </span>
        </CardTitle>
      </CardHeader>
      <CardBody>
        <p class="text-text-muted mb-4 text-sm">{t('clone.desc')}</p>
        <form class="flex flex-wrap items-end gap-3" onSubmit={(e) => void clone(e)}>
          <FieldGroup
            label={t('clone.slug')}
            htmlFor="clone-slug"
            help={t('clone.slug.help')}
            class="min-w-[16rem] flex-1"
            {...(error ? { error } : {})}
          >
            <Input
              id="clone-slug"
              value={slug}
              class="font-mono"
              invalid={error != null}
              onInput={(e) => {
                setSlug(e.currentTarget.value);
                if (error) setError(null);
              }}
            />
          </FieldGroup>
          <Button
            type="submit"
            variant="secondary"
            class="shrink-0"
            loading={busy}
            disabled={!live}
          >
            {t('clone.action')}
          </Button>
        </form>
      </CardBody>
      {/* Lo que NO se lleva, dicho ANTES de pulsar: es la sorpresa que arruina un
          staging recién hecho. */}
      <InlineHelp class="mx-4 mb-4">{t('clone.excludes')}</InlineHelp>
    </Card>
  );
}
