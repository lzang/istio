// Copyright 2017 Istio Authors
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//     http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package kube

import (
	"fmt"
	"reflect"
	"strings"
	"testing"
	"time"

	"istio.io/api/annotation"
	"istio.io/istio/pilot/pkg/model"
	"istio.io/istio/pkg/spiffe"

	coreV1 "k8s.io/api/core/v1"
	metaV1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/intstr"
)

var (
	domainSuffix = "company.com"
	clusterID    = "test-cluster"
)

func TestConvertProtocol(t *testing.T) {
	type protocolCase struct {
		name  string
		proto coreV1.Protocol
		out   model.Protocol
	}
	protocols := []protocolCase{
		{"", coreV1.ProtocolTCP, model.ProtocolTCP},
		{"http", coreV1.ProtocolTCP, model.ProtocolHTTP},
		{"http-test", coreV1.ProtocolTCP, model.ProtocolHTTP},
		{"http", coreV1.ProtocolUDP, model.ProtocolUDP},
		{"httptest", coreV1.ProtocolTCP, model.ProtocolTCP},
		{"https", coreV1.ProtocolTCP, model.ProtocolHTTPS},
		{"https-test", coreV1.ProtocolTCP, model.ProtocolHTTPS},
		{"http2", coreV1.ProtocolTCP, model.ProtocolHTTP2},
		{"http2-test", coreV1.ProtocolTCP, model.ProtocolHTTP2},
		{"grpc", coreV1.ProtocolTCP, model.ProtocolGRPC},
		{"grpc-test", coreV1.ProtocolTCP, model.ProtocolGRPC},
		{"grpc-web", coreV1.ProtocolTCP, model.ProtocolGRPCWeb},
		{"grpc-web-test", coreV1.ProtocolTCP, model.ProtocolGRPCWeb},
		{"mongo", coreV1.ProtocolTCP, model.ProtocolMongo},
		{"mongo-test", coreV1.ProtocolTCP, model.ProtocolMongo},
		{"redis", coreV1.ProtocolTCP, model.ProtocolRedis},
		{"redis-test", coreV1.ProtocolTCP, model.ProtocolRedis},
		{"mysql", coreV1.ProtocolTCP, model.ProtocolMySQL},
		{"mysql-test", coreV1.ProtocolTCP, model.ProtocolMySQL},
	}

	// Create the list of cases for all of the names in both upper and lowercase.
	cases := make([]protocolCase, 0, len(protocols)*2)
	for _, p := range protocols {
		name := p.name

		p.name = strings.ToLower(name)
		cases = append(cases, p)

		// Don't bother adding uppercase version for empty string.
		if name != "" {
			p.name = strings.ToUpper(name)
			cases = append(cases, p)
		}
	}

	for _, c := range cases {
		testName := strings.Replace(fmt.Sprintf("%s_%s", c.name, c.proto), "-", "_", -1)
		t.Run(testName, func(t *testing.T) {
			out := ConvertProtocol(c.name, c.proto)
			if out != c.out {
				t.Fatalf("convertProtocol(%q, %q) => %q, want %q", c.name, c.proto, out, c.out)
			}
		})
	}
}

func BenchmarkConvertProtocol(b *testing.B) {
	cases := []struct {
		name  string
		proto coreV1.Protocol
		out   model.Protocol
	}{
		{"grpc-web-lowercase", coreV1.ProtocolTCP, model.ProtocolGRPCWeb},
		{"GRPC-WEB-mixedcase", coreV1.ProtocolTCP, model.ProtocolGRPCWeb},
		{"https-lowercase", coreV1.ProtocolTCP, model.ProtocolHTTPS},
		{"HTTPS-mixedcase", coreV1.ProtocolTCP, model.ProtocolHTTPS},
	}

	for _, c := range cases {
		testName := strings.Replace(c.name, "-", "_", -1)
		b.Run(testName, func(b *testing.B) {
			for i := 0; i < b.N; i++ {
				out := ConvertProtocol(c.name, c.proto)
				if out != c.out {
					b.Fatalf("convertProtocol(%q, %q) => %q, want %q", c.name, c.proto, out, c.out)
				}
			}
		})
	}
}

