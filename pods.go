package client

import (
	"bufio"
	"bytes"
	"encoding/json"
	"fmt"
	"io/ioutil"
	"net/http"
	"net/url"
	"time"

	"golang.org/x/build/kubernetes/api"
	"golang.org/x/net/context"
	"golang.org/x/net/context/ctxhttp"
)

const (
	podPath      = apiPrefix + "/namespaces/%s/pods"
	watchPodPath = apiPrefix + "/watch/namespaces/%s/pods/%s"
)

func (c *Client) CreatePod(ctx context.Context, pod *api.Pod) (*api.Pod, error) {
	var podJSON bytes.Buffer
	if err := json.NewEncoder(&podJSON).Encode(pod); err != nil {
		return nil, fmt.Errorf("failed to encode pod in json: %v", err)
	}
	postURL := c.podURL(pod.Namespace)
	req, err := http.NewRequest("POST", postURL, &podJSON)
	if err != nil {
		return nil, fmt.Errorf("failed to create request: POST %q : %v", postURL, err)
	}
	res, err := ctxhttp.Do(ctx, c.Client, req)
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
	var podResult api.Pod
	if err := json.Unmarshal(body, &podResult); err != nil {
		return nil, fmt.Errorf("failed to decode pod resources: %v", err)
	}
	ctx, cancel := context.WithTimeout(ctx, 2*time.Minute)
	defer cancel()

	createdPod, err := c.AwaitPodNotPending(ctx, pod.Namespace, podResult.Name, podResult.ObjectMeta.ResourceVersion)
	if err != nil {
		// The pod did not leave the pending state. We should try to manually delete it before
		// returning an error.
		c.DeletePod(context.Background(), pod.Namespace, podResult.Name)
		return nil, fmt.Errorf("timed out waiting for pod %q to leave pending state: %v", pod.Name, err)
	}
	return createdPod, nil
}

// PodDelete deletes the specified Kubernetes pod.
func (c *Client) DeletePod(ctx context.Context, namespace, podName string) error {
	url := c.podURL(namespace) + "/" + podName
	return c.deleteURL(ctx, url)
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

// PodLog retrieves the container log for the first container
// in the pod.
func (c *Client) PodLog(ctx context.Context, namespace, podName string) (string, error) {
	url := c.podURL(namespace) + "/" + podName + "/log"
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

func (c *Client) podURL(namespace string) string {
	return c.Host + fmt.Sprintf(podPath, namespace)
}

func (c *Client) PodList(ctx context.Context, namespace, label string) ([]api.Pod, error) {
	var pods []api.Pod
	podURL, err := url.Parse(c.podURL(namespace))
	if err != nil {
		return pods, err
	}

	values := url.Values{}
	values.Set("labelSelector", label)
	podURL.RawQuery = values.Encode()

	url := podURL.String()
	req, err := http.NewRequest("GET", url, nil)
	if err != nil {
		return pods, fmt.Errorf("failed to create request: GET %q : %v", url, err)
	}
	res, err := ctxhttp.Do(ctx, c.Client, req)
	if err != nil {
		return pods, fmt.Errorf("failed to make request: GET %q: %v", url, err)
	}
	body, err := ioutil.ReadAll(res.Body)
	res.Body.Close()
	if err != nil {
		return pods, fmt.Errorf("failed to read response body: GET %q: %v", url, err)
	}
	if res.StatusCode != http.StatusOK {
		return pods, fmt.Errorf("http error %d GET %q: %q: %v", res.StatusCode, url, string(body), err)
	}
	var podList api.PodList
	if err := json.Unmarshal(body, &podList); err != nil {
		return pods, fmt.Errorf("failed to decode pod resources: %v", err)
	}

	return podList.Items, nil
}

func EmptyPod() api.Pod {
	return api.Pod{}
}
