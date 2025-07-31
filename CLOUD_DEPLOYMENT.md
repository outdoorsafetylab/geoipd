# Cloud Deployment Guide

This guide explains how to deploy the GeoIP service to cloud platforms like Google Cloud Run with persistent cloud storage to avoid MaxMind quota issues.

## Problem with Short-Lived Services

In cloud platforms like Cloud Run, instances are ephemeral and frequently created/destroyed. This causes:

- Frequent GeoIP database downloads from MaxMind
- Hitting daily download quotas
- Slow startup times

## Cloud Storage Solution

The service now supports storing the GeoIP database in cloud storage with ETag-based caching:

1. **First deployment**: Downloads database from MaxMind, stores in cloud storage
2. **Subsequent deployments**: Loads database from cloud storage (fast)
3. **Database updates**: Only downloads from MaxMind when ETag changes
4. **Cross-instance sharing**: All instances share the same cloud-stored database

## Configuration

### 1. Google Cloud Storage (Recommended for Cloud Run)

```yaml
# config/cloud.yaml
geoip2:
  edition: GeoLite2-City
  renew: ""  # Disable auto-renewal in cloud environments
  cloud_storage:
    provider: gcs
    bucket: my-geoip-bucket
    key_prefix: geoip/
```

### 2. Environment Variables

```bash
export GEOIP2_LICENSE_KEY="your_maxmind_license_key"
export GEOIP2_CLOUD_STORAGE_BUCKET="my-geoip-bucket"
```

## Google Cloud Run Deployment

### 1. Create Storage Bucket

```bash
# Create bucket
gsutil mb gs://my-geoip-bucket

# Set appropriate permissions
gsutil iam ch serviceAccount:your-service-account@project.iam.gserviceaccount.com:objectAdmin gs://my-geoip-bucket
```

### 2. Deploy to Cloud Run

```bash
# Build and deploy
gcloud run deploy geoipd \\
  --image gcr.io/your-project/geoipd \\
  --set-env-vars GEOIP2_LICENSE_KEY=your_license_key \\
  --set-env-vars GEOIP2_CLOUD_STORAGE_BUCKET=my-geoip-bucket \\
  --allow-unauthenticated \\
  --memory 512Mi \\
  --concurrency 1000
```

### 3. Service Account Permissions

Ensure your Cloud Run service has the `Storage Object Admin` role:

```bash
gcloud projects add-iam-policy-binding your-project \\
  --member="serviceAccount:your-service-account@project.iam.gserviceaccount.com" \\
  --role="roles/storage.objectAdmin"
```

## Docker Configuration

For local testing with cloud storage:

```bash
docker run -it --rm \\
  -p 8080:8080 \\
  -e GEOIP2_LICENSE_KEY=your_license_key \\
  -e GEOIP2_CLOUD_STORAGE_BUCKET=my-geoip-bucket \\
  -e GOOGLE_APPLICATION_CREDENTIALS=/path/to/service-account.json \\
  -v /path/to/service-account.json:/path/to/service-account.json \\
  outdoorsafetylab/geoipd
```

## Benefits

### Performance

- **First startup**: ~30-60 seconds (download from MaxMind + upload to cloud storage)
- **Subsequent startups**: ~5-10 seconds (download from cloud storage)
- **No database updates**: ~2-3 seconds (ETag check, no download)

### Cost Savings

- Dramatically reduced MaxMind API calls
- Faster scaling due to reduced startup time
- Shared database across all instances

### Reliability

- Automatic fallback to MaxMind if cloud storage fails
- ETag-based consistency across instances
- No dependency on Redis for ETag storage

## Monitoring

Check logs for cloud storage operations:

```shell
INFO: Initialized cloud storage: gcs://my-geoip-bucket
INFO: Found ETag in cloud storage: "abc123"
INFO: Successfully loaded database from cloud storage
```

## Troubleshooting

### Cloud Storage Access Issues

```shell
ERROR: Failed to initialize cloud storage: access denied
```

**Solution**: Verify service account has `Storage Object Admin` permissions.

### Database Not Found

```shell
INFO: Database not found in cloud storage: GeoLite2-City.mmdb
```

**Solution**: This is normal for first deployment. The service will download from MaxMind and store in cloud storage.

### ETag Mismatch

If you see frequent downloads despite cloud storage, check ETag handling:

```bash
gsutil ls -L gs://my-geoip-bucket/geoip/GeoLite2-City.mmdb
```

## Alternative Cloud Providers

### AWS S3 (Skeleton Implementation)

```yaml
geoip2:
  cloud_storage:
    provider: s3
    bucket: my-geoip-bucket
    region: us-east-1
```

### Azure Blob Storage (Skeleton Implementation)

```yaml
geoip2:
  cloud_storage:
    provider: azure
    bucket: my-container
    region: eastus
```

**Note**: S3 and Azure implementations require additional development. The GCS implementation is production-ready.
