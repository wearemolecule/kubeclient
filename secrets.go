package kubeclient

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io/ioutil"
	"net/http"

	"golang.org/x/build/kubernetes/api"
	"golang.org/x/net/context"
	"golang.org/x/net/context/ctxhttp"
)

const (
	secretPath = apiPrefix + "/namespaces/%s/secrets"
)

func (c *Client) CreateSecret(ctx context.Context, secret *api.Secret) (*api.Secret, error) {
	var secretJSON bytes.Buffer
	if err := json.NewEncoder(&secretJSON).Encode(secret); err != nil {
		return nil, fmt.Errorf("failed to encode secret in json: %v", err)
	}
	secretURL := c.secretURL(secret.Namespace)
	req, err := http.NewRequest("POST", secretURL, &secretJSON)
	if err != nil {
		return nil, fmt.Errorf("failed to create request: POST %q : %v", secretURL, err)
	}
	res, err := ctxhttp.Do(ctx, c.Client, req)
	if err != nil {
		return nil, fmt.Errorf("failed to make request: POST %q: %v", secretURL, err)
	}
	body, err := ioutil.ReadAll(res.Body)
	res.Body.Close()
	if err != nil {
		return nil, fmt.Errorf("failed to read request body for POST %q: %v", secretURL, err)
	}
	if res.StatusCode != http.StatusCreated {
		return nil, fmt.Errorf("http error: %d POST %q: %q: %v", res.StatusCode, secretURL, string(body), err)
	}
	var secretResult api.Secret
	if err := json.Unmarshal(body, &secretResult); err != nil {
		return nil, fmt.Errorf("failed to decode secret resources: %v", err)
	}
	return &secretResult, nil
}

// DeleteSecret deletes the specified Kubernetes pod.
func (c *Client) DeleteSecret(ctx context.Context, namespace, secretName string) error {
	url := c.secretURL(namespace) + "/" + secretName
	return DeleteKubeResource(ctx, url, c.Client)
}

// GetSecret gets the specified Kubernetes pod.
func (c *Client) GetSecret(ctx context.Context, namespace, secretName string) (*api.Secret, error) {
	var secret api.Secret
	url := c.secretURL(namespace) + "/" + secretName
	req, err := http.NewRequest("GET", url, nil)
	if err != nil {
		return &secret, fmt.Errorf("failed to create request: GET %q : %v", url, err)
	}
	res, err := ctxhttp.Do(ctx, c.Client, req)
	if err != nil {
		return &secret, fmt.Errorf("failed to make request: GET %q: %v", url, err)
	}
	body, err := ioutil.ReadAll(res.Body)
	res.Body.Close()
	if err != nil {
		return &secret, fmt.Errorf("failed to read response body: GET %q: %v", url, err)
	}
	if res.StatusCode != http.StatusOK {
		return &secret, fmt.Errorf("http error: %d GET %q: %q: %v", res.StatusCode, url, string(body), err)
	}
	if err := json.Unmarshal(body, &secret); err != nil {
		return &secret, fmt.Errorf("failed to decode secret json: %v", err)
	}

	return &secret, nil
}

func (c *Client) secretURL(namespace string) string {
	return c.Host + fmt.Sprintf(secretPath, namespace)
}
