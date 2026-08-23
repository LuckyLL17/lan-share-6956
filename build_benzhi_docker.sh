#!/bin/bash
# 构建评测专用镜像；第二个参数为目标平台（arm64 / amd64）。
set -e
IMAGE_NAME=${1:-my-go-task}
PLATFORM=${2:-linux/amd64}

docker buildx build --platform "$PLATFORM" -f benzhi.Dockerfile -t "$IMAGE_NAME" .

echo ""
echo "✅ Docker image '$IMAGE_NAME' built successfully!"
echo "📋 进入容器: docker run -it $IMAGE_NAME bash"
