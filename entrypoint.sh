#!/bin/sh

set -x
set -e

[ -z "${SIGNAL_CLI_CONFIG_DIR}" ] && echo "SIGNAL_CLI_CONFIG_DIR environmental variable needs to be set! Aborting!" && exit 1;

# Show warning on docker exec
cat <<EOF >> /root/.bashrc
echo "If you want to use signal-cli directly, don't forget to specify the config directory. e.g: \"signal-cli --config ${SIGNAL_CLI_CONFIG_DIR}\""
EOF

cap_prefix="-cap_"
caps="$cap_prefix$(seq -s ",$cap_prefix" 0 $(cat /proc/sys/kernel/cap_last_cap))"

# TODO: check mode
if [ "$MODE" = "json-rpc" ]
then
/usr/bin/jsonrpc2-helper
service supervisor start
supervisorctl start all
fi

export HOST_IP=$(hostname -I | awk '{print $1}')

# Start API as signal-api user
exec signal-cli-rest-api -signal-cli-config=${SIGNAL_CLI_CONFIG_DIR}
