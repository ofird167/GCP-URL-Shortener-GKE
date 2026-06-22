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

### 3. CI/CD Staged Pipeline Triggers
* **Staging deployment**: Automatically triggered on any push or merge to the `main` or `develop` branches.
* **Production deployment**: Automatically triggered by creating and pushing a Git release tag matching `v*` (e.g. `v1.0.4`):
  ```bash
  git tag v1.0.4
  git push origin v1.0.4
  ```

---

### 4. GKE CPU & Pod Resource Optimization
To prevent scheduling failures (`Insufficient cpu`) on the single shared-core `e2-medium` node, CPU requests are kept minimal:
* Pod requests are configured to **5m CPU** and **16Mi Memory** for both staging and production.
* This allows staging (1 replica) and production (3 replicas) to run alongside GKE system agents successfully.

---

### 5. Access and Test Endpoints

#### Staging (Internal ClusterIP)
Establish a secure port-forward tunnel to test staging:
```bash
kubectl port-forward service/url-shortener 8080:80 -n staging
```
Then query the staging service locally:
```bash
curl -i -X POST -H "Content-Type: application/json" -d '{"url":"https://github.com/ofird167"}' http://127.0.0.1:8080/shorten
```

#### Production (Public LoadBalancer)
Retrieve the external public IP of the production LoadBalancer:
```bash
kubectl get svc -n production
```
Query the production service directly using the public IP (e.g., `34.31.66.162`):
```bash
# Health check
curl -i http://34.31.66.162/healthz

# Shorten URL
curl -i -X POST -H "Content-Type: application/json" -d '{"url":"https://github.com/ofird167"}' http://34.31.66.162/shorten
```

---

### 6. Teardown & Clean Up
To destroy all GKE workloads, GCP Load Balancers, and networks safely to avoid unexpected GCP billing charges:
```bash
./destroy.sh
```
