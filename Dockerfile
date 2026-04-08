# syntax=docker/dockerfile:1.7

FROM --platform=$BUILDPLATFORM golang:1.25 AS builder

ARG TARGETOS
ARG TARGETARCH

WORKDIR /src

COPY go.mod go.sum ./
RUN go mod download

COPY backend ./backend
COPY cmd ./cmd
COPY internal ./internal

RUN CGO_ENABLED=0 GOOS=$TARGETOS GOARCH=$TARGETARCH go build \
	-trimpath \
	-ldflags="-s -w" \
	-o /out/paperless-ai-ext-backend \
	./backend

FROM --platform=$TARGETPLATFORM gcr.io/distroless/base-debian12

WORKDIR /app

ENV PAPERLESS_AIEXT_LISTEN_ADDR=:8080
ENV PAPERLESS_AIEXT_DB_PATH=/data/paperless-aiext.db

COPY --from=builder /out/paperless-ai-ext-backend /app/paperless-ai-ext-backend

VOLUME ["/data"]
EXPOSE 8080

ENTRYPOINT ["/app/paperless-ai-ext-backend"]