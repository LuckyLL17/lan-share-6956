# 评测用镜像：保留完整 Go 工具链（go1.21），依赖构建期预下载。
FROM golang:1.21
WORKDIR /app
COPY go.mod go.sum ./
RUN go mod download
COPY . .
RUN go build ./...
CMD ["bash"]

# 多架构交叉构建示例（如需交付双架构镜像）：
# docker buildx build --platform linux/arm64,linux/amd64 -f benzhi.Dockerfile -t <image> .
