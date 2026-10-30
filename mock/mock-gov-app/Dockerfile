# 多阶段构建：第一阶段用完整 Go 工具链编译，第二阶段只放一个静态二进制。
FROM golang:1.27-alpine AS build
ARG GOPROXY=https://goproxy.cn,direct
WORKDIR /src
COPY go.mod go.sum ./
RUN go mod download
COPY . .
RUN CGO_ENABLED=0 go build -trimpath -ldflags="-s -w" -o /mock-gov-app .

FROM scratch
COPY --from=build /mock-gov-app /mock-gov-app
EXPOSE 8081
ENTRYPOINT ["/mock-gov-app"]
