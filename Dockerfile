FROM golang:1.24.2 AS builder
WORKDIR /app

COPY go.mod go.sum ./
RUN go mod download

COPY cmd/ cmd/
COPY internal/ internal/
RUN CGO_ENABLED=0 GOOS=linux go build -o /hello-world ./cmd/helloapp

FROM gcr.io/distroless/base-debian12
WORKDIR /app
COPY --from=builder /hello-world /hello-world
EXPOSE 8080
ENTRYPOINT ["/hello-world"]
