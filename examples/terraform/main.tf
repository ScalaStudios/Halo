terraform {
  required_providers {
    halo = {
      source = "scalastudios/halo"
    }
  }
}

provider "halo" {}

resource "halo_group" "engineering" {
  name        = "Engineering"
  description = "Everyone who builds the product"
  kind        = "assigned"
}

resource "halo_group" "berlin" {
  name = "Berlin office"
  kind = "dynamic"
  rule = "user.location == \"Berlin\""
}

resource "halo_user" "ada" {
  email      = "ada@example.com"
  name       = "Ada Lovelace"
  title      = "Principal engineer"
  department = "Engineering"
  location   = "Berlin"
}

resource "halo_group_member" "ada_engineering" {
  group_id = halo_group.engineering.id
  user_id  = halo_user.ada.id
}

resource "halo_application" "grafana" {
  name          = "Grafana"
  protocol      = "oidc"
  type          = "web"
  homepage      = "https://grafana.example.com"
  redirect_uris = ["https://grafana.example.com/login/generic_oauth"]
  group_ids     = [halo_group.engineering.id]
}

resource "halo_access_policy" "grafana_passkeys" {
  name    = "Passkeys for Grafana"
  enabled = true
  mode    = "report"
  effect  = "require-phishing-resistant"
  conditions = {
    group_ids = [halo_group.engineering.id]
    app_ids   = [halo_application.grafana.id]
  }
}

resource "halo_api_resource" "billing" {
  name             = "Billing API"
  identifier       = "https://billing.example.com"
  access_token_ttl = 900
  scopes = [
    { name = "billing:read", description = "Read invoices" },
    { name = "billing:write", description = "Create and change invoices" },
  ]
}

output "grafana_client_id" {
  value = halo_application.grafana.client_id
}

output "grafana_client_secret" {
  value     = halo_application.grafana.client_secret
  sensitive = true
}