func TestServiceConversion(t *testing.T) {
	serviceName := "service1"
	namespace := "default"
	saA := "serviceaccountA"
	saB := "serviceaccountB"
	saC := "spiffe://accounts.google.com/serviceaccountC@cloudservices.gserviceaccount.com"
	saD := "spiffe://accounts.google.com/serviceaccountD@developer.gserviceaccount.com"

	oldTrustDomain := spiffe.GetTrustDomain()
	spiffe.SetTrustDomain(domainSuffix)
	defer spiffe.SetTrustDomain(oldTrustDomain)

	ip := "10.0.0.1"

	tnow := time.Now()
	localSvc := coreV1.Service{
		ObjectMeta: metaV1.ObjectMeta{
			Name:      serviceName,
			Namespace: namespace,
			Annotations: map[string]string{
				annotation.KubernetesServiceAccounts.Name: saA + "," + saB,
				annotation.CanonicalServiceAccounts.Name:  saC + "," + saD,
				"other/annotation":                        "test",
			},
			CreationTimestamp: metaV1.Time{Time: tnow},
		},
		Spec: coreV1.ServiceSpec{
			ClusterIP: ip,
			Ports: []coreV1.ServicePort{
				{
					Name:     "http",
					Port:     8080,
					Protocol: coreV1.ProtocolTCP,
				},
				{
					Name:     "https",
					Protocol: coreV1.ProtocolTCP,
					Port:     443,
				},
			},
		},
	}

	service := ConvertService(localSvc, domainSuffix, clusterID)
	if service == nil {
		t.Fatalf("could not convert service")
	}

	if service.CreationTime != tnow {
		t.Fatalf("incorrect creation time => %v, want %v", service.CreationTime, tnow)
	}

	if len(service.Ports) != len(localSvc.Spec.Ports) {
		t.Fatalf("incorrect number of ports => %v, want %v",
			len(service.Ports), len(localSvc.Spec.Ports))
	}

	if service.External() {
		t.Fatalf("service should not be external")
	}

	if service.Hostname != ServiceHostname(serviceName, namespace, domainSuffix) {
		t.Fatalf("service hostname incorrect => %q, want %q",
			service.Hostname, ServiceHostname(serviceName, namespace, domainSuffix))
	}

	if service.Address != ip {
		t.Fatalf("service IP incorrect => %q, want %q", service.Address, ip)
	}

	sa := service.ServiceAccounts
	if sa == nil || len(sa) != 4 {
		t.Fatalf("number of service accounts is incorrect")
	}
	expected := []string{
		saC, saD,
		"spiffe://company.com/ns/default/sa/" + saA,
		"spiffe://company.com/ns/default/sa/" + saB,
	}
	if !reflect.DeepEqual(sa, expected) {
		t.Fatalf("Unexpected service accounts %v (expecting %v)", sa, expected)
	}
}

func TestServiceConversionWithEmptyServiceAccountsAnnotation(t *testing.T) {
	serviceName := "service1"
	namespace := "default"

	ip := "10.0.0.1"

	localSvc := coreV1.Service{
		ObjectMeta: metaV1.ObjectMeta{
			Name:        serviceName,
			Namespace:   namespace,
			Annotations: map[string]string{},
		},
		Spec: coreV1.ServiceSpec{
			ClusterIP: ip,
			Ports: []coreV1.ServicePort{
				{
					Name:     "http",
					Port:     8080,
					Protocol: coreV1.ProtocolTCP,
				},
				{
					Name:     "https",
					Protocol: coreV1.ProtocolTCP,
					Port:     443,
				},
			},
		},
	}

	service := ConvertService(localSvc, domainSuffix, clusterID)
	if service == nil {
		t.Fatalf("could not convert service")
	}

	sa := service.ServiceAccounts
	if len(sa) != 0 {
		t.Fatalf("number of service accounts is incorrect: %d, expected 0", len(sa))
	}
}

