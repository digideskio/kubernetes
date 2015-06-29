/*
Copyright 2014 The Kubernetes Authors All rights reserved.

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

package e2e

import (
	"fmt"
	"io/ioutil"
	"math/rand"
	"net/http"
	"sort"
	"strings"
	"time"

	"github.com/GoogleCloudPlatform/kubernetes/pkg/api"
	"github.com/GoogleCloudPlatform/kubernetes/pkg/client"
	"github.com/GoogleCloudPlatform/kubernetes/pkg/fields"
	"github.com/GoogleCloudPlatform/kubernetes/pkg/labels"
	"github.com/GoogleCloudPlatform/kubernetes/pkg/types"
	"github.com/GoogleCloudPlatform/kubernetes/pkg/util"
	"github.com/GoogleCloudPlatform/kubernetes/pkg/util/wait"
	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
)

// This should match whatever the default/configured range is
var ServiceNodePortRange = util.PortRange{Base: 30000, Size: 2767}

var _ = Describe("Services", func() {
	var c *client.Client
	// Use these in tests.  They're unique for each test to prevent name collisions.
	var namespaces [2]string

	BeforeEach(func() {
		var err error
		c, err = loadClient()
		Expect(err).NotTo(HaveOccurred())

		By("Building a namespace api objects")
		for i := range namespaces {
			namespacePtr, err := createTestingNS(fmt.Sprintf("service-%d", i), c)
			Expect(err).NotTo(HaveOccurred())
			namespaces[i] = namespacePtr.Name
		}
	})

	AfterEach(func() {
		for _, ns := range namespaces {
			By(fmt.Sprintf("Destroying namespace %v", ns))
			if err := c.Namespaces().Delete(ns); err != nil {
				Failf("Couldn't delete namespace %s: %s", ns, err)
			}
		}
	})
	// TODO: We get coverage of TCP/UDP and multi-port services through the DNS test. We should have a simpler test for multi-port TCP here.
	It("should provide secure master service", func() {
		_, err := c.Services(api.NamespaceDefault).Get("kubernetes")
		Expect(err).NotTo(HaveOccurred())
	})

	It("should serve a basic endpoint from pods", func() {
		serviceName := "endpoint-test2"
		ns := namespaces[0]
		labels := map[string]string{
			"foo": "bar",
			"baz": "blah",
		}

		defer func() {
			err := c.Services(ns).Delete(serviceName)
			Expect(err).NotTo(HaveOccurred())
		}()

		// Named ports are used to identify the endpoints for the given service ports.
		// This is necessary because we don't want to rely on the fact that the
		// numeric port of an endpoint matches the numeric service port. In setups
		// like kubernetes-mesos different numeric ports are used by the endpoint
		// controller.
		portname1 := "portname1"

		service := &api.Service{
			ObjectMeta: api.ObjectMeta{
				Name: serviceName,
			},
			Spec: api.ServiceSpec{
				Selector: labels,
				Ports: []api.ServicePort{{
					Name:		portname1,
					Port:       80,
					TargetPort: util.NewIntOrStringFromInt(80),
				}},
			},
		}
		_, err := c.Services(ns).Create(service)
		Expect(err).NotTo(HaveOccurred())

		validateEndpointsOrFail(c, ns, serviceName, map[string][]string{})

		var names []string
		defer func() {
			for _, name := range names {
				err := c.Pods(ns).Delete(name, nil)
				Expect(err).NotTo(HaveOccurred())
			}
		}()

		name1 := "test1"
		addEndpointPodOrFail(c, ns, name1, labels, []api.ContainerPort{{ContainerPort: 80}})
		names = append(names, name1)

		validateEndpointsOrFail(c, ns, serviceName, map[string][]string{name1: {portname1}})

		name2 := "test2"
		addEndpointPodOrFail(c, ns, name2, labels, []api.ContainerPort{{ContainerPort: 80}})
		names = append(names, name2)

		validateEndpointsOrFail(c, ns, serviceName, map[string][]string{name1: {portname1}, name2: {portname1}})

		err = c.Pods(ns).Delete(name1, nil)
		Expect(err).NotTo(HaveOccurred())
		names = []string{name2}

		validateEndpointsOrFail(c, ns, serviceName, map[string][]string{name2: {portname1}})

		err = c.Pods(ns).Delete(name2, nil)
		Expect(err).NotTo(HaveOccurred())
		names = []string{}

		validateEndpointsOrFail(c, ns, serviceName, map[string][]string{})
	})

	It("should serve multiport endpoints from pods", func() {
		// repacking functionality is intentionally not tested here - it's better to test it in an integration test.
		serviceName := "multi-endpoint-test"
		ns := namespaces[0]

		defer func() {
			err := c.Services(ns).Delete(serviceName)
			Expect(err).NotTo(HaveOccurred())
		}()

		labels := map[string]string{"foo": "bar"}

		svc1port := "svc1"
		svc2port := "svc2"

		// Named ports are used to identify the endpoints for the given service ports.
		// This is necessary because we don't want to rely on the fact that the
		// numeric port of an endpoint matches the numeric service port. In setups
		// like kubernetes-mesos different numeric ports are used by the endpoint
		// controller.
		portname1 := "portname1"
		portname2 := "portname2"

		service := &api.Service{
			ObjectMeta: api.ObjectMeta{
				Name: serviceName,
			},
			Spec: api.ServiceSpec{
				Selector: labels,
				Ports: []api.ServicePort{
					{
						Name:       portname1,
						Port:       80,
						TargetPort: util.NewIntOrStringFromString(svc1port),
					},
					{
						Name:       portname2,
						Port:       81,
						TargetPort: util.NewIntOrStringFromString(svc2port),
					},
				},
			},
		}
		_, err := c.Services(ns).Create(service)
		Expect(err).NotTo(HaveOccurred())
		port1 := 100
		port2 := 101
		validateEndpointsOrFail(c, ns, serviceName, map[string][]string{})

		var names []string
		defer func() {
			for _, name := range names {
				err := c.Pods(ns).Delete(name, nil)
				Expect(err).NotTo(HaveOccurred())
			}
		}()

		containerPorts1 := []api.ContainerPort{
			{
				Name:          svc1port,
				ContainerPort: port1,
			},
		}
		containerPorts2 := []api.ContainerPort{
			{
				Name:          svc2port,
				ContainerPort: port2,
			},
		}

		podname1 := "podname1"
		addEndpointPodOrFail(c, ns, podname1, labels, containerPorts1)
		names = append(names, podname1)
		validateEndpointsOrFail(c, ns, serviceName, map[string][]string{podname1: {portname1}})

		podname2 := "podname2"
		addEndpointPodOrFail(c, ns, podname2, labels, containerPorts2)
		names = append(names, podname2)
		validateEndpointsOrFail(c, ns, serviceName, map[string][]string{podname1: {portname1}, podname2: {portname2}})

		podname3 := "podname3"
		addEndpointPodOrFail(c, ns, podname3, labels, append(containerPorts1, containerPorts2...))
		names = append(names, podname3)
		validateEndpointsOrFail(c, ns, serviceName, map[string][]string{podname1: {portname1}, podname2: {portname2}, podname3: {portname1, portname2}})

		err = c.Pods(ns).Delete(podname1, nil)
		Expect(err).NotTo(HaveOccurred())
		names = []string{podname2, podname3}

		validateEndpointsOrFail(c, ns, serviceName, map[string][]string{podname2: {portname2}, podname3: {portname1, portname2}})

		err = c.Pods(ns).Delete(podname2, nil)
		Expect(err).NotTo(HaveOccurred())
		names = []string{podname3}

		validateEndpointsOrFail(c, ns, serviceName, map[string][]string{podname3: {portname1, portname2}})

		err = c.Pods(ns).Delete(podname3, nil)
		Expect(err).NotTo(HaveOccurred())
		names = []string{}

		validateEndpointsOrFail(c, ns, serviceName, map[string][]string{})
	})

	It("should be able to create a functioning external load balancer", func() {
		if !providerIs("gce", "gke", "aws") {
			By(fmt.Sprintf("Skipping service external load balancer test; uses ServiceTypeLoadBalancer, a (gce|gke|aws) feature"))
			return
		}

		serviceName := "external-lb-test"
		ns := namespaces[0]

		t := NewWebserverTest(c, ns, serviceName)
		defer func() {
			defer GinkgoRecover()
			errs := t.Cleanup()
			if len(errs) != 0 {
				Failf("errors in cleanup: %v", errs)
			}
		}()

		service := t.BuildServiceSpec()
		service.Spec.Type = api.ServiceTypeLoadBalancer

		By("creating service " + serviceName + " with external load balancer in namespace " + ns)
		result, err := t.CreateService(service)
		Expect(err).NotTo(HaveOccurred())

		// Wait for the load balancer to be created asynchronously, which is
		// currently indicated by ingress point(s) being added to the status.
		result, err = waitForLoadBalancerIngress(c, serviceName, ns)
		Expect(err).NotTo(HaveOccurred())
		if len(result.Status.LoadBalancer.Ingress) != 1 {
			Failf("got unexpected number (%v) of ingress points for externally load balanced service: %v", result.Status.LoadBalancer.Ingress, result)
		}
		ingress := result.Status.LoadBalancer.Ingress[0]
		if len(result.Spec.Ports) != 1 {
			Failf("got unexpected len(Spec.Ports) for LoadBalancer service: %v", result)
		}
		port := result.Spec.Ports[0]
		if port.NodePort == 0 {
			Failf("got unexpected Spec.Ports[0].nodePort for LoadBalancer service: %v", result)
		}
		if !ServiceNodePortRange.Contains(port.NodePort) {
			Failf("got unexpected (out-of-range) port for LoadBalancer service: %v", result)
		}

		By("creating pod to be part of service " + serviceName)
		t.CreateWebserverRC(1)

		By("hitting the pod through the service's NodePort")
		testReachable(pickMinionIP(c), port.NodePort)

		By("hitting the pod through the service's external load balancer")
		testLoadBalancerReachable(ingress, 80)
	})

	It("should be able to create a functioning NodePort service", func() {
		serviceName := "nodeportservice-test"
		ns := namespaces[0]

		t := NewWebserverTest(c, ns, serviceName)
		defer func() {
			defer GinkgoRecover()
			errs := t.Cleanup()
			if len(errs) != 0 {
				Failf("errors in cleanup: %v", errs)
			}
		}()

		service := t.BuildServiceSpec()
		service.Spec.Type = api.ServiceTypeNodePort

		By("creating service " + serviceName + " with type=NodePort in namespace " + ns)
		result, err := c.Services(ns).Create(service)
		Expect(err).NotTo(HaveOccurred())
		defer func(ns, serviceName string) { // clean up when we're done
			By("deleting service " + serviceName + " in namespace " + ns)
			err := c.Services(ns).Delete(serviceName)
			Expect(err).NotTo(HaveOccurred())
		}(ns, serviceName)

		if len(result.Spec.Ports) != 1 {
			Failf("got unexpected number (%d) of Ports for NodePort service: %v", len(result.Spec.Ports), result)
		}

		nodePort := result.Spec.Ports[0].NodePort
		if nodePort == 0 {
			Failf("got unexpected nodePort (%d) on Ports[0] for NodePort service: %v", nodePort, result)
		}
		if !ServiceNodePortRange.Contains(nodePort) {
			Failf("got unexpected (out-of-range) port for NodePort service: %v", result)
		}

		By("creating pod to be part of service " + serviceName)
		t.CreateWebserverRC(1)

		By("hitting the pod through the service's NodePort")
		ip := pickMinionIP(c)
		testReachable(ip, nodePort)
	})

	It("should be able to change the type and nodeport settings of a service", func() {
		serviceName := "mutability-service-test"
		ns := namespaces[0]

		t := NewWebserverTest(c, ns, serviceName)
		defer func() {
			defer GinkgoRecover()
			errs := t.Cleanup()
			if len(errs) != 0 {
				Failf("errors in cleanup: %v", errs)
			}
		}()

		service := t.BuildServiceSpec()

		By("creating service " + serviceName + " with type unspecified in namespace " + t.Namespace)
		service, err := t.CreateService(service)
		Expect(err).NotTo(HaveOccurred())

		if service.Spec.Type != api.ServiceTypeClusterIP {
			Failf("got unexpected Spec.Type for default service: %v", service)
		}
		if len(service.Spec.Ports) != 1 {
			Failf("got unexpected len(Spec.Ports) for default service: %v", service)
		}
		port := service.Spec.Ports[0]
		if port.NodePort != 0 {
			Failf("got unexpected Spec.Ports[0].nodePort for default service: %v", service)
		}
		if len(service.Status.LoadBalancer.Ingress) != 0 {
			Failf("got unexpected len(Status.LoadBalancer.Ingresss) for default service: %v", service)
		}

		By("creating pod to be part of service " + t.ServiceName)
		t.CreateWebserverRC(1)

		By("changing service " + serviceName + " to type=NodePort")
		service.Spec.Type = api.ServiceTypeNodePort
		service, err = c.Services(ns).Update(service)
		Expect(err).NotTo(HaveOccurred())

		if service.Spec.Type != api.ServiceTypeNodePort {
			Failf("got unexpected Spec.Type for NodePort service: %v", service)
		}
		if len(service.Spec.Ports) != 1 {
			Failf("got unexpected len(Spec.Ports) for NodePort service: %v", service)
		}
		port = service.Spec.Ports[0]
		if port.NodePort == 0 {
			Failf("got unexpected Spec.Ports[0].nodePort for NodePort service: %v", service)
		}
		if !ServiceNodePortRange.Contains(port.NodePort) {
			Failf("got unexpected (out-of-range) port for NodePort service: %v", service)
		}

		if len(service.Status.LoadBalancer.Ingress) != 0 {
			Failf("got unexpected len(Status.LoadBalancer.Ingresss) for NodePort service: %v", service)
		}
		By("hitting the pod through the service's NodePort")
		ip := pickMinionIP(c)
		nodePort1 := port.NodePort // Save for later!
		testReachable(ip, nodePort1)

		By("changing service " + serviceName + " to type=LoadBalancer")
		service.Spec.Type = api.ServiceTypeLoadBalancer
		service, err = c.Services(ns).Update(service)
		Expect(err).NotTo(HaveOccurred())

		// Wait for the load balancer to be created asynchronously
		service, err = waitForLoadBalancerIngress(c, serviceName, ns)
		Expect(err).NotTo(HaveOccurred())

		if service.Spec.Type != api.ServiceTypeLoadBalancer {
			Failf("got unexpected Spec.Type for LoadBalancer service: %v", service)
		}
		if len(service.Spec.Ports) != 1 {
			Failf("got unexpected len(Spec.Ports) for LoadBalancer service: %v", service)
		}
		port = service.Spec.Ports[0]
		if port.NodePort != nodePort1 {
			Failf("got unexpected Spec.Ports[0].nodePort for LoadBalancer service: %v", service)
		}
		if len(service.Status.LoadBalancer.Ingress) != 1 {
			Failf("got unexpected len(Status.LoadBalancer.Ingresss) for LoadBalancer service: %v", service)
		}
		ingress1 := service.Status.LoadBalancer.Ingress[0]
		if ingress1.IP == "" && ingress1.Hostname == "" {
			Failf("got unexpected Status.LoadBalancer.Ingresss[0] for LoadBalancer service: %v", service)
		}
		By("hitting the pod through the service's NodePort")
		ip = pickMinionIP(c)
		testReachable(ip, nodePort1)
		By("hitting the pod through the service's LoadBalancer")
		testLoadBalancerReachable(ingress1, 80)

		By("changing service " + serviceName + " update NodePort")
		nodePort2 := nodePort1 - 1
		if !ServiceNodePortRange.Contains(nodePort2) {
			//Check for (unlikely) assignment at bottom of range
			nodePort2 = nodePort1 + 1
		}
		service.Spec.Ports[0].NodePort = nodePort2
		service, err = c.Services(ns).Update(service)
		Expect(err).NotTo(HaveOccurred())

		if service.Spec.Type != api.ServiceTypeLoadBalancer {
			Failf("got unexpected Spec.Type for updated-NodePort service: %v", service)
		}
		if len(service.Spec.Ports) != 1 {
			Failf("got unexpected len(Spec.Ports) for updated-NodePort service: %v", service)
		}
		port = service.Spec.Ports[0]
		if port.NodePort != nodePort2 {
			Failf("got unexpected Spec.Ports[0].nodePort for NodePort service: %v", service)
		}
		if len(service.Status.LoadBalancer.Ingress) != 1 {
			Failf("got unexpected len(Status.LoadBalancer.Ingresss) for NodePort service: %v", service)
		}
		ingress2 := service.Status.LoadBalancer.Ingress[0]
		// TODO: This is a problem on AWS; we can't just always be changing the LB
		Expect(ingress1).To(Equal(ingress2))

		By("hitting the pod through the service's updated NodePort")
		testReachable(ip, nodePort2)
		By("hitting the pod through the service's LoadBalancer")
		testLoadBalancerReachable(ingress2, 80)
		By("checking the old NodePort is closed")
		testNotReachable(ip, nodePort1)

		By("changing service " + serviceName + " back to type=ClusterIP")
		service.Spec.Type = api.ServiceTypeClusterIP
		service.Spec.Ports[0].NodePort = 0
		service, err = c.Services(ns).Update(service)
		Expect(err).NotTo(HaveOccurred())

		if service.Spec.Type != api.ServiceTypeClusterIP {
			Failf("got unexpected Spec.Type for back-to-ClusterIP service: %v", service)
		}
		if len(service.Spec.Ports) != 1 {
			Failf("got unexpected len(Spec.Ports) for back-to-ClusterIP service: %v", service)
		}
		port = service.Spec.Ports[0]
		if port.NodePort != 0 {
			Failf("got unexpected Spec.Ports[0].nodePort for back-to-ClusterIP service: %v", service)
		}

		// Wait for the load balancer to be destroyed asynchronously
		service, err = waitForLoadBalancerDestroy(c, serviceName, ns)
		Expect(err).NotTo(HaveOccurred())

		if len(service.Status.LoadBalancer.Ingress) != 0 {
			Failf("got unexpected len(Status.LoadBalancer.Ingresss) for back-to-ClusterIP service: %v", service)
		}
		By("checking the NodePort (original) is closed")
		ip = pickMinionIP(c)
		testNotReachable(ip, nodePort1)
		By("checking the NodePort (updated) is closed")
		ip = pickMinionIP(c)
		testNotReachable(ip, nodePort2)
		By("checking the LoadBalancer is closed")
		testLoadBalancerNotReachable(ingress2, 80)
	})

	It("should release the load balancer when Type goes from LoadBalancer -> NodePort", func() {
		serviceName := "service-release-lb"
		ns := namespaces[0]

		t := NewWebserverTest(c, ns, serviceName)
		defer func() {
			defer GinkgoRecover()
			errs := t.Cleanup()
			if len(errs) != 0 {
				Failf("errors in cleanup: %v", errs)
			}
		}()

		service := t.BuildServiceSpec()
		service.Spec.Type = api.ServiceTypeLoadBalancer

		By("creating service " + serviceName + " with type LoadBalancer")
		service, err := t.CreateService(service)
		Expect(err).NotTo(HaveOccurred())

		By("creating pod to be part of service " + t.ServiceName)
		t.CreateWebserverRC(1)

		if service.Spec.Type != api.ServiceTypeLoadBalancer {
			Failf("got unexpected Spec.Type for LoadBalancer service: %v", service)
		}
		if len(service.Spec.Ports) != 1 {
			Failf("got unexpected len(Spec.Ports) for LoadBalancer service: %v", service)
		}
		nodePort := service.Spec.Ports[0].NodePort
		if nodePort == 0 {
			Failf("got unexpected Spec.Ports[0].NodePort for LoadBalancer service: %v", service)
		}

		// Wait for the load balancer to be created asynchronously
		service, err = waitForLoadBalancerIngress(c, serviceName, ns)
		Expect(err).NotTo(HaveOccurred())

		if len(service.Status.LoadBalancer.Ingress) != 1 {
			Failf("got unexpected len(Status.LoadBalancer.Ingresss) for LoadBalancer service: %v", service)
		}
		ingress := service.Status.LoadBalancer.Ingress[0]
		if ingress.IP == "" && ingress.Hostname == "" {
			Failf("got unexpected Status.LoadBalancer.Ingresss[0] for LoadBalancer service: %v", service)
		}

		By("hitting the pod through the service's NodePort")
		ip := pickMinionIP(c)
		testReachable(ip, nodePort)
		By("hitting the pod through the service's LoadBalancer")
		testLoadBalancerReachable(ingress, 80)

		By("changing service " + serviceName + " to type=NodePort")
		service.Spec.Type = api.ServiceTypeNodePort
		service, err = c.Services(ns).Update(service)
		Expect(err).NotTo(HaveOccurred())

		if service.Spec.Type != api.ServiceTypeNodePort {
			Failf("got unexpected Spec.Type for NodePort service: %v", service)
		}
		if len(service.Spec.Ports) != 1 {
			Failf("got unexpected len(Spec.Ports) for NodePort service: %v", service)
		}
		if service.Spec.Ports[0].NodePort != nodePort {
			Failf("got unexpected Spec.Ports[0].NodePort for NodePort service: %v", service)
		}

		// Wait for the load balancer to be created asynchronously
		service, err = waitForLoadBalancerDestroy(c, serviceName, ns)
		Expect(err).NotTo(HaveOccurred())

		if len(service.Status.LoadBalancer.Ingress) != 0 {
			Failf("got unexpected len(Status.LoadBalancer.Ingresss) for NodePort service: %v", service)
		}

		By("hitting the pod through the service's NodePort")
		testReachable(ip, nodePort)
		By("checking the LoadBalancer is closed")
		testLoadBalancerNotReachable(ingress, 80)
	})

	It("should prevent NodePort collisions", func() {
		serviceName := "nodeport-collision"
		serviceName2 := serviceName + "2"
		ns := namespaces[0]

		t := NewWebserverTest(c, ns, serviceName)
		defer func() {
			defer GinkgoRecover()
			errs := t.Cleanup()
			if len(errs) != 0 {
				Failf("errors in cleanup: %v", errs)
			}
		}()

		service := t.BuildServiceSpec()
		service.Spec.Type = api.ServiceTypeNodePort

		By("creating service " + serviceName + " with type NodePort in namespace " + ns)
		result, err := t.CreateService(service)
		Expect(err).NotTo(HaveOccurred())

		if result.Spec.Type != api.ServiceTypeNodePort {
			Failf("got unexpected Spec.Type for new service: %v", result)
		}
		if len(result.Spec.Ports) != 1 {
			Failf("got unexpected len(Spec.Ports) for new service: %v", result)
		}
		port := result.Spec.Ports[0]
		if port.NodePort == 0 {
			Failf("got unexpected Spec.Ports[0].nodePort for new service: %v", result)
		}

		By("creating service " + serviceName + " with conflicting NodePort")

		service2 := t.BuildServiceSpec()
		service2.Name = serviceName2
		service2.Spec.Type = api.ServiceTypeNodePort
		service2.Spec.Ports[0].NodePort = port.NodePort

		By("creating service " + serviceName2 + " with conflicting NodePort")
		result2, err := t.CreateService(service2)
		if err == nil {
			Failf("Created service with conflicting NodePort: %v", result2)
		}
		expectedErr := fmt.Sprintf("Service \"%s\" is invalid: spec.ports[0].nodePort: invalid value '%d': provided port is already allocated", serviceName2, port.NodePort)
		Expect(fmt.Sprintf("%v", err)).To(Equal(expectedErr))

		By("deleting original service " + serviceName + " with type NodePort in namespace " + ns)
		err = t.DeleteService(serviceName)
		Expect(err).NotTo(HaveOccurred())

		By("creating service " + serviceName2 + " with no-longer-conflicting NodePort")
		_, err = t.CreateService(service2)
		Expect(err).NotTo(HaveOccurred())
	})

	It("should check NodePort out-of-range", func() {
		serviceName := "nodeport-range-test"
		ns := namespaces[0]

		t := NewWebserverTest(c, ns, serviceName)
		defer func() {
			defer GinkgoRecover()
			errs := t.Cleanup()
			if len(errs) != 0 {
				Failf("errors in cleanup: %v", errs)
			}
		}()

		service := t.BuildServiceSpec()
		service.Spec.Type = api.ServiceTypeNodePort

		By("creating service " + serviceName + " with type NodePort in namespace " + ns)
		service, err := t.CreateService(service)
		Expect(err).NotTo(HaveOccurred())

		if service.Spec.Type != api.ServiceTypeNodePort {
			Failf("got unexpected Spec.Type for new service: %v", service)
		}
		if len(service.Spec.Ports) != 1 {
			Failf("got unexpected len(Spec.Ports) for new service: %v", service)
		}
		port := service.Spec.Ports[0]
		if port.NodePort == 0 {
			Failf("got unexpected Spec.Ports[0].nodePort for new service: %v", service)
		}
		if !ServiceNodePortRange.Contains(port.NodePort) {
			Failf("got unexpected (out-of-range) port for new service: %v", service)
		}

		outOfRangeNodePort := 0
		for {
			outOfRangeNodePort = 1 + rand.Intn(65535)
			if !ServiceNodePortRange.Contains(outOfRangeNodePort) {
				break
			}
		}
		By(fmt.Sprintf("changing service "+serviceName+" to out-of-range NodePort %d", outOfRangeNodePort))
		service.Spec.Ports[0].NodePort = outOfRangeNodePort
		result, err := t.Client.Services(t.Namespace).Update(service)
		if err == nil {
			Failf("failed to prevent update of service with out-of-range NodePort: %v", result)
		}
		expectedErr := fmt.Sprintf("Service \"%s\" is invalid: spec.ports[0].nodePort: invalid value '%d': provided port is not in the valid range", serviceName, outOfRangeNodePort)
		Expect(fmt.Sprintf("%v", err)).To(Equal(expectedErr))

		By("deleting original service " + serviceName)
		err = t.DeleteService(serviceName)
		Expect(err).NotTo(HaveOccurred())

		By(fmt.Sprintf("creating service "+serviceName+" with out-of-range NodePort %d", outOfRangeNodePort))
		service = t.BuildServiceSpec()
		service.Spec.Type = api.ServiceTypeNodePort
		service.Spec.Ports[0].NodePort = outOfRangeNodePort
		service, err = t.CreateService(service)
		if err == nil {
			Failf("failed to prevent create of service with out-of-range NodePort (%d): %v", outOfRangeNodePort, service)
		}
		Expect(fmt.Sprintf("%v", err)).To(Equal(expectedErr))
	})

	It("should release NodePorts on delete", func() {
		serviceName := "nodeport-reuse"
		ns := namespaces[0]

		t := NewWebserverTest(c, ns, serviceName)
		defer func() {
			defer GinkgoRecover()
			errs := t.Cleanup()
			if len(errs) != 0 {
				Failf("errors in cleanup: %v", errs)
			}
		}()

		service := t.BuildServiceSpec()
		service.Spec.Type = api.ServiceTypeNodePort

		By("creating service " + serviceName + " with type NodePort in namespace " + ns)
		service, err := t.CreateService(service)
		Expect(err).NotTo(HaveOccurred())

		if service.Spec.Type != api.ServiceTypeNodePort {
			Failf("got unexpected Spec.Type for new service: %v", service)
		}
		if len(service.Spec.Ports) != 1 {
			Failf("got unexpected len(Spec.Ports) for new service: %v", service)
		}
		port := service.Spec.Ports[0]
		if port.NodePort == 0 {
			Failf("got unexpected Spec.Ports[0].nodePort for new service: %v", service)
		}
		if !ServiceNodePortRange.Contains(port.NodePort) {
			Failf("got unexpected (out-of-range) port for new service: %v", service)
		}
		port1 := port.NodePort

		By("deleting original service " + serviceName)
		err = t.DeleteService(serviceName)
		Expect(err).NotTo(HaveOccurred())

		By(fmt.Sprintf("creating service "+serviceName+" with same NodePort %d", port1))
		service = t.BuildServiceSpec()
		service.Spec.Type = api.ServiceTypeNodePort
		service.Spec.Ports[0].NodePort = port1
		service, err = t.CreateService(service)
		Expect(err).NotTo(HaveOccurred())
	})

	It("should correctly serve identically named services in different namespaces on different external IP addresses", func() {
		if !providerIs("gce", "gke", "aws") {
			By(fmt.Sprintf("Skipping service namespace collision test; uses ServiceTypeLoadBalancer, a (gce|gke|aws) feature"))
			return
		}

		serviceNames := []string{"s0"} // Could add more here, but then it takes longer.
		labels := map[string]string{
			"key0": "value0",
			"key1": "value1",
		}
		service := &api.Service{
			ObjectMeta: api.ObjectMeta{},
			Spec: api.ServiceSpec{
				Selector: labels,
				Ports: []api.ServicePort{{
					Port:       80,
					TargetPort: util.NewIntOrStringFromInt(80),
				}},
				Type: api.ServiceTypeLoadBalancer,
			},
		}

		ingressPoints := []string{}
		for _, namespace := range namespaces {
			for _, serviceName := range serviceNames {
				service.ObjectMeta.Name = serviceName
				service.ObjectMeta.Namespace = namespace
				By("creating service " + serviceName + " in namespace " + namespace)
				_, err := c.Services(namespace).Create(service)
				Expect(err).NotTo(HaveOccurred())
				defer func(namespace, serviceName string) { // clean up when we're done
					By("deleting service " + serviceName + " in namespace " + namespace)
					err := c.Services(namespace).Delete(serviceName)
					Expect(err).NotTo(HaveOccurred())
				}(namespace, serviceName)
			}
		}
		for _, namespace := range namespaces {
			for _, serviceName := range serviceNames {
				result, err := waitForLoadBalancerIngress(c, serviceName, namespace)
				Expect(err).NotTo(HaveOccurred())
				for i := range result.Status.LoadBalancer.Ingress {
					ingress := result.Status.LoadBalancer.Ingress[i].IP
					if ingress == "" {
						ingress = result.Status.LoadBalancer.Ingress[i].Hostname
					}
					ingressPoints = append(ingressPoints, ingress) // Save 'em to check uniqueness
				}
			}
		}
		validateUniqueOrFail(ingressPoints)
	})
})

func waitForLoadBalancerIngress(c *client.Client, serviceName, namespace string) (*api.Service, error) {
	// TODO: once support ticket 21807001 is resolved, reduce this timeout back to something reasonable
	const timeout = 20 * time.Minute
	var service *api.Service
	By(fmt.Sprintf("waiting up to %v for service %s in namespace %s to have a LoadBalancer ingress point", timeout, serviceName, namespace))
	for start := time.Now(); time.Since(start) < timeout; time.Sleep(5 * time.Second) {
		service, err := c.Services(namespace).Get(serviceName)
		if err != nil {
			Logf("Get service failed, ignoring for 5s: %v", err)
			continue
		}
		if len(service.Status.LoadBalancer.Ingress) > 0 {
			return service, nil
		}
		Logf("Waiting for service %s in namespace %s to have a LoadBalancer ingress point (%v)", serviceName, namespace, time.Since(start))
	}
	return service, fmt.Errorf("service %s in namespace %s doesn't have a LoadBalancer ingress point after %.2f seconds", serviceName, namespace, timeout.Seconds())
}

func waitForLoadBalancerDestroy(c *client.Client, serviceName, namespace string) (*api.Service, error) {
	// TODO: once support ticket 21807001 is resolved, reduce this timeout back to something reasonable
	const timeout = 10 * time.Minute
	var service *api.Service
	By(fmt.Sprintf("waiting up to %v for service %s in namespace %s to have no LoadBalancer ingress points", timeout, serviceName, namespace))
	for start := time.Now(); time.Since(start) < timeout; time.Sleep(5 * time.Second) {
		service, err := c.Services(namespace).Get(serviceName)
		if err != nil {
			Logf("Get service failed, ignoring for 5s: %v", err)
			continue
		}
		if len(service.Status.LoadBalancer.Ingress) == 0 {
			return service, nil
		}
		Logf("Waiting for service %s in namespace %s to have no LoadBalancer ingress points (%v)", serviceName, namespace, time.Since(start))
	}
	return service, fmt.Errorf("service %s in namespace %s still has LoadBalancer ingress points after %.2f seconds", serviceName, namespace, timeout.Seconds())
}

func validateUniqueOrFail(s []string) {
	By(fmt.Sprintf("validating unique: %v", s))
	sort.Strings(s)
	var prev string
	for i, elem := range s {
		if i > 0 && elem == prev {
			Fail("duplicate found: " + elem)
		}
		prev = elem
	}
}

func getPortNamesByPodUID(subsets []api.EndpointSubset) map[types.UID][]string {
	m := make(map[types.UID][]string)
	for _, ss := range subsets {
		for _, port := range ss.Ports {
			for _, addr := range ss.Addresses {
				Logf("Found port %v for pod %v", port.Name, addr.TargetRef.UID)
				if _, ok := m[addr.TargetRef.UID]; !ok {
					m[addr.TargetRef.UID] = make([]string, 0)
				}
				m[addr.TargetRef.UID] = append(m[addr.TargetRef.UID], port.Name)
			}
		}
	}
	return m
}

func translatePodNameToUIDOrFail(c *client.Client, ns string, expectedEndpoints map[string][]string) map[types.UID][]string {
	portNamesByUID := make(map[types.UID][]string)

	for name, portList := range expectedEndpoints {
		pod, err := c.Pods(ns).Get(name)
		if err != nil {
			Failf("failed to get pod %s, that's pretty weird. validation failed: %s", name, err)
		}
		portNamesByUID[pod.ObjectMeta.UID] = portList
		By(fmt.Sprintf(""))
	}
	By(fmt.Sprintf("successfully translated pod names to UIDs: %v -> %v on namespace %s", expectedEndpoints, portNamesByUID, ns))
	return portNamesByUID
}

func validatePortNamesOrFail(endpoints map[types.UID][]string, expectedEndpoints map[types.UID][]string) {
	if len(endpoints) != len(expectedEndpoints) {
		// should not happen because we check this condition before
		Failf("invalid number of endpoints got %v, expected %v", endpoints, expectedEndpoints)
	}
	for podUID := range expectedEndpoints {
		if _, ok := endpoints[podUID]; !ok {
			Failf("endpoint %v not found", podUID)
		}
		if len(endpoints[podUID]) != len(expectedEndpoints[podUID]) {
			Failf("invalid list of port names for uid %v. Got %v, expected %v", podUID, endpoints[podUID], expectedEndpoints[podUID])
		}
		sort.Strings(endpoints[podUID])
		sort.Strings(expectedEndpoints[podUID])
		for index := range endpoints[podUID] {
			if endpoints[podUID][index] != expectedEndpoints[podUID][index] {
				Failf("invalid list of port names for uid %v. Got %v, expected %v", podUID, endpoints[podUID], expectedEndpoints[podUID])
			}
		}
	}
}

func validateEndpointsOrFail(c *client.Client, ns, serviceName string, expectedEndpoints map[string][]string) {
	By(fmt.Sprintf("Validating endpoints %v with on service %s/%s", expectedEndpoints, ns, serviceName))
	for {
		endpoints, err := c.Endpoints(ns).Get(serviceName)
		if err == nil {
			By(fmt.Sprintf("Found endpoints %v", endpoints))

			portNamesByPodUID := getPortNamesByPodUID(endpoints.Subsets)

			By(fmt.Sprintf("Found port names by pod UID %v", portNamesByPodUID))
			expectedPortNamesByPodUID := translatePodNameToUIDOrFail(c, ns, expectedEndpoints)
			if len(portNamesByPodUID) == len(expectedEndpoints) {
				validatePortNamesOrFail(portNamesByPodUID, expectedPortNamesByPodUID)
				break
			} else {
				By(fmt.Sprintf("Unexpected number of endpoints: found %v, expected %v (ignoring for 1 second)", portNamesByPodUID, expectedPortNamesByPodUID))
			}
		} else {
			By(fmt.Sprintf("Failed to get endpoints: %v (ignoring for 1 second)", err))
		}
		time.Sleep(time.Second)
	}
	By(fmt.Sprintf("successfully validated endpoints %v with on service %s/%s", expectedEndpoints, ns, serviceName))
}

func addEndpointPodOrFail(c *client.Client, ns, name string, labels map[string]string, containerPorts []api.ContainerPort) {
	By(fmt.Sprintf("Adding pod %v in namespace %v", name, ns))
	pod := &api.Pod{
		ObjectMeta: api.ObjectMeta{
			Name:   name,
			Labels: labels,
		},
		Spec: api.PodSpec{
			Containers: []api.Container{
				{
					Name:  "test",
					Image: "gcr.io/google_containers/pause",
					Ports: containerPorts,
				},
			},
		},
	}
	_, err := c.Pods(ns).Create(pod)
	Expect(err).NotTo(HaveOccurred())
}

func collectAddresses(nodes *api.NodeList, addressType api.NodeAddressType) []string {
	ips := []string{}
	for i := range nodes.Items {
		item := &nodes.Items[i]
		for j := range item.Status.Addresses {
			nodeAddress := &item.Status.Addresses[j]
			if nodeAddress.Type == addressType {
				ips = append(ips, nodeAddress.Address)
			}
		}
	}
	return ips
}

func getMinionPublicIps(c *client.Client) ([]string, error) {
	nodes, err := c.Nodes().List(labels.Everything(), fields.Everything())
	if err != nil {
		return nil, err
	}

	ips := collectAddresses(nodes, api.NodeExternalIP)
	if len(ips) == 0 {
		ips = collectAddresses(nodes, api.NodeLegacyHostIP)
	}
	return ips, nil
}

func pickMinionIP(c *client.Client) string {
	publicIps, err := getMinionPublicIps(c)
	Expect(err).NotTo(HaveOccurred())
	if len(publicIps) == 0 {
		Failf("got unexpected number (%d) of public IPs", len(publicIps))
	}
	ip := publicIps[0]
	return ip
}

func testLoadBalancerReachable(ingress api.LoadBalancerIngress, port int) {
	ip := ingress.IP
	if ip == "" {
		ip = ingress.Hostname
	}

	testReachable(ip, port)
}

func testLoadBalancerNotReachable(ingress api.LoadBalancerIngress, port int) {
	ip := ingress.IP
	if ip == "" {
		ip = ingress.Hostname
	}

	testNotReachable(ip, port)
}

func testReachable(ip string, port int) {
	url := fmt.Sprintf("http://%s:%d", ip, port)
	if ip == "" {
		Failf("Got empty IP for reachability check (%s)", url)
	}
	if port == 0 {
		Failf("Got port==0 for reachability check (%s)", url)
	}

	By(fmt.Sprintf("Waiting up to %v for %s to be reachable", podStartTimeout, url))
	start := time.Now()
	expectNoError(wait.Poll(poll, podStartTimeout, func() (bool, error) {
		resp, err := httpGetNoConnectionPool(url)
		if err != nil {
			Logf("Got error waiting for reachability of %s: %v (%v)", url, err, time.Since(start))
			return false, nil
		}
		defer resp.Body.Close()
		body, err := ioutil.ReadAll(resp.Body)
		if err != nil {
			Logf("Got error reading response from %s: %v", url, err)
			return false, nil
		}
		if resp.StatusCode != 200 {
			return false, fmt.Errorf("received non-success return status %q trying to access %s; got body: %s", resp.Status, url, string(body))
		}
		if !strings.Contains(string(body), "test-webserver") {
			return false, fmt.Errorf("received response body without expected substring 'test-webserver': %s", string(body))
		}
		Logf("Successfully reached %v", url)
		return true, nil
	}))
}

func testNotReachable(ip string, port int) {
	url := fmt.Sprintf("http://%s:%d", ip, port)
	if ip == "" {
		Failf("Got empty IP for non-reachability check (%s)", url)
	}
	if port == 0 {
		Failf("Got port==0 for non-reachability check (%s)", url)
	}

	By(fmt.Sprintf("Waiting up to %v for %s to be *not* reachable", podStartTimeout, url))
	expectNoError(wait.Poll(poll, podStartTimeout, func() (bool, error) {
		resp, err := httpGetNoConnectionPool(url)
		if err != nil {
			Logf("Successfully waited for the url %s to be unreachable.", url)
			return true, nil
		}
		defer resp.Body.Close()
		body, err := ioutil.ReadAll(resp.Body)
		if err != nil {
			Logf("Expecting %s to be unreachable but was reachable and got an error reading response: %v", url, err)
			return false, nil
		}
		Logf("Able to reach service %s when should no longer have been reachable, status:%d and body: %s", url, resp.Status, string(body))
		return false, nil
	}))
}

// Does an HTTP GET, but does not reuse TCP connections
// This masks problems where the iptables rule has changed, but we don't see it
// This is intended for relatively quick requests (status checks), so we set a short (5 seconds) timeout
func httpGetNoConnectionPool(url string) (*http.Response, error) {
	tr := &http.Transport{
		DisableKeepAlives: true,
	}
	client := &http.Client{
		Transport: tr,
		Timeout:   5 * time.Second,
	}

	return client.Get(url)
}

// Simple helper class to avoid too much boilerplate in tests
type WebserverTest struct {
	ServiceName string
	Namespace   string
	Client      *client.Client

	TestId string
	Labels map[string]string

	rcs      map[string]bool
	services map[string]bool
	name     string
	image    string
}

func NewWebserverTest(client *client.Client, namespace string, serviceName string) *WebserverTest {
	t := &WebserverTest{}
	t.Client = client
	t.Namespace = namespace
	t.ServiceName = serviceName
	t.TestId = t.ServiceName + "-" + string(util.NewUUID())
	t.Labels = map[string]string{
		"testid": t.TestId,
	}

	t.rcs = make(map[string]bool)
	t.services = make(map[string]bool)

	t.name = "webserver"
	t.image = "gcr.io/google_containers/test-webserver"

	return t
}

// Build default config for a service (which can then be changed)
func (t *WebserverTest) BuildServiceSpec() *api.Service {
	service := &api.Service{
		ObjectMeta: api.ObjectMeta{
			Name: t.ServiceName,
		},
		Spec: api.ServiceSpec{
			Selector: t.Labels,
			Ports: []api.ServicePort{{
				Port:       80,
				TargetPort: util.NewIntOrStringFromInt(80),
			}},
		},
	}
	return service
}

// CreateWebserverRC creates rc-backed pods with the well-known webserver
// configuration and records it for cleanup.
func (t *WebserverTest) CreateWebserverRC(replicas int) *api.ReplicationController {
	rcSpec := rcByName(t.name, replicas, t.image, t.Labels)
	rcAct, err := t.createRC(rcSpec)
	if err != nil {
		Failf("Failed to create rc %s: %v", rcSpec.Name, err)
	}
	if err := verifyPods(t.Client, t.Namespace, t.name, false, replicas); err != nil {
		Failf("Failed to create %d pods with name %s: %v", replicas, t.name, err)
	}
	return rcAct
}

// createRC creates a replication controller and records it for cleanup.
func (t *WebserverTest) createRC(rc *api.ReplicationController) (*api.ReplicationController, error) {
	rc, err := t.Client.ReplicationControllers(t.Namespace).Create(rc)
	if err == nil {
		t.rcs[rc.Name] = true
	}
	return rc, err
}

// Create a service, and record it for cleanup
func (t *WebserverTest) CreateService(service *api.Service) (*api.Service, error) {
	result, err := t.Client.Services(t.Namespace).Create(service)
	if err == nil {
		t.services[service.Name] = true
	}
	return result, err
}

// Delete a service, and remove it from the cleanup list
func (t *WebserverTest) DeleteService(serviceName string) error {
	err := t.Client.Services(t.Namespace).Delete(serviceName)
	if err == nil {
		delete(t.services, serviceName)
	}
	return err
}

func (t *WebserverTest) Cleanup() []error {
	var errs []error
	for rcName := range t.rcs {
		By("stopping RC " + rcName + " in namespace " + t.Namespace)
		// First, resize the RC to 0.
		old, err := t.Client.ReplicationControllers(t.Namespace).Get(rcName)
		if err != nil {
			errs = append(errs, err)
		}
		old.Spec.Replicas = 0
		if _, err := t.Client.ReplicationControllers(t.Namespace).Update(old); err != nil {
			errs = append(errs, err)
		}
		// TODO(mbforbes): Wait.

		// Then, delete the RC altogether.
		if err := t.Client.ReplicationControllers(t.Namespace).Delete(rcName); err != nil {
			errs = append(errs, err)
		}
	}

	for serviceName := range t.services {
		By("deleting service " + serviceName + " in namespace " + t.Namespace)
		err := t.Client.Services(t.Namespace).Delete(serviceName)
		if err != nil {
			errs = append(errs, err)
		}
	}

	return errs
}
