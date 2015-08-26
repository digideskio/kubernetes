<!-- BEGIN MUNGE: UNVERSIONED_WARNING -->

<!-- BEGIN STRIP_FOR_RELEASE -->

<img src="http://kubernetes.io/img/warning.png" alt="WARNING"
     width="25" height="25">
<img src="http://kubernetes.io/img/warning.png" alt="WARNING"
     width="25" height="25">
<img src="http://kubernetes.io/img/warning.png" alt="WARNING"
     width="25" height="25">
<img src="http://kubernetes.io/img/warning.png" alt="WARNING"
     width="25" height="25">
<img src="http://kubernetes.io/img/warning.png" alt="WARNING"
     width="25" height="25">

<h2>PLEASE NOTE: This document applies to the HEAD of the source tree</h2>

If you are using a released version of Kubernetes, you should
refer to the docs that go with that version.

<strong>
The latest 1.0.x release of this document can be found
[here](http://releases.k8s.io/release-1.0/docs/proposals/component-registration.md).

Documentation for other releases can be found at
[releases.k8s.io](http://releases.k8s.io).
</strong>
--

<!-- END STRIP_FOR_RELEASE -->

<!-- END MUNGE: UNVERSIONED_WARNING -->

### Proposal: Component Registration

## Abstract

Kubernetes was designed to be extensible from the start, but one factor currently limits that capability: the hardcoding of what defines a Kubernetes cluster. To reach enterprise-class capabilities (high availability, notifications, self-healing, reporting, and maintainability) Kubernetes first needs to support dynamic component registration and readiness probes.


## Use Cases

This proposal primarily covers features that can be achieved internally by Kubernetes itself, but it's clear that there are also some desirable features that must be provided by external deployment, management, or monitoring systems. These external systems, however, require some features within Kubernetes to enable them. So while not all of these use cases can be solved by Kubernetes alone, we do want to make them possible.

### Deployment

As a k8s operator, I want a kubectl command that allows me to validate that the cluster has been deployed and is fully functional.

As a k8s operator, I want a kubectl command that allows me to check which components are deployed/installed on a specific cluster in order to infer which features it supports.

Note: Inferring features from components isn't very useful with vanilla k8s, but becomes more useful as 3rd party or optional components are added (e.g. OpenShift) and/or if components-controller is split into multiple components.

### Debugging

As a k8s operator, I want a single endpoint (and kubectl command) where I can see the current condition of the cluster so that I can debug observed failures (dropped connections, unresponsiveness, etc.).

Note: The endpoint should contain cause where possible, but at least the current condition of the components.

#### Notification

As a k8s operator, I want to be notified when the cluster requires human intervention so that I can intervene.

As an external notification system (e.g. PagerDuty), I want an endpoint that responds to regular requests with the current cluster condition so that I can determine (with my own rule set) when to notify the k8s operator.

Note: What condition requires human intervention is not a rule set that can be hard-coded, is likely to be different for each cloud platform, changes based on component configuration, and is likely to require a time-series database.

#### Reporting

As a k8s operator, I want to see a report/graph of the readiness/uptime/availability of the cluster over time so that I can make/satisfy SLAs.

Note: At least requires a time-series database and probably a reporting engine.

#### Self-Healing

As an k8s operator, I want the cluster to self-heal when individual components crash so that I don't get notified or need to intervene.

As a local process monitoring tool (e.g. monit), I want to be able to determine the condition of the component process I am monitoring to determine if it needs to be restarted.

As a remote container/vm monitoring tool (e.g. bosh monitor), I want to be able to determine the heath of the component container/vm I am monitoring.

Note: Self-healing requires knowing how to deploy the cluster components, which is different for each cloud platform and changes based on component configuration.


## Design

### API Changes

Add a single component status endpoint (`/components/status`) that lists the last known condition of all the k8s components.

Add a single component endpoint (`/components`) that lists the spec/status of all the k8s components.

Add individual component endpoints (`/component/status/<name>`) that show the status of each k8s component.

Add a LivenessProbe to the component spec that describes how to probe whether the component is running (including during bootstrapping/startup).

Add a ReadinessProbe to the component spec that describes how to probe whether the component is fully functional and ready to be used.

Group components by type, to allow multiple instances of each type, to enable HA cluster deployments.

Use the existing name generator with component type as the prefix to ensure component names are unique. Return the generated name in the create response.

### Kubectl (CLI)

Add a `kubectl get components` command to list the components and their last known conditions.

### Storage

Store component spec/status in etcd, with an apiserver storage wrapper.

### Status Updates

Add a component-controller that regularly probes registered components and updates their stored last known status.

Add self-registration to the components that uses the new Components API (Create call) when the component starts.

Add self-deregistration to the components that uses the new Components API (Remove call) when the component stops gracefully (e.g. after receiving SIGTERM).

Add self-updating to the components that uses the new Components API (Update call) when the component errors/panics.

Use a TCP probe to check liveness, because it indicates a server is listening on the expected host/path, even if its responses are unhealthy.

Update individual component `/healthz` endpoints to perform as an indicator of readiness, not just liveness. Readiness should include transitively checking the readiness of external dependencies (e.g. etcd or other databases).


## History

The `/componentstatuses` endpoint and associated `kubectl get componentstatuses` CLI command were added to facilitate retrieving the status of the cluster.

Limitations of the current implementation:

- Components are hardcoded
  - Components assumed to be localhost accessible
    - Not required/enforced by any other method
    - Incompatible with HA deployments (deploying components on different machines)
    - Incompatible with non-host docker networking (deploying components in different containers)
  - Components assumed to use default ports
    - Incompatible with custom port configurations
  - Components assumed to have a root `/healthz` endpoint
    - Incompatible with components behind reverse proxies with path prefixes
- Etcd is included as a component
  - Inconsistent with convention that `/healthz` success implies usability (aka, dependencies are also usable, even if not completely healthy)
  - Indicates a k8s cluster problem even if only 1 of 3 HA Etcd nodes are down
- Each http request triggers 5 other requests
  - Denial of Service risk from traffic amplification
- Simple probe impl
  - No caching
  - Requests made in serial
  - Inconsistent with other registry/storage APIs and implementations


## Example

The following terminal commands are an example of how the `kubectl get components` command might work, as implemented by PR #12324.

Notes:
- This examples use the [mesos/docker cluster](../getting-started-guides/mesos-docker.md).
- This example does not include self-deregistration on graceful shutdown, because none of the components support graceful shutdown yet.
- This example uses a 5s probe period and a 5s probe timeout with no retires. So the status resolution is about 5s.

```
$./cluster/kube-up.sh
...
Kubernetes master is running at https://172.17.0.41:6443
KubeDNS is running at https://172.17.0.41:6443/api/v1/proxy/namespaces/kube-system/services/kube-dns
KubeUI is running at https://172.17.0.41:6443/api/v1/proxy/namespaces/kube-system/services/kube-ui

$ ./cluster/kubectl.sh get components
NAME                            TYPE                      STATUS
k8sm-controller-manager-78fni   k8sm-controller-manager   Alive,Ready
k8sm-scheduler-jn6p9            k8sm-scheduler            Alive,Ready
kube-apiserver-gtlzk            kube-apiserver            Alive,Ready

$ docker-compose -f ./cluster/mesos/docker/docker-compose.yml stop scheduler
Stopping docker_scheduler_1...

$ ./cluster/kubectl.sh get components
NAME                            TYPE                      STATUS
k8sm-controller-manager-78fni   k8sm-controller-manager   Alive,Ready
k8sm-scheduler-jn6p9            k8sm-scheduler            Alive,Ready
kube-apiserver-gtlzk            kube-apiserver            Alive,Ready

$ ./cluster/kubectl.sh get components
NAME                            TYPE                      STATUS
k8sm-controller-manager-78fni   k8sm-controller-manager   Alive,Ready
k8sm-scheduler-jn6p9            k8sm-scheduler            NotAlive,NotReady
kube-apiserver-gtlzk            kube-apiserver            Alive,Ready

$ docker-compose -f ./cluster/mesos/docker/docker-compose.yml start scheduler
Starting docker_scheduler_1...

$ ./cluster/kubectl.sh get components
NAME                            TYPE                      STATUS
k8sm-controller-manager-78fni   k8sm-controller-manager   Alive,Ready
k8sm-scheduler-h3st3            k8sm-scheduler            Alive,NotReady
k8sm-scheduler-jn6p9            k8sm-scheduler            NotAlive,NotReady
kube-apiserver-gtlzk            kube-apiserver            Alive,Ready

$ ./cluster/kubectl.sh get components
NAME                            TYPE                      STATUS
k8sm-controller-manager-78fni   k8sm-controller-manager   Alive,Ready
k8sm-scheduler-h3st3            k8sm-scheduler            Alive,Ready
k8sm-scheduler-jn6p9            k8sm-scheduler            NotAlive,NotReady
kube-apiserver-gtlzk            kube-apiserver            Alive,Ready
```

## Next Steps

It's possible the above proposal could also be used to register AddOns that have been deployed with k8s, but then it becomes desirable to be able to use the pods API to proxy liveness/readiness probes, which increases complexity.

There was desire expressed to delegate to the service and/or endpoints APIs to store component location, but because service endpoints are only usable within a cluster, it's not guaranteed that the components (namely the component-controller) will be able to access them. The probes also still need to have a path to the liveness/readiness endpoints, which may not be the root endpoint exposed by the endpoints/service API.

Because this proposal includes API changes, it's likely to be put into the experimental API group.

Because of the desire for reverse compatibility, the existing `/componentstatuses` endpoint will likely be deprecated and eventually removed.

<!-- BEGIN MUNGE: GENERATED_ANALYTICS -->
[![Analytics](https://kubernetes-site.appspot.com/UA-36037335-10/GitHub/docs/proposals/component-registration.md?pixel)]()
<!-- END MUNGE: GENERATED_ANALYTICS -->
