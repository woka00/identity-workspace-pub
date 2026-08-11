# Frontend build. Node 24 is an actively supported LTS line; only the compiled
# static bundle is copied to the runtime image.
FROM node:24.18.0-alpine3.24 AS frontend
WORKDIR /src/frontend
COPY frontend/package.json frontend/package-lock.json ./
RUN npm ci --ignore-scripts
COPY frontend/ ./
RUN npm run build

# The module keeps Go 1.22 as its language compatibility baseline, but the
# production binary is compiled with a supported Go toolchain containing later
# standard-library security fixes.
FROM golang:1.26.5-alpine3.24 AS backend
WORKDIR /src/backend
COPY backend/go.mod backend/go.sum ./
RUN go mod download
COPY backend/ ./
RUN go test ./... \
 && go vet ./... \
 && CGO_ENABLED=0 go build -trimpath -ldflags="-s -w -buildid=" -o /out/identity-workspace-server ./cmd/server

FROM alpine:3.24.1
RUN apk add --no-cache ca-certificates tzdata \
 && addgroup -S -g 10001 identity \
 && adduser -S -D -H -u 10001 -G identity identity
WORKDIR /app
COPY --from=backend --chown=10001:10001 /out/identity-workspace-server ./identity-workspace-server
COPY --from=frontend --chown=10001:10001 /src/frontend/dist ./static
USER 10001:10001
EXPOSE 8080
ENTRYPOINT ["/app/identity-workspace-server"]
CMD ["serve"]
