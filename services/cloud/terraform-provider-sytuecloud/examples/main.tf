# SPDX-License-Identifier: Apache-2.0
# Example: manage a Sytue Cloud project end to end.
#
#   export SYTUE_CLOUD_TOKEN=scl_...            # API token (scope write/deploy)
#   export SYTUE_CLOUD_API=https://cloud-api.sytue.eu   # optional (this is the default)
#   terraform init && terraform apply

terraform {
  required_providers {
    sytuecloud = {
      source = "sytue/sytuecloud"
    }
  }
}

provider "sytuecloud" {
  # endpoint = "https://cloud-api.sytue.eu"  # or $SYTUE_CLOUD_API
  # token    = "scl_..."                     # or $SYTUE_CLOUD_TOKEN (keep it out of VCS)
}

resource "sytuecloud_project" "blog" {
  slug           = "blog"
  name           = "Company Blog"
  repo_url       = "https://github.com/acme/blog"
  git_provider   = "github"
  production_ref = "main"
  framework      = "astro"
  build_strategy = "" # auto
}

resource "sytuecloud_env_var" "api_token" {
  project_id = sytuecloud_project.blog.id
  key        = "API_TOKEN"
  value      = "s3cr3t" # marked Sensitive; source it from a var/vault in real use
  is_secret  = true
  scope      = "all"
}

resource "sytuecloud_domain" "primary" {
  project_id = sytuecloud_project.blog.id
  hostname   = "blog.acme.com"
}

output "domain_dns_txt_verification_token" {
  value = sytuecloud_domain.primary.verify_token
}

resource "sytuecloud_managed_service" "db" {
  project_id = sytuecloud_project.blog.id
  kind       = "postgres"
  name       = "blog-db"
  config = {
    version = "16"
  }
}

# Look up an existing project by name (or id).
data "sytuecloud_project" "existing" {
  name = "Company Blog"
}
