resource "google_secret_manager_secret" "app_secret" {
  secret_id = "url-shortener-secret"

  replication {
    auto {}
  }

  labels = {
    environment = var.environment
    owner       = var.owner
    managed_by  = "terraform"
  }
}

resource "google_secret_manager_secret_iam_member" "secret_accessor" {
  secret_id = google_secret_manager_secret.app_secret.id
  role      = "roles/secretmanager.secretAccessor"
  member    = "serviceAccount:${google_service_account.app_sa.email}"
}
