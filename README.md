# Go URL Shortener Service

> ⚡ **Looking for the Quickstart?** Jump straight to the [DevOps Quickstart Guide (TL;DR)](readmeTL;DR.md).

An end-to-end cloud-native URL shortener application built with Go, deployed via Helm to Google Kubernetes Engine (GKE), secured using GCP Secret Manager (relying on Workload Identity), and automated via GitHub Actions.

---

## 1. Local Development & Setup

### Prerequisites
- Go 1.25+
- Docker
- Gcloud CLI (authenticated)
- Terraform 1.0.0+
- Helm v3+

### Configuration
We use a git-ignored `/secrets/.env` file for local credentials and environment-specific parameters.
1. Create the `secrets/` directory and populate `.env` using the template `example.env` in the root:
   ```bash
   mkdir -p secrets
   cp example.env secrets/.env
   ```
2. Open `secrets/.env` and populate it with your environment values:
   - `GCP_PROJECT_ID`: The ID of your Google Cloud Project.
   - `GCS_BUCKET_NAME`: The name of the GCS bucket for remote Terraform state (e.g. `devops-project-tfstate-bucket`).
   - `GKE_CLUSTER_NAME`: Desired GKE cluster name.

### Running Locally
Run the service locally with debug logging (defaulting to in-memory mode if GCP Secret Manager is not reachable):
```bash
# Load env variables and start the application
export $(grep -v '^#' secrets/.env | xargs)
cd app/
go run main.go
```
The app will bind to `127.0.0.1:8080`.
- **Health check**: `curl http://127.0.0.1:8080/healthz`
- **Shorten URL**:
  ```bash
  curl -X POST http://127.0.0.1:8080/shorten \
    -H "Content-Type: application/json" \
    -d '{"url": "https://google.com"}'
  ```
- **Metrics**: `curl http://127.0.0.1:8080/metrics`

---

## 2. Infrastructure Automation (Terraform)

Terraform configurations are located in `/infra/terraform/`. We supply helper scripts in the root directory to run deployments safely using variables mapped from `/secrets/.env`.

### Provision Infrastructure
To initialize and provision the GKE cluster, VPC networks, Artifact Registry, and Secrets:
```bash
./apply.sh
```

### Connect to GKE Cluster
Upon successful completion, Terraform outputs the connection command. Execute it to update your local kubeconfig:
```bash
gcloud container clusters get-credentials <cluster_name> --zone <zone> --project <project_id>
```

### Destroy Infrastructure
To teardown all provisioned resources:
```bash
./destroy.sh
```

### Build & Deploy Application (Manual)
We supply a helper script to authenticate Docker, build the application container, push it to your private Google Artifact Registry, and deploy the workloads via Helm:
```bash
# Build & Deploy to Staging (Default)
./deploy.sh staging

# Build & Deploy to Production
./deploy.sh production
```

---

## 3. Git Branching & Release Strategy

### CI/CD Trigger Mappings
Our GitHub Actions pipeline matches your Git activity to environments automatically:
* **Staging Deployment**: Triggered automatically on push or merge to the `main` or `develop` branches. It deploys a single replica into the GKE `staging` namespace.
* **Production Deployment**: Triggered automatically when a release tag matching `v*` (e.g. `v1.0.4`) is pushed. It builds the release image, runs all security scans, and deploys **3 replicas** into the GKE `production` namespace under a public LoadBalancer.

### Branching Model
- **`develop`**: The primary integration branch. Pull requests merged here are automatically built and deployed to the **Staging** environment.
- **`main`**: The stable production branch. Code is merged here from `develop` via pull requests.
- **Feature Branches (`feature/*`)**: Created for individual features/bugfixes. Must be branched from and merged back into `develop`.

### Branch Protection Rules
Enforce the following rules on `main` and `develop` in GitHub:
1. **Require status checks to pass**: The `Lint and Test` and `Static Security Scan` CI jobs must pass before merging.
2. **Require pull request reviews**: At least 1 approving review is required before merging.
3. **No direct pushes**: All changes must flow through pull requests.

### Tagging & Production Releases
We use semantic versioning (e.g., `v1.0.4`) to tag stable milestones.
1. Create a release branch or merge `develop` into `main`.
2. Push a Git tag to trigger the production CD pipeline:
   ```bash
   git tag v1.0.4
   git push origin v1.0.4
   ```
This automatically builds the image and deploys it to the **Production** namespace on GKE.

---

## 4. GKE Node & Resource Optimizations

