# DevOps Quickstart Guide (TL;DR)

> 📚 **Looking for full details?** View the [Detailed Documentation Guide](README.md).

---

### 1. Setup Environment Variables
Create the `secrets/` directory and configure your `.env` variables:
1. Create a directory named `secrets/` at the root of the project:
   ```bash
   mkdir -p secrets
   ```
2. Copy the template variables file from root to `secrets/.env`:
   ```bash
   cp example.env secrets/.env
   ```
3. Populate all fields in `secrets/.env` (GCP Project ID `devops-project`, GCS state bucket `devops-project-tfstate-bucket`, registry username/password, and owner label).

---

### 2. Provision GCP Infrastructure & GKE
Run the deployment automation wrapper script from the root:
```bash
./apply.sh
```
*(This script loads the variables from secrets/.env, maps them to `TF_VAR_` prefixes, initializes the Terraform GCS remote backend, and applies the configuration to create the VPC network, GKE cluster, Artifact Registry, and Secret Manager).*

After the cluster is provisioned, configure your local `kubectl` to connect using the dynamically output command:
```bash
# Load env variables and execute the GKE connectivity command from Terraform output
export $(grep -v '^#' secrets/.env | xargs)
$(cd infra/terraform && terraform output -raw gke_connection_command)
```

---

### 3. Build & Push Docker Image (GCP Artifact Registry)
Build the image and push it to the private Google Artifact Registry repository:
```bash
# Load env variables
export $(grep -v '^#' secrets/.env | xargs)

# Authenticate Docker daemon to the regional Artifact Registry
gcloud auth configure-docker us-central1-docker.pkg.dev --quiet

# Build and tag
IMAGE_PATH="us-central1-docker.pkg.dev/${GCP_PROJECT_ID}/interview8-repo/url-shortener:latest"
docker build -t "$IMAGE_PATH" ./app

# Push image to GCP Artifact Registry
docker push "$IMAGE_PATH"
```

---

### 4. Deploy Application via Helm
Deploy the Helm release into the GKE cluster:
* **Staging**:
  ```bash
  helm upgrade --install url-shortener ./helm/url-shortener \
    --namespace staging \
    --create-namespace \
    --values ./helm/url-shortener/values-staging.yaml
  ```
* **Production**:
  ```bash
  helm upgrade --install url-shortener ./helm/url-shortener \
    --namespace production \
    --create-namespace \
    --values ./helm/url-shortener/values-production.yaml
  ```

---

### 5. Access and Test Endpoints (Local Tunnel)
If port `8080` is in use by local services, map the tunnel to a free port.

Establish the secure port-forward tunnel to the service:
- **Staging**:
  ```bash
  kubectl port-forward service/url-shortener 8080:80 -n staging
  ```
- **Production** (Alternatively, query the public LoadBalancer IP directly):
  ```bash
  kubectl port-forward service/url-shortener 8080:80 -n production
  ```

In a separate terminal tab/window, query the app endpoints:
* **Health Check**: `curl http://127.0.0.1:8080/healthz`
* **Shorten URL**:
  ```bash
  curl -X POST http://127.0.0.1:8080/shorten \
    -H "Content-Type: application/json" \
    -d '{"url": "https://google.com"}'
  ```
* **Metrics**: `curl http://127.0.0.1:8080/metrics`

---

### 6. Teardown & Clean Up
To destroy all GKE workloads, GCP Load Balancers, and networks safely to avoid unexpected GCP billing charges:
```bash
./destroy.sh
```
*(Note: `./destroy.sh` loads variables, initializes Terraform, and runs `terraform destroy -auto-approve` to tear down all GKE resources, registries, secrets, and VPC networks).*
