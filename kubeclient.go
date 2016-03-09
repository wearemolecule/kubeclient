// Copyright 2015 The Go Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

// Package kubernetes contains a minimal client for the Kubernetes API.
package kube

import (
	"crypto/tls"
	"crypto/x509"
	"errors"
	"fmt"
	"io/ioutil"
	"net/http"
	"os"

	"golang.org/x/net/context"
	"golang.org/x/net/context/ctxhttp"
)

const (
	apiPrefix = "/api/v1"
)

type Client struct {
	Host string

	*http.Client
}

func GetKubeClientFromEnv() (*Client, error) {
	certsPath := os.Getenv("CERTS_PATH")
	apiServer := os.ExpandEnv("https://${KUBERNETES_SERVICE_HOST}:${KUBERNETES_SERVICE_PORT}")

	certFile := fmt.Sprintf("%s/%s", certsPath, "cert.pem")
	keyFile := fmt.Sprintf("%s/%s", certsPath, "key.pem")
	caFile := fmt.Sprintf("%s/%s", certsPath, "ca.pem")
	certData, err := dataFromFile(certFile)
	if err != nil {
		return nil, errors.New("Couldn't load certificate")
	}
	keyData, err := dataFromFile(keyFile)
	if err != nil {
		return nil, errors.New("Couldn't load key")
	}
	caData, err := dataFromFile(caFile)
	if err != nil {
		return nil, errors.New("Couldn't load CA")
	}
	tlsConfig := &tls.Config{
		// Change default from SSLv3 to TLSv1.0 (because of POODLE vulnerability)
		MinVersion: tls.VersionTLS10,
		RootCAs:    rootCertPool(caData),
	}

	cert, err := tls.X509KeyPair(certData, keyData)
	if err != nil {
		return nil, errors.New("Failed to build X509 KeyPair")
	}
	tlsConfig.Certificates = []tls.Certificate{cert}

	tr := &http.Transport{
		TLSClientConfig: tlsConfig,
	}
	httpClient := &http.Client{
		Transport: tr,
	}

	client := Client{
		Host:   apiServer,
		Client: httpClient,
	}
	return &client, nil
}

func dataFromFile(file string) ([]byte, error) {
	fileData, err := ioutil.ReadFile(file)
	if err != nil {
		return []byte{}, err
	}
	return fileData, nil
}

// Pilfered from Kubernetes
// rootCertPool returns nil if caData is empty.  When passed along, this will mean "use system CAs".
// When caData is not empty, it will be the ONLY information used in the CertPool.
func rootCertPool(caData []byte) *x509.CertPool {
	// What we really want is a copy of x509.systemRootsPool, but that isn't exposed.  It's difficult to build (see the go
	// code for a look at the platform specific insanity), so we'll use the fact that RootCAs == nil gives us the system values
	// It doesn't allow trusting either/or, but hopefully that won't be an issue
	if len(caData) == 0 {
		return nil
	}

	// if we have caData, use it
	certPool := x509.NewCertPool()
	certPool.AppendCertsFromPEM(caData)
	return certPool
}

// Generic delete
func (c *Client) deleteURL(ctx context.Context, url string) error {
	req, err := http.NewRequest("DELETE", url, nil)
	if err != nil {
		return fmt.Errorf("failed to create request: DELETE %q : %v", url, err)
	}
	res, err := ctxhttp.Do(ctx, c.Client, req)
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
