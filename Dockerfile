FROM golang:1.24.2 AS builder
WORKDIR /app

COPY go.mod go.sum ./
RUN go mod download

COPY src/ ./src
RUN CGO_ENABLED=0 GOOS=linux go build -o /hello-app ./src

FROM gcr.io/distroless/base-debian12
COPY --from=builder /hello-app /hello-app
EXPOSE 8080
CMD ["/hello-app"]
