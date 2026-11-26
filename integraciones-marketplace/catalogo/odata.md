<a id="odata-genérico--alta-en-sytue-platform"></a>
# Generic OData — onboarding in Sytue Platform
**What it connects**: any OData v4 endpoint (in-house ERP/CRM, Dynamics on-prem, SAP gateways…).
1. You need the service's **service_url**: it ends in `/odata` or `/$metadata` and returns 200 in an authenticated browser.
2. Sytue Platform → Integrations → **OData**: paste the service_url.
3. Authentication depends on what the service exposes: a pasted token (Bearer) or the service's own credentials — the form asks for it.
**Verify**: the panel lists the EntitySets from `$metadata`.
**Errors**: `406` = the service does not negotiate JSON (`application/json;odata.metadata=minimal`): check that it is v4.
