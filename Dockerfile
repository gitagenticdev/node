FROM golang:1.21-alpine AS builder
WORKDIR /app
COPY go.mod ./
COPY main.go ./
RUN CGO_ENABLED=0 go build -ldflags="-s -w" -o node .

FROM alpine:3.19
RUN apk add --no-cache ca-certificates
WORKDIR /app
COPY --from=builder /app/node .
RUN addgroup -S app && adduser -S app -G app && chown -R app:app /app
USER app
EXPOSE 9000
CMD ["./node"]
