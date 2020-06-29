#!/bin/bash
set -o errexit
set -o nounset
set -o pipefail

# GIT_VERSION_BRANCH_PREFIX may be redefined as not everyone uses "origin" as their remote name
GIT_VERSION_BRANCH_PREFIX=${GIT_VERSION_BRANCH_PREFIX:-remotes/origin/version}

# LATEST_BRANCH_RELEASE gives the last release on the current branch, unless the current
# commit is a tagged release, in which case it returns the previous release.  This is to
# make sure it is always safe to upgrade from LATEST_BRANCH_RELEASE to the current build.
export LATEST_BRANCH_RELEASE=$(git describe --abbrev=0 HEAD^)
export LATEST_7_0_RELEASE=$(git describe --abbrev=0 ${GIT_VERSION_BRANCH_PREFIX}/7.0.x)
export LATEST_6_3_RELEASE=$(git describe --abbrev=0 ${GIT_VERSION_BRANCH_PREFIX}/6.3.x)
export LATEST_6_2_RELEASE=$(git describe --abbrev=0 ${GIT_VERSION_BRANCH_PREFIX}/6.2.x)

unset GIT_VERSION_BRANCH_PREFIX
