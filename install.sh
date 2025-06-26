#!/bin/bash

echo "INSTALL.SH: "
whoami

steamcmd.sh \
    +@ShutdownOnFailedCommand 1 \
    +@NoPromptForPassword 1 \
    +login anonymous \
    +force_install_dir $INSTALL_ROOT \
    +app_update 343050 validate \
    +quit

# Copy dedicated_server_mods_setup.lua
SERVER_MODS_SETUP="$MODS_PATH/dedicated_server_mods_setup.lua"
if [ -f "$SERVER_MODS_SETUP" ]; then
  cp "$SERVER_MODS_SETUP" "$MODS_ROOT/"
fi

# Copy modoverrides.lua if it exists
MOD_OVERRIDE="$MODS_PATH/modoverrides.lua"
if [ -f "$MOD_OVERRIDE" ]; then
  cp "$MOD_OVERRIDE" "$SHARD_ROOT/"
fi

# Copy $SHARD_NAME/modoverrides.lua if it exists
SHARD_MOD_OVERRIDE="$MODS_PATH/$SHARD_NAME/modoverrides.lua"
if [ -f "$SHARD_MOD_OVERRIDE" ]; then
  cp "$SHARD_MOD_OVERRIDE" "$SHARD_ROOT/"
fi