# AWS IAM Identity Center — SAML + SCIM
1. AWS Console → **IAM Identity Center → Settings → Add identity source → External IdP**... or, more simply: keep AWS as the IdP towards Sytue.
2. Copy to the Sytue Platform detail card: your **region** (e.g. `eu-south-2`), the **saml_id** (the `samlprovider/…` of the provider you created) and the **scim_endpoint** that AWS generates.
3. Create the **SCIM token** in AWS and paste it into the detail card: user/group provisioning goes through it.
4. Assign at least one permission set to the synced users.
**Verify**: in AWS, the “Automated provisioning” tab turns green and the users appear.
