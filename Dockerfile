# Use official Golang image to build
FROM golang:1.23 AS builder

WORKDIR /app

# Cache go modules
COPY go.mod go.sum ./
RUN go mod download

# Copy source code
COPY src/ ./src

# Build
RUN cd src && go build -o /hello-app

# Final minimal image
FROM gcr.io/distroless/base-debian12

COPY --from=builder /hello-app /hello-app

EXPOSE 8080
CMD ["/hello-app"]
