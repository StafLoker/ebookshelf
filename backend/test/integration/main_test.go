package integration_test

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"net"
	"net/http"
	"net/http/cookiejar"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"syscall"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

const strongPassword = "Str0ng-Passw0rd!"

var binPath string

func TestMain(m *testing.M) {
	dir, err := os.MkdirTemp("", "ebookshelf-bin")
	if err != nil {
		panic(err)
	}
	binPath = filepath.Join(dir, "app")
	out, err := exec.Command("go", "build", "-ldflags", "-X main.version=test", "-o", binPath, "../../cmd/app").CombinedOutput()
	if err != nil {
		_ = os.RemoveAll(dir)
		panic(fmt.Sprintf("build: %v\n%s", err, out))
	}

	code := m.Run()
	_ = os.RemoveAll(dir)
	os.Exit(code)
}

func newConfig(t *testing.T) (string, string) {
	t.Helper()

	ln, err := net.Listen("tcp", "127.0.0.1:0")
	require.NoError(t, err)
	port := ln.Addr().(*net.TCPAddr).Port
	require.NoError(t, ln.Close())

	dir := t.TempDir()
	content := fmt.Sprintf("[server]\nport = %d\n\n[security]\nsecret_key = %s\nencryption_key = %s\n\n[log]\nlevel = DEBUG\n\n[paths]\ndata_dir = %s\n",
		port, base64.StdEncoding.EncodeToString([]byte(strings.Repeat("s", 32))), base64.StdEncoding.EncodeToString([]byte(strings.Repeat("k", 32))), filepath.Join(dir, "data"))
	path := filepath.Join(dir, "config.ini")
	require.NoError(t, os.WriteFile(path, []byte(content), 0o600))

	return path, fmt.Sprintf("http://127.0.0.1:%d", port)
}

func runApp(t *testing.T, configPath string, args ...string) (string, int) {
	t.Helper()

	var stdout bytes.Buffer
	cmd := exec.Command(binPath, append([]string{"-c", configPath}, args...)...)
	cmd.Stdout = &stdout
	err := cmd.Run()
	if err == nil {
		return strings.TrimSpace(stdout.String()), 0
	}
	exitErr, ok := err.(*exec.ExitError)
	require.True(t, ok, err)

	return strings.TrimSpace(stdout.String()), exitErr.ExitCode()
}

func startServer(t *testing.T, configPath, baseURL string) {
	t.Helper()

	cmd := exec.Command(binPath, "-c", configPath)
	require.NoError(t, cmd.Start())
	t.Cleanup(func() {
		_ = cmd.Process.Signal(syscall.SIGTERM)
		_ = cmd.Wait()
	})

	deadline := time.Now().Add(10 * time.Second)
	for {
		conn, err := net.DialTimeout("tcp", strings.TrimPrefix(baseURL, "http://"), 100*time.Millisecond)
		if err == nil {
			_ = conn.Close()
			return
		}
		require.True(t, time.Now().Before(deadline), "server did not start")
		time.Sleep(20 * time.Millisecond)
	}
}

func newClient(t *testing.T) *http.Client {
	t.Helper()

	jar, err := cookiejar.New(nil)
	require.NoError(t, err)

	return &http.Client{Jar: jar}
}

type reply struct {
	status  int
	header  http.Header
	cookies []*http.Cookie
	body    map[string]any
}

func (r reply) data() map[string]any {
	d, _ := r.body["data"].(map[string]any)
	return d
}

func call(t *testing.T, client *http.Client, method, url string, body any) reply {
	t.Helper()

	payload, err := json.Marshal(body)
	require.NoError(t, err)
	req, err := http.NewRequestWithContext(context.Background(), method, url, bytes.NewReader(payload))
	require.NoError(t, err)
	resp, err := client.Do(req)
	require.NoError(t, err)
	defer func() { _ = resp.Body.Close() }()

	out := reply{status: resp.StatusCode, header: resp.Header, cookies: resp.Cookies()}
	_ = json.NewDecoder(resp.Body).Decode(&out.body)

	return out
}

func tokenFrom(link string) string {
	_, token, _ := strings.Cut(link, "#token=")
	return token
}
