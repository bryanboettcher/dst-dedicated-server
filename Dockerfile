FROM debian:latest

MAINTAINER Caio Mathielo <mathielo@gmail.com>

LABEL \
    description="Don't Starve Together dedicated server" \
    source="https://github.com/mathielo/dst-dedicated-server"

# Install required packages
RUN set -x
RUN dpkg --add-architecture i386
RUN apt-get update && apt-get upgrade -y
RUN DEBIAN_FRONTEND=noninteractive apt-get install --no-install-recommends -y wget ca-certificates lib32gcc1 lib32stdc++6 libcurl4-gnutls-dev:i386

# Create specific user to run DST server
RUN useradd -ms /bin/bash/ dst
WORKDIR /home/dst

# Download Steam CMD (https://developer.valvesoftware.com/wiki/SteamCMD#Downloading_SteamCMD)
RUN wget -q -O - "https://steamcdn-a.akamaihd.net/client/installer/steamcmd_linux.tar.gz" | tar zxvf -
RUN chown -R dst:dst ./
# Cleanup
RUN apt-get autoremove --purge -y wget
RUN apt-get clean && rm -rf /var/lib/apt/lists/*

USER dst
RUN mkdir -p .klei/DoNotStarveTogether server_dst/mods

# Install Don't Starve Together
RUN ./steamcmd.sh \
    +@ShutdownOnFailedCommand 1 \
    +@NoPromptForPassword 1 \
    +login anonymous \
    +force_install_dir /home/dst/server_dst \
    +app_update 343050 validate \
    +quit

VOLUME ["/home/dst/.klei/DoNotStarveTogether", "/home/dst/server_dst/mods"]

COPY ["start-container-server.sh", "/home/dst/"]
EXPOSE 10999/udp
ENTRYPOINT ["/home/dst/start-container-server.sh"]
