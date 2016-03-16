package kubeclient

import (
	"bytes"
	"fmt"
	"io/ioutil"
	"net/http"
	"net/url"

	"golang.org/x/net/context"
	"golang.org/x/net/context/ctxhttp"
)

type KubeResource interface {
	KubeResourcesURL() string
	KubeResourceNamespace() string
	KubeResourceLabel() string
}

func CreateKubeResource(ctx context.Context,
	kubeResource KubeResource,
	kubeResourceJSON bytes.Buffer,
	httpClient *http.Client) ([]byte, error) {

	postURL := kubeResource.KubeResourcesURL()
	req, err := http.NewRequest("POST", postURL, &kubeResourceJSON)
	if err != nil {
		return nil, fmt.Errorf("failed to create request: POST %q : %v", postURL, err)
	}
	res, err := ctxhttp.Do(ctx, httpClient, req)
	if err != nil {
		return nil, fmt.Errorf("failed to make request: POST %q: %v", postURL, err)
	}
	body, err := ioutil.ReadAll(res.Body)
	res.Body.Close()
	if err != nil {
		return nil, fmt.Errorf("failed to read request body for POST %q: %v", postURL, err)
	}
	if res.StatusCode != http.StatusCreated {
		return nil, fmt.Errorf("http error: %d POST %q: %q: %v", res.StatusCode, postURL, string(body), err)
	}

	return body, nil
}

func DeleteKubeResource(ctx context.Context, url string, httpClient *http.Client) error {
	req, err := http.NewRequest("DELETE", url, nil)
	if err != nil {
		return fmt.Errorf("failed to create request: DELETE %q : %v", url, err)
	}
	res, err := ctxhttp.Do(ctx, httpClient, req)
	if err != nil {
		return fmt.Errorf("failed to make request: DELETE %q: %v", url, err)
	}
	body, err := ioutil.ReadAll(res.Body)
	res.Body.Close()
	if err != nil {
		return fmt.Errorf("failed to read response body: DELETE %q: %v", url, err)
	}
	if res.StatusCode != http.StatusOK {
		return fmt.Errorf("http error: %d DELETE %q: %q: %v", res.StatusCode, url, string(body), err)
	}
	return nil
}

func UpdateKubeResources(ctx context.Context) error {
	return nil
}

func ListKubeResources(ctx context.Context, kubeResource KubeResource, httpClient *http.Client) ([]byte, error) {
	var results []byte
	kubeResourceURL, err := url.Parse(kubeResource.KubeResourcesURL())
	if err != nil {
		return results, err
	}

	values := url.Values{}
	values.Set("labelSelector", kubeResource.KubeResourceLabel())
	kubeResourceURL.RawQuery = values.Encode()

	url := kubeResourceURL.String()
	req, err := http.NewRequest("GET", url, nil)
	if err != nil {
		return results, fmt.Errorf("failed to create request: GET %q : %v", url, err)
	}
	res, err := ctxhttp.Do(ctx, httpClient, req)
	if err != nil {
		return results, fmt.Errorf("failed to make request: GET %q: %v", url, err)
	}
	results, err = ioutil.ReadAll(res.Body)
	res.Body.Close()
	if err != nil {
		return results, fmt.Errorf("failed to read response body: GET %q: %v", url, err)
	}
	if res.StatusCode != http.StatusOK {
		return results, fmt.Errorf("http error %d GET %q: %q: %v", res.StatusCode, url, string(results), err)
	}

	return results, nil
}
