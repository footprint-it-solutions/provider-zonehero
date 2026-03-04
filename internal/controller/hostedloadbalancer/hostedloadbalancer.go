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

package hostedloadbalancer

import (
	"context"
	"encoding/json"
	"fmt"
	"regexp"
	"strings"

	"github.com/crossplane/crossplane-runtime/pkg/feature"

	"github.com/pkg/errors"
	"k8s.io/apimachinery/pkg/types"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"

	"github.com/crossplane/crossplane-runtime/pkg/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	"github.com/crossplane/crossplane-runtime/pkg/connection"
	"github.com/crossplane/crossplane-runtime/pkg/controller"
	"github.com/crossplane/crossplane-runtime/pkg/event"
	"github.com/crossplane/crossplane-runtime/pkg/ratelimiter"
	"github.com/crossplane/crossplane-runtime/pkg/reconciler/managed"
	"github.com/crossplane/crossplane-runtime/pkg/resource"
	"github.com/crossplane/crossplane-runtime/pkg/statemetrics"

	xpv1 "github.com/crossplane/crossplane-runtime/apis/common/v1"
	"github.com/footprint-it-solutions/provider-zonehero/apis/hostedloadbalancer/v1alpha1"
	apisv1beta1 "github.com/footprint-it-solutions/provider-zonehero/apis/v1beta1"
	"github.com/footprint-it-solutions/provider-zonehero/internal/features"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/config"
	"gitlab.guerraz.net/HLB/hlb-terraform-provider/hlb"

	"github.com/google/go-cmp/cmp"

	"net"
)

const (
	errNotHostedLoadBalancer = "managed resource is not a HostedLoadBalancer custom resource"
	errTrackPCUsage          = "cannot track ProviderConfig usage"
	errGetPC                 = "cannot get ProviderConfig"
	errGetCreds              = "cannot get credentials"

	errNewClient = "cannot create HLB client"
	errCreateLB  = "cannot create load balancer"
	errUpdateLB  = "cannot update load balancer"
	errDeleteLB  = "cannot delete load balancer"

	LBStateActive          = "active"
	LBStateCreating        = "creating"
	LBStateDeleted         = "deleted"
	LBStateDeleting        = "deleting"
	LBStateFailed          = "failed"
	LBStatePendingCreation = "pending_creation"
	LBStatePendingDeletion = "pending_delete"
)

