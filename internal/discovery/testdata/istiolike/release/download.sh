#!/bin/sh
if [ "x$(uname)" = "xDarwin" ] ; then
  OSEXT="osx"
else
  OSEXT="linux"
fi
if [ "${MESH_VERSION}" = "" ] ; then
  MESH_VERSION="$(curl -sL https://github.com/meshproj/mesh/releases | \
                  grep -o 'releases/[0-9]*.[0-9]*.[0-9]*/' | sort -V | tail -1)"
  MESH_VERSION="${MESH_VERSION##*/}"
fi
URL="https://github.com/meshproj/mesh/releases/download/${MESH_VERSION}/mesh-${MESH_VERSION}-${OSEXT}.tar.gz"
