// Copyright 2017 Authors of Cilium
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

// +build !privileged_tests

package main

import (
	"net"
	"time"

	"github.com/cilium/cilium/pkg/annotation"
	"github.com/cilium/cilium/pkg/endpointmanager"
	"github.com/cilium/cilium/pkg/identity"
	"github.com/cilium/cilium/pkg/ipcache"
	"github.com/cilium/cilium/pkg/k8s"
	k8sConst "github.com/cilium/cilium/pkg/k8s/apis/cilium.io"
	"github.com/cilium/cilium/pkg/k8s/apis/cilium.io/utils"
	"github.com/cilium/cilium/pkg/k8s/apis/cilium.io/v2"
	"github.com/cilium/cilium/pkg/labels"
	"github.com/cilium/cilium/pkg/loadbalancer"
	"github.com/cilium/cilium/pkg/node"
	"github.com/cilium/cilium/pkg/option"
	"github.com/cilium/cilium/pkg/policy"
	"github.com/cilium/cilium/pkg/policy/api"
	"github.com/cilium/cilium/pkg/versioned"

	. "gopkg.in/check.v1"
	core_v1 "k8s.io/api/core/v1"
	"k8s.io/api/networking/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/tools/cache"
)

func (ds *DaemonSuite) TestK8sErrorLogTimeout(c *C) {
	errstr := "I am an error string"

	// ensure k8sErrMsg is empty for tests that use it
	k8sErrMsgMU.Lock()
	k8sErrMsg = map[string]time.Time{}
	k8sErrMsgMU.Unlock()

	// Returns true because it's the first time we see this message
	startTime := time.Now()
	c.Assert(k8sErrorUpdateCheckUnmuteTime(errstr, startTime), Equals, true)

	// Returns false because <= k8sErrLogTimeout time has passed
	noLogTime := startTime.Add(k8sErrLogTimeout)
	c.Assert(k8sErrorUpdateCheckUnmuteTime(errstr, noLogTime), Equals, false)

	// Returns true because k8sErrLogTimeout has passed
	shouldLogTime := startTime.Add(k8sErrLogTimeout).Add(time.Nanosecond)
	c.Assert(k8sErrorUpdateCheckUnmuteTime(errstr, shouldLogTime), Equals, true)
}

func (ds *DaemonSuite) Test_missingK8sNetworkPolicyV1(c *C) {
	type args struct {
		m    versioned.Map
		repo *policy.Repository
	}
	tests := []struct {
		name        string
		setupArgs   func() args
		setupWanted func() versioned.Map
	}{
		{
			name: "both equal",
			setupArgs: func() args {
				p1 := policy.NewPolicyRepository()
				return args{
					repo: p1,
					m:    versioned.NewMap(),
				}
			},
			setupWanted: func() versioned.Map {
				return versioned.NewMap()
			},
		},
		{
			name: "repository is missing a policy",
			setupArgs: func() args {
				p1 := policy.NewPolicyRepository()

				m := versioned.NewMap()
				m.Add("", versioned.Object{
					Data: &v1.NetworkPolicy{
						ObjectMeta: metav1.ObjectMeta{
							Name:      "bar",
							Namespace: "foo",
						},
					},
				})

				return args{
					m:    m,
					repo: p1,
				}
			},
			setupWanted: func() versioned.Map {
				m := versioned.NewMap()
				m.Add("", versioned.Object{
					Data: &v1.NetworkPolicy{
						ObjectMeta: metav1.ObjectMeta{
							Name:      "bar",
							Namespace: "foo",
						},
					},
				})
				return m
			},
		},
		{
			name: "repository contains all policies",
			setupArgs: func() args {
				p1 := policy.NewPolicyRepository()
				_, err := p1.Add(api.Rule{
					EndpointSelector: api.NewESFromLabels(labels.ParseSelectLabel("id=a")),
					Labels: labels.LabelArray{labels.NewLabel(k8sConst.PolicyLabelName, "bar", labels.LabelSourceK8s),
						labels.NewLabel(k8sConst.PolicyLabelNamespace, "foo", labels.LabelSourceK8s)},
				})
				c.Assert(err, IsNil)

				m := versioned.NewMap()
				m.Add("", versioned.Object{
					Data: &v1.NetworkPolicy{
						ObjectMeta: metav1.ObjectMeta{
							Name:      "bar",
							Namespace: "foo",
						},
					},
				})

				return args{
					m:    m,
					repo: p1,
				}
			},
			setupWanted: func() versioned.Map {
				return versioned.NewMap()
			},
		},
	}
	for _, tt := range tests {
		args := tt.setupArgs()
		want := tt.setupWanted()
		ds.d.policy = args.repo
		got := ds.d.missingK8sNetworkPolicyV1(args.m)
		c.Assert(got, DeepEquals, want, Commentf("Test name: %q", tt.name))
	}
}

