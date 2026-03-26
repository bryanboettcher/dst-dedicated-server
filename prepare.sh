#!/bin/bash

echo "PREPARE.SH: "
whoami

if [ -n "$PUID" ] || [ -n "$PGID" ]; then
  export PUID=${PUID:-$(grep dst /etc/passwd | cut -d":" -f3)}
  export PGID=${PGID:-$(grep dst /etc/passwd | cut -d":" -f4)}
  USERHOME=$(grep dst /etc/passwd | cut -d":" -f6)

  groupmod -o -g "${PGID}" dst
  usermod -o -u "${PUID}" dst
  usermod -d "${USERHOME}" dst
fi

# ensure shard directories exist
mkdir -p "$CLUSTER_ROOT" "$SHARD_ROOT"

# fix permissions on writable volumes (skip /dst/mods which may be a read-only ConfigMap)
chown -R dst:dst /dst/config /opt/dst_server /home/dst
