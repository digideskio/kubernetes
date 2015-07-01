# Kubernetes 101 - Walkthrough

## Kubectl CLI

The easiest way to interact with Kubernetes is via the command-line interface.

If you downloaded a pre-compiled release, kubectl should be under `platforms/<os>/<arch>`.

If you built from source, kubectl should be either under `_output/local/bin/<os>/<arch>` or `_output/dockerized/bin/<os>/<arch>`.

### Install

Do one of the following to install kubectl:
- Add the kubectl binary to your PATH
- Move kubectl into a dir already in PATH (like `/usr/local/bin`)
- Use `./cluster/kubectl.sh` instead of kubectl. It will auto-detect the location of kubectl and proxy commands to it.

### Configure

If you used `./cluster/kube-up.sh` to deploy your Kubernetes cluster, kubectl should already be locally configured.

By default, kubectl configuration lives at `~/.kube/config`.

If your cluster was deployed by other means, you may want to configure the path to the Kubernetes apiserver in your shell environment:

```sh
export KUBERNETES_MASTER=http://<ip>:<port>/api
```

Check that kubectl is properly configured by getting the cluster state:

```sh
kubectl cluster-info
```


## Pods
In Kubernetes, a group of one or more containers is called a _pod_. Containers in a pod are deployed together, and are started, stopped, and replicated as a group.

See [pods](../../docs/pods.md) for more details.


### Pod Definition

The simplest pod definition describes the deployment of a single container.  For example, an nginx web server pod might be defined as such:

```yaml
apiVersion: v1
kind: Pod
metadata:
  name: nginx
spec:
  containers:
  - name: nginx
    image: nginx
    ports:
    - name: http
      containerPort: 80
```

A pod definition is a declaration of a _desired state_.  Desired state is a very important concept in the Kubernetes model.  Many things present a desired state to the system, and it is Kubernetes' responsibility to make sure that the current state matches the desired state.  For example, when you create a Pod, you declare that you want the containers in it to be running.  If the containers happen to not be running (e.g. program failure, ...), Kubernetes will continue to (re-)create them for you in order to drive them to the desired state. This process continues until you delete the Pod.

See the [design document](../../DESIGN.md) for more details.


### Managing a Pod

If you have the source checked out, the following command (executed from the root of the project) will create the pod defined above. Otherwise download [the pod definition](pod-nginx.yaml) first.

```sh
kubectl create -f examples/walkthrough/pod-nginx.yaml
```

To list all pods:

```sh
kubectl get pods
```

To view the pod's http endpoint, first determine which IP the pod was given:

```sh
kubectl get pod nginx -o=template -t={{.status.podIP}}
```

Provided your pod IPs are accessible (depending on network setup), you should be able to access the pod's http endpoint in a browser or curl on port 80.

```sh
curl http://$(kubectl get pod nginx -o=template -t={{.status.podIP}})
```

To delete the pod by name:

```sh
kubectl delete pod nginx
```


### Volumes

That's great for a simple static web server, but what about persistent storage?

The container file system only lives as long as the container does. So if your app's state needs to survive reboots or crashes you'll need to configure some persistent storage.  To do so, declare a ```volume``` in the pod definition, and mount it into one or more of the pod containers:

Added a volume:
```
  volumes:
  - name: redis-persistent-storage
    emptyDir: {}
```

Reference the volume in our container section:
```
    volumeMounts:
    # name must match the volume name below
    - name: redis-persistent-storage
      # mount path within the container
      mountPath: /data/redis
```

Full definition ([source](pod-redis.yaml)):

```yaml
apiVersion: v1
kind: Pod
metadata:
  name: redis
spec:
  containers:
  - name: redis
    image: redis
    volumeMounts:
    - name: redis-persistent-storage
      mountPath: /data/redis
  volumes:
  - name: redis-persistent-storage
    emptyDir: {}
```

In Kubernetes, ```emptyDir``` Volumes live for the lifespan of the Pod, which may be longer than the lifespan of any one container, so if the container fails and is restarted, our persistent storage will live on.

If you want to mount a directory that already exists in the file system (e.g. ```/var/logs```) you can use the ```hostDir``` directive.

See [volumes](../../docs/volumes.md) for more details.


### Multiple Containers

_Note:
The examples below are syntactically correct, but some of the images (e.g. kubernetes/git-monitor) don't exist yet.  We're working on turning these into working examples._


However, often you want to have two different containers that work together.  An example of this would be a web server, and a helper job that polls a git repository for new updates:

```yaml
apiVersion: v1
kind: Pod
metadata:
  name: git-monitor
spec:
  containers:
  - name: nginx
    image: nginx
    volumeMounts:
    - mountPath: /srv/www
      name: www-data
      readOnly: true
  - name: git-monitor
    image: kubernetes/git-monitor
    env:
    - name: GIT_REPO
      value: http://github.com/some/repo.git
    volumeMounts:
    - mountPath: /data
      name: www-data
  volumes:
  - name: www-data
    emptyDir: {}
```

Note that we have also added a volume here.  In this case, the volume is mounted into both containers.  It is marked ```readOnly``` in the web server's case, since it doesn't need to write to the directory.

Finally, we have also introduced an environment variable to the ```git-monitor``` container, which allows us to parameterize that container with the particular git repository that we want to track.


### What's next?
Continue on to [Kubernetes 201](https://github.com/GoogleCloudPlatform/kubernetes/tree/master/examples/walkthrough/k8s201.md) or
for a complete application see the [guestbook example](https://github.com/GoogleCloudPlatform/kubernetes/tree/master/examples/guestbook/README.md)


[![Analytics](https://kubernetes-site.appspot.com/UA-36037335-10/GitHub/examples/walkthrough/README.md?pixel)]()
