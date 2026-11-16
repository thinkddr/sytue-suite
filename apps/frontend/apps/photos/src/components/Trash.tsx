/**
 * La papelera: lo que se quitó de la galería, con «restaurar» y con salida.
 *
 * El fichero sigue en Sytue Drive; aquí solo se decide si la foto se ve en
 * Sytue Fotos. Borrar para siempre se lleva el índice —y con él las recetas y
 * el sitio en los álbumes—, no el original: reindexar esa carpeta la traería
 * de vuelta.
 *
 * Y dice CUÁNDO se va cada foto: la organización purga lo que lleva más de N días
 * en la papelera, y una papelera que borra sin avisar es una trampa. La fecha la
 * pone el servicio (`purgeAt`); sin ella —papelera que no caduca— no se promete nada.
 */
import type { JSX } from 'preact';
import { useEffect, useState } from 'preact/hooks';
import { Button, ConfirmDialog, Dropdown, EmptyState, Icon, IconButton, MenuItem } from '@sytue/ui';
import type { SytueUser } from '@sytue/auth';
import type { Locale } from '@sytue/i18n';
import {
  papelera,
  purgar,
  restaurar,
  textoAlternativo,
  urlDeVariante,
  vaciarPapelera,
  type Foto,
} from '../client/photosApi.js';
import { useT } from '../i18n/index.js';
import { estiloDeTesela } from '../lib/filas.js';
import { diasParaPurga } from '../lib/plazoPapelera.js';
import { conAvisos, FalloDeCarga, useAvisoDeFallo } from './Fallo.js';
import { Marco } from './Marco.js';
import { Nav } from './Nav.js';
import { EsqueletoFilas } from './Esqueletos.js';

export const Trash = conAvisos(Papelera);

