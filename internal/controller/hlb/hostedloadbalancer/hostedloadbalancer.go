package hostedloadbalancer

import (
	"context"
	"fmt"
	"github.com/aws/aws-sdk-go-v2/aws"
	awsconfig "github.com/aws/aws-sdk-go-v2/config"
	// xpv1 "github.com/crossplane/crossplane-runtime/apis/common/v1"
	// "github.com/crossplane/crossplane-runtime/pkg/meta"
	// "github.com/google/go-cmp/cmp"
	// snsv1alpha1 "provider-aws-controlapi/apis/sns/v1alpha1"
	awsclient "github.com/footprint-it-solutions/provider-zonehero/internal/clients"
	// "provider-aws-controlapi/internal/clients/sns"
	// "strings"
	// "time"

	v1alpha1 "github.com/footprint-it-solutions/provider-zonehero/apis/hlb/v1alpha1"
	apisv1alpha1 "github.com/crossplane/provider-template/apis/v1alpha1"

	"github.com/hashicorp/terraform-plugin-framework/datasource"
	"github.com/hashicorp/terraform-plugin-framework/provider"
	"github.com/hashicorp/terraform-plugin-framework/provider/schema"
	"github.com/hashicorp/terraform-plugin-framework/providerserver"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/types"

	// "github.com/pkg/errors"
	// "k8s.io/client-go/util/workqueue"
	// ctrl "sigs.k8s.io/controller-runtime"
	// "sigs.k8s.io/controller-runtime/pkg/client"
	// "sigs.k8s.io/controller-runtime/pkg/controller"

	// "github.com/crossplane/crossplane-runtime/pkg/event"
	// "github.com/crossplane/crossplane-runtime/pkg/logging"
	// "github.com/crossplane/crossplane-runtime/pkg/ratelimiter"
	"github.com/crossplane/crossplane-runtime/pkg/reconciler/managed"
	"github.com/crossplane/crossplane-runtime/pkg/resource"
)


type LoadBalancerCreate struct {
	ClientKeepAlive              int               `json:"clientKeepAlive"`
	ConnectionDrainingTimeout    int               `json:"connectionDrainingTimeout"`
	Ec2IamRole                   string            `json:"ec2IamRole"`
	EnableCrossZoneLoadBalancing string            `json:"enableCrossZoneLoadBalancing"`
	EnableDeletionProtection     bool              `json:"enableDeletionProtection"`
	EnableHttp2                  bool              `json:"enableHttp2"`
	IdleTimeout                  int               `json:"idleTimeout"`
	Internal                     bool              `json:"internal"`
	IPAddressType                string            `json:"ipAddressType"`
	Name                         string            `json:"name"`
	NamePrefix                   string            `json:"namePrefix"`
	PreferredMaintenanceWindow   string            `json:"preferredMaintenanceWindow"`
	PreserveHostHeader           bool              `json:"preserveHostHeader"`
	SecurityGroups               []string          `json:"securityGroups"`
	Subnets                      []string          `json:"subnets"`
	Tags                         map[string]string `json:"tags"`
	XffHeaderProcessingMode      string            `json:"xffHeaderProcessingMode"`
	ZoneID                       string            `json:"zoneId"`
	ZoneName                     string            `json:"zoneName"`
}


type LoadBalancer struct {
	AccountID                    string            `json:"accountId"`
	ClientKeepAlive              int               `json:"clientKeepAlive"`
	ConnectionDrainingTimeout    int               `json:"connectionDrainingTimeout"`
	CreatedAt                    time.Time         `json:"createdAt"`
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
	Name                         string            `json:"name"`
	PreferredMaintenanceWindow   string            `json:"preferredMaintenanceWindow"`
	PreserveHostHeader           bool              `json:"preserveHostHeader"`
	SecurityGroups               []string          `json:"securityGroups"`
	State                        string            `json:"state"`
	Subnets                      []string          `json:"subnets"`
	Tags                         map[string]string `json:"tags"`
	UpdatedAt                    time.Time         `json:"updatedAt"`
	URI                          string            `json:"uri"`
	XffHeaderProcessingMode      string            `json:"xffHeaderProcessingMode"`
	ZoneID                       string            `json:"zoneId"`
	ZoneName                     string            `json:"zoneName"`
}


