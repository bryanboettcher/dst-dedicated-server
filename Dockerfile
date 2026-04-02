## Build the supervisor
FROM golang:1.22-bookworm AS supervisor
WORKDIR /build
COPY supervisor/ .
RUN CGO_ENABLED=0 go build -o dst-supervisor .

## DST dedicated server image
FROM steamcmd/steamcmd:debian

LABEL org.opencontainers.image.title="DST Server"
LABEL org.opencontainers.image.description="Don't Starve Together dedicated server with built-in process supervisor and health endpoints"
LABEL org.opencontainers.image.source="https://github.com/bryanboettcher/dst-dedicated-server"
LABEL org.opencontainers.image.documentation="https://github.com/bryanboettcher/dst-dedicated-server#readme"
LABEL org.opencontainers.image.authors="Bryan Boettcher <bryan.boettcher@gmail.com>"

COPY --from=supervisor /build/dst-supervisor /usr/local/bin/dst-supervisor
COPY install.sh prepare.sh /usr/local/bin/

# Install DST runtime dependencies and set up user/directories
RUN apt-get update && \
    apt-get install -y --no-install-recommends libcurl3-gnutls && \
    rm -rf /var/lib/apt/lists/* && \
    chmod +x /usr/local/bin/*.sh /usr/local/bin/dst-supervisor && \
    useradd -m -s /bin/bash -d /home/dst dst && \
    mkdir -p /opt/dst_server /dst/mods /dst/config && \
    chown -R dst:dst /opt/dst_server /dst /home/dst

VOLUME [ "/dst/mods", "/dst/config", "/opt/dst_server" ]

EXPOSE 8080/tcp

ENTRYPOINT [ "/usr/local/bin/dst-supervisor" ]
