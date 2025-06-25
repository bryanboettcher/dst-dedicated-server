#!/bin/bash
export MODS_PATH=${MODS_PATH:/dst/mods}
export CONFIG_PATH=${CONFIG_PATH:/dst/config}

export INSTALL_ROOT=${$INSTALL_ROOT:/opt/dst_server}
export CLUSTER_ROOT=${CLUSTER_ROOT:-"$CONFIG_PATH/$CLUSTER_NAME"}
export SHARD_ROOT=${SHARD_ROOT:-"$CLUSTER_ROOT/$SHARD_NAME"}
export MODS_ROOT=${MODS_ROOT:-"$CLUSTER_ROOT/mods"}
export USER_ROOT=${USER_ROOT:-/home/dst}

# ensure directories exist and are permissioned correctly
./prepare.sh && \

# install the game and mods
su -c "./install.sh" dst && \

# run the game
su -c "./run.sh" --pty dst