func (ds *DaemonSuite) Test_missingCNPv2(c *C) {
	type args struct {
		m    versioned.Map
		repo *policy.Repository
	}
	tests := []struct {
		name        string
		setupArgs   func() args
		setupWanted func() versioned.Map
	}{
		{
			name: "both equal",
			setupArgs: func() args {
				p1 := policy.NewPolicyRepository()
				return args{
					repo: p1,
					m:    versioned.NewMap(),
				}
			},
			setupWanted: func() versioned.Map {
				return versioned.NewMap()
			},
		},
		{
			name: "repository is missing a policy",
			setupArgs: func() args {
				p1 := policy.NewPolicyRepository()

				m := versioned.NewMap()
				m.Add("", versioned.Object{
					Data: &v2.CiliumNetworkPolicy{
						Spec: &api.Rule{
							EndpointSelector: api.NewESFromLabels(labels.ParseSelectLabel("id=a")),
							Labels:           labels.ParseLabelArray("k8s:name=bar", "k8s:namespace=bar"),
						},
					},
				})

				return args{
					m:    m,
					repo: p1,
				}
			},
			setupWanted: func() versioned.Map {
				m := versioned.NewMap()
				m.Add("", versioned.Object{
					Data: &v2.CiliumNetworkPolicy{
						Spec: &api.Rule{
							EndpointSelector: api.NewESFromLabels(labels.ParseSelectLabel("id=a")),
							Labels:           labels.ParseLabelArray("k8s:name=bar", "k8s:namespace=bar"),
						},
					},
				})
				return m
			},
		},
		{
			name: "repository contains all policies",
			setupArgs: func() args {
				p1 := policy.NewPolicyRepository()
				_, err := p1.Add(api.Rule{
					EndpointSelector: api.NewESFromLabels(labels.ParseSelectLabel("id=a")),
					Labels: utils.GetPolicyLabels("foo", "bar",
						utils.ResourceTypeCiliumNetworkPolicy),
				})
				c.Assert(err, IsNil)

				m := versioned.NewMap()
				m.Add("", versioned.Object{
					Data: &v2.CiliumNetworkPolicy{
						ObjectMeta: metav1.ObjectMeta{
							Name:      "bar",
							Namespace: "foo",
						},
						Spec: &api.Rule{
							EndpointSelector: api.NewESFromLabels(labels.ParseSelectLabel("id=a")),
							Labels:           labels.ParseLabelArray("k8s:name=bar", "k8s:namespace=bar"),
						},
					},
				})

				return args{
					m:    m,
					repo: p1,
				}
			},
			setupWanted: func() versioned.Map {
				return versioned.NewMap()
			},
		},
	}
	for _, tt := range tests {
		args := tt.setupArgs()
		want := tt.setupWanted()
		ds.d.policy = args.repo
		got := ds.d.missingCNPv2(args.m)
		c.Assert(got, DeepEquals, want, Commentf("Test name: %q", tt.name))
	}
}

