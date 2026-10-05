FROM golang:1.25-alpine AS build
WORKDIR /src
COPY go.mod go.sum ./
RUN go mod download
COPY . .
# 排除 Gin 的 MsgPack，並限制編譯器記憶體，以適應較小的容器建置環境。
RUN CGO_ENABLED=0 GOMAXPROCS=1 GOMEMLIMIT=1200MiB go build -tags=nomsgpack -p=1 -trimpath -ldflags="-s -w" -o /out/tg-spam-bot ./cmd/tg-spam-bot
RUN CGO_ENABLED=0 GOMAXPROCS=1 GOMEMLIMIT=1200MiB go build -tags=nomsgpack -p=1 -trimpath -ldflags="-s -w" -o /out/tg-spam-migrate ./cmd/tg-spam-migrate

FROM alpine:3.22
ARG TIMEZONE=Asia/Taipei
ENV TZ=${TIMEZONE}
# Go 的 time.LoadLocation 依賴 IANA 時區資料，runtime image 必須保留 tzdata 才能載入設定的時區。
RUN apk add --no-cache tzdata \
    && test -f "/usr/share/zoneinfo/${TZ}" \
    && cp "/usr/share/zoneinfo/${TZ}" /etc/localtime \
    && printf '%s\n' "${TZ}" > /etc/timezone \
    && addgroup -S app \
    && adduser -S -G app app
WORKDIR /app
COPY --from=build /out/tg-spam-bot /app/tg-spam-bot
COPY --from=build /out/tg-spam-migrate /app/tg-spam-migrate
COPY migrations /app/migrations
COPY configs /app/configs
RUN mkdir -p /app/logs \
    && chown -R app:app /app/logs
USER app
EXPOSE 8080
HEALTHCHECK --interval=10s --timeout=3s --retries=3 CMD wget -q -O /dev/null http://127.0.0.1:8080/health/ready || exit 1
ENTRYPOINT ["/app/tg-spam-bot"]
