#!/bin/bash
set -o errexit
set -o nounset
set -o pipefail

# latest_releases.sh sets LATEST_X_X_RELEASE variables
source $(dirname $0)/latest_releases.sh

# UPGRADE_MAP maps gravity version -> list of linux distros to upgrade from
declare -A UPGRADE_MAP
UPGRADE_MAP[$LATEST_BRANCH_RELEASE]="ubuntu:18"
UPGRADE_MAP[$LATEST_7_0_RELEASE]="ubuntu:18" # latest release on compatible LTS
# 6.2 and 6.3 ignored in PR builds per https://github.com/gravitational/gravity/pull/1760#pullrequestreview-437838773
# UPGRADE_MAP[$LATEST_6_3_RELEASE]="ubuntu:18" # compatible non-LTS version
# UPGRADE_MAP[$LATEST_6_2_RELEASE]="ubuntu:18" # compatible non-LTS version
# important versions in the field
UPGRADE_MAP[7.0.0]="ubuntu:16"

UPGRADE_VERSIONS=${!UPGRADE_MAP[@]}

source $(dirname $0)/utils.sh

function build_upgrade_suite {
  local suite=''
  local cluster_size='"flavor":"three","nodes":3,"role":"node"'
  for release in ${!UPGRADE_MAP[@]}; do
    for os in ${UPGRADE_MAP[$release]}; do
      suite+=$(build_upgrade_step $os $release 'overlay2' $cluster_size)
      suite+=' '
    done
  done
  echo -n $suite
}

function build_resize_suite {
  local suite=$(cat <<EOF
 resize={"to":3,"flavor":"one","nodes":1,"role":"node","state_dir":"/var/lib/telekube","os":"ubuntu:18","storage_driver":"overlay2"}
 shrink={"nodes":3,"flavor":"three","role":"node","os":"redhat:7"}
EOF
)
    echo -n $suite
}

function build_ops_install_suite {
  local suite=$(cat <<EOF
 install={"installer_url":"/installer/opscenter.tar","nodes":1,"flavor":"standalone","role":"node","os":"ubuntu:18","ops_advertise_addr":"example.com:443"}
EOF
)
  echo -n $suite
}

function build_install_suite {
  local suite=''
  local test_os="redhat:7"
  local cluster_size='"flavor":"three","nodes":3,"role":"node"'
  suite+=$(cat <<EOF
 install={${cluster_size},"os":"${test_os}","storage_driver":"overlay2"}
EOF
)
  suite+=' '
  suite+=$(build_ops_install_suite)
  echo -n $suite
}

SUITE=$(build_resize_suite)
SUITE="$SUITE $(build_upgrade_suite)"
SUITE="$SUITE $(build_install_suite)"

echo "$SUITE"

export SUITE UPGRADE_VERSIONS
