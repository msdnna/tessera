FROM golang:1.25-alpine AS builder

WORKDIR /app

RUN apk add --no-cache git

COPY go.mod go.sum ./
RUN go mod download

# Build metadata surfaced by GET /api/version. The build context is ./backend
# (no .git), so these are passed in from docker-compose rather than read from
# git here; both default to empty and the API omits an empty field.
ARG GIT_COMMIT=
ARG BUILD_DATE=

COPY . .
RUN CGO_ENABLED=0 GOOS=linux go build \
      -ldflags "-X main.commit=${GIT_COMMIT} -X main.builtAt=${BUILD_DATE}" \
      -o tessera . \
 && CGO_ENABLED=0 GOOS=linux go build -o migrate ./cmd/migrate

FROM alpine:3.21

RUN apk add --no-cache ca-certificates tzdata

WORKDIR /app

COPY --from=builder /app/tessera .
# Migrations CLI — run as `docker compose exec backend /app/migrate`.
COPY --from=builder /app/migrate .

EXPOSE 8080

CMD ["./tessera"]