type HLBProviderModel struct {
	APIKey     string
	AWSRegion  string
	AWSProfile string
	Partition  string
}


/*

Sequence of various lifecycle methods

Crossplane always start with Observe method which returns ExternalObservation' object. It contains values for ResourceExists and ResourceUpToDate which defines the methods to be called
If managed resource exists and ResourceExists is marked as false, then Create function is called
If ResourceExists is true and ResourceUpToDate is marked as false, then Update function is called
If managed resource is deleted and ResourceExists is true, then Delete function is called

*/

const (
	// runtime errors
	errNotMyType    = "managed resource is not a HostedLoadBalancer custom resource"
	errGetPC        = "cannot get ProviderConfig"
	errGetCreds     = "cannot get credentials"
	errNewClient 	= "cannot create new AWS Client"
)


// // A HostedLoadBalancer does nothing.
// type HostedLoadBalancer struct{}

// var (
// 	newHostedLoadBalancer = func(creds []byte) (interface{}, error) { return &HostedLoadBalancer{}, nil }
// )


// A connector is expected to produce an ExternalClient when its Connect method
// is called.
type connector struct {
	kube        client.Client
	newClientFn awsclient.NewClient(ctx context.Context, apiKey string, awsConfig aws.Config, partition string) AWSClient // takes creds as input and returns interface of type AWSClient
}

// Connect typically produces an ExternalClient by:
// 1. Tracking that the managed resource is using a ProviderConfig.
// 2. Getting the managed resource's ProviderConfig.
// 3. Getting the credentials specified by the ProviderConfig.
// 4. Using the credentials to form a client.

