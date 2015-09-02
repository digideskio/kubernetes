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

# Proposal: Component Registration

## Table of Contents

<!-- BEGIN MUNGE: GENERATED_TOC -->

- [Proposal: Component Registration](#proposal-component-registration)
  - [Table of Contents](#table-of-contents)
  - [Abstract](#abstract)
  - [Terminology](#terminology)
  - [Use Cases](#use-cases)
    - [Deployment](#deployment)
    - [Debugging](#debugging)
    - [Notification](#notification)
    - [Reporting](#reporting)
    - [Self-Healing](#self-healing)
  - [Design](#design)
    - [API Changes](#api-changes)
      - [Component API](#component-api)
      - [ComponentStatuses API](#componentstatuses-api)
    - [Kubectl (CLI)](#kubectl-cli)
    - [Storage](#storage)
    - [Status Updates](#status-updates)
  - [History](#history)
  - [Example](#example)
  - [Next Steps](#next-steps)

<!-- END MUNGE: GENERATED_TOC -->

## Abstract

Kubernetes was designed to be extensible from the start, but one factor currently limits that capability: the hardcoding of what defines a Kubernetes cluster. To reach enterprise-class capabilities (high availability, notifications, self-healing, reporting, and maintainability) Kubernetes first needs to support dynamic component registration and readiness probes.

## Terminology

To clarify, a "component" in this context is a core part of the Kubernetes cluster, required or optional, that knows about Kubernetes and exposes a method for probing the liveness and readiness of each instance.

The set of available/deployed/registered component types and/or instances may change over time.

Currently there are three types of components in a vanilla Kubernetes deployment, each with a single instance: apiserver, controller-manager, and scheduler.

Etcd should not be considered a component, because it is a dependency of the apiserver, does not know about Kubernetes, and thus cannot register itself or be expected to have both liveness and readiness probe endpoints. However, the current `/componentstatuses` endpoint includes etcd, and must continue to do so for reverse compatibility.


## Use Cases

This proposal primarily covers features that can be achieved internally by Kubernetes itself, but it's clear that there are also some desirable features that must be provided by external deployment, management, or monitoring systems. These external systems, however, require some features within Kubernetes to enable them. So while not all of these use cases can be solved by Kubernetes alone, we do want to make them possible.

### Deployment

As a k8s operator, I want a to be able to add/remove new components (instances or types) to a new or running cluster (ideally without manually registering it with the apiserver).

As a k8s operator, I want a kubectl command that allows me to validate that the cluster has been deployed and is fully functional.

As a k8s operator, I want a kubectl command that allows me to check which components are deployed/installed on a specific cluster in order to infer which features it supports.

Note: Inferring features from components isn't very useful with vanilla k8s, but becomes more useful as 3rd party or optional components are added (e.g. OpenShift) and/or if components-controller is split into multiple components.

### Debugging

As a k8s operator, I want a single endpoint (and kubectl command) where I can see the current condition of the cluster so that I can debug observed failures (dropped connections, unresponsiveness, etc.).

Note: The endpoint should contain cause where possible, but at least the current condition of the components.

### Notification

As a k8s operator, I want to be notified when the cluster requires human intervention so that I can intervene.

As an external notification system (e.g. PagerDuty), I want an endpoint that responds to regular requests with the current cluster condition so that I can determine (with my own rule set) when to notify the k8s operator.

Note: What condition requires human intervention is not a rule set that can be hard-coded, is likely to be different for each cloud platform, changes based on component configuration, and is likely to require a time-series database.

### Reporting

As a k8s operator, I want to see a report/graph of the readiness/uptime/availability of the cluster over time so that I can make/satisfy SLAs.

Note: At least requires a time-series database and probably a reporting engine.

### Self-Healing

As an k8s operator, I want the cluster to self-heal when individual components crash so that I don't get notified or need to intervene.

As a local process monitoring tool (e.g. monit), I want to be able to determine the condition of the component process I am monitoring to determine if it needs to be restarted.

As a remote container/vm monitoring tool (e.g. bosh monitor), I want to be able to determine the health of the component container/vm I am monitoring.

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

#### Component API

The following is a draft of how the Component model might look.

```
// ComponentType is the name of a group of one or more component instances
type ComponentType string

// ComponentSpec defines a component and how to verify its status
type ComponentSpec struct {
	// Type of the component
	Type ComponentType `json:"type"`
	// Periodic probe of component liveness.
	LivenessProbe *Probe `json:"livenessProbe,omitempty"`
	// Periodic probe of component readiness.
	ReadinessProbe *Probe `json:"readinessProbe,omitempty"`
}

// ComponentConditionType describes a type of component condition
type ComponentConditionType string

const (
	// ComponentAlive indicates that a component is running
	ComponentAlive ComponentConditionType = "Alive"
	// ComponentReady indicates that a component is ready for use
	ComponentReady ComponentConditionType = "Ready"
)

// ComponentCondition describes one aspect of the state of a component
type ComponentCondition struct {
	// Type of condition: Alive or Ready
	Type ComponentConditionType `json:"type"`
	// Status of the condition: True, False, or Unknown
	Status ConditionStatus `json:"status"`
	// Reason for the transition to the current status (machine-readable)
	Reason string `json:"reason,omitempty"`
	// Message that describes the current status (human-readable)
	Message string `json:"message,omitempty"`
}

// ComponentStatus describes the status of a component
type ComponentStatus struct {
	// Conditions of the component
	Conditions []ComponentCondition `json:"conditions,omitempty"`
	// LastUpdateTime of the component status, regardless of previous status.
	LastUpdateTime util.Time `json:"lastUpdateTime,omitempty"`
	// LastTransitionTime of the component status, from a different phase and/or condition
	LastTransitionTime util.Time `json:"lastTransitionTime,omitempty"`
}

// Component describes an instance of a specific micro-service, along with its definition and last known status
type Component struct {
	TypeMeta `json:",inline"`
	// Standard object's metadata.
	// More info: http://releases.k8s.io/HEAD/docs/devel/api-conventions.md#metadata
	ObjectMeta `json:"metadata,omitempty"`
	// Spec defines the behavior of a component and how to verify its status.
	// http://releases.k8s.io/HEAD/docs/devel/api-conventions.md#spec-and-status
	Spec ComponentSpec `json:"spec"`
	// Status defines the component's last known state.
	// http://releases.k8s.io/HEAD/docs/devel/api-conventions.md#spec-and-status
	Status ComponentStatus `json:"status"`
}

// ComponentList describes a list of components
type ComponentList struct {
	TypeMeta `json:",inline"`
	// Standard list metadata.
	// More info: http://releases.k8s.io/HEAD/docs/devel/api-conventions.md#types-kinds
	ListMeta `json:"metadata,omitempty"`
	// Items is a list of component objects.
	Items []Component `json:"items"`
}
```

#### ComponentStatuses API

In order the make room for the new Component models, the existing ComponentStatus models need to be renamed. This should be reverse compatible for the REST API user, but will break compilation of any third part components using the model objects directly.

The renamed ComponentStatuses API should then be deprecated. Minimally, deprecation could be done via the model comments, which are used to generate the API docs.

```
// Type and constants for component health validation.
// Deprecated: replaced by ComponentConditionType
type ComponentStatusesConditionType string

// These are the valid conditions for the component condition type.
const (
	ComponentStatusesHealthy ComponentStatusesConditionType = "Healthy"
)

// ComponentStatusesCondition describes one aspect of the state of a component.
// Deprecated: replaced by ComponentCondition
type ComponentStatusesCondition struct {
	Type    ComponentStatusesConditionType `json:"type"`
	Status  ConditionStatus                `json:"status"`
	Message string                         `json:"message,omitempty"`
	Error   string                         `json:"error,omitempty"`
}

// ComponentStatuses (and ComponentStatusesList) holds the cluster validation info.
// Deprecated: replaced by ComponentStatus
type ComponentStatuses struct {
	TypeMeta   `json:",inline"`
	ObjectMeta `json:"metadata,omitempty"`

	Conditions []ComponentStatusesCondition `json:"conditions,omitempty"`
}

// ComponentStatusesList describes a list of component statuses.
// Deprecated: replaced by ComponentList
type ComponentStatusesList struct {
	TypeMeta `json:",inline"`
	ListMeta `json:"metadata,omitempty"`

	Items []ComponentStatuses `json:"items"`
}
```

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

It was also suggested that this proposal overlaps somewhat with the Nodes API, and that nodes could also be registered as components.

There was desire expressed to delegate to or integration with the service and/or endpoints APIs to store component location, but because service endpoints are only usable within a cluster, it's not guaranteed that the components (namely the component-controller) will be able to access them. The probes also still need to have a path to the liveness/readiness endpoints, which may not be the root endpoint exposed by the endpoints/service API.

Because this proposal includes API changes, it's likely to be put into the experimental API group.

Because of the desire for reverse compatibility, the existing `/componentstatuses` endpoint will likely be deprecated and eventually removed.

<!-- BEGIN MUNGE: GENERATED_ANALYTICS -->
[![Analytics](https://kubernetes-site.appspot.com/UA-36037335-10/GitHub/docs/proposals/component-registration.md?pixel)]()
<!-- END MUNGE: GENERATED_ANALYTICS -->
