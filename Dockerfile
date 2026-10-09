# Frontend build. Node 24 is an actively supported LTS line; only the compiled
# static bundle is copied to the runtime image.
FROM node:24.18.0-alpine3.24 AS frontend
WORKDIR /src/frontend
ENV NODE_OPTIONS=--max-old-space-size=512
COPY frontend/package.json frontend/package-lock.json ./
RUN npm ci --ignore-scripts
COPY frontend/ ./
RUN npm run build

# The module keeps Go 1.22 as its language compatibility baseline, but the
# production binary is compiled with a supported Go toolchain containing later
# standard-library security fixes.
FROM golang:1.26.6-alpine3.24 AS backend
WORKDIR /src/backend
COPY backend/go.mod backend/go.sum ./
RUN go mod download
COPY backend/ ./
# Make the expensive Go build wait for the frontend build. On small
# single-core VPS hosts BuildKit would otherwise run both stages in parallel,
# exhausting RAM and spending many minutes swapping.
COPY --from=frontend /src/frontend/dist /tmp/frontend-dist
RUN GOMAXPROCS=1 go test -p 1 ./... \
 && go vet ./... \
 && CGO_ENABLED=0 go build -trimpath -ldflags="-s -w -buildid=" -o /out/avatar-id-server ./cmd/server

FROM alpine:3.24.1
RUN apk add --no-cache ca-certificates tzdata \
 && addgroup -S -g 10001 avatar \
 && adduser -S -D -H -u 10001 -G avatar avatar
WORKDIR /app
COPY --from=backend --chown=10001:10001 /out/avatar-id-server ./avatar-id-server
COPY --from=backend --chown=10001:10001 /tmp/frontend-dist ./static
USER 10001:10001
EXPOSE 8080
ENTRYPOINT ["/app/avatar-id-server"]
CMD ["serve"]
