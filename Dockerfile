# syntax=docker/dockerfile:1

# 1. 构建阶段：按目标平台交叉编译静态 Go 二进制。
ARG GO_VERSION=1.26
FROM --platform=$BUILDPLATFORM golang:${GO_VERSION}-alpine AS build

ARG TARGETOS
ARG TARGETARCH

WORKDIR /src

# 1.1 优先缓存依赖层，只有 go.mod/go.sum 变化时才重新下载依赖。
COPY go.mod go.sum ./
RUN --mount=type=cache,target=/go/pkg/mod go mod download

# 1.2 编译时复用 Go 构建缓存，并移除调试符号以减小镜像体积。
COPY . .
RUN --mount=type=cache,target=/root/.cache/go-build \
    mkdir -p /out && \
    CGO_ENABLED=0 \
    GOOS=${TARGETOS:-linux} \
    GOARCH=${TARGETARCH:-amd64} \
    go build -trimpath -ldflags="-s -w" -o /out/short-links ./cmd/api

# 2. 运行阶段：只保留运行所需文件，并以非 root 用户启动服务。
FROM alpine:3.22 AS runtime

RUN apk add --no-cache ca-certificates tzdata \
    && addgroup -S -g 10001 app \
    && adduser -S -D -H -u 10001 -G app app
COPY --from=build --chown=10001:10001 /out/short-links /usr/local/bin/short-links

ENV GIN_MODE=release
USER 10001:10001
EXPOSE 8080
STOPSIGNAL SIGTERM
ENTRYPOINT ["/usr/local/bin/short-links"]
