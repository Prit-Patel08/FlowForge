package clouddeps

import (
	"fmt"
	"net"
	"net/http"
	"os"
	"strconv"
	"strings"
	"sync"
	"time"
)

type Config struct {
	Required       bool
	PostgresAddr   string
	RedisAddr      string
	NATSHealthURL  string
	MinIOHealthURL string
	Timeout        time.Duration
}

type CheckResult struct {
	Name    string `json:"name"`
	Healthy bool   `json:"healthy"`
	Target  string `json:"target,omitempty"`
	Error   string `json:"error,omitempty"`
}

func LoadFromEnv() Config {
	return Config{
		Required:       envBool("FLOWFORGE_CLOUD_DEPS_REQUIRED", false),
		PostgresAddr:   envString("FLOWFORGE_CLOUD_POSTGRES_ADDR", "127.0.0.1:15432"),
		RedisAddr:      envString("FLOWFORGE_CLOUD_REDIS_ADDR", "127.0.0.1:16379"),
		NATSHealthURL:  envString("FLOWFORGE_CLOUD_NATS_HEALTH_URL", "http://127.0.0.1:18222/healthz"),
		MinIOHealthURL: envString("FLOWFORGE_CLOUD_MINIO_HEALTH_URL", "http://127.0.0.1:19000/minio/health/live"),
		Timeout:        envDurationMS("FLOWFORGE_CLOUD_PROBE_TIMEOUT_MS", 800),
	}
}

func Probe(cfg Config) ([]CheckResult, bool) {
	results := make([]CheckResult, 4)
	var wg sync.WaitGroup
	wg.Add(4)
	go func() {
		defer wg.Done()
		results[0] = probeTCP("cloud_postgres", cfg.PostgresAddr, cfg.Timeout)
	}()
	go func() {
		defer wg.Done()
		results[1] = probeTCP("cloud_redis", cfg.RedisAddr, cfg.Timeout)
	}()
	go func() {
		defer wg.Done()
		results[2] = probeHTTP("cloud_nats", cfg.NATSHealthURL, cfg.Timeout)
	}()
	go func() {
		defer wg.Done()
		results[3] = probeHTTP("cloud_minio", cfg.MinIOHealthURL, cfg.Timeout)
	}()
	wg.Wait()

	healthy := true
	for _, r := range results {
		if !r.Healthy {
			healthy = false
		}
	}
	return results, healthy
}

func probeTCP(name, addr string, timeout time.Duration) CheckResult {
	res := CheckResult{Name: name, Target: addr}
	conn, err := net.DialTimeout("tcp", addr, timeout)
	if err != nil {
		res.Error = err.Error()
		return res
	}
	_ = conn.Close()
	res.Healthy = true
	return res
}

func probeHTTP(name, url string, timeout time.Duration) CheckResult {
	res := CheckResult{Name: name, Target: url}
	client := &http.Client{Timeout: timeout}
	resp, err := client.Get(url)
	if err != nil {
		res.Error = err.Error()
		return res
	}
	defer resp.Body.Close()
	if resp.StatusCode/100 != 2 {
		res.Error = fmt.Sprintf("non-2xx status: %d", resp.StatusCode)
		return res
	}
	res.Healthy = true
	return res
}

func envString(key, fallback string) string {
	v := strings.TrimSpace(os.Getenv(key))
	if v == "" {
		return fallback
	}
	return v
}

func envBool(key string, fallback bool) bool {
	v := strings.TrimSpace(strings.ToLower(os.Getenv(key)))
	if v == "" {
		return fallback
	}
	switch v {
	case "1", "true", "yes", "on":
		return true
	case "0", "false", "no", "off":
		return false
	default:
		return fallback
	}
}

func envDurationMS(key string, fallbackMS int) time.Duration {
	v := strings.TrimSpace(os.Getenv(key))
	if v == "" {
		return time.Duration(fallbackMS) * time.Millisecond
	}
	n, err := strconv.Atoi(v)
	if err != nil || n <= 0 {
		return time.Duration(fallbackMS) * time.Millisecond
	}
	return time.Duration(n) * time.Millisecond
}
