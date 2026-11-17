<a id="power-bi--alta-en-sytue-platform"></a>
# Power BI — setup in Sytue Platform
**What it connects**: read queries against a published dataset (no user OAuth: a service credential).
1. In app.powerbi.com, find the **dataset_id**: open the semantic model → the GUID in the URL (`/datasets/XXXX-…`).
2. You need an identity with a Pro/Premium license on that dataset and its **api_base** (`https://api.powerbi.com`, or the one for your VNET/SSRS if it is on-prem).
3. Create the service credential (a service principal in Entra + a security group with access to the dataset, or a licensed technical user).
4. Sytue Platform → Integrations → **Power BI**: dataset_id and api_base + credential → “Connect”.
**Verify**: a test DAX query (`EVALUATE ROW("x", 1)`) from the panel.
**Errors**: `DatasetNotFound` = the GUID belongs to the report, not the dataset; 401 = the principal has no access to the workspace.
