# GitHub Actions Setup Guide

This document outlines the required secrets and variables that need to be configured in your GitHub repository settings for the CI/CD workflows to function properly.

## Required Repository Secrets

Navigate to your GitHub repository → Settings → Secrets and variables → Actions → Repository secrets, and add the following secrets:

### Google Cloud Platform Secrets

- **`GCP_SERVICE_ACCOUNT_KEY`** - JSON key for a service account with the following permissions:
  - Cloud Run Admin
  - Storage Admin  
  - Container Registry Service Agent
  - Service Account User

### DockerHub Secrets

- **`DOCKERHUB_USERNAME`** - Your DockerHub username
- **`DOCKERHUB_TOKEN`** - DockerHub access token (not password)
  - Create at: <https://hub.docker.com/settings/security>

## Required Repository Variables

Navigate to your GitHub repository → Settings → Secrets and variables → Actions → Variables, and add the following variables:

- **`GCP_PROJECT_ID`** - Your Google Cloud project ID (e.g., `outdoorsafetylab`)
- **`GCP_REGION`** - Google Cloud region for deployments (e.g., `asia-east1`)

## Google Cloud Storage Bucket Setup

The service stores and caches GeoIP2 database files in a GCS bucket for improved performance and to avoid repeated downloads from MaxMind. When the service starts, it:

1. **Checks GCS first** - Looks for existing database files to avoid downloading
2. **Downloads from MaxMind** - If no cached version exists or ETag has changed  
3. **Stores in GCS** - Caches the downloaded database for future use
4. **Shares across instances** - Multiple service instances can use the same cached data

### Prerequisites

You'll need a **MaxMind license key** to download GeoIP2 databases. Get one free at: <https://www.maxmind.com/en/geolite2/signup>

Set this as the `GEOIP2_LICENSE_KEY` environment variable or in your service configuration.

### Create the GCS Bucket

```bash
# Set your project ID
PROJECT_ID="your-project-id"

# Create the bucket (choose a unique name and appropriate region)
gsutil mb -p $PROJECT_ID -c STANDARD -l asia-east1 gs://geoipd

# Enable versioning (optional but recommended)
gsutil versioning set on gs://geoipd

# Set lifecycle policy to clean up old versions (optional)
cat > lifecycle.json << 'EOF'
{
  "lifecycle": {
    "rule": [
      {
        "action": {
          "type": "Delete"
        },
        "condition": {
          "numNewerVersions": 3
        }
      }
    ]
  }
}
EOF

gsutil lifecycle set lifecycle.json gs://geoipd
rm lifecycle.json
```

### Bucket Structure

The bucket will contain GeoIP2 database files:

```text
gs://geoipd/
├── GeoLite2-Country.mmdb     # Country-level GeoIP data
├── GeoLite2-City.mmdb        # City-level GeoIP data (if configured)
└── metadata/                 # ETags and modification times
```

### Required Bucket Permissions

The service account needs these permissions on the bucket:

- **Storage Object Viewer** - To read existing database files
- **Storage Object Creator** - To upload new database files
- **Storage Object Admin** - To manage metadata and lifecycle

```bash
# Grant bucket permissions to the service account (replace PROJECT_ID with your actual project ID)
gsutil iam ch serviceAccount:github-actions@PROJECT_ID.iam.gserviceaccount.com:objectAdmin gs://geoipd
```

### Service Configuration

The workflows are already configured with the necessary environment variables:

- `GEOIP2_CLOUD_STORAGE_BUCKET=geoipd` - Points to your GCS bucket
- `GEOIP2_CLOUD_STORAGE_PROVIDER=gcs` - Enables GCS storage

You'll also need to set the **MaxMind license key** in your Cloud Run service:

```bash
# Update your Cloud Run services with the MaxMind license key
gcloud run services update geoipd-alpha \
  --region=asia-east1 \
  --set-env-vars=GEOIP2_LICENSE_KEY=your_maxmind_license_key

gcloud run services update geoipd \
  --region=asia-east1 \
  --set-env-vars=GEOIP2_LICENSE_KEY=your_maxmind_license_key
```

Or add it as a secret in Google Secret Manager for better security:

```bash
# Store license key as a secret
echo "your_maxmind_license_key" | gcloud secrets create geoip2-license-key --data-file=-

# Update services to use the secret
gcloud run services update geoipd-alpha \
  --region=asia-east1 \
  --set-env-vars=GEOIP2_LICENSE_KEY=$(gcloud secrets versions access latest --secret=geoip2-license-key)
```

### Optional: Custom Bucket Configuration

If you want to use a different bucket name or add a key prefix, update the environment variables in both workflow files:

```yaml
# In .github/workflows/staging.yml and .github/workflows/production.yml
--set-env-vars=GEOIP2_CLOUD_STORAGE_BUCKET=your-custom-bucket-name \
--set-env-vars=GEOIP2_CLOUD_STORAGE_PROVIDER=gcs \
```

You can also configure a key prefix to organize files within the bucket by adding:

```yaml
--set-env-vars=GEOIP2_CLOUD_STORAGE_KEY_PREFIX=geoip2/ \
```

This would store files as `gs://your-bucket/geoip2/GeoLite2-Country.mmdb` instead of at the bucket root.

## Service Account Setup

Create a Google Cloud service account with these roles:

```bash
# Create service account
gcloud iam service-accounts create github-actions \
    --description="Service account for GitHub Actions" \
    --display-name="GitHub Actions"

# Grant necessary roles
gcloud projects add-iam-policy-binding PROJECT_ID \
    --member="serviceAccount:github-actions@PROJECT_ID.iam.gserviceaccount.com" \
    --role="roles/run.admin"

gcloud projects add-iam-policy-binding PROJECT_ID \
    --member="serviceAccount:github-actions@PROJECT_ID.iam.gserviceaccount.com" \
    --role="roles/storage.admin"

gcloud projects add-iam-policy-binding PROJECT_ID \
    --member="serviceAccount:github-actions@PROJECT_ID.iam.gserviceaccount.com" \
    --role="roles/containerregistry.ServiceAgent"

gcloud projects add-iam-policy-binding PROJECT_ID \
    --member="serviceAccount:github-actions@PROJECT_ID.iam.gserviceaccount.com" \
    --role="roles/iam.serviceAccountUser"

# Generate and download key
gcloud iam service-accounts keys create github-actions-key.json \
    --iam-account=github-actions@PROJECT_ID.iam.gserviceaccount.com
```

Use the contents of `github-actions-key.json` as the value for `GCP_SERVICE_ACCOUNT_KEY`.

## Workflow Overview

### Staging Workflow (`.github/workflows/staging.yml`)

- **Trigger**: Push to `master` branch
- **Actions**:
  - Builds Docker image with git metadata
  - Pushes to Google Container Registry
  - Deploys to Cloud Run service `geoipd-alpha`
  - Tests service with known IPs (8.8.8.8 and 1.1.1.1)

### Production Workflow (`.github/workflows/production.yml`)

- **Trigger**: Push of git tags
- **Actions**:
  - Builds Docker image with git metadata
  - Pushes to DockerHub (both `latest` and tagged versions)
  - Deploys to Cloud Run service `geoipd` using DockerHub image
  - Tests service with known IPs (8.8.8.8 and 1.1.1.1)

## Migration Notes

These workflows replicate the functionality of your existing Cloud Build setup:

- `build/ci/cloud-run.yaml` → GitHub Actions workflows with Cloud Run deployment
- `build/ci/dockerhub.yaml` → Production workflow DockerHub publishing

The workflows maintain the same build arguments, environment variables, and deployment configurations as your original Cloud Build setup.
