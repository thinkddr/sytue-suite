/* SPDX-License-Identifier: Apache-2.0 */
import { I18nProvider } from '@sytue/i18n/preact';
import { LinkedServices } from '@sytue/shell';
export function LinkedServicesDemo({ service }: { service: 'calendar' | 'contacts' | 'notes' }) {
  return (
    <I18nProvider locale="en">
      <main class="mx-auto max-w-xl p-6">
        <h1 class="sytue-h1 mb-6">Sytue</h1>
        <LinkedServices
          basePath="/calendar"
          service={service}
          labels={{
            create: 'Create fixture',
            title: 'Title',
            text: 'Text',
            start: 'Starts',
            end: 'Ends',
            allDay: 'All day',
            location: 'Location',
            organization: 'Organisation',
            email: 'Email',
            phone: 'Phone',
            add: 'Add',
            previousMonth: 'Previous month',
            nextMonth: 'Next month',
          }}
        />
      </main>
    </I18nProvider>
  );
}
