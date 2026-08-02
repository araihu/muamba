package transport

import (
	"context"
	"crypto/tls"
	"crypto/x509"
	"fmt"
	"io"
	"math"
	"net/http"
	"net/url"
	"os"
	"time"
)

const (
	defaultTimeout  = 60 * time.Second
	defaultMaxBytes = int64(100 << 20)
)

type Options struct {
	AllowHTTP             bool
	InsecureSkipTLSVerify bool
	CAFile                string
	Timeout               time.Duration
	MaxBytes              int64
}

type Client struct {
	options Options
	http    *http.Client
}

func New(options Options) (*Client, error) {
	if options.Timeout <= 0 {
		options.Timeout = defaultTimeout
	}
	if options.MaxBytes <= 0 {
		options.MaxBytes = defaultMaxBytes
	}
	roots, err := x509.SystemCertPool()
	if err != nil {
		return nil, fmt.Errorf("load system certificates: %w", err)
	}
	if options.CAFile != "" {
		pemBytes, readErr := os.ReadFile(options.CAFile)
		if readErr != nil {
			return nil, fmt.Errorf("read CA file: %w", readErr)
		}
		if !roots.AppendCertsFromPEM(pemBytes) {
			return nil, fmt.Errorf("CA file %s contains no certificates", options.CAFile)
		}
	}
	transport := http.DefaultTransport.(*http.Transport).Clone()
	transport.TLSClientConfig = &tls.Config{
		MinVersion:         tls.VersionTLS12,
		RootCAs:            roots,
		InsecureSkipVerify: options.InsecureSkipTLSVerify, //nolint:gosec // explicit user opt-in
	}
	client := &Client{options: options}
	client.http = &http.Client{
		Transport: transport,
		Timeout:   options.Timeout,
		CheckRedirect: func(request *http.Request, via []*http.Request) error {
			if len(via) >= 10 {
				return fmt.Errorf("redirect limit exceeded")
			}
			return client.validateURL(request.URL)
		},
	}
	return client, nil
}

func (c *Client) Fetch(ctx context.Context, rawURL string, destination io.Writer, maxBytes int64) (int64, error) {
	if maxBytes <= 0 {
		maxBytes = c.options.MaxBytes
	}
	parsed, err := url.Parse(rawURL)
	if err != nil {
		return 0, fmt.Errorf("parse URL: %w", err)
	}
	if err := c.validateURL(parsed); err != nil {
		return 0, err
	}
	request, err := http.NewRequestWithContext(ctx, http.MethodGet, parsed.String(), nil)
	if err != nil {
		return 0, err
	}
	response, err := c.http.Do(request)
	if err != nil {
		return 0, fmt.Errorf("GET %s: %w", parsed.Host, err)
	}
	defer func() { _ = response.Body.Close() }()
	if response.StatusCode != http.StatusOK {
		return 0, fmt.Errorf("GET %s: status %d", parsed.Host, response.StatusCode)
	}
	readLimit := maxBytes
	if readLimit < math.MaxInt64 {
		readLimit++
	}
	written, err := io.Copy(destination, io.LimitReader(response.Body, readLimit))
	if err != nil {
		return written, fmt.Errorf("read response from %s: %w", parsed.Host, err)
	}
	if written > maxBytes {
		return written, fmt.Errorf("response from %s exceeds %d bytes", parsed.Host, maxBytes)
	}
	return written, nil
}

func (c *Client) validateURL(parsed *url.URL) error {
	switch parsed.Scheme {
	case "https":
		return nil
	case "http":
		if c.options.AllowHTTP {
			return nil
		}
		return fmt.Errorf("HTTP requires --allow-http for host %s", parsed.Host)
	default:
		return fmt.Errorf("unsupported URL scheme %q", parsed.Scheme)
	}
}