### Single Node CPU/Memory Constraints
Because standard single-node GKE cluster node capacity is shared with GCP system services, resource allocation is strictly managed:
* **Staging Resource Limits**: Reduced to `5m` CPU requests and `16Mi` memory requests.
* **Production Resource Limits**: Reduced to `5m` CPU requests and `16Mi` memory requests per replica.
* **Result**: This allows 1 staging replica and 3 production replicas to run concurrently without encountering scheduling blockages due to `Insufficient cpu`.

---

## 5. Secrets Management & Rotation

We use **GCP Secret Manager** rather than baking secrets into environment variables in GKE manifests.
- **Workload Identity**: Kubernetes pods authenticate dynamically to GCP using GCP Workload Identity. The Kubernetes service account `url-shortener-sa` is bound to the GCP Service Account `url-shortener-app-sa@<project_id>.iam.gserviceaccount.com`.
- **API Fetch**: The Go app loads the GCP Secret Manager client SDK, queries `projects/<project_id>/secrets/url-shortener-secret/versions/latest`, and loads the credentials into memory.

### Secret Rotation Process
To rotate database passwords or API keys:
1. Access the Google Cloud Console or use `gcloud` to add a new version to the secret:
   ```bash
   echo -n "new-secret-value" | gcloud secrets versions add url-shortener-secret --data-file=-
   ```
2. **Zero-Downtime Update**: Because our application fetches `versions/latest` dynamically on startup (or can be configured to fetch periodically), a secret rotation does not require rebuilding or redeploying the Docker image. Pods can simply be restarted to pull the new value immediately:
   ```bash
   kubectl rollout restart deployment/url-shortener -n staging
   ```

---

## 6. Reliability, Rollbacks, and Safety

### Deployment Strategy
We implement **Rolling Updates** to ensure zero-downtime releases:
- `maxSurge: 1`: One additional pod is created during the rollout before destroying old pods.
- `maxUnavailable: 0`: No active pods are removed until the new pods are fully ready (evaluated via liveness and readiness probes).

### Rollback Conditions
A deployment should be automatically or manually rolled back if:
1. **Readiness Probe Failures**: New pods fail to transition to a `Ready` state within the timeout.
2. **High HTTP Error Rates**: HTTP 5xx errors spike above 1% of total requests on `/metrics`.
3. **CrashLoopBackOff**: Application containers fail to start or crash repeatedly.

### Simulation of Failed Deployment
1. To simulate a failed deployment, push an invalid configuration (e.g. referencing a non-existent container tag or invalid environment parameter):
   ```bash
   helm upgrade url-shortener ./helm/url-shortener -n staging --set image.tag=invalid-tag
   ```
2. GKE will spin up a new pod, but the container registry lookup will fail, keeping the old staging pods running. The deployment status will block.
3. Trigger a rollback to restore the last stable state:
   ```bash
   # View release history
   helm history url-shortener -n staging
   # Rollback to the previous revision (e.g. revision 1)
   helm rollback url-shortener 1 -n staging
   ```

---

## 7. Operations Runbook

### Incident A: The CI/CD Pipeline Fails on the CI Stage
If a pull request build fails during CI:
1. **Check Lint Failures**: Review the GitHub Actions logs under the `Lint and Test` step. Format issues can be resolved locally by running `gofmt -w app/`.
2. **Check Test Failures**: Ensure all tests in `app/main_test.go` pass. Run `go test -v ./...` locally.
3. **Check Test Coverage**: The quality gate enforces a minimum of **70% test coverage**. If the build fails because of coverage, write unit tests for any new code paths.
4. **Check Security Scans (Trivy)**: If Trivy finds `HIGH` or `CRITICAL` vulnerabilities in Go modules or the Docker base image:
   - Run `go list -m -u all` to check for updates, and update vulnerable libraries in `go.mod`.
   - Update the base builder and runtime images in `app/Dockerfile` to pull the latest security patches.

### Incident B: A Release Fails or Service Becomes Unhealthy in Production
If a production deployment triggers alerts or becomes unresponsive:
1. **Find Unhealthy Pods**:
   ```bash
   kubectl get pods -n production
   ```
2. **Fetch Logs for the Failing Pods**:
   ```bash
   kubectl logs -l app.kubernetes.io/name=url-shortener -n production --tail=100
   ```
3. **Verify GCP Secret Manager Access**: If pods log `Failed to access Secret Manager`, verify the Workload Identity binding on the GKE service account:
   ```bash
   kubectl describe serviceaccount url-shortener-sa -n production
   ```
4. **Trigger Immediate Rollback**:
   ```bash
   helm rollback url-shortener -n production
   ```
This rolls the production deployment back to the previous stable release instantly.
