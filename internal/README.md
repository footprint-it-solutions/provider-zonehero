# internal

This folder contains client (e.g. AWS SDK client to talk to the AWS API) and controller for a Managed Resource (e.g. HostedLoadBalancer). The controller reconciles the state of a managed resource.

The controller implements CRUD API methods - (CR)eate, (U)pdate and (D)elete. There is also the Observe method. 

Crossplane always starts with Observe method which returns ExternalObservation' object. It contains values for ResourceExists and ResourceUpToDate which defines the methods to be called. <br>
If managed resource exists and ResourceExists is marked as false, then Create function is called. <br>
If ResourceExists is true and ResourceUpToDate is marked as false, then Update function is called. <br>
If managed resource is deleted and ResourceExists is true, then Delete function is called. <br>
