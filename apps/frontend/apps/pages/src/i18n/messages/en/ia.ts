/**
 * English messages for artificial intelligence (ia).
 * Prefix: ai.*
 */
export const ia = {
  // Settings
  'ai.settings.provider': 'AI Provider',
  'ai.settings.model': 'Model',
  'ai.settings.key': 'Own API Key (BYOK)',
  'ai.settings.key_placeholder': 'Enter your secret API key…',
  'ai.settings.key_help':
    'Your key is stored encrypted with AES-256-GCM. It is never returned to the browser.',
  'ai.settings.check_key': 'Test key',
  'ai.settings.delete_key': 'Delete key',
  'ai.settings.status_configured': 'Key configured',
  'ai.settings.tested_at': 'Verified on',
  'ai.settings.quota_title': 'Monthly AI Quota',
  'ai.settings.quota_reset': 'Resets on {date}',
  'ai.settings.notice_eu_title': 'Non-EEA Data Transfer Notice',
  'ai.settings.notice_eu_desc':
    'The selected provider ({provider}) processes data outside the European Economic Area. Under GDPR art. 49.1.a, you must explicitly consent to this transfer to proceed.',
  'ai.settings.notice_eu_accept':
    'I consent to the international transfer of data outside the EEA for AI requests with this provider.',
  'ai.settings.success_save': 'Key successfully saved and verified.',
  'ai.settings.error_check': 'Error verifying key with provider.',
  'ai.settings.form_title': 'Set up your own key (BYOK)',
  'ai.settings.form_help':
    'Use your own Mistral AI, OpenAI, Anthropic or Zhipu AI credentials so you do not depend on the monthly quota.',
  'ai.settings.provider_mistral': 'Mistral AI (EU, GDPR)',
  'ai.settings.provider_openai': 'OpenAI (US)',
  'ai.settings.provider_anthropic': 'Anthropic (US)',
  'ai.settings.provider_zai': 'Zhipu AI (GLM)',
  'ai.region.eu_fr': 'EU, France',
  'ai.region.us': 'US',
  'ai.region.intl': 'International',
  'ai.settings.show_key': 'Show key',
  'ai.settings.hide_key': 'Hide key',
  'ai.settings.key_required': 'Enter a key.',
  'ai.settings.notice_required': 'You must accept the notice about transfers outside the EEA.',
  'ai.settings.check_ok': 'Key successfully checked with the provider.',
  'ai.settings.error_save': 'The key could not be saved.',
  'ai.settings.error_delete': 'The key could not be deleted.',
  'ai.settings.checking': 'Checking…',
  'ai.settings.save_activate': 'Save and activate',
  'ai.settings.key_deleted': 'Key deleted. The default monthly quota will apply.',
  'ai.settings.active_provider': 'Active provider: {provider} ({model})',
  'ai.settings.no_key_default': 'No key of your own set. The monthly limit of the plan applies.',
  'ai.settings.status_plan': 'Plan quota',
  'ai.settings.quota_unlimited': 'Unlimited',
  'ai.settings.quota_unlimited_help': 'Requests handled with your own API key.',
  'ai.settings.quota_jobs': 'Design, translation and export jobs.',
  'ai.settings.loading': 'Loading AI settings',
  'ai.settings.load_error': 'AI settings could not be loaded.',
  'ai.settings.load_error_help':
    'The service could not be reached. Your settings have not changed.',
  'ai.settings.key_invalid': 'The provider rejected the key. Check that it is correct and active.',
  'ai.settings.encryption_unavailable':
    'The service is not ready to store keys securely yet. Contact support.',
  'ai.settings.invalid_model': 'Choose a valid model for this provider.',
  'ai.settings.key_state': 'Key status',

  'ai.common.cancel': 'Cancel',

  // Shared context: quota, GDPR notice, errors and job state
  'ai.context.quota_label': 'Monthly AI quota',
  'ai.context.quota_used': '{used} of {limit} jobs this month',
  'ai.context.quota_reset': 'Renews on {date}.',
  'ai.context.quota_exhausted_title': 'You have used up the monthly AI quota',
  'ai.context.quota_exhausted_desc':
    'You can launch more jobs on {date}. An administrator can lift the limit by adding an API key of their own in the AI settings.',
  'ai.context.byok': 'Your organization uses its own API key: this job does not use quota.',
  'ai.context.loading': 'Checking the AI quota',
  'ai.context.load_error': 'The AI quota could not be checked. You can still try.',
  'ai.context.retry': 'Retry',
  'ai.notice.eu': 'Texts are processed inside the EU, in line with the GDPR.',
  'ai.notice.non_eu_title': 'This job leaves the European Economic Area',
  'ai.notice.non_eu_desc':
    'Your organization’s own key belongs to {provider}, which processes data outside the EEA. Your organization accepted that transfer (GDPR art. 49(1)(a)) when saving the key.',
  'ai.error.quota': 'The monthly AI quota is used up. It renews on {date}.',
  'ai.error.quota_nodate': 'The monthly AI quota is used up.',
  'ai.error.busy': 'AI jobs are already running. Wait for them to finish and try again.',
  'ai.error.admin': 'Only organization administrators can do this.',
  'ai.error.prompt': 'Describe your site before generating the proposal.',
  'ai.error.languages': 'Choose at least one target language.',
  'ai.error.session': 'Your session has expired. Reload the page and sign in again.',
  'ai.error.generic': 'The job could not be started. Try again in a few minutes.',
  'ai.error.draft_not_saved':
    'The draft could not be saved. Resolve the save problem and start AI again.',
  'ai.job.state_queued': 'Queued',
  'ai.job.state_running': 'Running',
  'ai.job.state_done': 'Done',
  'ai.job.state_failed': 'Failed',
  'ai.job.queued_text': 'Waiting for the server to accept the job…',
  'ai.job.running_text': 'The AI is working. This can take a few minutes.',
  'ai.job.done_text': 'The job has finished.',
  'ai.job.failed_text': 'The job could not be completed.',
  'ai.job.lost_text': 'The connection to the server was lost.',
  'ai.job.elapsed': 'Elapsed time: {time}',
  'ai.job.detail': 'Technical detail: {detail}',
  'ai.job.model': 'Model: {model}',
  'ai.job.badge_eu': 'Inside the EU',
  'ai.job.badge_non_eu': 'Outside the EEA',
  'ai.job.badge_byok': 'Own key',
  'ai.job.back': 'Back',
  'ai.job.retry': 'Retry',
  'ai.job.stop': 'Stop waiting',
  'ai.job.stop_help': 'The job will keep running on the server and will count toward your quota.',

  // Design
  'ai.design.title': 'Design with AI',
  'ai.design.desc':
    'Describe your business and the AI will propose the page sections, with texts ready to edit.',
  'ai.design.prompt_label': 'Describe your site or business',
  'ai.design.prompt_placeholder':
    'E.g. Sustainable women fashion store with catalog and appointments…',
  'ai.design.prompt_help':
    'The more specific you are (what you offer, to whom, and what visitors should do), the better the proposal.',
  'ai.design.ideas_label': 'Ideas to get started',
  'ai.design.idea_1': 'Gourmet shop for artisan cheeses and wines with online orders',
  'ai.design.idea_2': 'Architecture and renovation studio with a project gallery',
  'ai.design.idea_3': 'Dental clinic with appointments, medical team and FAQ',
  'ai.design.idea_4': 'Italian restaurant with tasting menu and table booking',
  'ai.design.style_label': 'Visual style',
  'ai.design.style_modern': 'Modern & clean',
  'ai.design.style_elegant': 'Elegant & refined',
  'ai.design.style_bold': 'Bold & vibrant',
  'ai.design.style_minimal': 'Minimalist',
  'ai.design.lang_label': 'Content language',
  'ai.design.button_generate': 'Generate proposal',
  'ai.design.job_title': 'Designing the proposal',
  'ai.design.result_title': 'Proposal ready',
  'ai.design.result_sections': {
    one: '{count} section ready to add to the canvas.',
    other: '{count} sections ready to add to the canvas.',
  },
  'ai.design.result_help':
    'They will be added to the page you are editing and you can adjust them afterwards.',
  'ai.design.result_empty': 'The AI did not return any usable section.',
  'ai.design.button_apply': 'Add to canvas',
  'ai.design.button_again': 'Try another idea',

  // Translate
  'ai.translate.title': 'Translate with AI',
  'ai.translate.desc': 'Translate the site texts into other languages without touching the design.',
  'ai.translate.from_label': 'Source language',
  'ai.translate.to_label': 'Target languages',
  'ai.translate.to_help':
    'Choose at least one. The source language cannot be a target. Applying the result adds new languages to the site.',
  'ai.translate.scope_label': 'Translation scope',
  'ai.translate.scope_site': 'Entire site',
  'ai.translate.scope_page': 'Current page',
  'ai.translate.scope_store': 'Store catalog only',
  'ai.translate.store_unavailable': 'Store catalogue translation is not available yet.',
  'ai.translate.scope_invalid': 'Choose an existing page or the whole site to translate.',
  'ai.translate.overwrite_label': 'Overwrite translations that already exist',
  'ai.translate.button_translate': 'Translate',
  'ai.translate.job_title': 'Translating the texts',
  'ai.translate.result_title': 'Translations ready',
  'ai.translate.result_texts': {
    one: '{count} text translated.',
    other: '{count} texts translated.',
  },
  'ai.translate.result_help': 'They will be added to the draft; the design does not change.',
  'ai.translate.result_empty': 'The AI did not return any translation.',
  'ai.translate.button_apply': 'Apply translations',
  'ai.translate.button_again': 'Translate into other languages',

  // Export
  'ai.export.title': 'Export with AI',
  'ai.export.desc':
    'Generate a package with your site ready to take to another platform, without depending on Sytue.',
  'ai.export.destination_label': 'Destination platform',
  'ai.export.dest_shopify': 'Shopify Online Store 2.0',
  'ai.export.dest_shopify_desc':
    'Complete theme with JSON templates and Liquid sections, plus the catalog as a CSV to import from the Shopify admin.',
  'ai.export.dest_react': 'React + Tailwind CSS',
  'ai.export.dest_react_desc':
    'Single-page project connected to the public Sytue API, with checkout and forms.',
  'ai.export.dest_nextjs': 'Next.js App Router',
  'ai.export.dest_nextjs_desc':
    'Server-rendered or static application with the payment gateway built in.',
  'ai.export.dest_woocommerce': 'WooCommerce / WordPress',
  'ai.export.dest_woocommerce_desc':
    'Native WordPress block theme, with product and attribute export.',
  'ai.export.badge_available': 'Available',
  'ai.export.badge_soon': 'Coming soon',
  'ai.export.button_export': 'Generate package',
  'ai.export.job_title': 'Generating the package for {destination}',
  'ai.export.result_title': 'Package ready',
  'ai.export.result_help': 'The theme and the catalog CSV come together in a single ZIP.',
  'ai.export.steps_title': 'How to install it in Shopify',
  'ai.export.step_1': 'Download the ZIP file.',
  'ai.export.step_2':
    'In the Shopify admin, go to Online Store > Themes and choose “Upload zip file”.',
  'ai.export.step_3':
    'For products, go to Products > Import and upload the products_export_shopify.csv file inside the ZIP.',
  'ai.export.button_download': 'Download ZIP',
  'ai.export.button_again': 'Generate another export',
};
