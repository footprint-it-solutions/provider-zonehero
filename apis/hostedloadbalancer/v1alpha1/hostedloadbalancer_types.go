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
	// +optional
	AccountID                    string            `json:"accountId,omitempty"`
	// +optional
	ClientKeepAlive              int               `json:"clientKeepAlive,omitempty"`
	// +optional
	ConnectionDrainingTimeout    int               `json:"connectionDrainingTimeout,omitempty"`
	// +optional
	CreatedAt                    metav1.Time       `json:"createdAt,omitempty"`
	// +optional
	DeploymentStatus             *DeploymentStatus `json:"deploymentStatus,omitempty"`
	// +optional
	DNSName                      string            `json:"dnsName,omitempty"`
	// +optional
	Ec2IamRole                   string            `json:"ec2IamRole,omitempty"`
	// +optional
	EnableCrossZoneLoadBalancing string            `json:"enableCrossZoneLoadBalancing,omitempty"`
	// +optional
	EnableDeletionProtection     bool              `json:"enableDeletionProtection,omitempty"`
	// +optional
	EnableHttp2                  bool              `json:"enableHttp2,omitempty"`
	// +optional
	ExpiresAt                    int               `json:"expiresAt,omitempty"`
	// +optional
	ID                           string            `json:"id,omitempty"`
	// +optional
	IdleTimeout                  int               `json:"idleTimeout,omitempty"`
	// +optional
	Internal                     bool              `json:"internal,omitempty"`
	// +optional
	IPAddressType                string            `json:"ipAddressType,omitempty"`
	// +optional
	Name                         string            `json:"name,omitempty"`
	// +optional
	NamePrefix                   string            `json:"namePrefix,omitempty"`
	// +optional
	PreferredMaintenanceWindow   string            `json:"preferredMaintenanceWindow,omitempty"`
	// +optional
	PreserveHostHeader           bool              `json:"preserveHostHeader,omitempty"`
	// +optional
	SecurityGroups               []string          `json:"securityGroups,omitempty"`
	// +optional
	State                        string            `json:"state,omitempty"`
	// +optional
	Subnets                      []string          `json:"subnets,omitempty"`
	// +optional
	Tags                         map[string]string `json:"tags,omitempty"`
	// +optional
	UpdatedAt                    metav1.Time       `json:"updatedAt,omitempty"`
	// +optional
	URI                          string            `json:"uri,omitempty"`
	// +optional
	XffHeaderProcessingMode      string            `json:"xffHeaderProcessingMode,omitempty"`
	// +optional
	ZoneID                       string            `json:"zoneId,omitempty"`
	// +optional
	ZoneName                     string            `json:"zoneName,omitempty"`
}

// // HostedLoadBalancerObservation are the observable fields of a HostedLoadBalancer.
// type HostedLoadBalancerObservation struct {
// 	ConfigurableField string `json:"configurableField"`
// 	ObservableField   string `json:"observableField,omitempty"`
// }


// LoadBalancerObservation are the observable fields of a LoadBalancer.
type LoadBalancerObservation struct {
	// ID is the unique identifier of the load balancer.
	ID string `json:"id,omitempty"`

	// DNSName is the DNS name of the load balancer.
	DNSName string `json:"dnsName,omitempty"`

	// State is the current state of the load balancer.
	State string `json:"state,omitempty"`

	// AccountID is the AWS account ID.
	AccountID string `json:"accountId,omitempty"`

	// CreatedAt is when the load balancer was created.
	CreatedAt *metav1.Time `json:"createdAt,omitempty"`

	// UpdatedAt is when the load balancer was last updated.
	UpdatedAt *metav1.Time `json:"updatedAt,omitempty"`

	// URI is the API URI for the load balancer.
	URI string `json:"uri,omitempty"`
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
