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

func (c *Client) ReplicationControllerList(ctx context.Context, namespace, label string) ([]api.ReplicationController, error) {
	var replicationControllers []api.ReplicationController
	replicationControllerURL, err := url.Parse(c.replicationControllersURL(namespace))
	if err != nil {
		return replicationControllers, err
	}

	values := url.Values{}
	values.Set("labelSelector", label)
	replicationControllerURL.RawQuery = values.Encode()

	url := replicationControllerURL.String()
	req, err := http.NewRequest("GET", url, nil)
	if err != nil {
		return replicationControllers, fmt.Errorf("failed to create request: GET %q : %v", url, err)
	}
	res, err := ctxhttp.Do(ctx, c.Client, req)
	if err != nil {
		return replicationControllers, fmt.Errorf("failed to make request: GET %q: %v", url, err)
	}
	body, err := ioutil.ReadAll(res.Body)
	res.Body.Close()
	if err != nil {
		return replicationControllers, fmt.Errorf("failed to read response body: GET %q: %v", url, err)
	}
	if res.StatusCode != http.StatusOK {
		return replicationControllers, fmt.Errorf("http error %d GET %q: %q: %v", res.StatusCode, url, string(body), err)
	}
	var replicationControllerList api.ReplicationControllerList
	if err := json.Unmarshal(body, &replicationControllerList); err != nil {
		return replicationControllers, fmt.Errorf("failed to decode rc resources: %v", err)
	}

	return replicationControllerList.Items, nil
}

func (c *Client) UpdateReplicationController(ctx context.Context, namespace, name, image, version string) error {
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

func (c *Client) replicationControllerURL(namespace, name string) string {
	return c.Host + fmt.Sprintf(replicationControllerPath, namespace, name)
}

func (c *Client) replicationControllersURL(namespace string) string {
	return c.Host + fmt.Sprintf(replicationControllersPath, namespace)
}
