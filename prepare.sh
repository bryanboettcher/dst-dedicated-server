#!/bin/bash

echo "PREPARE.SH: "
whoami

if [ -n "$PUID" -o -n "$PGID" ]; then
  FORCE_IDS=1
fi

if [ -n "$FORCE_IDS" ]; then
  export PUID=${PUID:-$(grep dst /etc/passwd | cut -d":" -f3)}
  export PGID=${PGID:-$(grep dst /etc/passwd | cut -d":" -f4)}
  USERHOME=$(grep dst /etc/passwd | cut -d":" -f6)

  groupmod -o -g "${PGID}" dst
  usermod -o -u "${PUID}" dst
  usermod -d "${USERHOME}" dst
fi

for path in "$INSTALL_ROOT" "$CLUSTER_ROOT" "$SHARD_ROOT" "$MODS_ROOT" "$USER_ROOT"; do
  mkdir -p $path && chown dst:dst -R $path && chmod 0775 -R $path
done