func (ds *DaemonSuite) Test_parseK8sEPv1(c *C) {
	type args struct {
		eps *core_v1.Endpoints
	}
	tests := []struct {
		name        string
		setupArgs   func() args
		setupWanted func() *k8s.Endpoints
	}{
		{
			name: "empty endpoint",
			setupArgs: func() args {
				return args{
					eps: &core_v1.Endpoints{
						ObjectMeta: metav1.ObjectMeta{
							Name:      "foo",
							Namespace: "bar",
						},
					},
				}
			},
			setupWanted: func() *k8s.Endpoints {
				return k8s.NewEndpoints()
			},
		},
		{
			name: "endpoint with an address and port",
			setupArgs: func() args {
				return args{
					eps: &core_v1.Endpoints{
						ObjectMeta: metav1.ObjectMeta{
							Name:      "foo",
							Namespace: "bar",
						},
						Subsets: []core_v1.EndpointSubset{
							{
								Addresses: []core_v1.EndpointAddress{
									{
										IP: "172.0.0.1",
									},
								},
								Ports: []core_v1.EndpointPort{
									{
										Name:     "http-test-svc",
										Port:     8080,
										Protocol: core_v1.ProtocolTCP,
									},
								},
							},
						},
					},
				}
			},
			setupWanted: func() *k8s.Endpoints {
				svcEP := k8s.NewEndpoints()
				svcEP.Ports["http-test-svc"] = loadbalancer.NewL4Addr(loadbalancer.TCP, 8080)
				svcEP.BackendIPs["172.0.0.1"] = true
				return svcEP
			},
		},
		{
			name: "endpoint with an address and 2 ports",
			setupArgs: func() args {
				return args{
					eps: &core_v1.Endpoints{
						ObjectMeta: metav1.ObjectMeta{
							Name:      "foo",
							Namespace: "bar",
						},
						Subsets: []core_v1.EndpointSubset{
							{
								Addresses: []core_v1.EndpointAddress{
									{
										IP: "172.0.0.1",
									},
								},
								Ports: []core_v1.EndpointPort{
									{
										Name:     "http-test-svc",
										Port:     8080,
										Protocol: core_v1.ProtocolTCP,
									},
									{
										Name:     "http-test-svc-2",
										Port:     8081,
										Protocol: core_v1.ProtocolTCP,
									},
								},
							},
						},
					},
				}
			},
			setupWanted: func() *k8s.Endpoints {
				svcEP := k8s.NewEndpoints()
				svcEP.Ports["http-test-svc"] = loadbalancer.NewL4Addr(loadbalancer.TCP, 8080)
				svcEP.Ports["http-test-svc-2"] = loadbalancer.NewL4Addr(loadbalancer.TCP, 8081)
				svcEP.BackendIPs["172.0.0.1"] = true
				return svcEP
			},
		},
		{
			name: "endpoint with 2 addresses and 2 ports",
			setupArgs: func() args {
				return args{
					eps: &core_v1.Endpoints{
						ObjectMeta: metav1.ObjectMeta{
							Name:      "foo",
							Namespace: "bar",
						},
						Subsets: []core_v1.EndpointSubset{
							{
								Addresses: []core_v1.EndpointAddress{
									{
										IP: "172.0.0.1",
									},
									{
										IP: "172.0.0.2",
									},
								},
								Ports: []core_v1.EndpointPort{
									{
										Name:     "http-test-svc",
										Port:     8080,
										Protocol: core_v1.ProtocolTCP,
									},
									{
										Name:     "http-test-svc-2",
										Port:     8081,
										Protocol: core_v1.ProtocolTCP,
									},
								},
							},
						},
					},
				}
			},
			setupWanted: func() *k8s.Endpoints {
				svcEP := k8s.NewEndpoints()
				svcEP.Ports["http-test-svc"] = loadbalancer.NewL4Addr(loadbalancer.TCP, 8080)
				svcEP.Ports["http-test-svc-2"] = loadbalancer.NewL4Addr(loadbalancer.TCP, 8081)
				svcEP.BackendIPs["172.0.0.1"] = true
				svcEP.BackendIPs["172.0.0.2"] = true
				return svcEP
			},
		},
		{
			name: "endpoint with 2 addresses, 1 address not ready and 2 ports",
			setupArgs: func() args {
				return args{
					eps: &core_v1.Endpoints{
						ObjectMeta: metav1.ObjectMeta{
							Name:      "foo",
							Namespace: "bar",
						},
						Subsets: []core_v1.EndpointSubset{
							{
								NotReadyAddresses: []core_v1.EndpointAddress{
									{
										IP: "172.0.0.3",
									},
								},
								Addresses: []core_v1.EndpointAddress{
									{
										IP: "172.0.0.1",
									},
									{
										IP: "172.0.0.2",
									},
								},
								Ports: []core_v1.EndpointPort{
									{
										Name:     "http-test-svc",
										Port:     8080,
										Protocol: core_v1.ProtocolTCP,
									},
									{
										Name:     "http-test-svc-2",
										Port:     8081,
										Protocol: core_v1.ProtocolTCP,
									},
								},
							},
						},
					},
				}
			},
			setupWanted: func() *k8s.Endpoints {
				svcEP := k8s.NewEndpoints()
				svcEP.Ports["http-test-svc"] = loadbalancer.NewL4Addr(loadbalancer.TCP, 8080)
				svcEP.Ports["http-test-svc-2"] = loadbalancer.NewL4Addr(loadbalancer.TCP, 8081)
				svcEP.BackendIPs["172.0.0.1"] = true
				svcEP.BackendIPs["172.0.0.2"] = true
				return svcEP
			},
		},
	}
	for _, tt := range tests {
		args := tt.setupArgs()
		want := tt.setupWanted()
		_, got := k8s.ParseEndpoints(args.eps)
		c.Assert(got, DeepEquals, want, Commentf("Test name: %q", tt.name))
	}
}

