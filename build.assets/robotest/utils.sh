#!/bin/bash
set -o errexit
set -o nounset
set -o pipefail

function semver_to_tarball {
  local version=${1:?specify a version}
  echo "telekube_${version}.tar"
}

function build_upgrade_step {
  local usage="$FUNCNAME os release storage-driver cluster-size"
  local os=${1:?$usage}
  local release=${2:?$usage}
  local storage_driver=${3:?$usage}
  local cluster_size=${4:?$usage}
  local service_opts='"service_uid":997,"service_gid":994' # see issue #1279
  local suite=''
  suite+=$(cat <<EOF
 upgrade={${cluster_size},${service_opts},"os":"${os}","storage_driver":"${storage_driver}","from":"/$(semver_to_tarball ${release})"}
EOF
)
  echo $suite
}
