# Stage 1: Build frontend
FROM oven/bun:1-alpine AS frontend
WORKDIR /app/web
COPY web/package.json web/bun.lock ./
RUN bun install --frozen-lockfile
COPY web/ .
ARG GIT_SHA=dev
ENV GIT_SHA=$GIT_SHA
RUN bun run build

# Stage 2: Build Go binary with embedded frontend
FROM golang:1-alpine AS backend
WORKDIR /app
COPY go.mod go.sum ./
RUN go mod download
COPY . .
COPY --from=frontend /app/web/dist ./web/dist
ARG VERSION=dev
ARG GIT_SHA=dev
RUN CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build \
  -ldflags="-s -w -X main.version=${VERSION} -X main.commit=${GIT_SHA}" \
  -o /meadowlark ./cmd/meadowlark

# Stage 3: Minimal runtime
FROM scratch
COPY --from=backend /etc/ssl/certs/ca-certificates.crt /etc/ssl/certs/
COPY --from=backend /meadowlark /meadowlark

LABEL org.opencontainers.image.source="https://github.com/fx/meadowlark"
LABEL org.opencontainers.image.description="Meadowlark -- Wyoming to OpenAI TTS Bridge"

EXPOSE 10300 8080
ENTRYPOINT ["/meadowlark"]