func (ds *DaemonSuite) Test_missingK8sPodV1(c *C) {
	defer endpointmanager.RemoveAll()
	type args struct {
		m     versioned.Map
		cache *ipcache.IPCache
	}
	tests := []struct {
		name        string
		setupArgs   func() args
		setupWanted func() versioned.Map
	}{
		{
			name: "both equal",
			setupArgs: func() args {
				return args{
					cache: ipcache.NewIPCache(),
					m:     versioned.NewMap(),
				}
			},
			setupWanted: func() versioned.Map {
				return versioned.NewMap()
			},
		},
		{
			name: "ipcache is missing a pod",
			setupArgs: func() args {
				endpointmanager.RemoveAll()
				endpointmanager.Insert(endpointCreator(123, identity.NumericIdentity(1000)))
				m := versioned.NewMap()
				m.Add("", versioned.Object{
					Data: &core_v1.Pod{
						ObjectMeta: metav1.ObjectMeta{
							Name:      "foo",
							Namespace: "bar",
						},
						Status: core_v1.PodStatus{
							PodIP: "127.0.0.1",
						},
					},
				})

				return args{
					m:     m,
					cache: ipcache.NewIPCache(),
				}
			},
			setupWanted: func() versioned.Map {
				m := versioned.NewMap()
				m.Add("", versioned.Object{
					Data: &core_v1.Pod{
						ObjectMeta: metav1.ObjectMeta{
							Name:      "foo",
							Namespace: "bar",
						},
						Status: core_v1.PodStatus{
							PodIP: "127.0.0.1",
						},
					},
				})
				return m
			},
		},
		{
			name: "ipcache contains the pod but endpointmanager doesn't contain any endpoint that manages the pod. Should be no-op",
			setupArgs: func() args {
				endpointmanager.RemoveAll()
				endpointmanager.Insert(endpointCreator(123, identity.NumericIdentity(1000)))
				cache := ipcache.NewIPCache()
				cache.Upsert("127.0.0.1", net.ParseIP("127.0.0.2"), ipcache.Identity{
					ID:     identity.ReservedIdentityInit,
					Source: ipcache.FromKubernetes,
				})
				m := versioned.NewMap()
				m.Add("", versioned.Object{
					Data: &core_v1.Pod{
						ObjectMeta: metav1.ObjectMeta{
							Name:      "foo",
							Namespace: "bar",
						},
						Status: core_v1.PodStatus{
							PodIP:  "127.0.0.1",
							HostIP: "127.0.0.2",
						},
					},
				})

				return args{
					m:     m,
					cache: cache,
				}
			},
			setupWanted: func() versioned.Map {
				return versioned.NewMap()
			},
		},
		{
			name: "ipcache contains the pod and endpointmanager contains the endpoint that manages the pod but ep doesn't have all labels",
			setupArgs: func() args {
				endpointmanager.RemoveAll()
				ep := endpointCreator(123, identity.NumericIdentity(1000))
				ep.SetK8sPodName("foo")
				ep.SetK8sNamespace("bar")
				endpointmanager.Insert(ep)
				cache := ipcache.NewIPCache()
				cache.Upsert("127.0.0.1", net.ParseIP("127.0.0.2"), ipcache.Identity{
					ID:     identity.ReservedIdentityInit,
					Source: ipcache.FromKubernetes,
				})
				m := versioned.NewMap()
				m.Add("", versioned.Object{
					Data: &core_v1.Pod{
						ObjectMeta: metav1.ObjectMeta{
							Name:      "foo",
							Namespace: "bar",
							Labels: map[string]string{
								"id.foo": "bar",
							},
						},
						Status: core_v1.PodStatus{
							PodIP:  "127.0.0.1",
							HostIP: "127.0.0.2",
						},
					},
				})

				return args{
					m:     m,
					cache: cache,
				}
			},
			setupWanted: func() versioned.Map {
				m := versioned.NewMap()
				m.Add("", versioned.Object{
					Data: &core_v1.Pod{
						ObjectMeta: metav1.ObjectMeta{
							Name:      "foo",
							Namespace: "bar",
							Labels: map[string]string{
								"id.foo": "bar",
							},
						},
						Status: core_v1.PodStatus{
							PodIP:  "127.0.0.1",
							HostIP: "127.0.0.2",
						},
					},
				})
				return m
			},
		},
		{
			name: "ipcache contains the pod and endpointmanager contains the endpoint that manages the pod and have all labels",
			setupArgs: func() args {
				endpointmanager.RemoveAll()
				ep := endpointCreator(123, identity.NumericIdentity(1000))
				ep.OpLabels.OrchestrationIdentity = labels.Map2Labels(map[string]string{"foo": "bar"}, labels.LabelSourceK8s)
				ep.SetK8sPodName("foo")
				ep.SetK8sNamespace("bar")
				endpointmanager.Insert(ep)
				cache := ipcache.NewIPCache()
				cache.Upsert("127.0.0.1", net.ParseIP("127.0.0.2"), ipcache.Identity{
					ID:     identity.ReservedIdentityInit,
					Source: ipcache.FromKubernetes,
				})
				m := versioned.NewMap()
				m.Add("", versioned.Object{
					Data: &core_v1.Pod{
						ObjectMeta: metav1.ObjectMeta{
							Name:      "foo",
							Namespace: "bar",
							Labels: map[string]string{
								"foo": "bar",
							},
						},
						Status: core_v1.PodStatus{
							PodIP:  "127.0.0.1",
							HostIP: "127.0.0.2",
						},
					},
				})

				return args{
					m:     m,
					cache: cache,
				}
			},
			setupWanted: func() versioned.Map {
				return versioned.NewMap()
			},
		},
		{
			name: "ipcache contains the pod and endpointmanager contains the endpoint that manages the pod but ep has old pod labels",
			setupArgs: func() args {
				endpointmanager.RemoveAll()
				ep := endpointCreator(123, identity.NumericIdentity(1000))
				ep.OpLabels.OrchestrationIdentity = labels.Map2Labels(map[string]string{"foo": "bar"}, labels.LabelSourceK8s)
				ep.SetK8sPodName("foo")
				ep.SetK8sNamespace("bar")
				endpointmanager.Insert(ep)
				cache := ipcache.NewIPCache()
				cache.Upsert("127.0.0.1", net.ParseIP("127.0.0.2"), ipcache.Identity{
					ID:     identity.ReservedIdentityInit,
					Source: ipcache.FromKubernetes,
				})
				m := versioned.NewMap()
				m.Add("", versioned.Object{
					Data: &core_v1.Pod{
						ObjectMeta: metav1.ObjectMeta{
							Name:      "foo",
							Namespace: "bar",
						},
						Status: core_v1.PodStatus{
							PodIP:  "127.0.0.1",
							HostIP: "127.0.0.2",
						},
					},
				})

				return args{
					m:     m,
					cache: cache,
				}
			},
			setupWanted: func() versioned.Map {
				m := versioned.NewMap()
				m.Add("", versioned.Object{
					Data: &core_v1.Pod{
						ObjectMeta: metav1.ObjectMeta{
							Name:      "foo",
							Namespace: "bar",
						},
						Status: core_v1.PodStatus{
							PodIP:  "127.0.0.1",
							HostIP: "127.0.0.2",
						},
					},
				})
				return m
			},
		},
	}
	for _, tt := range tests {
		args := tt.setupArgs()
		want := tt.setupWanted()
		ipcache.IPIdentityCache = args.cache
		got := missingK8sPodV1(args.m)
		c.Assert(got, DeepEquals, want, Commentf("Test name: %q", tt.name))
	}
}

