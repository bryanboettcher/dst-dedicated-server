#!/bin/bash

cd $INSTALL_ROOT/bin64

echo "RUN.SH: "
whoami

./dontstarve_dedicated_server_nullrenderer_x64 \
  -persistent_storage_root "$CONFIG_PATH" \
  -conf_dir "" \
  -cluster "$CLUSTER_NAME" \
  -shard "$SHARD_NAME" \
  -ugc_directory "$INSTALL_ROOT/ugc_mods"
