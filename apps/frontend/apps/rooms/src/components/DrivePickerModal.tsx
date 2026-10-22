/**
 * DrivePickerModal — selector de un fichero YA existente en Sytue Drive,
 * montado sobre SelectorDeFicheros de @sytue/shell.
 */
import type { JSX } from 'preact';
import { SelectorDeFicheros, type PickerItem } from '@sytue/shell';
import type { Locale } from '@sytue/i18n';
import type { Attachment } from '../client/chatApi.js';
import type { T } from '../i18n/index.js';

interface Props {
  onClose: () => void;
  /** Adjunta el fichero elegido como referencia de Drive (fileId + metadatos). */
  onPick: (att: Attachment) => void;
  /** Traductor ya atado al idioma. */
  t: T;
  /** Idioma de quien lee: el tamaño se escribe con su separador decimal. */
  locale: Locale;
}

export function DrivePickerModal({ onClose, onPick, t, locale }: Props): JSX.Element {
  return (
    <SelectorDeFicheros
      onClose={onClose}
      locale={locale}
      onPick={(item: PickerItem) => {
        if (!item.id) return;
        onPick({
          fileId: item.id,
          name: item.name,
          contentType: item.contentType || 'application/octet-stream',
          sizeBytes: item.size ?? 0,
        });
      }}
      textos={{
        pickTitle: t('drive.pickTitle'),
        cancel: t('action.cancel'),
        path: t('drive.path'),
        myDrive: t('drive.myDrive'),
        loading: t('drive.loading'),
        loadMore: t('action.loadMore'),
        emptyFolder: t('drive.emptyFolder'),
        byReference: t('drive.byReference'),
        loadFailed: t('drive.loadFailed'),
        loadMoreFailed: t('drive.loadMoreFailed'),
        retry: t('action.retry'),
        sessionExpired: t('session.expired'),
        sessionExpiredHelp: t('session.expiredHelp'),
        signInAgain: t('session.signInAgain'),
        checkConnection: t('error.checkConnection'),
      }}
    />
  );
}