// In the connect method we need to init a client for an external service, in our case it is AWS 
// for the method we need to extract some creds, in xplane this is done by reading a ProviderConfig
// which will reference a k8s secret which contains API keys, etc
// this method will extract data block from the k8s secret and pass to the newClientFn
// A call to the Connect method > fetch creds > instaniate a new AWS SDK Client > return the AWS SDK client
func (c *connector) Connect(ctx context.Context, mg resource.Managed) (managed.ExternalClient, error) {
	cr, ok := mg.(*v1alpha1.HostedLoadBalancer)
	if !ok {
		return nil, errors.New(errNotMyType)
	}

	// pc := &apisv1alpha1.ProviderConfig{}
	// if err := c.kube.Get(ctx, types.NamespacedName{Name: cr.GetProviderConfigReference().Name}, pc); err != nil {
	// 	return nil, errors.Wrap(err, errGetPC)
	// }

	// cd := pc.Spec.Credentials
	// data, err := resource.CommonCredentialExtractor(ctx, cd.Source, c.kube, cd.CommonCredentialSelectors)
	// if err != nil {
	// 	return nil, errors.Wrap(err, errGetCreds)
	// }


	// Configure AWS SDK
	var awsOpts []func(*awsconfig.LoadOptions) error
	
	awsOpts = append(awsOpts, awsconfig.WithSharedConfigProfile("b05315eb-37a5-4d25-a203-425fdf32df59"))
	awsOpts = append(awsOpts, awsconfig.WithRegion("eu-west-1")

	awsCfg, err := awsconfig.LoadDefaultConfig(ctx, awsOpts...)
	if err != nil {
		return nil, errors.Wrap(err, errNewClient)
	}

	// Create HLB client
	// svc, err := c.newServiceFn(data)
	aws_client, err := c.newClientFn(ctx, "b05315eb-37a5-4d25-a203-425fdf32df59", awsCfg, "")
	if err != nil {
		return nil, errors.Wrap(err, errNewClient)
	}
	return &external_client{aws_sdk: aws_client, c.kube}, nil
}


// An ExternalClient observes, then either creates, updates, or deletes an
// external resource (in this case HostedLoadBalancer) to ensure it reflects the managed resource's desired state.
type external_client struct {
	// A 'client' used to connect to the external resource API. In practice this
	// would be something like an AWS SDK client.
	aws_sdk	AWSClient
	kube   	client.Client
}


func (c *external_client) Observe(ctx context.Context, mg resource.Managed) (managed.ExternalObservation, error) {
	resp, err := c.aws_sdk.sendRequest(ctx, "GET", fmt.Sprintf("/aws_account/%s/load-balancers/%s", c.accountID, loadBalancerID), nil)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	var lb LoadBalancer
	if err := json.NewDecoder(resp.Body).Decode(&lb); err != nil {
		return nil, fmt.Errorf("failed to decode response: %w", err)
	}

	return &lb, nil
}


func (c *external_client) Create(ctx context.Context, mg resource.Managed) (managed.ExternalObservation, error) {
	resp, err := c.aws_sdk.sendRequest(ctx, "POST", fmt.Sprintf("/aws_account/%s/load-balancers", c.aws_sdk.accountID), *LoadBalancerCreate)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	var lb LoadBalancer
	if err := json.NewDecoder(resp.Body).Decode(&lb); err != nil {
		return nil, fmt.Errorf("failed to decode response: %w", err)
	}

	// Wait for the load balancer to be active
	return waitForLoadBalancerState(ctx, lb.ID, []string{LBStateActive}, DefaultCreateTimeout)
}


func isLoadBalancerInPendingState(state string) bool {
	pendingStates := map[string]bool{
		LBStatePendingCreation: true,
		LBStateCreating:        true,
		LBStatePendingUpdate:   true,
		LBStateUpdating:        true,
		LBStatePendingDeletion: true,
		LBStateDeleting:        true,
	}
	return pendingStates[state]
}


// Wait for at most timeout for load balancer identified with id to enter one of the states in target[]
func (c *external_client) waitForLoadBalancerState(ctx context.Context, id string, target []string, timeout time.Duration) (*LoadBalancer, error) {
	targetStates := make(map[string]bool, len(target))
	for _, s := range target {
		targetStates[s] = true
	}

	var lb *LoadBalancer
	err := retry.RetryContext(ctx, timeout, func() *retry.RetryError {
		var err error
		lb, err = GetLoadBalancer(ctx, id)
		if err != nil {
			return retry.NonRetryableError(fmt.Errorf("error getting load balancer (%s): %w", id, err))
		}

		if lb.State == LBStateFailed {
			extendedErrorMessage := "None"
			if lb.DeploymentStatus != nil && lb.DeploymentStatus.ErrorMessage != "" {
				extendedErrorMessage = lb.DeploymentStatus.ErrorMessage
			}
			return retry.NonRetryableError(fmt.Errorf("load balancer (%s) entered failed state, with message '%s'", id, extendedErrorMessage))
		}

		if targetStates[lb.State] {
			return nil
		}

		if isLoadBalancerInPendingState(lb.State) {
			return retry.RetryableError(fmt.Errorf("expected load balancer (%s) to be in state %v but was in state %s", id, target, lb.State))
		}

		return retry.NonRetryableError(fmt.Errorf("load balancer (%s) entered unexpected state %s", id, lb.State))
	})

	if err != nil {
		return nil, err
	}

	return lb, nil
}


func (c *external_client) GetLoadBalancer(ctx context.Context, loadBalancerID string) (*LoadBalancer, error) {
	resp, err := c.aws_sdk.sendRequest(ctx, "GET", fmt.Sprintf("/aws_account/%s/load-balancers/%s", c.accountID, loadBalancerID), nil)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	var lb LoadBalancer
	if err := json.NewDecoder(resp.Body).Decode(&lb); err != nil {
		return nil, fmt.Errorf("failed to decode response: %w", err)
	}

	return &lb, nil
}



func (c *external_client) Update(ctx context.Context, mg resource.Managed) (managed.ExternalObservation, error) {
}


func (c *external_client) Delete(ctx context.Context, mg resource.Managed) (managed.ExternalObservation, error) {
}
