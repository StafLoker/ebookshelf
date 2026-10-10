package integration_test

import (
	"net/http"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestAuth_RootFlow(t *testing.T) {
	configPath, baseURL := newConfig(t)
	link, code := runApp(t, configPath, "root", "create", "admin")
	require.Equal(t, 0, code)
	startServer(t, configPath, baseURL)
	client := newClient(t)

	weak := call(t, client, http.MethodPost, baseURL+"/api/auth/reset-password", map[string]string{"token": tokenFrom(link), "new_password": "weak"})
	strong := call(t, client, http.MethodPost, baseURL+"/api/auth/reset-password", map[string]string{"token": tokenFrom(link), "new_password": strongPassword})
	login := call(t, client, http.MethodPost, baseURL+"/api/auth/login", map[string]string{"username": "admin", "password": strongPassword})
	me := call(t, client, http.MethodGet, baseURL+"/api/auth/me", nil)

	assert.Equal(t, http.StatusBadRequest, weak.status)
	assert.Equal(t, "AUTH_021", weak.data()["code"])
	assert.Equal(t, http.StatusOK, strong.status)
	require.Equal(t, http.StatusOK, login.status)
	require.Len(t, login.cookies, 1)
	assert.True(t, login.cookies[0].HttpOnly)
	assert.Equal(t, http.SameSiteLaxMode, login.cookies[0].SameSite)
	assert.Equal(t, http.StatusOK, me.status)
	assert.Equal(t, "root", me.data()["role"])
	assert.Equal(t, "private, no-store", me.header.Get("Cache-Control"))
}

func TestAuth_UserFlow(t *testing.T) {
	configPath, baseURL := newConfig(t)
	link, _ := runApp(t, configPath, "root", "create", "admin")
	startServer(t, configPath, baseURL)
	rootClient := newClient(t)
	userClient := newClient(t)
	call(t, rootClient, http.MethodPost, baseURL+"/api/auth/reset-password", map[string]string{"token": tokenFrom(link), "new_password": strongPassword})
	call(t, rootClient, http.MethodPost, baseURL+"/api/auth/login", map[string]string{"username": "admin", "password": strongPassword})

	created := call(t, rootClient, http.MethodPost, baseURL+"/api/users", map[string]any{"username": "reader", "role": "user"})
	require.Equal(t, http.StatusCreated, created.status)
	call(t, userClient, http.MethodPost, baseURL+"/api/auth/reset-password", map[string]string{"token": created.data()["token"].(string), "new_password": strongPassword})
	login := call(t, userClient, http.MethodPost, baseURL+"/api/auth/login", map[string]string{"username": "reader", "password": strongPassword})
	list := call(t, userClient, http.MethodGet, baseURL+"/api/users", nil)
	anonymous := call(t, newClient(t), http.MethodGet, baseURL+"/api/users", nil)

	assert.Equal(t, http.StatusOK, login.status)
	assert.Equal(t, http.StatusForbidden, list.status)
	assert.Equal(t, "AUTH_010", list.data()["code"])
	assert.Equal(t, http.StatusUnauthorized, anonymous.status)
	assert.Equal(t, "AUTH_002", anonymous.data()["code"])
}

func TestRouter_UnknownRoute_JSONError(t *testing.T) {
	configPath, baseURL := newConfig(t)
	startServer(t, configPath, baseURL)

	missing := call(t, newClient(t), http.MethodGet, baseURL+"/api/nope", nil)
	wrongMethod := call(t, newClient(t), http.MethodGet, baseURL+"/api/auth/login", nil)

	assert.Equal(t, http.StatusNotFound, missing.status)
	assert.Equal(t, "REQ_002", missing.data()["code"])
	assert.Equal(t, http.StatusMethodNotAllowed, wrongMethod.status)
	assert.Equal(t, "REQ_003", wrongMethod.data()["code"])
}

func TestCLI_RootCommands(t *testing.T) {
	configPath, _ := newConfig(t)
	first, firstCode := runApp(t, configPath, "root", "create", "admin")

	_, secondCode := runApp(t, configPath, "root", "create", "other")
	reset, resetCode := runApp(t, configPath, "root", "reset-password")
	version, versionCode := runApp(t, configPath, "--version")

	assert.Equal(t, 0, firstCode)
	assert.Contains(t, first, "/reset-password#token=")
	assert.Equal(t, 1, secondCode)
	assert.Equal(t, 0, resetCode)
	assert.Contains(t, reset, "/reset-password#token=")
	assert.NotEqual(t, first, reset)
	assert.Equal(t, 0, versionCode)
	assert.Equal(t, "test", version)
}
