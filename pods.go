package kubeclient

import (
	"bufio"
	"bytes"
	"encoding/json"
	"fmt"
	"io/ioutil"
	"net/http"
	"time"

	"golang.org/x/build/kubernetes/api"
	"golang.org/x/net/context"
	"golang.org/x/net/context/ctxhttp"
)

const (
	podsPath     = apiPrefix + "/namespaces/%s/pods"
	podPath      = apiPrefix + "/namespaces/%s/pods/%s"
	watchPodPath = apiPrefix + "/watch/namespaces/%s/pods/%s"
)

func (c *Client) CreatePod(ctx context.Context, pod *api.Pod) (*api.Pod, error) {
	var podJSON bytes.Buffer
	if err := json.NewEncoder(&podJSON).Encode(pod); err != nil {
		return nil, fmt.Errorf("failed to encode pod in json: %v", err)
	}

	apiResult, err := CreateKubeResource(ctx, &PodResource{c.Host, pod.Namespace, ""}, podJSON, c.Client)
	if err != nil {
		return nil, fmt.Errorf("Create failed: %v", err)
	}

	var podResult api.Pod
	if err := json.Unmarshal(apiResult, &podResult); err != nil {
		return nil, fmt.Errorf("failed to decode pod resources: %v", err)
	}
	ctx, cancel := context.WithTimeout(ctx, 2*time.Minute)
	defer cancel()

	createdPod, err := c.AwaitPodNotPending(ctx, pod.Namespace, podResult.Name, podResult.ObjectMeta.ResourceVersion)
	if err != nil {
		// The pod did not leave the pending state. We should try to manually delete it before returning an error.
		c.DeletePod(context.Background(), pod.Namespace, podResult.Name)
		return nil, fmt.Errorf("timed out waiting for pod %q to leave pending state: %v", pod.Name, err)
	}
	return createdPod, nil
}

// PodDelete deletes the specified Kubernetes pod.
func (c *Client) DeletePod(ctx context.Context, namespace, podName string) error {
	url := c.podURL(namespace, podName)
	return DeleteKubeResource(ctx, url, c.Client)
}

func (c *Client) UpdatePod(ctx context.Context, namespace, podName, image, version string) error {
	return nil
}

func (c *Client) PodList(ctx context.Context, namespace, label string) ([]api.Pod, error) {
	var pods []api.Pod

	apiResult, err := ListKubeResources(ctx, &PodResource{c.Host, namespace, label}, c.Client)
	if err != nil {
		return pods, fmt.Errorf("Resource List failed: %v", err)
	}
	var podList api.PodList
	if err := json.Unmarshal(apiResult, &podList); err != nil {
		return pods, fmt.Errorf("failed to decode pod resources: %v", err)
	}

	return podList.Items, nil
}

// PodLog retrieves the container log for the first container
// in the pod.
func (c *Client) PodLog(ctx context.Context, namespace, podName string) (string, error) {
	url := c.podURL(namespace, podName) + "/log"
	req, err := http.NewRequest("GET", url, nil)
	if err != nil {
		return "", fmt.Errorf("failed to create request: GET %q : %v", url, err)
	}
	res, err := ctxhttp.Do(ctx, c.Client, req)
	if err != nil {
		return "", fmt.Errorf("failed to make request: GET %q: %v", url, err)
	}
	body, err := ioutil.ReadAll(res.Body)
	res.Body.Close()
	if err != nil {
		return "", fmt.Errorf("failed to read response body: GET %q: %v", url, err)
	}
	if res.StatusCode != http.StatusOK {
		return "", fmt.Errorf("http error %d GET %q: %q: %v", res.StatusCode, url, string(body), err)
	}
	return string(body), nil
}

type PodResource struct {
	Host      string
	Namespace string
	Label     string
}

func (pod *PodResource) KubeResourcesURL() string {
	return pod.Host + fmt.Sprintf(podsPath, pod.Namespace)
}

func (pod *PodResource) KubeResourceNamespace() string {
	return pod.Namespace
}

func (pod *PodResource) KubeResourceLabel() string {
	return pod.Label
}

