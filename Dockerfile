FROM debian:latest

# based entirely from https://github.com/mathielo/dst-dedicated-server
# but I changed enough that it seemed fair to update the author so
# mathielo isn't responsible for my nonsense

LABEL org.opencontainers.image.title="DST Server"
LABEL org.opencontainers.image.description="Don't Starve Together dedicated server"
LABEL org.opencontainers.image.source="https://github.com/bryanboettcher/dst-dedicated-server"
LABEL org.opencontainers.image.authors="Bryan Boettcher <bryan.boettcher@gmail.com>"

# Create specific user to run DST server
RUN useradd -m -s /bin/bash/ -d /home/dst dst
WORKDIR /home/dst

COPY entrypoint.sh prepare.sh install.sh run.sh /home/dst/

# Install required packages
RUN set -x && \
    dpkg --add-architecture i386 && \
    apt update && apt upgrade -y && \
    DEBIAN_FRONTEND=noninteractive apt install --no-install-recommends -y wget ca-certificates lib32gcc1 lib32stdc++6 libcurl4-gnutls-dev:i386 && \
    # Download Steam CMD (https://developer.valvesoftware.com/wiki/SteamCMD#Downloading_SteamCMD)
    wget -q -O - "https://steamcdn-a.akamaihd.net/client/installer/steamcmd_linux.tar.gz" | tar zxvf - && \
    chown -R dst:dst ./ && \
    mkdir -p /dst/mods /dst/config /opt/dst_server && \
    chown -R dst:dst /dst && \
    chown -R dst:dst /opt/dst_server && \
    chmod -R 0775 /dst && \
    chmod -R 0775 /opt/dst_server && \
    # Cleanup
    apt-get autoremove --purge -y wget && \
    apt-get clean && rm -rf /var/lib/apt/lists/*

VOLUME [ "/dst/mods", "/dst/config", "/opt/dst_server", "/home/dst" ]

ENTRYPOINT [ "/home/dst/entrypoint.sh" ]