// Setup adds a controller that reconciles HostedLoadBalancer managed resources.
func Setup(mgr ctrl.Manager, o controller.Options) error {
	name := managed.ControllerName(v1alpha1.HostedLoadBalancerGroupKind)

	cps := []managed.ConnectionPublisher{managed.NewAPISecretPublisher(mgr.GetClient(), mgr.GetScheme())}
	if o.Features.Enabled(features.EnableAlphaExternalSecretStores) {
		cps = append(cps, connection.NewDetailsManager(mgr.GetClient(), apisv1beta1.StoreConfigGroupVersionKind))
	}

	opts := []managed.ReconcilerOption{
		managed.WithExternalConnecter(&connector{
			kube:        mgr.GetClient(),
			usage:       resource.NewProviderConfigUsageTracker(mgr.GetClient(), &apisv1beta1.ProviderConfigUsage{}),
			newClientFn: hlb.NewClient}),
		managed.WithLogger(o.Logger.WithValues("controller", name)),
		managed.WithPollInterval(o.PollInterval),
		managed.WithRecorder(event.NewAPIRecorder(mgr.GetEventRecorderFor(name))),
		managed.WithConnectionPublishers(cps...),
		managed.WithManagementPolicies(),
	}

	if o.Features.Enabled(feature.EnableAlphaChangeLogs) {
		opts = append(opts, managed.WithChangeLogger(o.ChangeLogOptions.ChangeLogger))
	}

	if o.MetricOptions != nil {
		opts = append(opts, managed.WithMetricRecorder(o.MetricOptions.MRMetrics))
	}

	if o.MetricOptions != nil && o.MetricOptions.MRStateMetrics != nil {
		stateMetricsRecorder := statemetrics.NewMRStateRecorder(
			mgr.GetClient(), o.Logger, o.MetricOptions.MRStateMetrics, &v1alpha1.HostedLoadBalancerList{}, o.MetricOptions.PollStateMetricInterval,
		)
		if err := mgr.Add(stateMetricsRecorder); err != nil {
			return errors.Wrap(err, "cannot register MR state metrics recorder for kind v1alpha1.HostedLoadBalancerList")
		}
	}

	r := managed.NewReconciler(mgr, resource.ManagedKind(v1alpha1.HostedLoadBalancerGroupVersionKind), opts...)

	return ctrl.NewControllerManagedBy(mgr).
		Named(name).
		WithOptions(o.ForControllerRuntime()).
		WithEventFilter(resource.DesiredStateChanged()).
		For(&v1alpha1.HostedLoadBalancer{}).
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
	cr, ok := mg.(*v1alpha1.HostedLoadBalancer)
	if !ok {
		return nil, errors.New(errNotHostedLoadBalancer)
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

	return &external{hlb: svc}, nil
}

// An ExternalClient observes, then either creates, updates, or deletes an
// external resource to ensure it reflects the managed resource's desired state.
type external struct {
	// A 'client' used to connect to the external resource API. In practice this
	// would be something like an AWS SDK client. In our case this is Client from the ZoneHero HLB library
	hlb *hlb.Client
}

// The process is a continuous loop that is triggered by any change to a HostedLoadBalancer resource or after a set poll interval (defaulting to 1 minute).
// Every single loop starts with a call to the Observe method. The result of Observe determines what happens next.
func (c *external) Observe(ctx context.Context, mg resource.Managed) (managed.ExternalObservation, error) {
	cr, ok := mg.(*v1alpha1.HostedLoadBalancer)
	if !ok {
		return managed.ExternalObservation{}, errors.New(errNotHostedLoadBalancer)
	}

	// Step 1: Check if the resource has been created yet.
	// If the external-name annotation is not set, it means Create has not been called.
	externalName := meta.GetExternalName(cr)

	// use the externalName in a call to the ZoneHero API, on first run this will give us 404 and we can trigger create method
	// otherwise, if we receive 200 from the ZoneHero API then the load balancer exists
	lb, err := c.hlb.GetLoadBalancer(ctx, externalName)
	if err != nil {
		fmt.Printf("Error getting load balancer: %+v\n", err)

		// Check for a specific 404 Not Found error. If we find it, we
		// know the resource needs to be created.
		if strings.Contains(err.Error(), "status code: 404") {
			return managed.ExternalObservation{ResourceExists: false}, nil
		}

		// Next, check if the error is a transient DNS error.
		var dnsErr *net.DNSError
		if errors.As(err, &dnsErr) {
			// This is a network issue. We create a more user-friendly error
			// message and return it to trigger a retry.
			customErr := fmt.Errorf("failed to connect due to DNS issue: The ZoneHero host is not available:  %s: %s", dnsErr.Name, dnsErr.Err)
			return managed.ExternalObservation{}, customErr
		}

		// For any other error (e.g., network issues, 500 errors, or an
		// error from a 'Failed' resource), we return the error. This
		// tells the controller to retry the operation after a backoff
		// period, which is the safe and correct behavior.
		return managed.ExternalObservation{}, errors.Wrap(err, "failed to get hosted load balancer")
	}

	// The resource exists, so we can now check its state.
	obs, done, err := c.determineObservationByState(cr, lb)
	if done {
		return obs, err
	}

	// Step 2: Update the status of your Kubernetes resource with what you observed.
	// This is crucial for users to see the state of the external resource.
	cr.Status.AtProvider = v1alpha1.HostedLoadBalancerObservation{
		ID:        lb.ID,
		DNSName:   lb.DNSName,
		State:     lb.State,
		AccountID: lb.AccountID,
		URI:       lb.URI,
	}
	if lb.CreatedAt.Unix() > 0 {
		cr.Status.AtProvider.CreatedAt = &metav1.Time{Time: lb.CreatedAt}
	}

	// Step 3: Call IsUpToDate to check for drift and return the final observation.
	return managed.ExternalObservation{
		// The resource definitely exists at this point.
		ResourceExists: true,

		// Call the helper function here. Its boolean result is assigned
		// directly to the ResourceUpToDate field.
		ResourceUpToDate: IsUpToDate(&cr.Spec.ForProvider, lb),

		// Return any details that may be required to connect to the external
		// resource. These will be stored as the connection secret.
		ConnectionDetails: managed.ConnectionDetails{},
	}, nil
}

func (c *external) determineObservationByState(cr *v1alpha1.HostedLoadBalancer, lb *hlb.LoadBalancer) (managed.ExternalObservation, bool, error) {
	switch lb.State {
	case LBStateFailed:
		extendedErrorMessage := "None"
		if lb.DeploymentStatus != nil && lb.DeploymentStatus.ErrorMessage != "" {
			extendedErrorMessage = lb.DeploymentStatus.ErrorMessage
		}
		message := fmt.Sprintf("load balancer (%s) entered failed state, with message '%s'", lb.ID, extendedErrorMessage)
		cr.SetConditions(xpv1.Unavailable().WithMessage(message))
		return managed.ExternalObservation{
			ResourceExists:   true,
			ResourceUpToDate: true, // No drift, the failure is external.
		}, true, nil
	case LBStatePendingDeletion, LBStateDeleting:
		cr.SetConditions(xpv1.Deleting())
		return managed.ExternalObservation{
			ResourceExists:   true,
			ResourceUpToDate: true,
		}, true, nil
	case LBStateDeleted:
		return managed.ExternalObservation{ResourceExists: false}, true, nil
	case LBStatePendingCreation, LBStateCreating:
		cr.SetConditions(xpv1.Creating())
		return managed.ExternalObservation{
			ResourceExists:   true,
			ResourceUpToDate: true, // Waiting for provider.
		}, true, nil
	case LBStateActive:
		cr.SetConditions(xpv1.Available())
		return managed.ExternalObservation{}, false, nil
	default:
		// If it's an unknown state, it's safest to consider it unavailable.
		cr.SetConditions(xpv1.Unavailable().WithMessage("The external resource is in an unknown state: " + lb.State))
		return managed.ExternalObservation{
			ResourceExists:   true,
			ResourceUpToDate: true, // Prevent updates.
		}, true, nil
	}
}

func (c *external) Create(ctx context.Context, mg resource.Managed) (managed.ExternalCreation, error) {
	cr, ok := mg.(*v1alpha1.HostedLoadBalancer)
	if !ok {
		return managed.ExternalCreation{}, errors.New(errNotHostedLoadBalancer)
	}

	c.hlb.SetDebug(true)

	// Build create request
	input := GenerateCreateInput(&cr.Spec.ForProvider)

	cr.SetConditions(xpv1.Creating())

	lb, err := c.hlb.CreateLoadBalancer(ctx, input)
	if err != nil {
		var dnsErr *net.DNSError
		if errors.As(err, &dnsErr) {
			customErr := fmt.Errorf("failed to connect due to DNS issue: The ZoneHero host is not available:  %s: %s", dnsErr.Name, dnsErr.Err)
			return managed.ExternalCreation{}, customErr
		}

		// Step 1: Check for the specific "entered failed state" error.
		if strings.Contains(err.Error(), "entered failed state") {
			// Step 2: Parse the load balancer ID from the error string.
			// We use a regular expression to find the ID within the parentheses.
			re := regexp.MustCompile(`\((\S+)\)`)
			matches := re.FindStringSubmatch(err.Error())

			// The first submatch (index 1) is our captured group (the ID).
			if len(matches) > 1 {
				lbID := matches[1]
				// Step 3: Set the external-name and update the resource status.
				// This is critical to break the creation loop.
				meta.SetExternalName(cr, lbID)
				cr.SetConditions(xpv1.Unavailable().WithMessage(err.Error()))
			}

			// Step 4: Return a nil error to stop the creation loop.
			// We are telling the controller that the creation "succeeded" in that
			// an external resource now exists, and the next Observe call will
			// handle its failed state.
			return managed.ExternalCreation{}, nil
		}

		return managed.ExternalCreation{}, errors.Wrap(err, errCreateLB)
	}

	// Set external name
	meta.SetExternalName(cr, lb.ID)

	// Update status
	cr.Status.AtProvider = v1alpha1.HostedLoadBalancerObservation{
		ID:        lb.ID,
		DNSName:   lb.DNSName,
		State:     lb.State,
		AccountID: lb.AccountID,
		URI:       lb.URI,
	}
	if lb.CreatedAt.Unix() > 0 {
		cr.Status.AtProvider.CreatedAt = &metav1.Time{Time: lb.CreatedAt}
	}

	return managed.ExternalCreation{
		// Optionally return any details that may be required to connect to the
		// external resource. These will be stored as the connection secret.
		ConnectionDetails: managed.ConnectionDetails{},
	}, nil
}

func (c *external) Update(ctx context.Context, mg resource.Managed) (managed.ExternalUpdate, error) {
	cr, ok := mg.(*v1alpha1.HostedLoadBalancer)
	if !ok {
		return managed.ExternalUpdate{}, errors.New(errNotHostedLoadBalancer)
	}

	input := &hlb.LoadBalancerUpdate{
		Name:                         &cr.Spec.ForProvider.Name,
		Ec2IamRole:                   &cr.Spec.ForProvider.Ec2IamRole,
		EnableDeletionProtection:     &cr.Spec.ForProvider.EnableDeletionProtection,
		EnableHttp2:                  &cr.Spec.ForProvider.EnableHttp2,
		IdleTimeout:                  &cr.Spec.ForProvider.IdleTimeout,
		PreserveHostHeader:           &cr.Spec.ForProvider.PreserveHostHeader,
		EnableCrossZoneLoadBalancing: &cr.Spec.ForProvider.EnableCrossZoneLoadBalancing,
		ClientKeepAlive:              &cr.Spec.ForProvider.ClientKeepAlive,
		XffHeaderProcessingMode:      &cr.Spec.ForProvider.XffHeaderProcessingMode,
		ConnectionDrainingTimeout:    &cr.Spec.ForProvider.ConnectionDrainingTimeout,
		PreferredMaintenanceWindow:   &cr.Spec.ForProvider.PreferredMaintenanceWindow,
		Tags:                         &cr.Spec.ForProvider.Tags,
	}

	if cr.Spec.ForProvider.LaunchConfig != nil {
		input.LaunchConfig = &hlb.LaunchConfig{
			InstanceType:     cr.Spec.ForProvider.LaunchConfig.InstanceType,
			MinInstanceCount: cr.Spec.ForProvider.LaunchConfig.MinInstanceCount,
			MaxInstanceCount: cr.Spec.ForProvider.LaunchConfig.MaxInstanceCount,
			TargetCPUUsage:   cr.Spec.ForProvider.LaunchConfig.TargetCPUUsage,
		}
	}

	if cr.Spec.ForProvider.AccessLogs != nil {
		input.AccessLogs = &hlb.AccessLogs{
			Bucket:  cr.Spec.ForProvider.AccessLogs.Bucket,
			Enabled: cr.Spec.ForProvider.AccessLogs.Enabled,
			Prefix:  cr.Spec.ForProvider.AccessLogs.Prefix,
		}
	}

	id := meta.GetExternalName(cr)
	_, err := c.hlb.UpdateLoadBalancer(ctx, id, input)
	if err != nil {
		var dnsErr *net.DNSError
		if errors.As(err, &dnsErr) {
			customErr := fmt.Errorf("failed to connect due to DNS issue: The ZoneHero host is not available:  %s: %s", dnsErr.Name, dnsErr.Err)
			return managed.ExternalUpdate{}, customErr
		}

		return managed.ExternalUpdate{}, errors.Wrap(err, errUpdateLB)
	}

	return managed.ExternalUpdate{
		// Optionally return any details that may be required to connect to the
		// external resource. These will be stored as the connection secret.
		ConnectionDetails: managed.ConnectionDetails{},
	}, nil
}

func (c *external) Delete(ctx context.Context, mg resource.Managed) (managed.ExternalDelete, error) {
	cr, ok := mg.(*v1alpha1.HostedLoadBalancer)
	if !ok {
		return managed.ExternalDelete{}, errors.New(errNotHostedLoadBalancer)
	}

	c.hlb.SetDebug(true)

	// // Set the "Deleting" condition.
	// // This sets the Ready condition to False with a reason of "Deleting".
	// cr.SetConditions(xpv1.Deleting())

	id := meta.GetExternalName(cr)
	err := c.hlb.DeleteLoadBalancer(ctx, id)
	if err != nil {
		var dnsErr *net.DNSError
		if errors.As(err, &dnsErr) {
			customErr := fmt.Errorf("failed to connect due to DNS issue: The ZoneHero host is not available:  %s: %s", dnsErr.Name, dnsErr.Err)
			return managed.ExternalDelete{}, customErr
		}

		// The ZoneHero API will not allow to delete a load balancer that has listeners attached to it
		// The API will return 409
		// At this stage we need to handle the error and abort the deletion process
		if strings.Contains(err.Error(), "status code: 409") {
			// we do NOT update the status condition.
			// We simply return the error to trigger a retry. The resource
			// will remain in its last known state (e.g., Available)
			// until the conflict is resolved.
			return managed.ExternalDelete{}, errors.Wrap(err, "cannot delete a load balancer that has listeners attached")
		}

		// For all other types of errors, we set the condition to Deleting
		// before returning the error to signal that there is a problem with deletion.
		cr.SetConditions(xpv1.Deleting())
		return managed.ExternalDelete{}, errors.Wrap(err, errDeleteLB)
	}

	// If err is nil, the deletion was successful.
	// We set the Deleting condition here as a final status update
	// before the resource is removed from the API server.
	cr.SetConditions(xpv1.Deleting())
	return managed.ExternalDelete{}, nil
}

func (c *external) Disconnect(ctx context.Context) error {
	return nil
}

// this helper function translates hlb.LoadBalancerCreate type to a type compatible with v1alpha1
// By using a translation function (GenerateCreateInput), you gain complete control over how the
// hlb.LoadBalancerCreate struct is populated. This is the perfect place to implement your defaulting logic.
// this helper gunction is required because of Golang strict type-checking
func GenerateCreateInput(p *v1alpha1.HostedLoadBalancerParameters) *hlb.LoadBalancerCreate {
	create := &hlb.LoadBalancerCreate{
		Name:                         p.Name,
		Internal:                     p.Internal,
		Subnets:                      p.Subnets,
		SecurityGroups:               p.SecurityGroups,
		Ec2IamRole:                   p.Ec2IamRole,
		EnableDeletionProtection:     p.EnableDeletionProtection,
		EnableHttp2:                  p.EnableHttp2,
		IdleTimeout:                  p.IdleTimeout,
		IPAddressType:                p.IPAddressType,
		PreserveHostHeader:           p.PreserveHostHeader,
		EnableCrossZoneLoadBalancing: p.EnableCrossZoneLoadBalancing,
		ClientKeepAlive:              p.ClientKeepAlive,
		XffHeaderProcessingMode:      p.XffHeaderProcessingMode,
		ConnectionDrainingTimeout:    p.ConnectionDrainingTimeout,
		PreferredMaintenanceWindow:   p.PreferredMaintenanceWindow,
		Tags:                         p.Tags,
		ZoneID:                       p.ZoneID,
		ZoneName:                     p.ZoneName,
	}

	if p.AccessLogs != nil {
		create.AccessLogs = &hlb.AccessLogs{
			Bucket:  p.AccessLogs.Bucket,
			Enabled: p.AccessLogs.Enabled,
			Prefix:  p.AccessLogs.Prefix,
		}
	}

	if p.LaunchConfig != nil {
		create.LaunchConfig = &hlb.LaunchConfig{
			InstanceType:     p.LaunchConfig.InstanceType,
			MinInstanceCount: p.LaunchConfig.MinInstanceCount,
			MaxInstanceCount: p.LaunchConfig.MaxInstanceCount,
			TargetCPUUsage:   p.LaunchConfig.TargetCPUUsage,
		}
	}

	return create
}

// IsUpToDate checks ONLY the configurable fields.
func IsUpToDate(p *v1alpha1.HostedLoadBalancerParameters, lb *hlb.LoadBalancer) bool {
	if !isBaseUpToDate(p, lb) {
		return false
	}
	if !isAdvancedUpToDate(p, lb) {
		return false
	}
	return isLaunchConfigUpToDate(p.LaunchConfig, lb.LaunchConfig)
}

func isBaseUpToDate(p *v1alpha1.HostedLoadBalancerParameters, lb *hlb.LoadBalancer) bool {
	if p.ClientKeepAlive != 0 && p.ClientKeepAlive != lb.ClientKeepAlive {
		return false
	}
	if p.ConnectionDrainingTimeout != 0 && p.ConnectionDrainingTimeout != lb.ConnectionDrainingTimeout {
		return false
	}
	if p.Ec2IamRole != "" && p.Ec2IamRole != lb.Ec2IamRole {
		return false
	}
	if p.EnableCrossZoneLoadBalancing != "" && p.EnableCrossZoneLoadBalancing != lb.EnableCrossZoneLoadBalancing {
		return false
	}
	if p.EnableDeletionProtection && p.EnableDeletionProtection != lb.EnableDeletionProtection {
		return false
	}
	if p.EnableHttp2 && p.EnableHttp2 != lb.EnableHttp2 {
		return false
	}
	if p.IdleTimeout != 0 && p.IdleTimeout != lb.IdleTimeout {
		return false
	}
	return true
}

func isAdvancedUpToDate(p *v1alpha1.HostedLoadBalancerParameters, lb *hlb.LoadBalancer) bool {
	if p.Name != "" && p.Name != lb.Name {
		return false
	}
	if p.PreferredMaintenanceWindow != "" && p.PreferredMaintenanceWindow != lb.PreferredMaintenanceWindow {
		return false
	}
	if p.PreserveHostHeader && p.PreserveHostHeader != lb.PreserveHostHeader {
		return false
	}
	if p.XffHeaderProcessingMode != "" && p.XffHeaderProcessingMode != lb.XffHeaderProcessingMode {
		return false
	}
	if len(p.SecurityGroups) > 0 && !cmp.Equal(p.SecurityGroups, lb.SecurityGroups) {
		return false
	}
	if len(p.Tags) > 0 && !cmp.Equal(p.Tags, lb.Tags) {
		return false
	}
	if p.AccessLogs != nil && !cmp.Equal(p.AccessLogs, lb.AccessLogs) {
		return false
	}
	return true
}

func isLaunchConfigUpToDate(p *v1alpha1.LaunchConfig, lb *hlb.LaunchConfig) bool {
	if p == nil {
		return true
	}
	if lb == nil {
		return false
	}
	if p.InstanceType != "" && p.InstanceType != lb.InstanceType {
		return false
	}
	if p.MinInstanceCount != 0 && p.MinInstanceCount != lb.MinInstanceCount {
		return false
	}
	if p.MaxInstanceCount != 0 && p.MaxInstanceCount != lb.MaxInstanceCount {
		return false
	}
	if p.TargetCPUUsage != 0 && p.TargetCPUUsage != lb.TargetCPUUsage {
		return false
	}
	return true
}
