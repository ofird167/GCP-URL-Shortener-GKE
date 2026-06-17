resource "google_artifact_registry_repository" "repo" {
  location      = var.region
  repository_id = "GCP-URL-Shortener-GKE-repo"
  description   = "Docker registry for URL shortener app"
  format        = "DOCKER"

  labels = {
    environment = var.environment
    owner       = var.owner
    managed_by  = "terraform"
  }
}
