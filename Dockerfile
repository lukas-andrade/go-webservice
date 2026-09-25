FROM golang:1.27.1-alpine AS build
WORKDIR /src

COPY go.mod go.sum ./
RUN go mod download

COPY cmd ./cmd
COPY internal ./internal
RUN CGO_ENABLED=0 GOOS=linux go build -trimpath -ldflags="-s -w" -o /out/echo-service ./cmd/server

FROM gcr.io/distroless/static-debian12:nonroot

LABEL org.opencontainers.image.title="echo-service" \
      org.opencontainers.image.description="HTTP service that echoes requests back as JSON" \
      org.opencontainers.image.source="https://github.com/lukas-andrade/go-webservice"

COPY --from=build /out/echo-service /echo-service

USER nonroot:nonroot
EXPOSE 8080 9090
ENTRYPOINT ["/echo-service"]
