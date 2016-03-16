package kubeclient

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io/ioutil"
	"net/http"
	"net/url"

	"golang.org/x/build/kubernetes/api"
	"golang.org/x/net/context"
	"golang.org/x/net/context/ctxhttp"
)

const (
	replicationControllersPath = apiPrefix + "/namespaces/%s/replicationcontrollers"
	replicationControllerPath  = apiPrefix + "/namespaces/%s/replicationcontrollers/%s"
)

func (c *Client) CreateReplicationController(ctx context.Context,
	rc *api.ReplicationController) (*api.ReplicationController, error) {

	var rcJSON bytes.Buffer
	if err := json.NewEncoder(&rcJSON).Encode(rc); err != nil {
		return nil, fmt.Errorf("failed to encode rc in json: %v", err)
	}

	apiResult, err := CreateKubeResource(ctx, &ReplicationControllerResource{c.Host, rc.Namespace, ""}, rcJSON, c.Client)
	if err != nil {
		return nil, fmt.Errorf("Create failed: %v", err)
	}

	var rcResult api.ReplicationController
	if err := json.Unmarshal(apiResult, &rcResult); err != nil {
		return nil, fmt.Errorf("failed to decode rc resources: %v", err)
	}

	return &rcResult, nil
}

func (c *Client) DeleteReplicationController(ctx context.Context, namespace, replicationControllerName string) error {
	url := c.replicationControllerURL(namespace, replicationControllerName)
	return DeleteKubeResource(ctx, url, c.Client)
}

func (c *Client) UpdateReplicationControllerImage(ctx context.Context, namespace, name, image, version string) error {
	replicationControllerURL, err := url.Parse(c.replicationControllerURL(namespace, name))
	if err != nil {
		return err
	}
	values := url.Values{}
	replicationControllerURL.RawQuery = values.Encode()
	url := replicationControllerURL.String()
	// TODO: Add support for container name lookup
	body := []byte(fmt.Sprintf(`[{"op": "replace", "path": "/spec/template/spec/containers/0/image", "value":"%s:%s"}]`, image, version))

	req, err := http.NewRequest("PATCH", url, bytes.NewBuffer(body))
	if err != nil {
		return fmt.Errorf("failed to create request: PATCH %q : %v", url, err)
	}
	req.Header.Set("Content-Type", "application/json-patch+json")
	res, err := ctxhttp.Do(ctx, c.Client, req)
	if err != nil {
		return fmt.Errorf("failed to make request: PATCH %q: %v", url, err)
	}
	body, err = ioutil.ReadAll(res.Body)
	res.Body.Close()
	if err != nil {
		return fmt.Errorf("failed to read response body: PATCH %q: %v", url, err)
	}
	if res.StatusCode != http.StatusOK {
		return fmt.Errorf("http error %d PATCH %q: %q: %v", res.StatusCode, url, string(body), err)
	}
	var replicationControllerList api.ReplicationControllerList
	if err := json.Unmarshal(body, &replicationControllerList); err != nil {
		return fmt.Errorf("failed to decode rc resources: %v", err)
	}

	return nil
}

func (c *Client) ReplicationControllerList(ctx context.Context, namespace, label string) ([]api.ReplicationController, error) {
	var replicationControllers []api.ReplicationController

	apiResult, err := ListKubeResources(ctx, &ReplicationControllerResource{c.Host, namespace, label}, c.Client)
	if err != nil {
		return replicationControllers, fmt.Errorf("Resource List failed: %v", err)
	}

	var replicationControllerList api.ReplicationControllerList
	if err := json.Unmarshal(apiResult, &replicationControllerList); err != nil {
		return replicationControllers, fmt.Errorf("failed to decode rc resources: %v", err)
	}

	return replicationControllerList.Items, nil
}

func (c *Client) replicationControllerURL(namespace, name string) string {
	return c.Host + fmt.Sprintf(replicationControllerPath, namespace, name)
}

type ReplicationControllerResource struct {
	Host      string
	Namespace string
	Label     string
}

func (rc *ReplicationControllerResource) KubeResourcesURL() string {
	return rc.Host + fmt.Sprintf(replicationControllersPath, rc.Namespace)
}

func (rc *ReplicationControllerResource) KubeResourceNamespace() string {
	return rc.Namespace
}

func (rc *ReplicationControllerResource) KubeResourceLabel() string {
	return rc.Label
}
