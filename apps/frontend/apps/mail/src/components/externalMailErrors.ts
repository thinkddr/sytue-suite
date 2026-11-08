/* SPDX-License-Identifier: Apache-2.0 */
import { HttpError } from '@sytue/api-clients';

export type ExternalMailErrorKey =
  | 'externalMail.error'
  | 'externalMail.tlsError'
  | 'externalMail.authError'
  | 'externalMail.icloudAuthError'
  | 'externalMail.sendUnknown'
  | 'externalMail.blockedError'
  | 'externalMail.tooLarge'
  | 'externalMail.cancelled'
  | 'externalMail.scopeMissing'
  | 'externalMail.timeout'
  | 'externalMail.exists';

/**
 * The message for a failed external mailbox call, by the API's stable `code`.
 * iCloud refuses the Apple Account password: its failed sign-in says to use an
 * app-specific password instead of «check the username and app password».
 */
export function errorKey(error: unknown, icloud = false): ExternalMailErrorKey {
  const code = error instanceof HttpError ? error.code : '';
  if (code === 'mail_tls') return 'externalMail.tlsError';
  if (code === 'mail_auth' && icloud) return 'externalMail.icloudAuthError';
  if (code === 'mail_auth' || code === 'external_reconnect_required')
    return 'externalMail.authError';
  if (code === 'mail_send_unknown') return 'externalMail.sendUnknown';
  if (code === 'mail_blocked') return 'externalMail.blockedError';
  if (code === 'mail_too_large') return 'externalMail.tooLarge';
  if (code === 'mail_account_exists') return 'externalMail.exists';
  return 'externalMail.error';
}

/**
 * The message for `oauth_error` on the return from the provider. Cancelling,
 * unticking the mail permission and letting the sign-in expire are different
 * things to fix; all of them read «check the password» before.
 *
 * `state` is not only an expired sign-in: it is also one replaced by a newer
 * «Add» of the same provider (another tab, or the desktop app) or returned to
 * another app. `externalMail.timeout` says only that it no longer counts.
 */
export function returnErrorKey(code: string): ExternalMailErrorKey {
  switch (code) {
    case 'denied':
      return 'externalMail.cancelled';
    case 'scope':
      return 'externalMail.scopeMissing';
    case 'state':
      return 'externalMail.timeout';
    case 'exists':
      return 'externalMail.exists';
    default:
      return 'externalMail.authError';
  }
}
