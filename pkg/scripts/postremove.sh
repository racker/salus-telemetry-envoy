#!/bin/bash

SERVICE=telemetry-envoy

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

# debs pass upgrade, remove, or purge
# rpms pass number of versions installed, so 0 means none
if [ "$1" == "remove" ] || [ "$1" == "purge" ] || [ "$1" == "0" ]; then
  systemctl disable ${SERVICE}

  rm -f ${SYSTEMD_PATH}/system/${SERVICE}.service

  systemctl daemon-reload
fi
