#!/bin/bash

SERVICE=telemetry-envoy

function uses_systemd {
  [[ "$(readlink /proc/1/exe)" == */systemd ]]
}

function pre_remove_systemd {
    systemctl stop ${SERVICE}.service
}

# debs pass upgrade, remove, or purge
# rpms pass number of versions installed, so 0 means none remain
if [ "$1" == "remove" ] || [ "$1" == "purge" ] || [ "$1" == "0" ]; then

  if uses_systemd; then
    pre_remove_systemd
  fi
  # else manual service stopping is needed

fi

