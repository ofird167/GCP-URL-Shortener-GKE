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
3. Open `secrets/.env` and populate all fields (GCP Project ID `ofirdevops`, GCS state bucket `ofirdevops-tfstate-bucket`, registry username/password, and owner label).

---

### 2. Provision GCP Infrastructure & GKE
Run the deployment automation wrapper script from the root:
```bash
./apply.sh
```
*(This script loads variables from secrets/.env, initializes Terraform with GCS state backend, and provisions the VPC network, GKE cluster, Artifact Registry, and Secret Manager).*

---

### 3. Build & Deploy Manually
To build the application container, push it to Google Artifact Registry, and deploy it via Helm, run:
* **Staging** (Default):
  ```bash
  ./deploy.sh staging
  ```
* **Production**:
  ```bash
  ./deploy.sh production
  ```
*(This script automatically fetches GKE credentials, logs Docker into Artifact Registry, builds/pushes the image, performs the Helm installation/upgrade, and outputs connection commands).*

---

### 4. CI/CD Staged Pipeline Triggers
* **Staging deployment**: Automatically triggered on any push or merge to the `main` or `develop` branches.
* **Production deployment**: Automatically triggered by creating and pushing a Git release tag matching `v*` (e.g. `v1.0.0`):
  ```bash
  git tag v1.0.0
  git push origin v1.0.0
  ```

---

### 5. GKE CPU & Pod Resource Optimization
To prevent scheduling failures on the single shared-core `e2-medium` GKE node, CPU requests are kept minimal:
* Pod requests are configured to **5m CPU** and **16Mi Memory** for both staging and production.
* This allows staging (1 replica) and production (3 replicas) to run alongside GKE system agents successfully.

---

### 6. Access and Test Endpoints & Landing Page

#### Staging (Internal ClusterIP)
1. Establish a secure port-forward tunnel to test staging:
   ```bash
   kubectl port-forward service/url-shortener 8080:80 -n staging
   ```
2. Open your web browser and navigate to:
   * **`http://127.0.0.1:8080/`** (to interact with the Landing Page UI)
3. You can also query the API directly:
   ```bash
   curl -i -X POST -H "Content-Type: application/json" -d '{"url":"https://github.com/ofird167"}' http://127.0.0.1:8080/shorten
   ```

#### Production (Public LoadBalancer)
1. Retrieve the external public IP of the production LoadBalancer:
   ```bash
   kubectl get svc url-shortener -n production
   ```
2. Navigate directly to the returned `EXTERNAL-IP` in your browser (e.g. `http://34.31.66.162/`).
3. You can also test the endpoints via curl:
   ```bash
   # Health check
   curl -i http://<EXTERNAL-IP>/healthz
   
   # Shorten URL
   curl -i -X POST -H "Content-Type: application/json" -d '{"url":"https://github.com/ofird167"}' http://<EXTERNAL-IP>/shorten
   ```

---

### 7. Teardown & Clean Up
To destroy all GKE workloads, GCP Load Balancers, and networks safely to avoid unexpected GCP billing charges:
```bash
./destroy.sh
```
