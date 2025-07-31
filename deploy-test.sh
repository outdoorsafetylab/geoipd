#!/bin/bash

# Manual deployment script for testing GCS integration
# Project: outdoorsafetylab
# Bucket: geoip2

set -e

PROJECT_ID="outdoorsafetylab"
SERVICE_NAME="geoipd"
REGION="asia-northeast1"  # Change this to your preferred region
IMAGE_NAME="asia.gcr.io/$PROJECT_ID/geoipd"

echo "🚀 Deploying GeoIP service to Cloud Run"
echo "======================================="
echo "Project: $PROJECT_ID"
echo "Service: $SERVICE_NAME"
echo "Region: $REGION"
echo "Bucket: geoip2"
echo ""

# Check if GEOIP2_LICENSE_KEY is set
if [ -z "$GEOIP2_LICENSE_KEY" ]; then
    echo "❌ Error: GEOIP2_LICENSE_KEY environment variable is not set"
    echo "Please set it first:"
    echo "export GEOIP2_LICENSE_KEY='your_maxmind_license_key'"
    exit 1
fi

echo "✅ License key configured"

# Build Docker image
echo "🔨 Building Docker image..."
docker build \
    --build-arg GIT_HASH=$(git rev-parse --short HEAD) \
    --build-arg GIT_TAG=$(git describe --tags --always) \
    -t $IMAGE_NAME \
    -f build/Dockerfile \
    .

echo "✅ Docker image built"

# Push to Container Registry (requires Docker authentication)
echo "📦 Pushing to Container Registry..."
echo "Note: Make sure you're authenticated with: gcloud auth configure-docker asia.gcr.io"
docker push $IMAGE_NAME

echo "✅ Image pushed"

# Deploy to Cloud Run
echo "🚀 Deploying to Cloud Run..."
gcloud run deploy $SERVICE_NAME \
    --image=$IMAGE_NAME \
    --region=$REGION \
    --platform=managed \
    --set-env-vars=GEOIP2_LICENSE_KEY="$GEOIP2_LICENSE_KEY" \
    --set-env-vars=GEOIP2_CLOUD_STORAGE_BUCKET=geoip2 \
    --set-env-vars=GEOIP2_CLOUD_STORAGE_PROVIDER=gcs \
    --memory=1Gi \
    --cpu=1 \
    --allow-unauthenticated \
    --project=$PROJECT_ID

echo ""
echo "🎉 Deployment complete!"
echo ""

# Get service URL
SERVICE_URL=$(gcloud run services describe $SERVICE_NAME --region=$REGION --project=$PROJECT_ID --format="value(status.url)")

echo "✅ Service URL: $SERVICE_URL"
echo ""
echo "🧪 Testing the service..."
echo "========================="

# Test the service
echo "Testing version endpoint..."
curl -s "$SERVICE_URL/v1/version" | jq . || echo "Response: $(curl -s "$SERVICE_URL/v1/version")"

echo ""
echo "Testing GeoIP lookup..."
curl -s "$SERVICE_URL/v1/city?ip=8.8.8.8" | jq .IP,.Country.Names.en || echo "Response: $(curl -s "$SERVICE_URL/v1/city?ip=8.8.8.8")"

echo ""
echo "📋 To check logs for cloud storage operations:"
echo "gcloud logs tail \"run.googleapis.com%2Frequest_log\" --filter=\"resource.labels.service_name=$SERVICE_NAME\" --project=$PROJECT_ID"
echo ""
echo "🔍 Look for these log messages:"
echo "- 'Initialized cloud storage: gcs://geoip2'"
echo "- 'Database not found in cloud storage' (first run)"
echo "- 'Downloading DB' (first run)" 
echo "- 'Successfully stored database in cloud storage'"
echo "- 'Successfully loaded database from cloud storage' (subsequent runs)"