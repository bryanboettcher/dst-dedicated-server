FROM steamcmd/steamcmd:debian

LABEL org.opencontainers.image.title="DST Server"
LABEL org.opencontainers.image.description="Don't Starve Together dedicated server"
LABEL org.opencontainers.image.source="https://github.com/bryanboettcher/dst-dedicated-server"
LABEL org.opencontainers.image.authors="Bryan Boettcher <bryan.boettcher@gmail.com>"

COPY entrypoint.sh install.sh prepare.sh run.sh /usr/local/bin/

# Set up user and directories; DST server is installed at runtime via install.sh
RUN chmod +x /usr/local/bin/*.sh && \
    useradd -m -s /bin/bash -d /home/dst dst && \
    mkdir -p /opt/dst_server /dst/mods /dst/config && \
    chown -R dst:dst /opt/dst_server /dst /home/dst

VOLUME [ "/dst/mods", "/dst/config", "/opt/dst_server" ]

ENTRYPOINT [ "/usr/local/bin/entrypoint.sh" ]