func (ds *DaemonSuite) Test_missingK8sNodeV1(c *C) {
	defer node.DeleteAllNodes()
	prevClusterName := option.Config.ClusterName
	option.Config.ClusterName = "default"
	defer func() {
		option.Config.ClusterName = prevClusterName
	}()
	type args struct {
		m     versioned.Map
		cache *ipcache.IPCache
	}
	tests := []struct {
		name        string
		setupArgs   func() args
		setupWanted func() versioned.Map
	}{
		{
			name: "both equal",
			setupArgs: func() args {
				return args{
					cache: ipcache.NewIPCache(),
					m:     versioned.NewMap(),
				}
			},
			setupWanted: func() versioned.Map {
				return versioned.NewMap()
			},
		},
		{
			name: "ipcache is missing a node",
			setupArgs: func() args {
				m := versioned.NewMap()
				m.Add("", versioned.Object{
					Data: &core_v1.Node{
						ObjectMeta: metav1.ObjectMeta{
							Name: "foo",
							Annotations: map[string]string{
								annotation.CiliumHostIP: "127.0.0.1",
							},
						},
					},
				})

				return args{
					m:     m,
					cache: ipcache.NewIPCache(),
				}
			},
			setupWanted: func() versioned.Map {
				m := versioned.NewMap()
				m.Add("", versioned.Object{
					Data: &core_v1.Node{
						ObjectMeta: metav1.ObjectMeta{
							Name: "foo",
							Annotations: map[string]string{
								annotation.CiliumHostIP: "127.0.0.1",
							},
						},
					},
				})
				return m
			},
		},
		{
			name: "ipcache and the node package contains the node. Should be no-op",
			setupArgs: func() args {
				node.DeleteAllNodes()
				cache := ipcache.NewIPCache()
				cache.Upsert("172.20.0.1", net.ParseIP("172.20.0.2"), ipcache.Identity{
					ID:     identity.ReservedIdentityInit,
					Source: ipcache.FromKubernetes,
				})
				node.UpdateNode(&node.Node{
					Name:    "foo",
					Cluster: "default",
					IPAddresses: []node.Address{
						{
							AddressType: core_v1.NodeAddressType(core_v1.NodeInternalIP),
							IP:          net.ParseIP("172.20.0.1"),
						},
					},
				}, 0, net.ParseIP("172.20.0.2"))

				m := versioned.NewMap()
				m.Add("", versioned.Object{
					Data: &core_v1.Node{
						ObjectMeta: metav1.ObjectMeta{
							Name: "foo",
							Annotations: map[string]string{
								annotation.CiliumHostIP: "172.20.0.1",
							},
						},
					},
				})

				return args{
					m:     m,
					cache: cache,
				}
			},
			setupWanted: func() versioned.Map {
				return versioned.NewMap()
			},
		},
		{
			name: "node doesn't contain any cilium host IP. should be no-op",
			setupArgs: func() args {
				node.DeleteAllNodes()
				cache := ipcache.NewIPCache()
				cache.Upsert("127.0.0.1", net.ParseIP("127.0.0.2"), ipcache.Identity{
					ID:     identity.ReservedIdentityInit,
					Source: ipcache.FromKubernetes,
				})
				m := versioned.NewMap()
				m.Add("", versioned.Object{
					Data: &core_v1.Node{
						ObjectMeta: metav1.ObjectMeta{
							Name: "foo",
							Labels: map[string]string{
								"foo": "bar",
							},
						},
					},
				})

				return args{
					m:     m,
					cache: cache,
				}
			},
			setupWanted: func() versioned.Map {
				return versioned.NewMap()
			},
		},
		{
			name: "ipcache contains the node but the CiliumHostIP is not the right one for the given nodeIP",
			setupArgs: func() args {
				node.DeleteAllNodes()
				cache := ipcache.NewIPCache()
				cache.Upsert("172.20.9.1", net.ParseIP("172.20.1.1"), ipcache.Identity{
					ID:     identity.ReservedIdentityInit,
					Source: ipcache.FromKubernetes,
				})
				node.UpdateNode(&node.Node{
					Name:    "bar",
					Cluster: "default",
					IPAddresses: []node.Address{
						{
							AddressType: core_v1.NodeAddressType(core_v1.NodeInternalIP),
							IP:          net.ParseIP("172.20.0.1"),
						},
					},
				}, 0, net.ParseIP("172.20.2.1"))
				m := versioned.NewMap()
				m.Add("", versioned.Object{
					Data: &core_v1.Node{
						ObjectMeta: metav1.ObjectMeta{
							Name: "bar",
							Annotations: map[string]string{
								annotation.CiliumHostIP: "172.20.9.1",
							},
						},
					},
				})

				return args{
					m:     m,
					cache: cache,
				}
			},
			setupWanted: func() versioned.Map {
				m := versioned.NewMap()
				m.Add("", versioned.Object{
					Data: &core_v1.Node{
						ObjectMeta: metav1.ObjectMeta{
							Name: "bar",
							Annotations: map[string]string{
								annotation.CiliumHostIP: "172.20.9.1",
							},
						},
					},
				})
				return m
			},
		},
	}
	for _, tt := range tests {
		args := tt.setupArgs()
		want := tt.setupWanted()
		ipcache.IPIdentityCache = args.cache
		got := ds.d.missingK8sNodeV1(args.m)
		c.Assert(got, DeepEquals, want, Commentf("Test name: %q", tt.name))
	}
}

