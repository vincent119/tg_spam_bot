FROM golang:1.25-alpine AS build
WORKDIR /src
COPY go.mod go.sum ./
RUN go mod download
COPY . .
# 此服務不使用 Gin 的 MsgPack 功能；排除該編解碼器可降低容器建置記憶體需求。
RUN CGO_ENABLED=0 GOMAXPROCS=1 go build -tags=nomsgpack -p=1 -trimpath -ldflags="-s -w" -o /out/tg-spam-bot ./cmd/tg-spam-bot

FROM alpine:3.22
RUN addgroup -S app && adduser -S -G app app
WORKDIR /app
COPY --from=build /out/tg-spam-bot /usr/local/bin/tg-spam-bot
COPY configs /app/configs
USER app
EXPOSE 8080
HEALTHCHECK --interval=10s --timeout=3s --retries=3 CMD wget -q -O /dev/null http://127.0.0.1:8080/health/ready || exit 1
ENTRYPOINT ["/usr/local/bin/tg-spam-bot"]
