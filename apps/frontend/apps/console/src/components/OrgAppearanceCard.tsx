/**
 * La apariencia por defecto de la organización: su color (ADR 0071) y su forma.
 *
 * Lo que se fija aquí NO es la elección de nadie: es lo que ve quien no ha elegido lo
 * suyo en su menú de cuenta. Se guarda al pulsar y no hay botón: no es un formulario,
 * es una elección entre las opciones que la suite ofrece.
 *
 * TRAS GUARDAR SE REFRESCA LA SESIÓN. El valor viaja en el id_token (`org_color`,
 * `org_forma`), así que sin ese refresco la consola seguiría enseñando —y pintando— el
 * anterior hasta que caducara el token del propio administrador: guardar, recargar y
 * ver el valor viejo.
 *
 * Una tarjeta por eje (`OrgColorCard`, `OrgFormaCard`) sobre el mismo cuerpo: lo único
 * que cambia es el nombre del eje, sus opciones y cómo se rotulan.
 */
import type { JSX } from 'preact';
import { useState } from 'preact/hooks';
import {
  Card,
  CardBody,
  CardHeader,
  CardTitle,
  Icon,
  THEME_COLORS,
  THEME_FORMAS,
  cx,
  themeColorSwatch,
} from '@sytue/ui';
import { translate, type Locale } from '@sytue/i18n';
import { useT } from '../i18n/index.js';

function OrgAppearanceCard<T extends string>({
  eje,
  opciones,
  valor,
  titulo,
  descripcion,
  fallo,
  rotulo,
  muestra,
}: {
  /** El eje en Sytue ID: `PUT /api/admin/org/<eje>` con `{ <eje>: valor }`. */
  eje: 'color' | 'forma';
  opciones: readonly T[];
  valor: string;
  titulo: string;
  descripcion: string;
  fallo: string;
  rotulo: (opcion: T) => string;
  /** Color CSS de la muestra que acompaña al rótulo, si la opción es un color. */
  muestra?: (opcion: T) => string;
}): JSX.Element {
  const [actual, setActual] = useState(valor);
  const [ocupado, setOcupado] = useState(false);
  const [error, setError] = useState(false);

  async function elegir(o: T) {
    if (o === actual || ocupado) return;
    setOcupado(true);
    setError(false);
    try {
      const res = await fetch(`/api/admin/org/${eje}`, {
        method: 'PUT',
        headers: { 'content-type': 'application/json' },
        body: JSON.stringify({ [eje]: o }),
      });
      // Sin mirar la respuesta, un 403 marcaba la opción y el valor seguía siendo
      // el anterior: éxito fingido.
      if (!res.ok) throw new Error(String(res.status));
      setActual(o);
      // Mejor esfuerzo: si falla, el valor llega igual con el siguiente token.
      await fetch('/auth/refresh', { method: 'POST' }).catch(() => undefined);
    } catch {
      setError(true);
    } finally {
      setOcupado(false);
    }
  }

  return (
    <Card>
      <CardHeader>
        <CardTitle>{titulo}</CardTitle>
      </CardHeader>
      <CardBody class="flex flex-col gap-4">
        <p class="text-text-muted text-sm">{descripcion}</p>
        <div role="radiogroup" aria-label={titulo} class="flex flex-wrap gap-2">
          {opciones.map((o) => (
            <button
              key={o}
              type="button"
              role="radio"
              aria-checked={actual === o}
              disabled={ocupado}
              onClick={() => void elegir(o)}
              class={cx(
                'sytue-focus-ring rounded-control flex h-10 items-center gap-2 border px-3 text-sm font-medium',
                actual === o
                  ? 'border-primary bg-selected text-text'
                  : 'border-border-strong text-text-muted hover:bg-bg-subtle',
              )}
            >
              {/* La muestra sale de la hoja, no del primario en vigor: hay que ver
                  también el color que no está pintado. */}
              {muestra ? (
                <span
                  aria-hidden="true"
                  class="size-4 rounded-full"
                  style={{ background: muestra(o) }}
                />
              ) : null}
              {rotulo(o)}
              {/* La marca, además del borde: la elección no puede leerse solo por color. */}
              {actual === o ? <Icon name="check" size={16} /> : null}
            </button>
          ))}
        </div>
        {error ? (
          <p class="text-danger-text text-xs" role="alert">
            {fallo}
          </p>
        ) : null}
      </CardBody>
    </Card>
  );
}

/** El color por defecto de la organización. */
export function OrgColorCard({ locale, color }: { locale: Locale; color: string }): JSX.Element {
  const t = useT(locale);
  return (
    <OrgAppearanceCard
      eje="color"
      opciones={THEME_COLORS}
      valor={color}
      titulo={t('orgColor.title')}
      descripcion={t('orgColor.description')}
      fallo={t('orgColor.error')}
      rotulo={(c) => translate(locale, `account.color.${c}`)}
      muestra={themeColorSwatch}
    />
  );
}

/** La forma por defecto de la organización: «Marco» o «Carril». */
export function OrgFormaCard({ locale, forma }: { locale: Locale; forma: string }): JSX.Element {
  const t = useT(locale);
  return (
    <OrgAppearanceCard
      eje="forma"
      opciones={THEME_FORMAS}
      valor={forma}
      titulo={t('orgForma.title')}
      descripcion={t('orgForma.description')}
      fallo={t('orgForma.error')}
      rotulo={(f) => translate(locale, `account.forma.${f}`)}
    />
  );
}
