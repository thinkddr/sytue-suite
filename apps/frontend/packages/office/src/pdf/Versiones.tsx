/*
 * SPDX-License-Identifier: Apache-2.0
 * SPDX-FileCopyrightText: (c) 2026 Daniel Delgado Ramos
 *
 * «Versiones»: volver a como estaba el PDF antes de un cambio.
 *
 * Cada anotación, relleno, censura o firma reescribe el fichero en Drive y el visor
 * no tiene «Deshacer»: un sello en la página que no era, o una censura de más, se
 * quedaban. Drive ya guarda cada escritura como una versión, así que aquí se listan
 * (con la fila común de `@sytue/ui`, la misma que el explorador de Drive), se puede
 * ABRIR una anterior en otra pestaña para mirarla y RESTAURARLA. Restaurar no borra
 * nada —lo que había pasa a ser una versión más—, y eso se dice antes de hacerlo.
 *
 * Con firmas se avisa además de lo que de verdad se pierde: si la firma se puso
 * después de la versión elegida, el documento restaurado no la lleva. La firma no
 * desaparece del historial, pero el PDF vigente deja de estar firmado.
 *
 * Con el PDF en la bóveda (ADR 0086) el historial es el del repositorio cifrado: la
 * fuente del estudio lo lista, abre una versión descifrándola aquí y la restaura, sin
 * que el servidor vea ninguna.
 */
import { useCallback, useEffect, useState } from 'preact/hooks';
import { Button, EmptyState, InlineHelp, Spinner, VersionList, useConfirm } from '@sytue/ui';
import type { FileVersion } from '@sytue/api-clients';
import type { Locale } from '@sytue/i18n';
import type { DocsT } from '../i18n';
import { ErrorPDF } from './api';
import { mensajeDeError } from './errores';
import { useFuentePdf } from './fuente';

export function Versiones({
  t,
  locale,
  path,
  firmado,
  onRestaurada,
}: {
  t: DocsT;
  locale?: Locale | undefined;
  path: string;
  /** El documento vigente lleva firmas: restaurar puede dejarlo sin la última. */
  firmado: boolean;
  /** Tras restaurar: el visor vuelve a leer el documento. */
  onRestaurada: (version: number) => void;
}) {
  const { confirmar, dialogo } = useConfirm();
  const [lista, setLista] = useState<FileVersion[] | null>(null);
  const [siguiente, setSiguiente] = useState<string | null>(null);
  const [error, setError] = useState(false);
  const [cargandoMas, setCargandoMas] = useState(false);
  const [fallo, setFallo] = useState('');
  const [restaurando, setRestaurando] = useState<number | null>(null);
  const fuente = useFuentePdf(path);

  const cargar = useCallback(
    async (cursor?: string) => {
      if (cursor) setCargandoMas(true);
      else {
        setLista(null);
        setError(false);
      }
      try {
        const p = await fuente.versiones.listar(cursor);
        setLista((antes) => (cursor ? [...(antes ?? []), ...p.versions] : p.versions));
        setSiguiente(p.nextCursor ?? null);
      } catch {
        // Sin la primera página no hay nada que enseñar; sin las siguientes, lo ya
        // cargado sigue valiendo y basta con poder pedirlas otra vez.
        if (!cursor) setError(true);
      } finally {
        setCargandoMas(false);
      }
    },
    [fuente],
  );

  useEffect(() => {
    void cargar();
  }, [cargar]);

  const restaurar = async (v: FileVersion) => {
    if (restaurando !== null) return;
    // Una sola frase por caso, del catálogo: el aviso de las firmas va DETRÁS de lo
    // que pasa siempre, porque es lo que hace dudar.
    const mensaje = [
      t('pdf.versions.confirm', { version: v.version }),
      firmado ? t('pdf.versions.confirm.signed') : '',
    ]
      .filter(Boolean)
      .join(' ');
    const ok = await confirmar({
      titulo: t('pdf.versions.confirm.title', { version: v.version }),
      mensaje,
      confirmLabel: t('pdf.versions.restore'),
    });
    if (!ok) return;
    setRestaurando(v.version);
    setFallo('');
    try {
      await fuente.versiones.restaurar(v.version);
      onRestaurada(v.version);
    } catch (e) {
      setFallo(
        e instanceof ErrorPDF && e.estado === 403
          ? t('pdf.versions.denied')
          : mensajeDeError(e, t, 'pdf.versions.failed'),
      );
    } finally {
      setRestaurando(null);
    }
  };

  if (lista === null && !error) {
    return (
      <div class="grid h-40 place-items-center" aria-busy="true">
        <Spinner size={22} />
        <span class="sr-only" role="status">
          {t('pdf.versions.loading')}
        </span>
      </div>
    );
  }

  if (error || lista === null) {
    return (
      <div class="grid h-40 place-items-center p-4 text-center">
        <div>
          <p class="text-text-muted text-base">{t('pdf.versions.loadError')}</p>
          <Button variant="secondary" size="sm" class="mt-2" onClick={() => void cargar()}>
            {t('pdf.retry')}
          </Button>
        </div>
      </div>
    );
  }

  const hayAnteriores = lista.length > 1;
  return (
    <div>
      <InlineHelp tone="neutral" class="mb-3">
        {t('pdf.versions.help')}
      </InlineHelp>
      {fallo ? (
        <p role="alert" class="text-danger-text mb-3 text-sm">
          {fallo}
        </p>
      ) : null}
      {hayAnteriores ? (
        <VersionList
          versions={lista}
          {...(locale ? { locale } : {})}
          versionLabel={(v) => t('pdf.versions.version', { version: v.version })}
          currentLabel={t('pdf.versions.current')}
          actions={(v) =>
            v.isCurrent ? null : (
              <>
                {/* Un enlace de verdad: se abre en otra pestaña con el visor del
                    navegador, sin tocar el documento que se está editando aquí. En la
                    bóveda no hay URL que valga: se descifra aquí y se abre en claro. */}
                {fuente.versiones.enlace ? (
                  <Button
                    variant="ghost"
                    size="sm"
                    href={fuente.versiones.enlace(v.version)}
                    target="_blank"
                    rel="noopener"
                    aria-label={t('pdf.versions.openAria', { version: v.version })}
                  >
                    {t('pdf.versions.open')}
                  </Button>
                ) : fuente.versiones.abrir ? (
                  <Button
                    variant="ghost"
                    size="sm"
                    aria-label={t('pdf.versions.openAria', { version: v.version })}
                    onClick={() => {
                      setFallo('');
                      void fuente.versiones
                        .abrir?.(v.version)
                        .catch((e: unknown) =>
                          setFallo(mensajeDeError(e, t, 'pdf.versions.failed')),
                        );
                    }}
                  >
                    {t('pdf.versions.open')}
                  </Button>
                ) : null}
                <Button
                  variant="secondary"
                  size="sm"
                  loading={restaurando === v.version}
                  disabled={restaurando !== null}
                  aria-label={t('pdf.versions.restoreAria', { version: v.version })}
                  onClick={() => void restaurar(v)}
                >
                  {t('pdf.versions.restore')}
                </Button>
              </>
            )
          }
        />
      ) : (
        <EmptyState class="py-6" title={t('pdf.versions.empty')} />
      )}
      {siguiente !== null ? (
        <div class="flex justify-center pt-3">
          <Button
            variant="ghost"
            size="sm"
            loading={cargandoMas}
            onClick={() => void cargar(siguiente)}
          >
            {t('pdf.versions.loadMore')}
          </Button>
        </div>
      ) : null}
      {dialogo}
    </div>
  );
}
