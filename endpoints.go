package kubeclient

import (
	"encoding/json"
	"fmt"

	"golang.org/x/build/kubernetes/api"
	"golang.org/x/net/context"
)

const (
	endpointsPath = apiPrefix + "/namespaces/%s/endpoints"
)

func (c *Client) EndpointsList(ctx context.Context, namespace, label string) ([]api.Endpoints, error) {
	var endpoints []api.Endpoints

	apiResult, err := ListKubeResources(ctx, &EndpointResource{c.Host, namespace, ""}, c.Client)
	if err != nil {
		return endpoints, fmt.Errorf("Resource List failed: %v", err)
	}

	var endpointsList api.EndpointsList
	if err := json.Unmarshal(apiResult, &endpointsList); err != nil {
		return endpoints, fmt.Errorf("failed to decode endpoint resources: %v", err)
	}

	return endpointsList.Items, nil
}

type EndpointResource struct {
	Host      string
	Namespace string
	Label     string
}

func (rc *EndpointResource) KubeResourcesURL() string {
	return rc.Host + fmt.Sprintf(endpointsPath, rc.Namespace)
}

func (rc *EndpointResource) KubeResourceNamespace() string {
	return rc.Namespace
}

func (rc *EndpointResource) KubeResourceLabel() string {
	return rc.Label
}
