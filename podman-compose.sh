#!/bin/bash

IMAGE="localhost/dst-server:latest"


case "${1}" in
  start|up)

    detach=""
    if [ "${2}" == "-d" ]; then
      detach="-d"
    fi


    podman pod create -p 10999:10999/udp -n dst-server

    podman run --name dst_caves --group-add root -d \
      -it --pod dst-server -e SHARD_NAME=Caves \
      -v ./DSTClusterConfig:/home/dst/.klei/DoNotStarveTogether/DSTWhalesCluster \
      -v ./volumes/mods:/home/dst/server_dst/mods ${IMAGE}

    podman run --name dst_master --group-add root ${detach} \
      -it --pod dst-server -e SHARD_NAME=Master \
      -v ./DSTClusterConfig:/home/dst/.klei/DoNotStarveTogether/DSTWhalesCluster \
      -v ./volumes/mods:/home/dst/server_dst/mods ${IMAGE}
    ;;

  stop|down)
    podman pod stop dst-server
    podman pod rm dst-server
    ;;

  restart)
    ${0} stop
    ${0} start
    ;;

  *)
    echo "Usage: ${0} {start|stop|restart|up|down}"
    exit 1
    ;;
esac
