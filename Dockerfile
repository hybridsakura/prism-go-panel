# syntax=docker/dockerfile:1

# ---------- 构建阶段 ----------
FROM golang:1.27-alpine AS build
WORKDIR /src

# 利用 Docker 层缓存：先拷贝依赖描述文件
COPY go.mod go.sum ./
RUN go mod download

COPY . .
# 纯 Go 编译（SQLite 驱动为纯 Go 实现，无需 CGO）
RUN CGO_ENABLED=0 GOOS=linux go build -trimpath -ldflags="-s -w" \
    -o /out/rustdesk-api-server ./cmd/server

# ---------- 运行阶段 ----------
FROM alpine:3.21
RUN apk add --no-cache ca-certificates tzdata

COPY --from=build /out/rustdesk-api-server /usr/local/bin/rustdesk-api-server

# 数据目录（SQLite）与可选的 Web 控制端目录
RUN mkdir -p /data /webui-dist
VOLUME ["/data"]

ENV HOST=0.0.0.0 \
    PORT=21114 \
    DB_URL=/data/rustdesk-api.db \
    TZ=Asia/Shanghai

EXPOSE 21114/tcp

HEALTHCHECK --interval=30s --timeout=5s --start-period=10s \
    CMD wget -qO- http://127.0.0.1:21114/healthz || exit 1

ENTRYPOINT ["rustdesk-api-server"]
