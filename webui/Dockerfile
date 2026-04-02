FROM golang:1.22-bookworm AS builder
WORKDIR /build
COPY . .
RUN CGO_ENABLED=0 go build -o dst-webui .

FROM gcr.io/distroless/static-debian12:nonroot

LABEL org.opencontainers.image.title="DST Server Web UI"
LABEL org.opencontainers.image.description="Web dashboard for Don't Starve Together dedicated server management"
LABEL org.opencontainers.image.source="https://github.com/bryanboettcher/dst-dedicated-server"
LABEL org.opencontainers.image.documentation="https://github.com/bryanboettcher/dst-dedicated-server#web-ui"
LABEL org.opencontainers.image.authors="Bryan Boettcher <bryan.boettcher@gmail.com>"

COPY --from=builder /build/dst-webui /dst-webui

EXPOSE 8080/tcp

ENTRYPOINT ["/dst-webui"]
