/**
 * DrivePickerModal — selector de fichero de Sytue Drive para «Adjuntar desde
 * Drive» del compositor (intent cross-app attach-drive-to-mail), montado
 * sobre SelectorDeFicheros de @sytue/shell.
 */
import { SelectorDeFicheros, type PickerItem } from '@sytue/shell';
import type { Locale } from '@sytue/i18n';
import { useT } from '../i18n/index.js';

interface Props {
  /** Idioma ya negociado en el servidor; llega por props desde la isla raíz. */
  locale: Locale;
  onClose: () => void;
  /** Adjunta el fichero elegido como referencia (nombre + enlace público). */
  onPick: (name: string, link: string) => void;
}

export function DrivePickerModal({ locale, onClose, onPick }: Props) {
  const t = useT(locale);

  return (
    <SelectorDeFicheros
      locale={locale}
      onClose={onClose}
      onPick={(item: PickerItem) => {
        if (!item.link) return;
        onPick(item.name, item.link);
      }}
      textos={{
        pickTitle: t('drive.title'),
        cancel: t('common.cancel'),
        path: t('drive.breadcrumb'),
        myDrive: t('drive.myDrive'),
        loading: t('drive.loadingFiles'),
        loadMore: t('common.loadMore'),
        emptyFolder: t('drive.emptyFolder'),
        byReference: t('drive.footer'),
        loadFailed: t('drive.errorTitle'),
        loadMoreFailed: t('drive.loadMoreFailed'),
        retry: t('common.retry'),
        sessionExpired: t('drive.sessionExpiredTitle'),
        sessionExpiredHelp: t('drive.sessionExpiredDescription'),
        signInAgain: t('drive.signInAgain'),
        checkConnection: t('drive.errorDescription'),
      }}
    />
  );
}
