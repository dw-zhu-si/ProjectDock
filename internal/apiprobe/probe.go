package apiprobe

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"strings"
	"time"

	"projectdock/internal/model"
)

const (
	maxRequestBody  = 1 << 20
	maxResponseBody = 2 << 20
)

var blockedHeaders = map[string]bool{
	"authorization":       true,
	"cookie":              true,
	"proxy-authorization": true,
	"host":                true,
	"content-length":      true,
	"connection":          true,
	"transfer-encoding":   true,
	"upgrade":             true,
}

type Request struct {
	Method  string            `json:"method"`
	URL     string            `json:"url"`
	Headers map[string]string `json:"headers"`
	Body    string            `json:"body"`
}

type Response struct {
	Status     int                 `json:"status"`
	StatusText string              `json:"statusText"`
	DurationMS int64               `json:"durationMs"`
	Headers    map[string][]string `json:"headers"`
	Body       string              `json:"body"`
	Truncated  bool                `json:"truncated"`
}

type Service struct {
	client *http.Client
}

func NewService() *Service {
	dialer := &net.Dialer{Timeout: 5 * time.Second}
	transport := &http.Transport{
		Proxy: nil,
		DialContext: func(ctx context.Context, network, address string) (net.Conn, error) {
			host, _, err := net.SplitHostPort(address)
			if err != nil {
				return nil, err
			}
			if host != "localhost" && host != "127.0.0.1" && host != "::1" {
				return nil, errors.New("接口调试只允许连接回环地址")
			}
			return dialer.DialContext(ctx, network, address)
		},
		DisableKeepAlives: true,
	}
	return &Service{client: &http.Client{
		Transport: transport,
		Timeout:   20 * time.Second,
		CheckRedirect: func(req *http.Request, via []*http.Request) error {
			if err := model.ValidateLoopbackURL(req.URL.String()); err != nil {
				return errors.New("重定向离开回环地址，已阻止")
			}
			if len(via) >= 5 {
				return errors.New("重定向次数超过限制")
			}
			return nil
		},
	}}
}

func (s *Service) Do(ctx context.Context, input Request) (Response, error) {
	method := strings.ToUpper(strings.TrimSpace(input.Method))
	switch method {
	case http.MethodGet, http.MethodPost, http.MethodPut, http.MethodPatch, http.MethodDelete, http.MethodHead:
	default:
		return Response{}, errors.New("只支持 GET、POST、PUT、PATCH、DELETE 或 HEAD")
	}
	if err := model.ValidateLoopbackURL(input.URL); err != nil {
		return Response{}, err
	}
	if len(input.Body) > maxRequestBody {
		return Response{}, errors.New("请求正文不能超过 1 MiB")
	}
	request, err := http.NewRequestWithContext(ctx, method, input.URL, strings.NewReader(input.Body))
	if err != nil {
		return Response{}, fmt.Errorf("创建接口请求: %w", err)
	}
	for name, value := range input.Headers {
		lower := strings.ToLower(strings.TrimSpace(name))
		if blockedHeaders[lower] || strings.HasPrefix(lower, "proxy-") {
			return Response{}, fmt.Errorf("请求头 %s 不允许在调试台设置", name)
		}
		request.Header.Set(name, value)
	}
	started := time.Now()
	response, err := s.client.Do(request)
	if err != nil {
		return Response{}, fmt.Errorf("调用本地接口: %w", err)
	}
	defer response.Body.Close()
	limited := io.LimitReader(response.Body, maxResponseBody+1)
	body, err := io.ReadAll(limited)
	if err != nil {
		return Response{}, fmt.Errorf("读取接口响应: %w", err)
	}
	truncated := len(body) > maxResponseBody
	if truncated {
		body = body[:maxResponseBody]
	}
	return Response{
		Status:     response.StatusCode,
		StatusText: response.Status,
		DurationMS: time.Since(started).Milliseconds(),
		Headers:    sanitizeResponseHeaders(response.Header),
		Body:       string(body),
		Truncated:  truncated,
	}, nil
}

func sanitizeResponseHeaders(headers http.Header) map[string][]string {
	result := make(map[string][]string, len(headers))
	for name, values := range headers {
		lower := strings.ToLower(name)
		if lower == "set-cookie" || lower == "www-authenticate" || strings.Contains(lower, "token") {
			result[name] = []string{"[已隐藏]"}
			continue
		}
		result[name] = append([]string(nil), values...)
	}
	return result
}

func IsLoopbackURL(raw string) bool {
	parsed, err := url.Parse(raw)
	if err != nil {
		return false
	}
	host := parsed.Hostname()
	return host == "localhost" || host == "127.0.0.1" || host == "::1"
}
