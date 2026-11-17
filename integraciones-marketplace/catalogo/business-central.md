# Business Central — setup in Sytue Platform
**What it connects**: financial read/write (Financials.ReadWrite.All) against ONE company.
1. portal.azure.com → Entra → **App registrations → New** (multi-tenant) → Web redirect URI: `https://integrations.sytue.eu/api/v1/oauth/callback`.
2. **API permissions** → **Dynamics 365 Business Central** → delegated `Financials.ReadWrite.All` + `offline_access` → admin consent.
3. Copy the Client ID + secret.
4. In BC: search for **“Microsoft Entra Applications”** → new → add the Client ID with the **D365 BUS FULL ACCESS** permission (or the minimal set you want).
5. Get the **service_url**: in the company, “Help → Debug” or page 810: it ends in `/api/v2.0/companies(<id>)`.
6. Sytue Platform → Integrations → **Business Central**: tenant, client_id, secret and service_url → “Connect”.
**Verify**: company read test on the integration's detail card.
**Errors**: `401` from BC = the registration in “Entra Applications” is missing; 403 = insufficient D365 permission.
