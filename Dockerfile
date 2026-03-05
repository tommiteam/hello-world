FROM golang:1.24.2 AS builder
WORKDIR /app

COPY go.mod go.sum ./
RUN go mod download

COPY cmd/ cmd/
COPY internal/ internal/
RUN CGO_ENABLED=0 GOOS=linux go build -o /hello-app ./cmd/helloapp

FROM gcr.io/distroless/base-debian12
WORKDIR /
COPY --from=builder /hello-app /hello-app
COPY config.yaml /config.yaml
EXPOSE 8080
ENTRYPOINT ["/hello-app"]
