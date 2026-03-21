#!/bin/bash
export INSTALL_ROOT=/opt/dst_server
export MODS_PATH=/dst/mods
export CONFIG_PATH=/dst/config
export CLUSTER_NAME=${CLUSTER_NAME:-DST_Cluster}
export SHARD_NAME=${SHARD_NAME:-Overworld}

export CLUSTER_ROOT=${CLUSTER_ROOT:-"$CONFIG_PATH/$CLUSTER_NAME"}
export SHARD_ROOT=${SHARD_ROOT:-"$CLUSTER_ROOT/$SHARD_NAME"}
export USER_ROOT=${USER_ROOT:-/home/dst}

# ensure directories exist and are permissioned correctly
./prepare.sh && \

# install the game and mods
su -c "./install.sh" dst && \

# run the game
su -c "./run.sh" --pty dst
