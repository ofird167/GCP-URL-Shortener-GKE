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

# Set deployment environment (staging or production, defaults to staging)
ENV="${1:-staging}"
if [ "$ENV" != "staging" ] && [ "$ENV" != "production" ]; then
  echo "Error: Invalid environment '$ENV'. Must be 'staging' or 'production'."
  exit 1
fi

echo "[INFO] Connecting to GKE cluster: $GKE_CLUSTER_NAME..."
gcloud container clusters get-credentials "$GKE_CLUSTER_NAME" --zone "$GCP_ZONE" --project "$GCP_PROJECT_ID"

echo "[INFO] Authenticating Docker daemon to Artifact Registry..."
gcloud auth configure-docker "${GCP_REGION}-docker.pkg.dev" --quiet

# Define target image path (using the lowercase repo name)
IMAGE_PATH="${GCP_REGION}-docker.pkg.dev/${GCP_PROJECT_ID}/gcp-url-shortener-gke-repo/url-shortener:latest"

echo "[INFO] Building Docker image..."
docker build -t "$IMAGE_PATH" ./app

echo "[INFO] Pushing Docker image to Artifact Registry..."
docker push "$IMAGE_PATH"

echo "[INFO] Deploying application to namespace '$ENV' using Helm..."
helm upgrade --install url-shortener ./helm/url-shortener \
  --namespace "$ENV" \
  --create-namespace \
  --values "./helm/url-shortener/values-${ENV}.yaml" \
  --set config.gcpProjectId="$GCP_PROJECT_ID" \
  --set image.registry="${GCP_REGION}-docker.pkg.dev/${GCP_PROJECT_ID}/gcp-url-shortener-gke-repo" \
  --set image.pullPolicy="Always"

echo "[INFO] Triggering GKE deployment rollout restart..."
kubectl rollout restart deployment/url-shortener -n "$ENV"

echo "[INFO] Waiting for deployment rollout to complete..."
kubectl rollout status deployment/url-shortener -n "$ENV" --timeout=120s

if [ "$ENV" == "staging" ]; then
  echo ""
  echo "======================================================================"
  echo "  DEPLOYMENT SUCCESSFUL (Staging)"
  echo "======================================================================"
  echo "Opening secure port-forward tunnel to staging service..."
  echo "You can access the landing page at: http://127.0.0.1:8080/"
  echo "Press Ctrl+C to stop port-forwarding and close the tunnel."
  echo "======================================================================"
  echo ""
  kubectl port-forward service/url-shortener 8080:80 -n staging
else
  echo ""
  echo "[INFO] Waiting for external LoadBalancer IP allocation..."
  EXTERNAL_IP=""
  for i in {1..30}; do
    EXTERNAL_IP=$(kubectl get svc url-shortener -n production -o jsonpath='{.status.loadBalancer.ingress[0].ip}' 2>/dev/null || echo "")
    if [ -n "$EXTERNAL_IP" ] && [ "$EXTERNAL_IP" != "<pending>" ]; then
      break
    fi
    sleep 5
  done

  if [ -z "$EXTERNAL_IP" ] || [ "$EXTERNAL_IP" == "<pending>" ]; then
    echo "======================================================================"
    echo "  DEPLOYMENT SUCCESSFUL (Production)"
    echo "======================================================================"
    echo "The external LoadBalancer IP is still provisioning."
    echo "You can check the status later by running:"
    echo "  kubectl get svc url-shortener -n production"
    echo "======================================================================"
  else
    echo "======================================================================"
    echo "  DEPLOYMENT SUCCESSFUL (Production)"
    echo "======================================================================"
    echo "Your landing page is live! Access it here:"
    echo "  http://${EXTERNAL_IP}/"
    echo "======================================================================"
  fi
fi
