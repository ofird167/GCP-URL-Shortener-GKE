terraform {
  backend "gcs" {
    # The bucket name is injected dynamically via -backend-config="bucket=..." during terraform init
    prefix = "terraform/state"
  }
}