func TestExternalServiceConversion(t *testing.T) {
	serviceName := "service1"
	namespace := "default"

	extSvc := coreV1.Service{
		ObjectMeta: metaV1.ObjectMeta{
			Name:      serviceName,
			Namespace: namespace,
		},
		Spec: coreV1.ServiceSpec{
			Ports: []coreV1.ServicePort{
				{
					Name:     "http",
					Port:     80,
					Protocol: coreV1.ProtocolTCP,
				},
			},
			Type:         coreV1.ServiceTypeExternalName,
			ExternalName: "google.com",
		},
	}

	service := ConvertService(extSvc, domainSuffix, clusterID)
	if service == nil {
		t.Fatalf("could not convert external service")
	}

	if len(service.Ports) != len(extSvc.Spec.Ports) {
		t.Fatalf("incorrect number of ports => %v, want %v",
			len(service.Ports), len(extSvc.Spec.Ports))
	}

	if !service.External() {
		t.Fatalf("service should be external")
	}

	if service.Hostname != ServiceHostname(serviceName, namespace, domainSuffix) {
		t.Fatalf("service hostname incorrect => %q, want %q",
			service.Hostname, ServiceHostname(serviceName, namespace, domainSuffix))
	}
}

func TestExternalClusterLocalServiceConversion(t *testing.T) {
	serviceName := "service1"
	namespace := "default"

	extSvc := coreV1.Service{
		ObjectMeta: metaV1.ObjectMeta{
			Name:      serviceName,
			Namespace: namespace,
		},
		Spec: coreV1.ServiceSpec{
			Ports: []coreV1.ServicePort{
				{
					Name:     "http",
					Port:     80,
					Protocol: coreV1.ProtocolTCP,
				},
			},
			Type:         coreV1.ServiceTypeExternalName,
			ExternalName: "some.test.svc.cluster.local",
		},
	}

	domainSuffix := "cluster.local"

	service := ConvertService(extSvc, domainSuffix, clusterID)
	if service == nil {
		t.Fatalf("could not convert external service")
	}

	if len(service.Ports) != len(extSvc.Spec.Ports) {
		t.Fatalf("incorrect number of ports => %v, want %v",
			len(service.Ports), len(extSvc.Spec.Ports))
	}

	if !service.External() {
		t.Fatalf("ExternalName service (even if .cluster.local) should be external")
	}

	if service.Hostname != ServiceHostname(serviceName, namespace, domainSuffix) {
		t.Fatalf("service hostname incorrect => %q, want %q",
			service.Hostname, ServiceHostname(serviceName, namespace, domainSuffix))
	}
}

func TestLBServiceConversion(t *testing.T) {
	serviceName := "service1"
	namespace := "default"

	addresses := []coreV1.LoadBalancerIngress{
		{
			IP: "127.68.32.112",
		},
		{
			IP: "127.68.32.113",
		},
	}

	extSvc := coreV1.Service{
		ObjectMeta: metaV1.ObjectMeta{
			Name:      serviceName,
			Namespace: namespace,
		},
		Spec: coreV1.ServiceSpec{
			Ports: []coreV1.ServicePort{
				{
					Name:     "http",
					Port:     80,
					Protocol: coreV1.ProtocolTCP,
				},
			},
			Type: coreV1.ServiceTypeLoadBalancer,
		},
		Status: coreV1.ServiceStatus{
			LoadBalancer: coreV1.LoadBalancerStatus{
				Ingress: addresses,
			},
		},
	}

	service := ConvertService(extSvc, domainSuffix, clusterID)
	if service == nil {
		t.Fatalf("could not convert external service")
	}

	if len(service.Attributes.ClusterExternalAddresses[clusterID]) == 0 {
		t.Fatalf("no load balancer addresses found")
	}

	for i, addr := range addresses {
		var want string
		if len(addr.IP) > 0 {
			want = addr.IP
		} else {
			want = addr.Hostname
		}
		got := service.Attributes.ClusterExternalAddresses[clusterID][i]
		if got != want {
			t.Fatalf("Expected address %s but got %s", want, got)
		}
	}
}

