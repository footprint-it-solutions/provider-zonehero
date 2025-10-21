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

package listener

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/config"

	xpv1 "github.com/crossplane/crossplane-runtime/apis/common/v1"
	"github.com/crossplane/crossplane-runtime/pkg/controller"
	"github.com/crossplane/crossplane-runtime/pkg/event"
	"github.com/crossplane/crossplane-runtime/pkg/feature"
	"github.com/crossplane/crossplane-runtime/pkg/meta"
	"github.com/crossplane/crossplane-runtime/pkg/ratelimiter"
	"github.com/crossplane/crossplane-runtime/pkg/reconciler/managed"
	"github.com/crossplane/crossplane-runtime/pkg/resource"
	"github.com/crossplane/crossplane-runtime/pkg/statemetrics"
	"github.com/pkg/errors"
	kresource "k8s.io/apimachinery/pkg/api/resource"
	"k8s.io/apimachinery/pkg/types"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"

	"github.com/footprint-it-solutions/provider-zonehero/apis/listener/v1alpha1"
	apisv1beta1 "github.com/footprint-it-solutions/provider-zonehero/apis/v1beta1"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	"gitlab.guerraz.net/HLB/hlb-terraform-provider/hlb"
)

const (
	errNotListener           = "managed resource is not a Listener custom resource"
	errNotListenerController = "cannot setup Listener controller"
	errTrackPCUsage          = "cannot track ProviderConfig usage"
	errGetPC                 = "cannot get ProviderConfig"
	errGetCPC                = "cannot get ClusterProviderConfig"
	errGetCreds              = "cannot get credentials"

	errNewClient      = "cannot create ZoneHero client"
	errCreateListener = "cannot create listener"
	errUpdateListener = "cannot update listener"
	errDeleteListener = "cannot delete listener"
)

// Setup adds a controller that reconciles Listener managed resources.
func Setup(mgr ctrl.Manager, o controller.Options) error {
	name := managed.ControllerName(v1alpha1.ListenerGroupVersionKind.String())

	opts := []managed.ReconcilerOption{
		managed.WithExternalConnecter(&connector{
			kube:        mgr.GetClient(),
			usage:       resource.NewProviderConfigUsageTracker(mgr.GetClient(), &apisv1beta1.ProviderConfigUsage{}),
			newClientFn: hlb.NewClient}),
		managed.WithLogger(o.Logger.WithValues("controller", name)),
		managed.WithPollInterval(o.PollInterval),
		managed.WithRecorder(event.NewAPIRecorder(mgr.GetEventRecorderFor(name))),
	}

	if o.Features.Enabled(feature.EnableBetaManagementPolicies) {
		opts = append(opts, managed.WithManagementPolicies())
	}

	if o.Features.Enabled(feature.EnableAlphaChangeLogs) {
		opts = append(opts, managed.WithChangeLogger(o.ChangeLogOptions.ChangeLogger))
	}

	if o.MetricOptions != nil {
		opts = append(opts, managed.WithMetricRecorder(o.MetricOptions.MRMetrics))
	}

	if o.MetricOptions != nil && o.MetricOptions.MRStateMetrics != nil {
		stateMetricsRecorder := statemetrics.NewMRStateRecorder(
			mgr.GetClient(), o.Logger, o.MetricOptions.MRStateMetrics, &v1alpha1.ListenerList{}, o.MetricOptions.PollStateMetricInterval,
		)
		if err := mgr.Add(stateMetricsRecorder); err != nil {
			return errors.Wrap(err, "cannot register MR state metrics recorder for kind v1alpha1.Listener")
		}
	}

	r := managed.NewReconciler(mgr, resource.ManagedKind(v1alpha1.ListenerGroupVersionKind), opts...)

	return ctrl.NewControllerManagedBy(mgr).
		Named(name).
		WithOptions(o.ForControllerRuntime()).
		WithEventFilter(resource.DesiredStateChanged()).
		For(&v1alpha1.Listener{}).
		Complete(ratelimiter.NewReconciler(name, r, o.GlobalRateLimiter))
}

// A connector is expected to produce an ExternalClient when its Connect method
// is called.
type connector struct {
	kube        client.Client
	usage       resource.Tracker
	newClientFn func(ctx context.Context, apiKey string, awsConfig aws.Config, partition string) (*hlb.Client, error)
}

