#!/bin/bash

USER=telemetry-envoy
GROUP=telemetry-envoy
SCRIPT_DIR=/usr/lib/telemetry-envoy/scripts
DATA_DIR=/var/lib/telemetry-envoy
ETC_DIR=/etc/salus
SERVICE=telemetry-envoy

function uses_systemd {
  [[ "$(readlink /proc/1/exe)" == */systemd ]]
}

function post_install_systemd {
  if [ -d /lib/systemd ]; then
    # Debian/Ubuntu
    SYSTEMD_PATH=/lib/systemd
  elif [ -d /usr/lib/systemd ]; then
    # Redhat
    SYSTEMD_PATH=/usr/lib/systemd
  else
    echo "ERROR: unable to detect SYSTEMD_PATH"
    exit 1
  fi

  cp -f ${SCRIPT_DIR}/${SERVICE}.service ${SYSTEMD_PATH}/system/

  # load/reload service unit
  systemctl daemon-reload

  # for upgrades, restart service...only if already active
  systemctl try-restart ${SERVICE}
}

setcap CAP_SETFCAP+p /usr/local/bin/telemetry-envoy

# Only adjust top level of data directory since just need to fix up initial, empty dir.
# A recursive chown on a fully populated data directory could become time consuming.
chown ${USER}:${GROUP} ${DATA_DIR}

# telemetry-envoy owns config
chown -R ${USER}:${GROUP} ${ETC_DIR}
# ...and only telemetry-envoy can access config, since there are sensitive fields
chmod -R go-rwx ${ETC_DIR}

if uses_systemd; then
  post_install_systemd
fi
# else manual service setup is needed...for now