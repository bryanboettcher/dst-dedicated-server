FROM steamcmd/steamcmd:debian

LABEL org.opencontainers.image.title="DST Server"
LABEL org.opencontainers.image.description="Don't Starve Together dedicated server"
LABEL org.opencontainers.image.source="https://github.com/bryanboettcher/dst-dedicated-server"
LABEL org.opencontainers.image.authors="Bryan Boettcher <bryan.boettcher@gmail.com>"

COPY entrypoint.sh install.sh prepare.sh run.sh /usr/local/bin/

# Install DST runtime dependencies and set up user/directories
RUN apt-get update && \
    apt-get install -y --no-install-recommends libcurl3-gnutls && \
    rm -rf /var/lib/apt/lists/* && \
    chmod +x /usr/local/bin/*.sh && \
    useradd -m -s /bin/bash -d /home/dst dst && \
    mkdir -p /opt/dst_server /dst/mods /dst/config && \
    chown -R dst:dst /opt/dst_server /dst /home/dst

VOLUME [ "/dst/mods", "/dst/config", "/opt/dst_server" ]

ENTRYPOINT [ "/usr/local/bin/entrypoint.sh" ]