func TestProbesToPortsConversion(t *testing.T) {

	expected := model.PortList{
		{
			Name:     "mgmt-3306",
			Port:     3306,
			Protocol: model.ProtocolTCP,
		},
		{
			Name:     "mgmt-9080",
			Port:     9080,
			Protocol: model.ProtocolHTTP,
		},
	}

	handlers := []coreV1.Handler{
		{
			TCPSocket: &coreV1.TCPSocketAction{
				Port: intstr.IntOrString{StrVal: "mysql", Type: intstr.String},
			},
		},
		{
			TCPSocket: &coreV1.TCPSocketAction{
				Port: intstr.IntOrString{IntVal: 3306, Type: intstr.Int},
			},
		},
		{
			HTTPGet: &coreV1.HTTPGetAction{
				Path: "/foo",
				Port: intstr.IntOrString{StrVal: "http-two", Type: intstr.String},
			},
		},
		{
			HTTPGet: &coreV1.HTTPGetAction{
				Path: "/foo",
				Port: intstr.IntOrString{IntVal: 9080, Type: intstr.Int},
			},
		},
	}

	podSpec := &coreV1.PodSpec{
		Containers: []coreV1.Container{
			{
				Name: "scooby",
				Ports: []coreV1.ContainerPort{
					{
						Name:          "mysql",
						ContainerPort: 3306,
					},
					{
						Name:          "http-two",
						ContainerPort: 9080,
					},
					{
						Name:          "http",
						ContainerPort: 80,
					},
				},
				LivenessProbe:  &coreV1.Probe{},
				ReadinessProbe: &coreV1.Probe{},
			},
		},
	}

	for _, handler1 := range handlers {
		for _, handler2 := range handlers {
			if (handler1.TCPSocket != nil && handler2.TCPSocket != nil) ||
				(handler1.HTTPGet != nil && handler2.HTTPGet != nil) {
				continue
			}

			podSpec.Containers[0].LivenessProbe.Handler = handler1
			podSpec.Containers[0].ReadinessProbe.Handler = handler2

			mgmtPorts, err := ConvertProbesToPorts(podSpec)
			if err != nil {
				t.Fatalf("Failed to convert Probes to Ports: %v", err)
			}

			if !reflect.DeepEqual(mgmtPorts, expected) {
				t.Fatalf("incorrect number of management ports => %v, want %v",
					len(mgmtPorts), len(expected))
			}
		}
	}
}

func TestSecureNamingSANCustomIdentity(t *testing.T) {

	pod := &coreV1.Pod{}

	identity := "foo"

	pod.Annotations = make(map[string]string)
	pod.Annotations[annotation.Identity.Name] = identity

	san := SecureNamingSAN(pod)

	expectedSAN := fmt.Sprintf("spiffe://%v/%v", spiffe.GetTrustDomain(), identity)

	if san != expectedSAN {
		t.Fatalf("SAN match failed, SAN:%v  expectedSAN:%v", san, expectedSAN)
	}

}

func TestSecureNamingSAN(t *testing.T) {

	pod := &coreV1.Pod{}

	pod.Annotations = make(map[string]string)

	ns := "anything"
	sa := "foo"
	pod.Namespace = ns
	pod.Spec.ServiceAccountName = sa

	san := SecureNamingSAN(pod)

	expectedSAN := fmt.Sprintf("spiffe://%v/ns/%v/sa/%v", spiffe.GetTrustDomain(), ns, sa)

	if san != expectedSAN {
		t.Fatalf("SAN match failed, SAN:%v  expectedSAN:%v", san, expectedSAN)
	}
}
