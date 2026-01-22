FROM golang:1.23 AS builder
WORKDIR /app

# Copy module files first for better caching
COPY go.mod go.sum ./
RUN go mod download

# Copy the source
COPY src/ ./src

# Build the app (package is ./src)
RUN CGO_ENABLED=0 GOOS=linux go build -o /hello-app ./src

FROM gcr.io/distroless/base-debian12
COPY --from=builder /hello-app /hello-app
EXPOSE 8080
CMD ["/hello-app"]
