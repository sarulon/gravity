#!/bin/bash
set -o errexit
set -o nounset
set -o pipefail

function semver_to_tarball {
  local version=${1:?specify a version}
  echo "telekube_${version}.tar"
}

function build_upgrade_step {
  local usage="$FUNCNAME from_tarball to_tarball os cluster-size"
  local from_tarball=${1:?$usage}
  local to_tarball=${2:?$usage}
  local os=${3:?$usage}
  local cluster_size=${4:?$usage}
  local storage_driver='"storage_driver":"overlay2"'
  local service_opts='"service_uid":997,"service_gid":994' # see issue #1279
  local suite=''
  suite+=$(cat <<EOF
 upgrade={${cluster_size},"os":"${os}","from":"$from_tarball","installer_url":"$to_tarball",${service_opts},${storage_driver}}
EOF
)
  echo $suite
}