func (ds *DaemonSuite) Test_missingK8sNamespaceV1(c *C) {
	type args struct {
		m versioned.Map
	}
	tests := []struct {
		name        string
		setupArgs   func() args
		setupWanted func() versioned.Map
	}{
		{
			name: "both equal",
			setupArgs: func() args {
				return args{
					m: versioned.NewMap(),
				}
			},
			setupWanted: func() versioned.Map {
				return versioned.NewMap()
			},
		},
		{
			name: "endpointmanager doesn't contain any endpoint that is part of that namespace. Should be no-op",
			setupArgs: func() args {
				ep := endpointCreator(123, identity.NumericIdentity(1000))
				endpointmanager.Insert(ep)
				m := versioned.NewMap()
				m.Add("", versioned.Object{
					Data: &core_v1.Namespace{
						ObjectMeta: metav1.ObjectMeta{
							Name: "bar",
						},
					},
				})

				return args{
					m: m,
				}
			},
			setupWanted: func() versioned.Map {
				return versioned.NewMap()
			},
		},
		{
			name: "endpointmanager contains the endpoint that is part of that namespace but ep doesn't have all labels",
			setupArgs: func() args {
				ep := endpointCreator(123, identity.NumericIdentity(1000))
				ep.SetK8sNamespace("foo")
				endpointmanager.Insert(ep)
				m := versioned.NewMap()
				m.Add("", versioned.Object{
					Data: &core_v1.Namespace{
						ObjectMeta: metav1.ObjectMeta{
							Name: "foo",
							Labels: map[string]string{
								"id.foo": "bar",
							},
						},
					},
				})

				return args{
					m: m,
				}
			},
			setupWanted: func() versioned.Map {
				m := versioned.NewMap()
				m.Add("", versioned.Object{
					Data: &core_v1.Namespace{
						ObjectMeta: metav1.ObjectMeta{
							Name: "foo",
							Labels: map[string]string{
								"id.foo": "bar",
							},
						},
					},
				})
				return m
			},
		},
		{
			name: "endpointmanager contains the endpoint that is part of that namespace and have all labels",
			setupArgs: func() args {
				ep := endpointCreator(123, identity.NumericIdentity(1000))
				ep.OpLabels.OrchestrationIdentity = labels.Map2Labels(
					map[string]string{policy.JoinPath(k8sConst.PodNamespaceMetaLabels, "id.foo"): "bar"},
					labels.LabelSourceK8s)
				ep.SetK8sNamespace("foo")
				endpointmanager.Insert(ep)
				m := versioned.NewMap()
				m.Add("", versioned.Object{
					Data: &core_v1.Namespace{
						ObjectMeta: metav1.ObjectMeta{
							Name: "foo",
							Labels: map[string]string{
								"id.foo": "bar",
							},
						},
					},
				})

				return args{
					m: m,
				}
			},
			setupWanted: func() versioned.Map {
				return versioned.NewMap()
			},
		},
	}
	for _, tt := range tests {
		args := tt.setupArgs()
		want := tt.setupWanted()
		got := ds.d.missingK8sNamespaceV1(args.m)
		c.Assert(got, DeepEquals, want, Commentf("Test name: %q", tt.name))
	}
}

