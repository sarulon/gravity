#!/bin/bash
set -o errexit
set -o nounset
set -o pipefail

# latest_releases.sh sets LATEST_X_X_RELEASE variables
source $(dirname $0)/latest_releases.sh

# UPGRADE_MAP maps gravity version -> list of linux distros to upgrade from
declare -A UPGRADE_MAP
UPGRADE_MAP[$LATEST_BRANCH_RELEASE]="centos:7 debian:9 ubuntu:18"
UPGRADE_MAP[$LATEST_7_0_RELEASE]="centos:7 debian:9 ubuntu:18" # compatible LTS version
UPGRADE_MAP[$LATEST_6_3_RELEASE]="redhat:7" # compatible non-LTS version
UPGRADE_MAP[$LATEST_6_2_RELEASE]="redhat:7" # compatible non-LTS version
# important versions in the field
UPGRADE_MAP[7.0.0]="ubuntu:16"
# UPGRADE_MAP[6.3.0]="ubuntu:16"  # disabled due to https://github.com/gravitational/gravity/issues/1009
UPGRADE_MAP[6.2.0]="ubuntu:16"

UPGRADE_VERSIONS=${!UPGRADE_MAP[@]}

source $(dirname $0)/utils.sh

function build_upgrade_size_suite {
  local suite=''
  local from_tarball=/$(semver_to_tarball $LATEST_7_0_RELEASE)
  local to_tarball='/installer/telekube.tar'
  local os="redhat:7"
  local cluster_sizes=( \
    '"flavor":"three","nodes":3,"role":"node"' \
    '"flavor":"six","nodes":6,"role":"node"' \
    '"flavor":"one","nodes":1,"role":"node"')
  for size in ${cluster_sizes[@]}; do
      suite+=$(build_upgrade_step $from_tarball $to_tarball $os $size)
    suite+=' '
  done
  echo -n $suite
}

function build_upgrade_to_suite {
  local suite=''
  local to_tarball='/installer/telekube.tar'
  local cluster_size='"flavor":"three","nodes":3,"role":"node"'
  for release in ${!UPGRADE_MAP[@]}; do
    local from_tarball=/$(semver_to_tarball $release)
    for os in ${UPGRADE_MAP[$release]}; do
      suite+=$(build_upgrade_step $from_tarball $to_tarball $os $cluster_size)
      suite+=' '
    done
  done
  echo -n $suite
}

function build_resize_suite {
  local suite=$(cat <<EOF
 resize={"to":3,"flavor":"one","nodes":1,"role":"node","state_dir":"/var/lib/telekube","os":"ubuntu:18","storage_driver":"overlay2"}
 resize={"to":6,"flavor":"three","nodes":3,"role":"node","state_dir":"/var/lib/telekube","os":"ubuntu:18","storage_driver":"overlay2"}
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
  local test_os="redhat:7 debian:9 ubuntu:16 ubuntu:18"
  local cluster_sizes=( \
    '"flavor":"three","nodes":3,"role":"node"' \
    '"flavor":"six","nodes":6,"role":"node"')
  for os in $test_os; do
    for size in ${cluster_sizes[@]}; do
      suite+=$(cat <<EOF
 install={${size},"os":"${os}","storage_driver":"overlay2"}
EOF
)
      suite+=' '
    done
  done
  suite+=$(build_ops_install_suite)
  echo -n $suite
}

SUITE="$(build_install_suite)"
SUITE+=" $(build_resize_suite)"
SUITE+=" $(build_upgrade_to_suite)"
SUITE+=" $(build_upgrade_size_suite)"

echo "$SUITE"

export SUITE UPGRADE_VERSIONS
