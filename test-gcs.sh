#!/bin/bash

# Test script for GCS cloud storage functionality
# Project: outdoorsafetylab
# Bucket: geoip2

set -e

echo "🧪 Testing GCS Cloud Storage Integration"
echo "======================================="

# Check if GEOIP2_LICENSE_KEY is set
if [ -z "$GEOIP2_LICENSE_KEY" ]; then
    echo "❌ Error: GEOIP2_LICENSE_KEY environment variable is not set"
    echo "Please set it first:"
    echo "export GEOIP2_LICENSE_KEY='your_maxmind_license_key'"
    exit 1
fi

# Set GCS configuration
export GEOIP2_CLOUD_STORAGE_BUCKET="geoip2"
export GEOIP2_CLOUD_STORAGE_PROVIDER="gcs"

echo "✅ Configuration:"
echo "   Project: outdoorsafetylab"
echo "   Bucket: $GEOIP2_CLOUD_STORAGE_BUCKET"
echo "   License Key: ${GEOIP2_LICENSE_KEY:0:8}..." # Show only first 8 chars
echo ""

# Check if gcloud is authenticated
echo "🔐 Checking Google Cloud authentication..."
if ! gcloud auth application-default print-access-token > /dev/null 2>&1; then
    echo "❌ Google Cloud not authenticated for Application Default Credentials"
    echo "Please run: gcloud auth application-default login"
    exit 1
fi

echo "✅ Google Cloud authenticated"

# Check if bucket exists and is accessible
echo "🪣 Checking GCS bucket access..."
if ! gsutil ls gs://$GEOIP2_CLOUD_STORAGE_BUCKET > /dev/null 2>&1; then
    echo "❌ Cannot access bucket gs://$GEOIP2_CLOUD_STORAGE_BUCKET"
    echo "Please ensure:"
    echo "1. Bucket exists: gsutil mb gs://$GEOIP2_CLOUD_STORAGE_BUCKET"
    echo "2. You have access: gsutil iam get gs://$GEOIP2_CLOUD_STORAGE_BUCKET"
    exit 1
fi

echo "✅ GCS bucket accessible"

# Start Redis (if not running)
echo "🔴 Starting Redis for testing..."
if ! redis-cli ping > /dev/null 2>&1; then
    echo "Starting Redis..."
    redis-server --daemonize yes --port 6379
    sleep 2
fi

echo "✅ Redis running"

# Build the application
echo "🔨 Building application..."
make clean && make

# Run the test
echo "🚀 Starting GeoIP service with GCS support..."
echo "Watch the logs for:"
echo "- 'Initialized cloud storage: gcs://geoip2'"
echo "- 'Database not found in cloud storage' (first run)"
echo "- 'Downloading DB' (first run)"
echo "- 'Successfully stored database in cloud storage'"
echo ""

./geoip serve test-gcs