// Connect typically produces an ExternalClient by:
// 1. Tracking that the managed resource is using a ProviderConfig.
// 2. Getting the managed resource's ProviderConfig.
// 3. Getting the credentials specified by the ProviderConfig.
// 4. Using the credentials to form a client.
func (c *connector) Connect(ctx context.Context, mg resource.Managed) (managed.ExternalClient, error) {
	cr, ok := mg.(*v1alpha1.Listener)
	if !ok {
		return nil, errors.New(errNotListener)
	}

	if err := c.usage.Track(ctx, mg); err != nil {
		return nil, errors.Wrap(err, errTrackPCUsage)
	}

	pc := &apisv1beta1.ProviderConfig{}
	if err := c.kube.Get(ctx, types.NamespacedName{Name: cr.GetProviderConfigReference().Name}, pc); err != nil {
		return nil, errors.Wrap(err, errGetPC)
	}

	cd := pc.Spec.Credentials
	data, err := resource.CommonCredentialExtractor(ctx, cd.Source, c.kube, cd.CommonCredentialSelectors)
	if err != nil {
		return nil, errors.Wrap(err, errGetCreds)
	}

	type Credentials struct {
		APIKey       string `json:"api_key"`
		AWSRegion    string `json:"aws_region"`
		AWSProfile   string `json:"aws_profile"`
		AWSPartition string `json:"partition"`
	}

	var creds Credentials
	if err := json.Unmarshal(data, &creds); err != nil {
		return nil, errors.Wrap(err, "cannot unmarshal credentials from secret")
	}

	opts := []func(*config.LoadOptions) error{}

	apiKey := creds.APIKey

	if creds.AWSRegion != "" {
		opts = append(opts, config.WithRegion(creds.AWSRegion))
	}

	if creds.AWSProfile != "" {
		opts = append(opts, config.WithSharedConfigProfile(creds.AWSProfile))
	}

	awsCfg, err := config.LoadDefaultConfig(ctx, opts...)
	if err != nil {
		return nil, fmt.Errorf("error loading AWS config: %w", err)
	}

	svc, err := c.newClientFn(ctx, apiKey, awsCfg, creds.AWSPartition)
	if err != nil {
		return nil, errors.Wrap(err, errNewClient)
	}

	return &external{zonehero_api_client: svc}, nil
}

// An ExternalClient observes, then either creates, updates, or deletes an
// external resource to ensure it reflects the managed resource's desired state.
type external struct {
	// A 'client' used to connect to the external resource API. In practice this
	// would be something like an AWS SDK client.
	zonehero_api_client *hlb.Client
}

func (c *external) Observe(ctx context.Context, mg resource.Managed) (managed.ExternalObservation, error) {
	// If the managed resource is marked for deletion then deleted it.
	// Because there is no external resource to observe, we return false for
	// ResourceExists.
	cr, ok := mg.(*v1alpha1.Listener)
	if !ok {
		return managed.ExternalObservation{}, errors.New(errNotListener)
	}
	if meta.WasDeleted(mg) {
		return managed.ExternalObservation{
			ResourceExists: false,
		}, nil
	}

	// These fmt statements should be removed in the real implementation.
	fmt.Printf("Observing: %+v", cr)

	// Step 1: Check if the resource has been created yet.
	// If the external-name annotation is not set, it means Create has not been called.
	externalName := meta.GetExternalName(cr)

	// use the externalName in a call to the ZoneHero API, on first run this will give us 404 and we can trigger create method
	// otherwise, if we receive 200 from the ZoneHero API then the listener exists
	loadBalancerID := cr.Spec.ForProvider.LoadBalancerID
	listener, err := c.zonehero_api_client.GetListener(ctx, loadBalancerID, externalName)
	if err != nil {
		// Create new listener
		return managed.ExternalObservation{
			ResourceExists: false,
		}, nil
	}

	// Now the resource is in sync and ready to use, so mark it as available.
	cr.Status.SetConditions(xpv1.Available())
	// Step 4: Call IsUpToDate to check for drift and return the final observation.
	return managed.ExternalObservation{
		// The resource definitely exists at this point.
		ResourceExists: true,

		// Call the helper function here. Its boolean result is assigned
		// directly to the ResourceUpToDate field.
		ResourceUpToDate: IsUpToDate(&cr.Spec.ForProvider, listener),

		// Return any details that may be required to connect to the external
		// resource. These will be stored as the connection secret.
		ConnectionDetails: managed.ConnectionDetails{},
	}, nil
}

func (c *external) Create(ctx context.Context, mg resource.Managed) (managed.ExternalCreation, error) {
	cr, ok := mg.(*v1alpha1.Listener)
	if !ok {
		return managed.ExternalCreation{}, errors.New(errNotListener)
	}
	cr.Status.SetConditions(xpv1.Creating())

	fmt.Printf("Creating: %+v", cr)

	// Build create request
	input := GenerateCreateInput(&cr.Spec.ForProvider)
	loadBalancerID := cr.Spec.ForProvider.LoadBalancerID

	listener, err := c.zonehero_api_client.CreateListener(ctx, loadBalancerID, input)
	if err != nil {
		return managed.ExternalCreation{}, errors.Wrap(err, errCreateListener)
	}

	// Set external name
	meta.SetExternalName(cr, listener.ID)

	// Update status
	cr.Status.AtProvider = GenerateListenerObservation(listener)
	if !listener.CreatedAt.IsZero() {
		cr.Status.AtProvider.CreatedAt = &metav1.Time{Time: listener.CreatedAt}
	}
	// if listener.CreatedAt.Unix() > 0 {
	// 	cr.Status.AtProvider.CreatedAt = &metav1.Time{Time: listener.CreatedAt}
	// }
	return managed.ExternalCreation{}, nil
}

