#!/usr/bin/env bash
set -e

# Path to active secrets environment file
ENV_FILE="secrets/.env"

if [ ! -f "$ENV_FILE" ]; then
  echo "Error: $ENV_FILE not found. Please create it under the secrets/ directory first."
  exit 1
fi

# Load environment variables, ignoring comments and blank lines
export $(grep -v '^#' "$ENV_FILE" | xargs)

# Map env variables to TF_VAR_ format required by Terraform
export TF_VAR_project_id="$GCP_PROJECT_ID"
export TF_VAR_region="$GCP_REGION"
export TF_VAR_zone="$GCP_ZONE"
export TF_VAR_cluster_name="$GKE_CLUSTER_NAME"
export TF_VAR_owner="${OWNER:-devops-user}"
export TF_VAR_environment="${ENVIRONMENT:-staging}"

# Change directory to the Terraform infra path
cd infra/terraform

# Initialize Terraform with dynamic GCS state backend bucket config
terraform init -backend-config="bucket=$GCS_BUCKET_NAME"

# Run terraform destroy with auto-approval and forward any additional parameters
terraform destroy -auto-approve "$@"
