/*
Copyright 2015 The Kubernetes Authors All rights reserved.

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

package staticpods

import (
	"fmt"
	"io/ioutil"
	"path/filepath"

	log "github.com/golang/glog"
	"k8s.io/kubernetes/contrib/mesos/pkg/scheduler/meta"
	"k8s.io/kubernetes/contrib/mesos/pkg/scheduler/podtask"
	"k8s.io/kubernetes/contrib/mesos/pkg/scheduler/resource"
	"k8s.io/kubernetes/pkg/api"
	"k8s.io/kubernetes/pkg/api/validation"
	utilyaml "k8s.io/kubernetes/pkg/util/yaml"
)

type defaultFunc func(pod *api.Pod) error

// tryDecodeSinglePod was copied from pkg/kubelet/config/common.go v1.0.5
func tryDecodeSinglePod(data []byte, defaultFn defaultFunc) (parsed bool, pod *api.Pod, err error) {
	// JSON is valid YAML, so this should work for everything.
	json, err := utilyaml.ToJSON(data)
	if err != nil {
		return false, nil, err
	}
	obj, err := api.Scheme.Decode(json)
	if err != nil {
		return false, pod, err
	}
	// Check whether the object could be converted to single pod.
	if _, ok := obj.(*api.Pod); !ok {
		err = fmt.Errorf("invalid pod: %+v", obj)
		return false, pod, err
	}
	newPod := obj.(*api.Pod)
	// Apply default values and validate the pod.
	if err = defaultFn(newPod); err != nil {
		return true, pod, err
	}
	if errs := validation.ValidatePod(newPod); len(errs) > 0 {
		err = fmt.Errorf("invalid pod: %v", errs)
		return true, pod, err
	}
	return true, newPod, nil
}

type FilterFunc func(pod *api.Pod) (bool, error)

func (filter FilterFunc) ReadPodsInDir(dirpath string) (<-chan *api.Pod, <-chan error) {
	pods := make(chan *api.Pod)
	errors := make(chan error)
	go func() {
		defer close(pods)
		defer close(errors)
		files, err := ioutil.ReadDir(dirpath)
		if err != nil {
			errors <- fmt.Errorf("error scanning static pods directory: %q: %v", dirpath, err)
			return
		}
		for _, f := range files {
			if f.IsDir() || f.Size() == 0 {
				continue
			}
			filename := filepath.Join(dirpath, f.Name())
			log.V(1).Infof("reading static pod conf from file %q", filename)

			data, err := ioutil.ReadFile(filename)
			if err != nil {
				errors <- fmt.Errorf("failed to read static pod file: %q: %v", filename, err)
				continue
			}

			keep := false
			defaultFn := func(pod *api.Pod) (err error) {
				keep, err = filter(pod)
				return
			}
			parsed, pod, err := tryDecodeSinglePod(data, defaultFn)
			if !parsed {
				if err != nil {
					errors <- fmt.Errorf("error parsing static pod file %q: %v", filename, err)
				}
				continue
			}
			if err != nil {
				errors <- fmt.Errorf("error validating static pod file %q: %v", filename, err)
				continue
			}
			if keep {
				annotate(&pod.ObjectMeta, map[string]string{meta.StaticPodFilename: f.Name()})
				pods <- pod
			}
		}
	}()
	return pods, errors
}

func MakeValidator(limitCPU resource.CPUShares, limitMem resource.MegaBytes, accumCPU, accumMem *float64, minimalResources bool) FilterFunc {
	return FilterFunc(func(pod *api.Pod) (bool, error) {
		// TODO(sttts): allow unlimited static pods as well and patch in the default resource limits
		unlimitedCPU := resource.LimitPodCPU(pod, limitCPU)
		if unlimitedCPU {
			return false, fmt.Errorf("found static pod without limit on cpu resources")
		}

		unlimitedMem := resource.LimitPodMem(pod, limitMem)
		if unlimitedMem {
			return false, fmt.Errorf("found static pod without limit on memory resources")
		}

		cpu := resource.PodCPULimit(pod)
		mem := resource.PodMemLimit(pod)
		if minimalResources {
			// see SchedulerServer.AccountForPodResources
			cpu = podtask.MinimalCpus
			mem = podtask.MinimalMem
		}

		log.V(2).Infof("reserving %.2f cpu shares and %.2f MB of memory to static pod %s/%s", cpu, mem, pod.Namespace, pod.Name)

		*accumCPU += float64(cpu)
		*accumMem += float64(mem)
		return true, nil
	})
}

func annotate(meta *api.ObjectMeta, kv map[string]string) {
	if meta.Annotations == nil {
		meta.Annotations = make(map[string]string)
	}
	for k, v := range kv {
		meta.Annotations[k] = v
	}
}

func MakeProcurement(pods <-chan *api.Pod) podtask.Procurement {
	// entries is a pre-validated list of static pod specs that serve as templates for the static pod builder func
	return podtask.StaticPodProcurement(func(assignedSlave string) ([]byte, error) {
		applyBindingsTo := FilterFunc(func(pod *api.Pod) (bool, error) {
			// add binding annotation so that static pods aren't picked up by the main scheduling loop.
			// static pods are always completely managed by the kubelet.
			annotate(&pod.ObjectMeta, map[string]string{meta.BindingHostKey: assignedSlave})
			return true, nil
		})
		return GZipPodList(applyBindingsTo.list(pods))
	})
}

func (filter FilterFunc) do(in <-chan *api.Pod) <-chan *api.Pod {
	out := make(chan *api.Pod)
	go func() {
		close(out)
		for pod := range in {
			if ok, err := filter(pod); err != nil {
				log.Errorf("pod failed selection: %v", err)
			} else if ok {
				out <- pod
			}
		}
	}()
	return out
}

func (filter FilterFunc) list(pods <-chan *api.Pod) *api.PodList {
	list := &api.PodList{}
	for p := range pods {
		list.Items = append(list.Items, *p)
	}
	return list
}