func (ds *DaemonSuite) Test_addCiliumNetworkPolicyV2(c *C) {
	// ciliumV2Store cache.Store, oldRules api.Rules, cnp *cilium_v2.CiliumNetworkPolicy
	type args struct {
		ciliumV2Store cache.Store
		cnp           *v2.CiliumNetworkPolicy
		repo          *policy.Repository
	}
	type wanted struct {
		err  error
		repo *policy.Repository
	}
	tests := []struct {
		name        string
		setupArgs   func() args
		setupWanted func() wanted
	}{
		{
			name: "simple policy added",
			setupArgs: func() args {
				return args{
					ciliumV2Store: &cache.FakeCustomStore{},
					cnp: &v2.CiliumNetworkPolicy{
						ObjectMeta: metav1.ObjectMeta{
							Name:      "db",
							Namespace: "production",
						},
						Spec: &api.Rule{
							EndpointSelector: api.EndpointSelector{
								LabelSelector: &metav1.LabelSelector{
									MatchLabels: map[string]string{
										"env": "cluster-1",
									},
								},
							},
						},
					},
					repo: policy.NewPolicyRepository(),
				}
			},
			setupWanted: func() wanted {
				r := policy.NewPolicyRepository()
				r.AddList(api.Rules{
					{
						EndpointSelector: api.EndpointSelector{
							LabelSelector: &metav1.LabelSelector{
								MatchLabels: map[string]string{
									"env": "cluster-1",
									labels.LabelSourceK8s + "." + k8sConst.PodNamespaceLabel: "production",
								},
							},
						},
						Ingress:     nil,
						Egress:      nil,
						Labels:      utils.GetPolicyLabels("production", "db", utils.ResourceTypeCiliumNetworkPolicy),
						Description: "",
					},
				})
				return wanted{
					err:  nil,
					repo: r,
				}
			},
		},
		{
			name: "have a rule with user labels and update it without user labels, all other rules should be deleted",
			setupArgs: func() args {
				r := policy.NewPolicyRepository()
				lbls := utils.GetPolicyLabels("production", "db", utils.ResourceTypeCiliumNetworkPolicy)
				lbls = append(lbls, labels.ParseLabelArray("foo=bar")...)
				r.AddList(api.Rules{
					{
						EndpointSelector: api.EndpointSelector{
							LabelSelector: &metav1.LabelSelector{
								MatchLabels: map[string]string{
									"env": "cluster-1",
									labels.LabelSourceK8s + "." + k8sConst.PodNamespaceLabel: "production",
								},
							},
						},
						Ingress:     nil,
						Egress:      nil,
						Labels:      lbls,
						Description: "",
					},
				})
				return args{
					ciliumV2Store: &cache.FakeCustomStore{},
					cnp: &v2.CiliumNetworkPolicy{
						ObjectMeta: metav1.ObjectMeta{
							Name:      "db",
							Namespace: "production",
						},
						Spec: &api.Rule{
							EndpointSelector: api.EndpointSelector{
								LabelSelector: &metav1.LabelSelector{
									MatchLabels: map[string]string{
										"env": "cluster-1",
									},
								},
							},
						},
					},
					repo: r,
				}
			},
			setupWanted: func() wanted {
				r := policy.NewPolicyRepository()
				r.AddList(api.Rules{
					{
						EndpointSelector: api.EndpointSelector{
							LabelSelector: &metav1.LabelSelector{
								MatchLabels: map[string]string{
									"env": "cluster-1",
									labels.LabelSourceK8s + "." + k8sConst.PodNamespaceLabel: "production",
								},
							},
						},
						Ingress:     nil,
						Egress:      nil,
						Labels:      utils.GetPolicyLabels("production", "db", utils.ResourceTypeCiliumNetworkPolicy),
						Description: "",
					},
				})
				return wanted{
					err:  nil,
					repo: r,
				}
			},
		},
		{
			name: "have a rule without user labels and update it with user labels, all other rules should be deleted",
			setupArgs: func() args {
				r := policy.NewPolicyRepository()
				r.AddList(api.Rules{
					{
						EndpointSelector: api.EndpointSelector{
							LabelSelector: &metav1.LabelSelector{
								MatchLabels: map[string]string{
									"env": "cluster-1",
									labels.LabelSourceK8s + "." + k8sConst.PodNamespaceLabel: "production",
								},
							},
						},
						Ingress:     nil,
						Egress:      nil,
						Labels:      utils.GetPolicyLabels("production", "db", utils.ResourceTypeCiliumNetworkPolicy),
						Description: "",
					},
				})
				return args{
					ciliumV2Store: &cache.FakeCustomStore{},
					cnp: &v2.CiliumNetworkPolicy{
						ObjectMeta: metav1.ObjectMeta{
							Name:      "db",
							Namespace: "production",
						},
						Spec: &api.Rule{
							EndpointSelector: api.EndpointSelector{
								LabelSelector: &metav1.LabelSelector{
									MatchLabels: map[string]string{
										"env": "cluster-1",
									},
								},
							},
							Labels: labels.ParseLabelArray("foo=bar"),
						},
					},
					repo: r,
				}
			},
			setupWanted: func() wanted {
				r := policy.NewPolicyRepository()
				lbls := utils.GetPolicyLabels("production", "db", utils.ResourceTypeCiliumNetworkPolicy)
				lbls = append(lbls, labels.ParseLabelArray("foo=bar")...)
				r.AddList(api.Rules{
					{
						EndpointSelector: api.EndpointSelector{
							LabelSelector: &metav1.LabelSelector{
								MatchLabels: map[string]string{
									"env": "cluster-1",
									labels.LabelSourceK8s + "." + k8sConst.PodNamespaceLabel: "production",
								},
							},
						},
						Ingress:     nil,
						Egress:      nil,
						Labels:      lbls,
						Description: "",
					},
				})
				return wanted{
					err:  nil,
					repo: r,
				}
			},
		},
		{
			name: "have a rule policy installed with multiple rules and apply an empty spec should delete all rules installed",
			setupArgs: func() args {
				r := policy.NewPolicyRepository()
				r.AddList(api.Rules{
					{
						EndpointSelector: api.EndpointSelector{
							LabelSelector: &metav1.LabelSelector{
								MatchLabels: map[string]string{
									"env": "cluster-1",
									labels.LabelSourceK8s + "." + k8sConst.PodNamespaceLabel: "production",
								},
							},
						},
						Ingress: []api.IngressRule{
							{
								FromEndpoints: []api.EndpointSelector{
									{
										LabelSelector: &metav1.LabelSelector{
											MatchLabels: map[string]string{
												"env": "cluster-1",
												labels.LabelSourceK8s + "." + k8sConst.PodNamespaceLabel: "production",
											},
										},
									},
								},
							},
						},
						Egress:      nil,
						Labels:      utils.GetPolicyLabels("production", "db", utils.ResourceTypeCiliumNetworkPolicy),
						Description: "",
					},
				})
				return args{
					ciliumV2Store: &cache.FakeCustomStore{},
					cnp: &v2.CiliumNetworkPolicy{
						ObjectMeta: metav1.ObjectMeta{
							Name:      "db",
							Namespace: "production",
						},
					},
					repo: r,
				}
			},
			setupWanted: func() wanted {
				r := policy.NewPolicyRepository()
				r.AddList(api.Rules{})
				return wanted{
					err:  nil,
					repo: r,
				}
			},
		},
	}
	for _, tt := range tests {
		args := tt.setupArgs()
		want := tt.setupWanted()
		ds.d.policy = args.repo
		err := ds.d.addCiliumNetworkPolicyV2(args.ciliumV2Store, args.cnp)
		c.Assert(err, DeepEquals, want.err, Commentf("Test name: %q", tt.name))
		c.Assert(ds.d.policy.GetRulesList().Policy, DeepEquals, want.repo.GetRulesList().Policy, Commentf("Test name: %q", tt.name))
	}
}
