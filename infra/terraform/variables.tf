variable "project_id" {
  type        = string
  description = "The GCP Project ID where resources will be created."
}

variable "region" {
  type        = string
  description = "The GCP region for provisioning resources."
}

variable "zone" {
  type        = string
  description = "The GCP zone for the GKE cluster node pool."
}

variable "cluster_name" {
  type        = string
  description = "The name of the GKE cluster."
}

variable "owner" {
  type        = string
  description = "The owner tag for the resources."
}

variable "environment" {
  type        = string
  description = "The environment tag (e.g., staging, production)."
}
