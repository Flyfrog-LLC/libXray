package libXray

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"time"

	xnet "github.com/xtls/xray-core/common/net"
	"github.com/xtls/xray-core/core"
)

const (
	pingDelayTimeout int64 = 11000
	pingDelayError   int64 = 10000
)

type geoLocation struct {
	Ip string `json:"ip,omitempty"`
	Cc string `json:"cc,omitempty"`
}

func Ping(datDir string, config string, timeout int, url string, times int) string {
	initEnv(datDir)
	server, err := startXray(config)
	if err != nil {
		return fmt.Sprintf("%d::%s", pingDelayError, err)
	}

	if err := server.Start(); err != nil {
		return fmt.Sprintf("%d::%s", pingDelayError, err)
	}
	defer server.Close()

	return measureDelay(server, timeout, url, times)
}

func measureDelay(inst *core.Instance, timeout int, url string, times int) string {
	httpTimeout := time.Second * time.Duration(timeout)
	c, err := coreHTTPClient(inst, httpTimeout)
	if err != nil {
		return fmt.Sprintf("%d::%s", pingDelayError, err)
	}
	delaySum := int64(0)
	count := int64(0)
	isValid := false
	lastErr := ""
	for i := 0; i < times; i++ {
		delay, err := pingHTTPRequest(c, url)
		if delay != pingDelayTimeout {
			delaySum += delay
			count += 1
			isValid = true
		} else {
			lastErr = err.Error()
		}
	}
	if !isValid {
		return fmt.Sprintf("%d::%s", pingDelayTimeout, lastErr)
	}
	country, err := geolocationHTTPRequest(c)
	if err != nil {
		fmt.Println("geolocation error: ", err)
	}

	return fmt.Sprintf("%d:%s:%s", delaySum/count, country, lastErr)
}

func coreHTTPClient(inst *core.Instance, timeout time.Duration) (*http.Client, error) {
	if inst == nil {
		return nil, errors.New("core instance nil")
	}

	tr := &http.Transport{
		DisableKeepAlives: true,
		DialContext: func(ctx context.Context, network, addr string) (net.Conn, error) {
			dest, err := xnet.ParseDestination(fmt.Sprintf("%s:%s", network, addr))
			if err != nil {
				return nil, err
			}
			return core.Dial(ctx, inst, dest)
		},
	}

	c := &http.Client{
		Transport: tr,
		Timeout:   timeout,
	}

	return c, nil
}

func pingHTTPRequest(c *http.Client, url string) (int64, error) {
	start := time.Now()
	req, _ := http.NewRequest("GET", url, nil)
	_, err := c.Do(req)
	if err != nil {
		return pingDelayTimeout, err
	}
	return time.Since(start).Milliseconds(), nil
}

func geolocationHTTPRequest(c *http.Client) (string, error) {
	req, _ := http.NewRequest("GET", "https://ident.me/json", nil)
	resp, err := c.Do(req)
	if err != nil {
		return "", err
	}
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return "", err
	}

	var location geoLocation
	if err = json.Unmarshal(body, &location); err != nil {
		return "", err
	}
	return location.Cc, nil
}