function Papelera({
  locale,
  user,
  hubBaseUrl,
}: {
  locale: Locale;
  user: SytueUser;
  hubBaseUrl: string;
}): JSX.Element {
  const t = useT(locale);
  const [fotos, setFotos] = useState<Foto[] | null>(null);
  /** Días que pasa una foto aquí antes de borrarse; 0 = la papelera no caduca. */
  const [retencion, setRetencion] = useState(0);
  /**
   * Desde cuándo se cuentan los días: el instante de la carga, y no `Date.now()` al
   * pintar (no sería puro). Todas las teselas cuentan desde el mismo instante.
   */
  const [ahora, setAhora] = useState(0);
  const [fallo, setFallo] = useState<unknown>(null);
  const avisarFallo = useAvisoDeFallo(locale);
  const [aviso, setAviso] = useState('');
  /** `null` = nada que confirmar; una foto = esa; `'todas'` = vaciar entera. */
  const [confirmando, setConfirmando] = useState<Foto | 'todas' | null>(null);

  const cargar = () => {
    setFallo(null);
    papelera()
      .then((p) => {
        setAhora(Date.now());
        setRetencion(p.retentionDays);
        setFotos(p.items);
      })
      .catch(setFallo);
  };
  useEffect(cargar, []);

  const volver = async (f: Foto) => {
    try {
      await restaurar(f.id);
      setFotos((l) => (l ?? []).filter((x) => x.id !== f.id));
    } catch (e) {
      avisarFallo(e);
    }
  };

  const borrarParaSiempre = async (f: Foto) => {
    setConfirmando(null);
    try {
      await purgar(f.id);
      setFotos((l) => (l ?? []).filter((x) => x.id !== f.id));
      setAviso(t('trash.purged', { count: 1 }));
    } catch (e) {
      avisarFallo(e);
    }
  };

  const vaciar = async () => {
    setConfirmando(null);
    try {
      const { deleted } = await vaciarPapelera();
      setFotos([]);
      setAviso(t('trash.purged', { count: deleted }));
    } catch (e) {
      avisarFallo(e);
    }
  };

  const vaciandoTodo = confirmando === 'todas';

  return (
    <Marco user={user} hubBaseUrl={hubBaseUrl} locale={locale} title={t('nav.trash')}>
      <div class="mx-auto flex h-full w-full max-w-7xl flex-col px-4 py-6">
        <div class="flex flex-wrap items-center gap-2">
          <Nav t={t} locale={locale} activa="trash" />
          {fotos && fotos.length > 0 ? (
            <span class="mb-4 ml-auto">
              <Button
                variant="danger"
                size="sm"
                leadingIcon={<Icon name="trash" />}
                onClick={() => setConfirmando('todas')}
              >
                {t('trash.empty')}
              </Button>
            </span>
          ) : null}
        </div>
        {/* El plazo, dicho una vez para toda la papelera. Vacía lo dice su estado vacío. */}
        {retencion > 0 && fotos && fotos.length > 0 ? (
          <p class="text-text-muted text-sm">{t('trash.retention', { count: retencion })}</p>
        ) : null}
        <p role="status" aria-live="polite" class="text-text-muted min-h-5 text-sm">
          {aviso}
        </p>
        {fallo !== null ? (
          <FalloDeCarga locale={locale} fallo={fallo} alReintentar={cargar} />
        ) : fotos === null ? (
          <EsqueletoFilas />
        ) : fotos.length === 0 ? (
          <EmptyState
            class="flex-1"
            title={t('trash.empty.title')}
            description={
              retencion > 0
                ? t('trash.retention', { count: retencion })
                : t('trash.empty.description')
            }
            spot="emptyPhotos"
          />
        ) : (
          <ul class="filas">
            {fotos.map((f) => (
              <li key={f.id} style={estiloDeTesela(f)} class="bg-bg-subtle relative rounded-sm">
                <img
                  src={urlDeVariante(f.id, 'miniatura')}
                  alt={textoAlternativo(f)}
                  width={f.width}
                  height={f.height}
                  loading="lazy"
                  decoding="async"
                  class="h-full w-full rounded-sm object-cover opacity-70"
                />
                <PlazoDePurga t={t} dias={diasParaPurga(f.purgeAt, ahora)} />
                {/* UN MANDO POR FOTO, con las dos salidas dentro. Eran dos botones
                    de icono en cada tesela —dieciocho en nueve fotos—, y con el dedo
                    no caben: a 44 px cada uno piden 92 y una foto vertical mide 74
                    de ancho en un teléfono. El menú cabe en cualquiera, y «borrar
                    para siempre» deja de estar a un dedo de «restaurar».
                    Su nombre es el de la foto: es el menú DE esa foto. */}
                <span class="absolute right-1 bottom-1">
                  <Dropdown
                    label={f.name}
                    align="end"
                    trigger={(props) => (
                      <IconButton {...props} label={f.name} tone="media" size="sm">
                        <Icon name="ellipsis" size={16} />
                      </IconButton>
                    )}
                  >
                    {(cerrar) => (
                      <>
                        <MenuItem
                          icon={<Icon name="undo" size={16} />}
                          onClick={() => {
                            cerrar();
                            void volver(f);
                          }}
                        >
                          {t('trash.restore')}
                        </MenuItem>
                        <MenuItem
                          danger
                          icon={<Icon name="trash" size={16} />}
                          onClick={() => {
                            cerrar();
                            setConfirmando(f);
                          }}
                        >
                          {t('trash.purge')}
                        </MenuItem>
                      </>
                    )}
                  </Dropdown>
                </span>
              </li>
            ))}
          </ul>
        )}

        {/* El diálogo de la casa y no el `confirm()` del navegador, que habla el
            idioma del sistema en vez del de la sesión. */}
        <ConfirmDialog
          open={confirmando !== null}
          onClose={() => setConfirmando(null)}
          onConfirm={() => {
            if (vaciandoTodo) void vaciar();
            else if (confirmando) void borrarParaSiempre(confirmando);
          }}
          title={vaciandoTodo ? t('trash.empty') : t('trash.purge')}
          confirmLabel={vaciandoTodo ? t('trash.empty') : t('trash.purge')}
          tone="danger"
        >
          <p class="text-text text-sm leading-relaxed">
            {vaciandoTodo
              ? t('trash.emptyConfirm', { count: fotos?.length ?? 0 })
              : t('trash.purgeConfirm')}
          </p>
        </ConfirmDialog>
      </div>
    </Marco>
  );
}

/**
 * «Se borrará en N días» sobre la foto, arriba a la izquierda: abajo a la derecha va
 * su menú. El par de todo lo que va sobre una foto (`SOBRE_FOTO` en PhotosApp.tsx,
 * `negro60` en Fyne): negro al 60 % con tinta blanca da 5,7:1 sobre una foto blanca.
 * En una tesela estrecha se recorta con «…», y el texto entero sigue en el DOM para
 * el lector de pantalla.
 */
export function PlazoDePurga({
  t,
  dias,
}: {
  t: ReturnType<typeof useT>;
  dias: number | null;
}): JSX.Element | null {
  if (dias === null) return null;
  return (
    <span class="pointer-events-none absolute inset-x-1 top-1 flex">
      <span class="min-w-0 truncate rounded-sm bg-black/60 px-1.5 py-0.5 text-xs font-medium text-white">
        {dias === 0 ? t('trash.purgesToday') : t('trash.purgesIn', { count: dias })}
      </span>
    </span>
  );
}
