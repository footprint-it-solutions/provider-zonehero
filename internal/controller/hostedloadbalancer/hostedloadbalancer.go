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
	"time"

	"github.com/crossplane/crossplane-runtime/pkg/feature"

	"github.com/pkg/errors"
	"k8s.io/apimachinery/pkg/types"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"github.com/crossplane/crossplane-runtime/pkg/meta"

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

	"gitlab.guerraz.net/HLB/hlb-terraform-provider/hlb"
	//"gitlab.guerraz.net/HLB/hlb-terraform-provider/apis/v1alpha1"
	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/config"

	 "github.com/google/go-cmp/cmp"
)

const (
	errNotHostedLoadBalancer    = "managed resource is not a HostedLoadBalancer custom resource"
	errTrackPCUsage = "cannot track ProviderConfig usage"
	errGetPC        = "cannot get ProviderConfig"
	errGetCreds     = "cannot get credentials"

	errGetLoadBalancer = "cannot get LoadBalancer"
	errGetSecret       = "cannot get credentials Secret"
	errNewClient       = "cannot create HLB client"
	errCreateLB        = "cannot create load balancer"
	errDescribeLB      = "cannot describe load balancer"
	errUpdateLB        = "cannot update load balancer"
	errDeleteLB        = "cannot delete load balancer"
	errUpdateStatus    = "cannot update LoadBalancer status"

	LBCrossAZPolicyAvoid   = "avoid"
	LBCrossAZPolicyFull    = "full"
	LBCrossAZPolicyOff     = "off"
	LBEc2IamRoleDebug      = "lb-ssm"
	LBEc2IamRoleStandard   = "lb-standard"
	LBIpAddressDualStack   = "dualstack"
	LBIpAddressTypeV4Only  = "ipv4"
	LBIpAddressTypeV6Only  = "dualstack-without-public-ipv4"
	LBStateActive          = "active"
	LBStateCreating        = "creating"
	LBStateDeleted         = "deleted"
	LBStateDeleting        = "deleting"
	LBStateFailed          = "failed"
	LBStatePendingCreation = "pending_creation"
	LBStatePendingDeletion = "pending_delete"
	LBStatePendingUpdate   = "pending_update"
	LBStateUpdating        = "updating"

	// Default timeouts
	DefaultCreateTimeout = 30 * time.Minute
	DefaultUpdateTimeout = 30 * time.Minute
	DefaultDeleteTimeout = 30 * time.Minute
)


// A NoOpService does nothing.
type NoOpService struct{}

var (
	newNoOpService = func(_ []byte) (interface{}, error) { return &NoOpService{}, nil }
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
			kube:         mgr.GetClient(),
			usage:        resource.NewProviderConfigUsageTracker(mgr.GetClient(), &apisv1beta1.ProviderConfigUsage{}),
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
	kube         client.Client
	usage        resource.Tracker
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
		APIKey string `json:"api_key"`
		AWSRegion string `json:"aws_region"`
		AWSProfile string `json:"aws_profile"`
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
		return nil, fmt.Errorf("error loading AWS config: %v", err)
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
	// would be something like an AWS SDK client.
	hlb *hlb.Client
}

