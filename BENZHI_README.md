# lan-share-6956 Docker 交付说明

## 项目概览
- 一个面向家庭或小团队的局域网文件共享工具，类似简易版的"局域网网盘"。同一局域网内的设备可以自动发现彼此，无需注册账号、无需云服务即可互传文件，支持文件夹共享、文件预览、断点续传与设备消息。所有数据留在本地，速度快且完全隐私。
- Go module: `lan-share`

## 标准命令

```bash
go build ./...
go test ./...
```

## 实际启动入口

```bash
go run .
```

## Docker 构建

```bash
./build_benzhi_docker.sh lan-share-6956-benzhi linux/amd64
docker run --rm -it lan-share-6956-benzhi bash
```

## 环境

- 基础镜像: `golang:1.21`
- 依赖在镜像构建阶段预下载，容器内可直接执行 Go 构建和测试命令。
- 代码目录: `/app`
- 源码中检测到的服务端口: `4096`, `8765`
