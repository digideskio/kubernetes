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

# Example:
# export KUBERNETES_PROVIDER=mesos/docker
# go run hack/e2e.go -v -up -check_cluster_size=false
# go run hack/e2e.go -v -test -check_version_skew=false
# go run hack/e2e.go -v -down

set -o errexit
set -o nounset
set -o pipefail

KUBE_ROOT=$(cd "$(dirname "${BASH_SOURCE}")/../../.." && pwd)
provider_root="${KUBE_ROOT}/cluster/${KUBERNETES_PROVIDER}"

source "${provider_root}/${KUBE_CONFIG_FILE-"config-default.sh"}"
source "${KUBE_ROOT}/cluster/common.sh"
source "${KUBE_ROOT}/cluster/kube-util.sh" #default no-op method impls


# Run kubernetes scripts inside docker.
# This bypasses the need to set up network routing when running docker in a VM (e.g. boot2docker).
# Trap signals and kills the docker container for better signal handing
function cluster::mesos::docker::run_in_docker {
  entrypoint="$1"
  if [[ "${entrypoint}" = "./"* ]]; then
    # relative to project root
    entrypoint="/go/src/github.com/GoogleCloudPlatform/kubernetes/${entrypoint}"
  fi
  shift
  args="$@"

  container_id=$(
    docker run \
      -d \
      -e "KUBERNETES_PROVIDER=${KUBERNETES_PROVIDER}" \
      -v "${KUBE_ROOT}:/go/src/github.com/GoogleCloudPlatform/kubernetes" \
      -v "$(dirname ${KUBECONFIG}):/root/.kube" \
      -v "/var/run/docker.sock:/var/run/docker.sock" \
      --link docker_mesosmaster1_1:mesosmaster1 \
      --link docker_mesosslave1_1:mesosslave1 \
      --link docker_mesosslave2_1:mesosslave2 \
      --link docker_apiserver_1:apiserver \
      --entrypoint="${entrypoint}" \
      mesosphere/kubernetes-mesos-test \
      ${args}
  )

  docker logs -f ${container_id} &

  # trap and kill for better signal handing
  trap 'echo "Killing container ${container_id}" 1>&2 && docker kill ${container_id}' INT TERM
  exit_status=$(docker wait ${container_id})
  trap - INT TERM

  if [ "$exit_status" != 0 ]; then
    echo "Exited ${exit_status}" 1>&2
  fi

  docker rm -f ${container_id} > /dev/null

  return ${exit_status}
}

# Generate kubeconfig data for the created cluster.
# Assumed vars:
#   KUBE_ROOT
#   KUBECONFIG or DEFAULT_KUBECONFIG
#   KUBE_MASTER_IP
#
# Vars set:
#   CONTEXT
#   KUBECONFIG
function create-kubeconfig {
  local kubectl="${KUBE_ROOT}/cluster/kubectl.sh"

  export CONTEXT="${KUBERNETES_PROVIDER}"
  export KUBECONFIG=${KUBECONFIG:-$DEFAULT_KUBECONFIG}
  # KUBECONFIG determines the file we write to, but it may not exist yet
  if [[ ! -e "${KUBECONFIG}" ]]; then
    mkdir -p $(dirname "${KUBECONFIG}")
    touch "${KUBECONFIG}"
  fi
  "${kubectl}" config set-cluster "${CONTEXT}" --server="http://${KUBE_MASTER_IP}"
  "${kubectl}" config set-context "${CONTEXT}" --cluster="${CONTEXT}" --user="${CONTEXT}"
  "${kubectl}" config use-context "${CONTEXT}" --cluster="${CONTEXT}"

   echo "Wrote config for ${CONTEXT} to ${KUBECONFIG}" 1>&2
}

# Perform preparations required to run e2e tests
function prepare-e2e {
  echo "TODO: prepare-e2e" 1>&2
}

# Execute prior to running tests to build a release if required for env
function test-build-release {
  # Make a release
  export KUBERNETES_CONTRIB=mesos
  export KUBE_RELEASE_RUN_TESTS=N
  "${KUBE_ROOT}/build/release.sh"
}

# Must ensure that the following ENV vars are set
function detect-master {
  #  echo "KUBE_MASTER: $KUBE_MASTER" 1>&2

  docker_id=$(docker ps --filter="name=docker_apiserver" --quiet)
  if [[ "${docker_id}" == *'\n'* ]]; then
    echo "ERROR: Multiple API Servers running in docker" 1>&2
    return 1
  fi

  details_json=$(docker inspect ${docker_id})
  master_ip=$(jq '.[0].NetworkSettings.IPAddress' --raw-output <<< "$details_json")
  master_port=$(jq '.[0].NetworkSettings.Ports[][].HostPort' --raw-output <<< "$details_json")

  KUBE_MASTER_IP="${master_ip}:${master_port}"
  KUBE_SERVER="http://${KUBE_MASTER_IP}"

  echo "KUBE_MASTER_IP: $KUBE_MASTER_IP" 1>&2
}

