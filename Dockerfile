FROM steamcmd/steamcmd:debian

LABEL org.opencontainers.image.title="DST Server"
LABEL org.opencontainers.image.description="Don't Starve Together dedicated server"
LABEL org.opencontainers.image.source="https://github.com/bryanboettcher/dst-dedicated-server"
LABEL org.opencontainers.image.authors="Bryan Boettcher <bryan.boettcher@gmail.com>"

COPY entrypoint.sh install.sh prepare.sh run.sh /usr/local/bin/

RUN \
    # create service account
    useradd -m -s /bin/bash/ -d /home/dst dst && \
    # create local directories for volume mounts
    mkdir -p /opt/dst_server /dst/{mods,config} && \
    # set permissions
    chmod 0777 -R /opt/dst_server /dst && \
    # install DST
    steamcmd.sh \
        +@ShutdownOnFailedCommand 1 \
        +@NoPromptForPassword 1 \
        +login anonymous \
        +force_install_dir /opt/dst_server \
        +app_update 343050 validate \
        +quit

USER dst

VOLUME [ "/dst/mods", "/dst/config", "/opt/dst_server" ]

ENTRYPOINT [ "/usr/local/bin/entrypoint.sh" ]