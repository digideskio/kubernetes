# NAME
kube-apiserver \- Provides the API for kubernetes orchestration.

# SYNOPSIS
**kube-apiserver** [OPTIONS]

# DESCRIPTION

The **kubernetes** API server validates and configures data for 3 types of objects: pods, services, and replicationcontrollers. Beyond just servicing REST operations, the API Server does two other things as well: 1. Schedules pods to worker nodes. Right now the scheduler is very simple. 2. Synchronize pod information (where they are, what ports they are exposing) with the service configuration.

The the kube-apiserver several options.

# OPTIONS
*       **--address=127.0.0.1**: DEPRECATED: see --insecure-bind-address instead
*       **--admission-control="AlwaysAdmit"**: Ordered list of plug-ins to do admission control of resources into cluster. Comma-delimited list of: NamespaceAutoProvision, DenyExecOnPrivileged, AlwaysAdmit, LimitRanger, AlwaysDeny, ServiceAccount, SecurityContextDeny, ResourceQuota, NamespaceLifecycle, NamespaceExists
*       **--admission-control-config-file=""**: File with admission control configuration.
*       **--advertise-address=<nil>**: The IP address on which to advertise the apiserver to members of the cluster. This address must be reachable by the rest of the cluster. If blank, the --bind-address will be used. If --bind-address is unspecified, the host's default interface will be used.
*       **--allow-privileged=false**: If true, allow privileged containers.
*       **--alsologtostderr=false**: log to standard error as well as files
*       **--api-burst=200**: API burst amount for the read only port
*       **--api-prefix="/api"**: The prefix for API requests on the server. Default '/api'.
*       **--api-rate=10**: API rate limit as QPS for the read only port
*       **--authorization-mode="AlwaysAllow"**: Selects how to do authorization on the secure port.  One of: AlwaysAllow,AlwaysDeny,ABAC
*       **--authorization-policy-file=""**: File with authorization policy in csv format, used with --authorization-mode=ABAC, on the secure port.
*       **--basic-auth-file=""**: If set, the file that will be used to admit requests to the secure port of the API server via http basic authentication.
*       **--bind-address=0.0.0.0**: The IP address on which to serve the --read-only-port and --secure-port ports. The associated interface(s) must be reachable by the rest of the cluster, and by CLI/web clients. If blank, all interfaces will be used (0.0.0.0).
*       **--cert-dir="/var/run/kubernetes"**: The directory where the TLS certs are located (by default /var/run/kubernetes). If --tls-cert-file and --tls-private-key-file are provided, this flag will be ignored.
*       **--client-ca-file=""**: If set, any request presenting a client certificate signed by one of the authorities in the client-ca-file is authenticated with an identity corresponding to the CommonName of the client certificate.
*       **--cloud-config=""**: The path to the cloud provider configuration file.  Empty string for no configuration file.
*       **--cloud-provider=""**: The provider for cloud services.  Empty string for no provider.
*       **--cluster-name="kubernetes"**: The instance prefix for the cluster
*       **--cors-allowed-origins=[]**: List of allowed origins for CORS, comma separated.  An allowed origin can be a regular expression to support subdomain matching.  If this list is empty CORS will not be enabled.
*       **--etcd-config=""**: The config file for the etcd client. Mutually exclusive with -etcd-servers.
*       **--etcd-prefix="/registry"**: The prefix for all resource paths in etcd.
*       **--etcd-servers=[]**: List of etcd servers to watch (http://ip:port), comma separated. Mutually exclusive with -etcd-config
*       **--event-ttl=1h0m0s**: Amount of time to retain events. Default 1 hour.
*       **--external-hostname=""**: The hostname to use when generating externalized URLs for this master (e.g. Swagger API Docs.)
*       **--httptest.serve=**: if non-empty, httptest.NewServer serves on this address and blocks
*       **--insecure-bind-address=127.0.0.1**: The IP address on which to serve the --insecure-port (set to 0.0.0.0 for all interfaces). Defaults to localhost.
*       **--insecure-port=8080**: The port on which to serve unsecured, unauthenticated access. Default 8080. It is assumed that firewall rules are set up such that this port is not reachable from outside of the cluster and that port 443 on the cluster's public address is proxied to this port. This is performed by nginx in the default setup.
*       **--kubelet-certificate-authority=""**: Path to a cert. file for the certificate authority.
*       **--kubelet-client-certificate=""**: Path to a client key file for TLS.
*       **--kubelet-client-key=""**: Path to a client key file for TLS.
*       **--kubelet-https=true**: Use https for kubelet connections
*       **--kubelet-port=10250**: Kubelet port
*       **--kubelet-timeout=5s**: Timeout for kubelet operations
*       **--log-backtrace-at=**:0: when logging hits line file:N, emit a stack trace
*       **--log-dir=**: If non-empty, write log files in this directory
*       **--log-flush-frequency=5s**: Maximum number of seconds between log flushes
*       **--logtostderr=true**: log to standard error instead of files
*       **--long-running-request-regexp="(/|^)((watch|proxy)(/|$)|(logs|portforward|exec)/?$)"**: A regular expression matching long running requests which should be excluded from maximum inflight request handling.
*       **--master-service-namespace="default"**: The namespace from which the kubernetes master services should be injected into pods
*       **--max-requests-inflight=400**: The maximum number of requests in flight at a given time.  When the server exceeds this, it rejects requests.  Zero for no limit.
*       **--min-request-timeout=1800**: An optional field indicating the minimum number of seconds a handler must keep a request open before timing it out. Currently only honored by the watch request handler, which picks a randomized value above this number as the connection timeout, to spread out load.
*       **--old-etcd-prefix="/registry"**: The previous prefix for all resource paths in etcd, if any.
*       **--port=8080**: DEPRECATED: see --insecure-port instead
*       **--profiling=true**: Enable profiling via web interface host:port/debug/pprof/
*       **--public-address-override=0.0.0.0**: DEPRECATED: see --bind-address instead
*       **--runtime-config=**: A set of key=value pairs that describe runtime configuration that may be passed to the apiserver. api/<version> key can be used to turn on/off specific api versions. api/all and api/legacy are special keys to control all and legacy api versions respectively.
*       **--secure-port=6443**: The port on which to serve HTTPS with authentication and authorization. If 0, don't serve HTTPS at all.
*       **--service-account-key-file=""**: File containing PEM-encoded x509 RSA private or public key, used to verify ServiceAccount tokens. If unspecified, --tls-private-key-file is used.
*       **--service-account-lookup=false**: If true, validate ServiceAccount tokens exist in etcd as part of authentication.
*       **--service-cluster-ip-range=<nil>**: A CIDR notation IP range from which to assign service cluster IPs. This must not overlap with any IP ranges assigned to nodes for pods.
*       **--service-node-port-range=**: A port range to reserve for services with NodePort visibility.  Example: '30000-32767'.  Inclusive at both ends of the range.
*       **--ssh-keyfile=""**: If non-empty, use secure SSH proxy to the nodes, using this user keyfile
*       **--ssh-user=""**: If non-empty, use secure SSH proxy to the nodes, using this user name
*       **--stderrthreshold=2**: logs at or above this threshold go to stderr
*       **--storage-version=""**: The version to store resources with. Defaults to server preferred
*       **--tls-cert-file=""**: File containing x509 Certificate for HTTPS.  (CA cert, if any, concatenated after server cert). If HTTPS serving is enabled, and --tls-cert-file and --tls-private-key-file are not provided, a self-signed certificate and key are generated for the public address and saved to /var/run/kubernetes.
*       **--tls-private-key-file=""**: File containing x509 private key matching --tls-cert-file.
*       **--token-auth-file=""**: If set, the file that will be used to secure the secure port of the API server via token authentication.
*       **--v=0**: log level for V logs
*       **--version=false**: Print version information and quit
*       **--vmodule=**: comma-separated list of pattern=N settings for file-filtered logging

# EXAMPLES
```
/usr/bin/kube-apiserver --logtostderr=true --v=0 --etcd_servers=http://127.0.0.1:4001 --insecure_bind_address=127.0.0.1 --insecure_port=8080 --kubelet_port=10250 --service-cluster-ip-range=10.1.1.0/24 --allow_privileged=false
```


[![Analytics](https://kubernetes-site.appspot.com/UA-36037335-10/GitHub/docs/man/kube-apiserver.md?pixel)]()
