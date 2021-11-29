FROM debian:latest

LABEL \
    description="SteamCMD Base image" \
    source="https://github.com/mathielo/dst-dedicated-server"

# Install required packages
ARG DEBIAN_FRONTEND=noninteractive

RUN set -x
RUN dpkg --add-architecture i386
RUN apt-get update 
RUN apt-get upgrade -y
RUN apt-get install --no-install-recommends -y wget ca-certificates lib32gcc-s1 lib32stdc++6 libcurl4-gnutls-dev:i386

# Create specific user to run DST server
RUN useradd -ms /bin/bash/ steam
WORKDIR /home/steam

# Download Steam CMD (https://developer.valvesoftware.com/wiki/SteamCMD#Downloading_SteamCMD)
RUN wget -q -O - "https://steamcdn-a.akamaihd.net/client/installer/steamcmd_linux.tar.gz" | tar zxvf -
RUN chown -R steam:steam ./
# Cleanup
RUN apt-get autoremove --purge -y wget
RUN apt-get clean
RUN rm -rf /var/lib/apt/lists/*
