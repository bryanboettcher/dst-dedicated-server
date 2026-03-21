FROM steamcmd/steamcmd:debian

LABEL org.opencontainers.image.title="DST Server"
LABEL org.opencontainers.image.description="Don't Starve Together dedicated server"
LABEL org.opencontainers.image.source="https://github.com/bryanboettcher/dst-dedicated-server"
LABEL org.opencontainers.image.authors="Bryan Boettcher <bryan.boettcher@gmail.com>"

COPY entrypoint.sh install.sh prepare.sh run.sh /usr/local/bin/

# Set up user and directories
RUN chmod +x /usr/local/bin/*.sh && \
    useradd -m -s /bin/bash -d /home/dst dst && \
    mkdir -p /opt/dst_server /dst/mods /dst/config

# Install DST server (baked into image for Docker users; K8s overlays with emptyDir)
RUN steamcmd \
        +@ShutdownOnFailedCommand 1 \
        +@NoPromptForPassword 1 \
        +login anonymous \
        +force_install_dir /opt/dst_server \
        +app_update 343050 validate \
        +quit && \
    chown -R dst:dst /opt/dst_server /dst

VOLUME [ "/dst/mods", "/dst/config", "/opt/dst_server" ]

ENTRYPOINT [ "/usr/local/bin/entrypoint.sh" ]
