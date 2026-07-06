FROM golang:1.22-alpine AS build
WORKDIR /src/backend
COPY prototype/backend/go.mod ./
COPY prototype/backend/*.go ./
RUN CGO_ENABLED=0 go build -trimpath -ldflags="-s -w" -o /out/mediamtx-console

FROM alpine:3.20
RUN apk add --no-cache ca-certificates \
    && addgroup -S app && adduser -S app -G app
WORKDIR /app
COPY --from=build /out/mediamtx-console /app/mediamtx-console
USER app
EXPOSE 8080
HEALTHCHECK --interval=10s --timeout=3s --start-period=5s --retries=3 \
    CMD wget -q -O- http://127.0.0.1:8080/health || exit 1
ENTRYPOINT ["/app/mediamtx-console"]
