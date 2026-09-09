# Build stage: compile a static binary.
FROM golang:1.27-alpine AS build
WORKDIR /src
COPY go.mod go.sum ./
RUN go mod download
COPY . .
RUN CGO_ENABLED=0 go build -trimpath -ldflags="-s -w" -o /out/simple-status ./cmd/simple-status

# Runtime stage: distroless/static ships CA certificates (needed for https
# probes) and runs as a non-root user.
FROM gcr.io/distroless/static-debian12:nonroot
COPY --from=build /out/simple-status /simple-status
EXPOSE 8080
ENTRYPOINT ["/simple-status"]
CMD ["--config", "/etc/simple-status/config.yml"]
