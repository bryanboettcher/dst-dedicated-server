#!/bin/bash

echo "INSTALL.SH: "
whoami

steamcmd \
    +@ShutdownOnFailedCommand 1 \
    +@NoPromptForPassword 1 \
    +login anonymous \
    +force_install_dir $INSTALL_ROOT \
    +app_update 343050 validate \
    +quit

# Copy dedicated_server_mods_setup.lua to the server install's mods directory
# (this is where DST looks for it to know which workshop mods to download)
SERVER_MODS_SETUP="$MODS_PATH/dedicated_server_mods_setup.lua"
if [ -f "$SERVER_MODS_SETUP" ]; then
  cp "$SERVER_MODS_SETUP" "$INSTALL_ROOT/mods/"
fi

# Copy modoverrides.lua (cluster-wide default) if it exists
MOD_OVERRIDE="$MODS_PATH/modoverrides.lua"
if [ -f "$MOD_OVERRIDE" ]; then
  cp "$MOD_OVERRIDE" "$SHARD_ROOT/"
fi

# Copy $SHARD_NAME/modoverrides.lua (shard-specific override) if it exists
SHARD_MOD_OVERRIDE="$MODS_PATH/$SHARD_NAME/modoverrides.lua"
if [ -f "$SHARD_MOD_OVERRIDE" ]; then
  cp "$SHARD_MOD_OVERRIDE" "$SHARD_ROOT/"
fi
