#!/bin/bash
# Build Frappe image with FPM CLI support
# Usage: ./build-frappe-fpm-image.sh [frappe-version] [fpm-version]

set -e

GREEN='\033[0;32m'
BLUE='\033[0;34m'
YELLOW='\033[1;33m'
NC='\033[0m'

FRAPPE_VERSION=${1:-latest}
FPM_VERSION=${2:-latest}
IMAGE_NAME="frappe-fpm"
IMAGE_TAG="v${FRAPPE_VERSION}"

echo -e "${BLUE}Building Frappe image with FPM CLI...${NC}"
echo "  Frappe version: $FRAPPE_VERSION"
echo "  FPM version: $FPM_VERSION"
echo ""

# Build for amd64 and arm64
echo -e "${YELLOW}Building multi-arch image...${NC}"
podman build \
  --platform linux/amd64,linux/arm64 \
  --build-arg FRAPPE_VERSION=$FRAPPE_VERSION \
  --build-arg FPM_VERSION=$FPM_VERSION \
  -t $IMAGE_NAME:$IMAGE_TAG \
  -f Dockerfile.frappe-fpm \
  .

echo ""
echo -e "${GREEN}✅ Image built successfully!${NC}"
echo ""
echo "Image: $IMAGE_NAME:$IMAGE_TAG"
echo ""
echo -e "${YELLOW}Next steps:${NC}"
echo "1. Load into Kind:"
echo "   kind load docker-image $IMAGE_NAME:$IMAGE_TAG --name frappe-operator-dev"
echo ""
echo "2. Tag for registry:"
echo "   podman tag $IMAGE_NAME:$IMAGE_TAG myregistry.com/$IMAGE_NAME:$IMAGE_TAG"
echo ""
echo "3. Push to registry:"
echo "   podman push myregistry.com/$IMAGE_NAME:$IMAGE_TAG"
echo ""
echo "4. Use in FrappeBench:"
echo "   spec:"
echo "     imageConfig:"
echo "       repository: myregistry.com/$IMAGE_NAME"
echo "       tag: $IMAGE_TAG"

