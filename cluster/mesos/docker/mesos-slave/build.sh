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

# Builds a docker image that contains the kubernetes-mesos binaries.

set -o errexit
set -o nounset
set -o pipefail

IMAGE_REPO=${IMAGE_REPO:-mesosphere/kubernetes-mesos-slave}
IMAGE_TAG=${IMAGE_TAG:-latest}

script_dir=$(cd $(dirname "${BASH_SOURCE}") && pwd -P)
cd ${script_dir}

echo "Building docker image ${IMAGE_REPO}:${IMAGE_TAG}"
exec docker build -t ${IMAGE_REPO}:${IMAGE_TAG} "$@" .