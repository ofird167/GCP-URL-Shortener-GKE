output "gke_connection_command" {
  value       = "gcloud container clusters get-credentials ${google_container_cluster.primary.name} --zone ${google_container_cluster.primary.location} --project ${var.project_id}"
  description = "The command to update local kubeconfig and connect to the cluster."
}

output "gke_cluster_endpoint" {
  value       = google_container_cluster.primary.endpoint
  description = "The endpoint URL of the GKE cluster."
}

output "artifact_registry_url" {
  value       = "${google_artifact_registry_repository.repo.location}-docker.pkg.dev/${var.project_id}/${google_artifact_registry_repository.repo.repository_id}"
  description = "The URL of the Artifact Registry repository."
}

output "secret_manager_secret_id" {
  value       = google_secret_manager_secret.app_secret.id
  description = "The resource ID of the Secret Manager secret."
}

output "app_service_account_email" {
  value       = google_service_account.app_sa.email
  description = "The email address of the IAM Service Account used by the app."
}