// The process is a continuous loop that is triggered by any change to a HostedLoadBalancer resource or after a set poll interval (defaulting to 1 minute).
// Every single loop starts with a call to the Observe method. The result of Observe determines what happens next.
func (c *external) Observe(ctx context.Context, mg resource.Managed) (managed.ExternalObservation, error) {
	cr, ok := mg.(*v1alpha1.HostedLoadBalancer)
	if !ok {
		return managed.ExternalObservation{}, errors.New(errNotHostedLoadBalancer)
	}

	// // These fmt statements should be removed in the real implementation.
	// fmt.Printf("Observing: %+v \n", cr)

	// Step 1: Check if the resource has been created yet.
	// If the external-name annotation is not set, it means Create has not been called.
	externalName := meta.GetExternalName(cr)
	fmt.Printf("GetExternalName: %+v \n", externalName)

	// use the externalName in a call to the ZoneHero API, on first run this will give us 404 and we can trigger create method
	// otherwise, if we receive 200 from the ZoneHero API then the load balancer exists
	lb, err := c.hlb.GetLoadBalancer(ctx, externalName)
	if err != nil {
		// Create new load balancer
		return managed.ExternalObservation{
			ResourceExists: false,
		}, nil
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


	// Step 3: Set the Ready condition based on the observed state.
	// if err == nil then it means that we can read lb.State

	// the first state that a LB enters is LBStatePendingCreation , then LBStateCreating
	// depending on the outcome of the provisioning step, the LB will transition to either LBStateActive  or LBStateFailed , both of which are final states (they will not change unless another operation is started from the API).
	// when you modify a LB, similarly the state transitions to LBStatePendingUpdate , then LBStateUpdating , and again the LB will transition to LBStateActive  (as far as I remember, it cannot enter LBStateFailed , because the properties that can lead to that state are read only and require a redeployment, (like changing subnets))


	// | State            | Ready Condition (status, reason) | Synced Condition (status) | Helper Function to Use |
	// | ---------------- | ---------------------------------| -------------------------| ---------------------- |
	// | Pending Creation | False, Creating                  | False                    | xpv1.Creating()      |
	// | Available        | True, Available                  | True                     | xpv1.Available()     |
	// | Pending Update   | True, Available                  | False                    | (Return ResourceUpToDate: false from Observe) |
	// | Pending Deletion | False, Deleting                  | True                     | xpv1.Deleting()      |
	// | Failed/Error     | False, Unavailable               | False                    | xpv1.Unavailable()   |
	switch lb.State {
	case LBStatePendingCreation:
		cr.SetConditions(xpv1.Creating())
	case LBStateCreating:
		cr.SetConditions(xpv1.Creating())
	case LBStateActive:
		cr.SetConditions(xpv1.Available())
	case LBStateFailed:
		extendedErrorMessage := "None"
		if lb.DeploymentStatus != nil && lb.DeploymentStatus.ErrorMessage != "" {
			extendedErrorMessage = lb.DeploymentStatus.ErrorMessage
		}
		message := fmt.Sprintf("load balancer (%s) entered failed state, with message '%s'", lb.ID, extendedErrorMessage)
		cr.SetConditions(xpv1.Unavailable().WithMessage(message))
	case LBStatePendingDeletion:
		cr.SetConditions(xpv1.Deleting())
	case LBStateDeleting:
		cr.SetConditions(xpv1.Deleting())
	case LBStateDeleted:
		cr.SetConditions(xpv1.Unavailable().WithMessage("The external resource is not available as it was deleted."))
	default:
		// If it's an unknown state, it's safest to consider it unavailable.
		cr.SetConditions(xpv1.Unavailable().WithMessage("The external resource is in an unknown state: " + lb.State))
	}


	// Step 4: Call IsUpToDate to check for drift and return the final observation.
	return managed.ExternalObservation{
		// The resource definitely exists at this point.
		ResourceExists:   true,

		// Call the helper function here. Its boolean result is assigned
		// directly to the ResourceUpToDate field.
		ResourceUpToDate: IsUpToDate(&cr.Spec.ForProvider, lb),

		// Return any details that may be required to connect to the external
		// resource. These will be stored as the connection secret.
		ConnectionDetails: managed.ConnectionDetails{},
	}, nil
}

func (c *external) Create(ctx context.Context, mg resource.Managed) (managed.ExternalCreation, error) {
	cr, ok := mg.(*v1alpha1.HostedLoadBalancer)
	if !ok {
		return managed.ExternalCreation{}, errors.New(errNotHostedLoadBalancer)
	}

	c.hlb.SetDebug(true)

	fmt.Printf("Creating: %+v \n", cr)
	// Build create request
	input := GenerateCreateInput(&cr.Spec.ForProvider)

	cr.SetConditions(xpv1.Creating())

	lb, err := c.hlb.CreateLoadBalancer(ctx, input)
	if err != nil {
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


	fmt.Printf("Updating: %+v", cr)
	id := meta.GetExternalName(cr)
	_, err := c.hlb.UpdateLoadBalancer(ctx, id, input)
	if err != nil {
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

	// Set the "Deleting" condition.
    // This sets the Ready condition to False with a reason of "Deleting".
    cr.SetConditions(xpv1.Deleting())

	fmt.Printf("Deleting: %+v", cr)
	id := meta.GetExternalName(cr)
	err := c.hlb.DeleteLoadBalancer(ctx, id)
	if err != nil {
		return managed.ExternalDelete{}, errors.Wrap(err, errDeleteLB)
	}

	return managed.ExternalDelete{}, nil
}

func (c *external) Disconnect(ctx context.Context) error {
	return nil
}


// Helper functions
func stringValue(s *string) string {
	if s == nil {
		return ""
	}
	return *s
}

func boolValue(b *bool) bool {
	if b == nil {
		return false
	}
	return *b
}

func intValue(i *int) int {
	if i == nil {
		return 0
	}
	return *i
}

func ptrInt(i int) *int {
	return &i
}


// this helper function translates hlb.LoadBalancerCreate type to a type compatible with v1alpha1
// By using a translation function (GenerateCreateInput), you gain complete control over how the 
// hlb.LoadBalancerCreate struct is populated. This is the perfect place to implement your defaulting logic.
// this helper gunction is required because of Golang strict type-checking
func GenerateCreateInput(p *v1alpha1.HostedLoadBalancerParameters) *hlb.LoadBalancerCreate {
    return &hlb.LoadBalancerCreate{
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
}


// IsUpToDate checks ONLY the configurable fields.
func IsUpToDate(p *v1alpha1.HostedLoadBalancerParameters, lb *hlb.LoadBalancer) bool {
    // Compare a configurable field from the spec...
    if p.ClientKeepAlive != lb.ClientKeepAlive {
        return false
    }
    // ...with the corresponding field from the observed resource.

    if p.ConnectionDrainingTimeout != lb.ConnectionDrainingTimeout {
        return false
    }

	if p.Ec2IamRole != lb.Ec2IamRole {
        return false
    }
	
	if p.EnableCrossZoneLoadBalancing != lb.EnableCrossZoneLoadBalancing {
        return false
    }

	if p.EnableDeletionProtection != lb.EnableDeletionProtection {
        return false
    }

	if p.EnableHttp2 != lb.EnableHttp2 {
        return false
    }

	if p.IdleTimeout != lb.IdleTimeout {
        return false
    }

	if p.Name != lb.Name {
        return false
    }

	if p.PreferredMaintenanceWindow != lb.PreferredMaintenanceWindow {
        return false
    }

	if p.PreserveHostHeader != lb.PreserveHostHeader {
        return false
    }

	if p.XffHeaderProcessingMode != lb.XffHeaderProcessingMode {
        return false
    }

    // Use cmp.Equal for slices and maps
	if !cmp.Equal(p.SecurityGroups, lb.SecurityGroups) {
        return false
    }
	if !cmp.Equal(p.Tags, lb.Tags) {
        return false
    }


	return true
}
