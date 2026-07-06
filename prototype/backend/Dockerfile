FROM golang:1.22-alpine AS build
WORKDIR /src/backend
COPY prototype/backend/go.mod ./
COPY prototype/backend/*.go ./
RUN go build -trimpath -ldflags="-s -w" -o /out/mediamtx-console

FROM alpine:3.20
WORKDIR /app
COPY --from=build /out/mediamtx-console /app/mediamtx-console
EXPOSE 8080
ENTRYPOINT ["/app/mediamtx-console"]
