<a id="gitlab-saas--saml-sso-por-grupo-plan-premium"></a>
# GitLab (SaaS) — SAML SSO per GROUP (Premium+ plan)
1. Your group on gitlab.com → **Settings → SAML SSO for GitLab.com Groups**.
2. Paste the Sytue ID metadata into “IdP URL”/certificate; **Name ID** `email`.
3. Copy the group's **GitLab group SAML URL** (ACS) and **Identifier** (Entity ID) to the Sytue Platform integration page.
4. “Test SAML” and assign default memberships.
**Errors**: `audience mismatch` = the Identifier does not exactly match the issued Entity ID.