// awaitPodNotPending will return a pod's status in a
// podStatusResult when the pod is no longer in the pending
// state.
// The podResourceVersion is required to prevent a pod's entire
// history from being retrieved when the watch is initiated.
// If there is an error polling for the pod's status, or if
// ctx.Done is closed, podStatusResult will contain an error.
func (c *Client) AwaitPodNotPending(ctx context.Context, namespace, podName, podResourceVersion string) (*api.Pod, error) {
	if podResourceVersion == "" {
		return nil, fmt.Errorf("resourceVersion for pod %v must be provided", podName)
	}
	ctx, cancel := context.WithCancel(ctx)
	defer cancel()

	podStatusResult, err := c.WatchPod(ctx, namespace, podName, podResourceVersion)
	if err != nil {
		return nil, err
	}
	var psr PodStatusResult
	for {
		select {
		case psr = <-podStatusResult:
			if psr.Err != nil {
				return nil, psr.Err
			}
			if psr.Pod.Status.Phase != api.PodPending {
				return psr.Pod, nil
			}
		}
	}
}

// PodStatusResult wraps a api.PodStatus and error
type PodStatusResult struct {
	Pod  *api.Pod
	Type string
	Err  error
}

type watchPodStatus struct {
	// The type of watch update contained in the message
	Type string `json:"type"`
	// Pod details
	Object api.Pod `json:"object"`
}

// WatchPod long-polls the Kubernetes watch API to be notified
// of changes to the specified pod. Changes are sent on the returned
// PodStatusResult channel as they are received.
// The podResourceVersion is required to prevent a pod's entire
// history from being retrieved when the watch is initiated.
// The provided context must be canceled or timed out to stop the watch.
// If any error occurs communicating with the Kubernetes API, the
// error will be sent on the returned PodStatusResult channel and
// it will be closed.
func (c *Client) WatchPod(ctx context.Context, namespace, podName, podResourceVersion string) (<-chan PodStatusResult, error) {
	if podResourceVersion == "" {
		return nil, fmt.Errorf("resourceVersion for pod %v must be provided", podName)
	}
	statusChan := make(chan PodStatusResult)

	go func() {
		defer close(statusChan)
		// Make request to Kubernetes API
		watchPodUrl := fmt.Sprintf(watchPodPath, namespace, podName)
		getURL := c.Host + watchPodUrl
		req, err := http.NewRequest("GET", getURL, nil)
		req.URL.Query().Add("resourceVersion", podResourceVersion)
		if err != nil {
			statusChan <- PodStatusResult{Err: fmt.Errorf("failed to create request: GET %q : %v", getURL, err)}
			return
		}
		res, err := ctxhttp.Do(ctx, c.Client, req)
		defer res.Body.Close()
		if err != nil {
			statusChan <- PodStatusResult{Err: fmt.Errorf("failed to make request: GET %q: %v", getURL, err)}
			return
		}

		var wps watchPodStatus
		reader := bufio.NewReader(res.Body)

		// bufio.Reader.ReadBytes is blocking, so we watch for
		// context timeout or cancellation in a goroutine
		// and close the response body when see see it. The
		// response body is also closed via defer when the
		// request is made, but closing twice is OK.
		go func() {
			<-ctx.Done()
			res.Body.Close()
		}()

		for {
			line, err := reader.ReadBytes('\n')
			if ctx.Err() != nil {
				statusChan <- PodStatusResult{Err: ctx.Err()}
				return
			}
			if err != nil {
				statusChan <- PodStatusResult{Err: fmt.Errorf("error reading streaming response body: %v", err)}
				return
			}
			if err := json.Unmarshal(line, &wps); err != nil {
				statusChan <- PodStatusResult{Err: fmt.Errorf("failed to decode watch pod status: %v", err)}
				return
			}
			statusChan <- PodStatusResult{Pod: &wps.Object, Type: wps.Type}
		}
	}()
	return statusChan, nil
}

func (c *Client) podURL(namespace, name string) string {
	return c.Host + fmt.Sprintf(podPath, namespace, name)
}
