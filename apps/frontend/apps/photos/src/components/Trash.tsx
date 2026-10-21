/**
 * La papelera: lo que se quitó de la galería, con «restaurar» y con salida.
 *
 * El fichero sigue en Sytue Drive; aquí solo se decide si la foto se ve en
 * Sytue Fotos. Borrar para siempre se lleva el índice —y con él las recetas y
 * el sitio en los álbumes—, no el original: reindexar esa carpeta la traería
 * de vuelta.
 */
import type { JSX } from 'preact';
import { useEffect, useState } from 'preact/hooks';
import {
  Button,
  ConfirmDialog,
  Dropdown,
  EmptyState,
  Icon,
  IconButton,
  Illustration,
  MenuItem,
  Spinner,
} from '@sytue/ui';
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
import { Marco } from './Marco.js';
import { Nav } from './Nav.js';

export function Trash({
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
  const [error, setError] = useState('');
  const [aviso, setAviso] = useState('');
  /** `null` = nada que confirmar; una foto = esa; `'todas'` = vaciar entera. */
  const [confirmando, setConfirmando] = useState<Foto | 'todas' | null>(null);

  useEffect(() => {
    papelera()
      .then(setFotos)
      .catch((e) => setError(String(e)));
  }, []);

  const volver = async (f: Foto) => {
    try {
      await restaurar(f.id);
      setFotos((l) => (l ?? []).filter((x) => x.id !== f.id));
    } catch (e) {
      setError(String(e));
    }
  };

  const borrarParaSiempre = async (f: Foto) => {
    setConfirmando(null);
    try {
      await purgar(f.id);
      setFotos((l) => (l ?? []).filter((x) => x.id !== f.id));
      setAviso(t('trash.purged', { count: 1 }));
    } catch (e) {
      setError(String(e));
    }
  };

  const vaciar = async () => {
    setConfirmando(null);
    try {
      const { deleted } = await vaciarPapelera();
      setFotos([]);
      setAviso(t('trash.purged', { count: deleted }));
    } catch (e) {
      setError(String(e));
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
        <p role="status" aria-live="polite" class="text-text-muted min-h-5 text-sm">
          {aviso}
        </p>
        {error ? (
          <p class="text-danger-text mb-4 text-sm" role="alert">
            {error}
          </p>
        ) : null}
        {fotos === null ? (
          <Spinner />
        ) : fotos.length === 0 ? (
          <EmptyState
            class="flex-1"
            title={t('trash.empty.title')}
            description={t('trash.empty.description')}
            illustration={<Illustration name="emptyPhotos" />}
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
