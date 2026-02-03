# --- ステージ1: ビルド環境 ---
FROM golang:1.23-alpine AS builder
WORKDIR /app
RUN apk add --no-cache git
COPY . .
RUN go mod tidy
RUN CGO_ENABLED=0 GOOS=linux go build -o main .

# --- ステージ2: 実行環境 ---
FROM alpine:latest
WORKDIR /root/

# メインの実行ファイルをコピー
COPY --from=builder /app/main .

# ▼▼▼ 追加: HTMLなどの静的ファイルもコピーする ▼▼▼
COPY --from=builder /app/static ./static

EXPOSE 8081
CMD ["./main"]