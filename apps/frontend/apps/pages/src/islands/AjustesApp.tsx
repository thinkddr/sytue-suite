import type { JSX } from 'preact';
import type { Locale } from '@sytue/i18n';
import type { SytueUser } from '@sytue/auth';
import { Marco } from '../components/Marco.js';
import { AjustesIA } from '../ia/AjustesIA.js';
import { administra } from '../lib/roles.js';
import { t } from '../i18n/index.js';

interface Props {
  locale: Locale;
  user: SytueUser;
  hubBaseUrl: string;
}

export function AjustesApp({ locale, user, hubBaseUrl }: Props): JSX.Element {
  return (
    <Marco user={user} hubBaseUrl={hubBaseUrl} locale={locale} activa="settings">
      <div class="mx-auto max-w-4xl p-4 sm:p-6">
        <h1 class="sytue-h2 text-text mb-6">{t(locale, 'app_settings.title')}</h1>
        <AjustesIA locale={locale} administra={administra(user)} />
      </div>
    </Marco>
  );
}

export default AjustesApp;
