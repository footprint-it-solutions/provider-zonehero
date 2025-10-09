/*
Copyright 2025 The Crossplane Authors.

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

package v1alpha1

import (
	"reflect"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime/schema"

	xpv1 "github.com/crossplane/crossplane-runtime/apis/common/v1"
)


type DeploymentStatus struct {
	ErrorMessage string `json:"errorMessage,omitempty"`
	Metadata     []byte `json:"metadata,omitempty"`
	Version      string `json:"version,omitempty"`
}

// HostedLoadBalancerParameters are the configurable fields of a HostedLoadBalancer.
type HostedLoadBalancerParameters struct {
	// AccessLogs                   *AccessLogs       `json:"accessLogs,omitempty"`
	AccountID                    string            `json:"accountId"`
	ClientKeepAlive              int               `json:"clientKeepAlive"`
	ConnectionDrainingTimeout    int               `json:"connectionDrainingTimeout"`
	CreatedAt                    metav1.Time       `json:"createdAt"`
	DeploymentStatus             *DeploymentStatus `json:"deploymentStatus,omitempty"`
	DNSName                      string            `json:"dnsName"`
	Ec2IamRole                   string            `json:"ec2IamRole"`
	EnableCrossZoneLoadBalancing string            `json:"enableCrossZoneLoadBalancing"`
	EnableDeletionProtection     bool              `json:"enableDeletionProtection"`
	EnableHttp2                  bool              `json:"enableHttp2"`
	ExpiresAt                    int               `json:"expiresAt"`
	ID                           string            `json:"id"`
	IdleTimeout                  int               `json:"idleTimeout"`
	Internal                     bool              `json:"internal"`
	IPAddressType                string            `json:"ipAddressType"`
	// LaunchConfig                 *LaunchConfig     `json:"launchConfig,omitempty"`
	Name                         string            `json:"name"`
	NamePrefix                   string            `json:"namePrefix,omitempty"`
	PreferredMaintenanceWindow   string            `json:"preferredMaintenanceWindow"`
	PreserveHostHeader           bool              `json:"preserveHostHeader"`
	SecurityGroups               []string          `json:"securityGroups"`
	State                        string            `json:"state"`
	Subnets                      []string          `json:"subnets"`
	Tags                         map[string]string `json:"tags"`
	UpdatedAt                    metav1.Time       `json:"updatedAt"`
	URI                          string            `json:"uri"`
	XffHeaderProcessingMode      string            `json:"xffHeaderProcessingMode"`
	ZoneID                       string            `json:"zoneId"`
	ZoneName                     string            `json:"zoneName"`
}

// HostedLoadBalancerObservation are the observable fields of a HostedLoadBalancer.
type HostedLoadBalancerObservation struct {
	ConfigurableField string `json:"configurableField"`
	ObservableField   string `json:"observableField,omitempty"`
}

// A HostedLoadBalancerSpec defines the desired state of a HostedLoadBalancer.
type HostedLoadBalancerSpec struct {
	xpv1.ResourceSpec `json:",inline"`
	ForProvider       HostedLoadBalancerParameters `json:"forProvider"`
}

// A HostedLoadBalancerStatus represents the observed state of a HostedLoadBalancer.
type HostedLoadBalancerStatus struct {
	xpv1.ResourceStatus `json:",inline"`
	AtProvider          HostedLoadBalancerObservation `json:"atProvider,omitempty"`
}

// A HostedLoadBalancer is an example API type.
// +kubebuilder:object:root=true
// +kubebuilder:printcolumn:name="READY",type="string",JSONPath=".status.conditions[?(@.type=='Ready')].status"
// +kubebuilder:printcolumn:name="SYNCED",type="string",JSONPath=".status.conditions[?(@.type=='Synced')].status"
// +kubebuilder:printcolumn:name="EXTERNAL-NAME",type="string",JSONPath=".metadata.annotations.crossplane\\.io/external-name"
// +kubebuilder:printcolumn:name="AGE",type="date",JSONPath=".metadata.creationTimestamp"
// +kubebuilder:subresource:status
// +kubebuilder:resource:scope=Cluster,categories={crossplane,managed,zonehero}
type HostedLoadBalancer struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   HostedLoadBalancerSpec   `json:"spec"`
	Status HostedLoadBalancerStatus `json:"status,omitempty"`
}

// HostedLoadBalancerList contains a list of HostedLoadBalancer
// +kubebuilder:object:root=true
type HostedLoadBalancerList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []HostedLoadBalancer `json:"items"`
}

// HostedLoadBalancer type metadata.
var (
	HostedLoadBalancerKind             = reflect.TypeOf(HostedLoadBalancer{}).Name()
	HostedLoadBalancerGroupKind        = schema.GroupKind{Group: Group, Kind: HostedLoadBalancerKind}.String()
	HostedLoadBalancerKindAPIVersion   = HostedLoadBalancerKind + "." + SchemeGroupVersion.String()
	HostedLoadBalancerGroupVersionKind = SchemeGroupVersion.WithKind(HostedLoadBalancerKind)
)

func init() {
	SchemeBuilder.Register(&HostedLoadBalancer{}, &HostedLoadBalancerList{})
}
