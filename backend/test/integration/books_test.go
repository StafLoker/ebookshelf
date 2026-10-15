package integration_test

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"mime/multipart"
	"net/http"
	"os"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"ebookshelf/test/testutil"
)

func upload(t *testing.T, client *http.Client, url, path string) reply {
	t.Helper()

	var body bytes.Buffer
	data, err := os.ReadFile(path)
	require.NoError(t, err)
	writer := multipart.NewWriter(&body)
	part, err := writer.CreateFormFile("file", "book.fb2")
	require.NoError(t, err)
	_, err = part.Write(data)
	require.NoError(t, err)
	require.NoError(t, writer.Close())
	req, err := http.NewRequestWithContext(context.Background(), http.MethodPost, url, &body)
	require.NoError(t, err)
	req.Header.Set("Content-Type", writer.FormDataContentType())
	resp, err := client.Do(req)
	require.NoError(t, err)
	defer func() { _ = resp.Body.Close() }()

	return decodeReply(t, resp)
}

func TestBooks_AddCommitListDownload(t *testing.T) {
	configPath, baseURL := newConfig(t)
	link, _ := runApp(t, configPath, "root", "create", "admin")
	startServer(t, configPath, baseURL)
	client := newClient(t)
	call(t, client, http.MethodPost, baseURL+"/api/auth/reset-password", map[string]string{"token": tokenFrom(link), "new_password": strongPassword})
	call(t, client, http.MethodPost, baseURL+"/api/auth/login", map[string]string{"username": "admin", "password": strongPassword})
	path := testutil.WriteFB2(t, t.TempDir(), "Тихий Дон", "Михаил")

	uploaded := upload(t, client, baseURL+"/api/books", path)
	require.Equal(t, http.StatusCreated, uploaded.status)
	id := fmt.Sprint(uploaded.data()["id"])
	before := call(t, client, http.MethodGet, baseURL+"/api/books?q=дон", nil)
	committed := call(t, client, http.MethodPost, baseURL+"/api/books/"+id+"/commit", nil)
	again := upload(t, client, baseURL+"/api/books", path)
	list := call(t, client, http.MethodGet, baseURL+"/api/books?q=дон", nil)
	file, err := client.Get(baseURL + "/api/books/1/file")
	require.NoError(t, err)
	defer func() { _ = file.Body.Close() }()
	content, _ := io.ReadAll(file.Body)

	assert.Equal(t, float64(0), before.body["metadata"].(map[string]any)["total"])
	assert.Equal(t, http.StatusOK, committed.status)
	assert.Equal(t, http.StatusConflict, again.status)
	assert.Equal(t, "BOOK_001", again.data()["code"])
	assert.Equal(t, float64(1), list.body["metadata"].(map[string]any)["total"])
	assert.Contains(t, file.Header.Get("Content-Disposition"), "attachment")
	assert.Contains(t, string(content), "FictionBook")

	edited := call(t, client, http.MethodPut, baseURL+"/api/books/1", map[string]any{
		"title": "Поднятая целина", "language": "ru", "authors": []map[string]string{{"last_name": "Михаил"}},
	})
	deleted := call(t, client, http.MethodDelete, baseURL+"/api/books/1", nil)
	gone := call(t, client, http.MethodGet, baseURL+"/api/books/1", nil)

	assert.Equal(t, http.StatusOK, edited.status)
	assert.Equal(t, "Поднятая целина", edited.data()["title"])
	assert.Empty(t, edited.data()["tags"])
	assert.Equal(t, http.StatusOK, deleted.status)
	assert.Equal(t, http.StatusNotFound, gone.status)
}
