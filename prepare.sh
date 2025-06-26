#!/bin/bash

echo "PREPARE.SH: "
whoami

if [ -n "$PUID" -o -n "$PGID" ]; then
  export PUID=${PUID:-$(grep dst /etc/passwd | cut -d":" -f3)}
  export PGID=${PGID:-$(grep dst /etc/passwd | cut -d":" -f4)}
  USERHOME=$(grep dst /etc/passwd | cut -d":" -f6)

  groupmod -o -g "${PGID}" dst
  usermod -o -u "${PUID}" dst
  usermod -d "${USERHOME}" dst
fi