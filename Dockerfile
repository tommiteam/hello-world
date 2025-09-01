FROM golang:1.23 AS builder

WORKDIR /app

# Copy source code
COPY src/ ./src

# Build
RUN cd src && go mod init hello && go mod tidy && go build -o /hello-app

FROM gcr.io/distroless/base-debian12
COPY --from=builder /hello-app /hello-app
EXPOSE 8080
CMD ["/hello-app"]
