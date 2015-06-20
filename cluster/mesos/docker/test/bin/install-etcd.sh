#!/bin/bash

# Copyright 2014 The Kubernetes Authors All rights reserved.
#
# Licensed under the Apache License, Version 2.0 (the "License");
# you may not use this file except in compliance with the License.
# You may obtain a copy of the License at
#
#     http://www.apache.org/licenses/LICENSE-2.0
#
# Unless required by applicable law or agreed to in writing, software
# distributed under the License is distributed on an "AS IS" BASIS,
# WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
# See the License for the specific language governing permissions and
# limitations under the License.

# Installs etcd into /usr/local/bin

set -ex

ETCD_VERSION=${ETCD_VERSION:-v2.0.11}

full_name=etcd-${ETCD_VERSION}-linux-amd64
archive_url=https://github.com/coreos/etcd/releases/download/${ETCD_VERSION}/${full_name}.tar.gz

download_dir=/tmp/etcd-${ETCD_VERSION}

mkdir ${download_dir}

function cleanup {
    rm -rf ${download_dir}
}

trap cleanup EXIT

cd ${download_dir}

echo "Downloading etcd (${archive_url})..."
curl -s -L ${archive_url} | tar xvz

echo "Installing etcd (/usr/local/bin/etcd)..."
mv ./${full_name}/etcd* /usr/local/bin/