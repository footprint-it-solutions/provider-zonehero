/*
Copyright 2026 The Footprint IT Solutions Authors.
*/

package v1alpha1

import (
	"reflect"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime/schema"

	xpv1 "github.com/crossplane/crossplane-runtime/apis/common/v1"
)

// HeroNodeGroupParameters defines the configuration for the HLB Node Group.
type HeroNodeGroupParameters struct {
	// CustomAMI is the ID of the custom Ubuntu AMI to use.
	// +kubebuilder:validation:Required
	CustomAMI string `json:"customAMI"`

	// ClusterName is the name of the EKS cluster for discovery tags.
	// +kubebuilder:validation:Required
	ClusterName string `json:"clusterName"`

	// InstanceType is the EC2 instance type to use (e.g., t3.large).
	// +kubebuilder:default="t3.large"
	InstanceType string `json:"instanceType,omitempty"`

	// APIKeySecretRef references the secret containing the API Key for the HLB application.
	// +optional
	APIKeySecretRef *xpv1.SecretKeySelector `json:"apiKeySecretRef,omitempty"`
}

// HeroNodeGroupObservation defines the observed state.
type HeroNodeGroupObservation struct {
	// NodePoolName is the name of the created Karpenter NodePool.
	NodePoolName string `json:"nodePoolName,omitempty"`
}

// HeroNodeGroupSpec defines the desired state of a HeroNodeGroup.
type HeroNodeGroupSpec struct {
	xpv1.ResourceSpec `json:",inline"`
	ForProvider       HeroNodeGroupParameters `json:"forProvider"`
}

// HeroNodeGroupStatus represents the observed state.
type HeroNodeGroupStatus struct {
	xpv1.ResourceStatus `json:",inline"`
	AtProvider          HeroNodeGroupObservation `json:"atProvider,omitempty"`
}

// +kubebuilder:object:root=true
// +kubebuilder:subresource:status
// +kubebuilder:resource:scope=Cluster,categories={crossplane,managed,zonehero}

// HeroNodeGroup is a managed resource for Hero Load Balancer Node Pools.
type HeroNodeGroup struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   HeroNodeGroupSpec   `json:"spec"`
	Status HeroNodeGroupStatus `json:"status,omitempty"`
}

// GetCondition of this HeroNodeGroup.
func (mg *HeroNodeGroup) GetCondition(ct xpv1.ConditionType) xpv1.Condition {
	return mg.Status.GetCondition(ct)
}

// SetConditions of this HeroNodeGroup.
func (mg *HeroNodeGroup) SetConditions(c ...xpv1.Condition) {
	mg.Status.SetConditions(c...)
}

// GetWriteConnectionSecretToReference of this HeroNodeGroup.
func (mg *HeroNodeGroup) GetWriteConnectionSecretToReference() *xpv1.SecretReference {
	return mg.Spec.WriteConnectionSecretToReference
}

// SetWriteConnectionSecretToReference of this HeroNodeGroup.
func (mg *HeroNodeGroup) SetWriteConnectionSecretToReference(r *xpv1.SecretReference) {
	mg.Spec.WriteConnectionSecretToReference = r
}

// GetPublishConnectionDetailsTo of this HeroNodeGroup.
func (mg *HeroNodeGroup) GetPublishConnectionDetailsTo() *xpv1.PublishConnectionDetailsTo {
	return mg.Spec.PublishConnectionDetailsTo
}

// SetPublishConnectionDetailsTo of this HeroNodeGroup.
func (mg *HeroNodeGroup) SetPublishConnectionDetailsTo(r *xpv1.PublishConnectionDetailsTo) {
	mg.Spec.PublishConnectionDetailsTo = r
}

// GetDeletionPolicy of this HeroNodeGroup.
func (mg *HeroNodeGroup) GetDeletionPolicy() xpv1.DeletionPolicy {
	return mg.Spec.DeletionPolicy
}

// SetDeletionPolicy of this HeroNodeGroup.
func (mg *HeroNodeGroup) SetDeletionPolicy(p xpv1.DeletionPolicy) {
	mg.Spec.DeletionPolicy = p
}

// GetManagementPolicies of this HeroNodeGroup.
func (mg *HeroNodeGroup) GetManagementPolicies() xpv1.ManagementPolicies {
	return mg.Spec.ManagementPolicies
}

// SetManagementPolicies of this HeroNodeGroup.
func (mg *HeroNodeGroup) SetManagementPolicies(p xpv1.ManagementPolicies) {
	mg.Spec.ManagementPolicies = p
}

// GetProviderConfigReference of this HeroNodeGroup.
func (mg *HeroNodeGroup) GetProviderConfigReference() *xpv1.Reference {
	return mg.Spec.ProviderConfigReference
}

// SetProviderConfigReference of this HeroNodeGroup.
func (mg *HeroNodeGroup) SetProviderConfigReference(r *xpv1.Reference) {
	mg.Spec.ProviderConfigReference = r
}

// +kubebuilder:object:root=true

// HeroNodeGroupList contains a list of HeroNodeGroup.
type HeroNodeGroupList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []HeroNodeGroup `json:"items"`
}

var (
	HeroNodeGroupKind             = reflect.TypeOf(HeroNodeGroup{}).Name()
	HeroNodeGroupGroupKind        = schema.GroupKind{Group: Group, Kind: HeroNodeGroupKind}.String()
	HeroNodeGroupKindAPIVersion   = HeroNodeGroupKind + "." + SchemeGroupVersion.String()
	HeroNodeGroupGroupVersionKind = SchemeGroupVersion.WithKind(HeroNodeGroupKind)
)

func init() {
	SchemeBuilder.Register(&HeroNodeGroup{}, &HeroNodeGroupList{})
}