func (c *external) Update(ctx context.Context, mg resource.Managed) (managed.ExternalUpdate, error) {
	cr, ok := mg.(*v1alpha1.Listener)
	if !ok {
		return managed.ExternalUpdate{}, errors.New(errNotListener)
	}

	fmt.Printf("Updating: %+v", cr)

	input := GenerateUpdateInput(&cr.Spec.ForProvider)

	listenerID := meta.GetExternalName(cr)
	loadBalancerID := cr.Spec.ForProvider.LoadBalancerID
	_, err := c.zonehero_api_client.UpdateListener(ctx, loadBalancerID, listenerID, input)
	if err != nil {
		return managed.ExternalUpdate{}, errors.Wrap(err, errUpdateListener)
	}

	return managed.ExternalUpdate{
		// Optionally return any details that may be required to connect to the
		// external resource. These will be stored as the connection secret.
		ConnectionDetails: managed.ConnectionDetails{},
	}, nil
}

func (c *external) Delete(ctx context.Context, mg resource.Managed) (managed.ExternalDelete, error) {
	cr, ok := mg.(*v1alpha1.Listener)
	if !ok {
		return managed.ExternalDelete{}, errors.New(errNotListener)
	}

	c.zonehero_api_client.SetDebug(true)

	// Set the "Deleting" condition.
	// This sets the Ready condition to False with a reason of "Deleting".
	cr.SetConditions(xpv1.Deleting())

	listenerID := meta.GetExternalName(cr)
	loadBalancerID := cr.Spec.ForProvider.LoadBalancerID
	err := c.zonehero_api_client.DeleteListener(ctx, loadBalancerID, listenerID)
	if err != nil {
		return managed.ExternalDelete{}, errors.Wrap(err, errDeleteListener)
	}

	return managed.ExternalDelete{}, nil
}

func (c *external) Disconnect(ctx context.Context) error {
	return nil
}

// -#---- HELPER FUNCTIONS ---- #-
func GenerateListenerObservation(listener *hlb.Listener) v1alpha1.ListenerObservation {
	overprovisioningFactor := kresource.NewQuantity(int64(listener.OverprovisioningFactor*100), kresource.DecimalSI)
	obs := v1alpha1.ListenerObservation{
		ID:                     listener.ID,
		OverprovisioningFactor: overprovisioningFactor,
	}
	if !listener.CreatedAt.IsZero() {
		obs.CreatedAt = &metav1.Time{Time: listener.CreatedAt}
	}
	if !listener.UpdatedAt.IsZero() {
		obs.UpdatedAt = &metav1.Time{Time: listener.UpdatedAt}
	}
	return obs
}

func GenerateCreateInput(p *v1alpha1.ListenerParameters) *hlb.ListenerCreate {
	return &hlb.ListenerCreate{
		ALPNPolicy:               p.ALPNPolicy,
		CertificateSecretsName:   p.CertificateSecretsName,
		EnableDeletionProtection: p.EnableDeletionProtection,
		OverprovisioningFactor:   float64(p.OverprovisioningFactor.Value()) / 100.0,
		Port:                     p.Port,
		Protocol:                 p.Protocol,
		TargetGroupARN:           p.TargetGroupARN,
	}
}

// this helper function translates hlb.ListenerUpdate type to a type compatible with v1alpha1
// By using a translation function (GenerateCreateInput), you gain complete control over how the
// hlb.LoadBalancerCreate struct is populated. This is the perfect place to implement your defaulting logic.
// this helper gunction is required because of Golang strict type-checking
func GenerateUpdateInput(p *v1alpha1.ListenerParameters) *hlb.ListenerUpdate {
	overprovisioningFactor := float64(p.OverprovisioningFactor.Value()) / 100.0
	return &hlb.ListenerUpdate{
		ALPNPolicy:               &p.ALPNPolicy,
		CertificateSecretsName:   &p.CertificateSecretsName,
		EnableDeletionProtection: &p.EnableDeletionProtection,
		OverprovisioningFactor:   &overprovisioningFactor,
		Port:                     &p.Port,
		Protocol:                 &p.Protocol,
		TargetGroupARN:           &p.TargetGroupARN,
	}
}

// IsUpToDate checks ONLY the configurable fields.
func IsUpToDate(p *v1alpha1.ListenerParameters, listener *hlb.Listener) bool {
	// Compare a configurable field from the spec...
	if p.ALPNPolicy != listener.ALPNPolicy {
		return false
	}
	// ...with the corresponding field from the observed resource.

	if p.CertificateSecretsName != listener.CertificateSecretsName {
		return false
	}

	if p.EnableDeletionProtection != listener.EnableDeletionProtection {
		return false
	}

	if (float64(p.OverprovisioningFactor.Value()) / 100.0) != listener.OverprovisioningFactor {
		return false
	}

	if p.Port != listener.Port {
		return false
	}

	if p.Protocol != listener.Protocol {
		return false
	}

	if p.TargetGroupARN != listener.TargetGroupARN {
		return false
	}

	return true
}
