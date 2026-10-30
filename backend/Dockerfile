# 多阶段构建：迁移 SQL 已经通过 go:embed 编进二进制，运行镜像里只有一个文件。
FROM golang:1.27-alpine AS build
ARG GOPROXY=https://goproxy.cn,direct
WORKDIR /src
COPY go.mod go.sum ./
RUN go mod download
COPY . .
RUN CGO_ENABLED=0 go build -trimpath -ldflags="-s -w" -o /opshub ./cmd/opshub

FROM scratch
COPY --from=build /opshub /opshub
EXPOSE 8080
ENTRYPOINT ["/opshub"]