# Get minion IP addresses and store in KUBE_MINION_IP_ADDRESSES[]
# These Mesos slaves MAY host Kublets,
# but might not have a Kublet running unless a kubernetes task has been scheduled on them.
function detect-minions {
  docker_ids=$(docker ps --filter="name=docker_mesosslave" --quiet)
  while read -r docker_id; do

    details_json=$(docker inspect ${docker_id})
    minion_ip=$(jq '.[0].NetworkSettings.IPAddress' --raw-output <<< "$details_json")
    minion_port=$(jq '.[0].NetworkSettings.Ports[][].HostPort' --raw-output <<< "$details_json")
    #TODO: filter for 505* port

    KUBE_MINION_IP_ADDRESSES+=("${minion_ip}")
#    KUBE_MINION_IP_ADDRESSES+=("${minion_ip}:${minion_port}")

  done <<< "$docker_ids"
  echo "KUBE_MINION_IP_ADDRESSES: [${KUBE_MINION_IP_ADDRESSES[*]}]" 1>&2
}

# Verify prereqs on host machine
function verify-prereqs {
  echo "TODO: verify-prereqs" 1>&2
  # Verify that docker, docker-compose, and jq exist
  # Verify that all the right docker images exist:
  # mesosphere/kubernetes-mesos-test, etc.
}

# Instantiate a kubernetes cluster
function kube-up {
  echo "Building Docker images" 1>&2
  # TODO: version images (k8s version, git sha, and dirty state) to avoid re-building them every time.
  "${provider_root}/km/build.sh"
  "${provider_root}/mesos-slave/build.sh"
  "${provider_root}/test/build.sh"

  echo "Starting ${KUBERNETES_PROVIDER} cluster" 1>&2
  pushd "${provider_root}" > /dev/null
  docker-compose up -d
  popd > /dev/null

  detect-master
  detect-minions

  export KUBECONFIG=${KUBECONFIG:-${DEFAULT_KUBECONFIG}}

  # await-health-check could be run locally, but it would require GNU timeout installed on mac...
  # "${provider_root}/common/bin/await-health-check" -t=120 ${KUBE_SERVER}/healthz
  cluster::mesos::docker::run_in_docker await-health-check -t=120 http://apiserver:8888/healthz

  create-kubeconfig

  echo "Deploying Addons" 1>&2
  "${provider_root}/deploy-addons.sh"
}

function validate-cluster {
  echo "Validating ${KUBERNETES_PROVIDER} cluster" 1>&2

  # Do not validate cluster size. There will be zero k8s minions until a pod is created.

  # Validate cluster reachability and responsiveness
  "${KUBE_ROOT}/cluster/kubectl.sh" cluster-info
  exit_status=$?

  if [ ${exit_status} = 0 ]; then
    echo "VALIDATION SUITE SUCCESSFUL" 1>&2
  else
    echo "VALIDATION SUITE FAILED" 1>&2
  fi
}

# Delete a kubernetes cluster
function kube-down {
  echo "Stopping ${KUBERNETES_PROVIDER} cluster" 1>&2
  # TODO: cleanup containers owned by kubernetes
  # Nuclear option: docker ps -q -a | xargs docker rm -f
  pushd "${provider_root}" > /dev/null
  docker-compose stop
  docker-compose rm -f
  popd > /dev/null
  # TODO: delete /Users/<user>/.kube/config ?
}

function test-setup {
  echo "Building required docker images" 1>&2
  "${KUBE_ROOT}/cluster/mesos/docker/km/build.sh"
  "${KUBE_ROOT}/cluster/mesos/docker/test/build.sh"
  "${KUBE_ROOT}/cluster/mesos/docker/mesos-slave/build.sh"
}

# Execute after running tests to perform any required clean-up
function test-teardown {
  echo "test-teardown" 1>&2
  kube-down
}

## Below functions used by hack/e2e-suite/services.sh

# SSH to a node by name or IP ($1) and run a command ($2).
function ssh-to-node {
  echo "TODO: ssh-to-node" 1>&2
}

# Restart the kube-proxy on a node ($1)
function restart-kube-proxy {
  echo "TODO: restart-kube-proxy" 1>&2
}

# Restart the apiserver
function restart-apiserver {
  echo "TODO: restart-apiserver" 1>&2
}
