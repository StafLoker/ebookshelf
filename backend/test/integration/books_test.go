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

func TestBooks_UploadCommitListDownload(t *testing.T) {
	configPath, baseURL := newConfig(t)
	link, _ := runApp(t, configPath, "root", "create", "admin")
	startServer(t, configPath, baseURL)
	client := newClient(t)
	call(t, client, http.MethodPost, baseURL+"/api/auth/reset-password", map[string]string{"token": tokenFrom(link), "new_password": strongPassword})
	call(t, client, http.MethodPost, baseURL+"/api/auth/login", map[string]string{"username": "admin", "password": strongPassword})
	path := testutil.WriteFB2(t, t.TempDir(), "Тихий Дон", "Михаил")

	uploaded := upload(t, client, baseURL+"/api/uploads", path)
	require.Equal(t, http.StatusCreated, uploaded.status)
	id := fmt.Sprint(uploaded.data()["id"])
	hidden := call(t, client, http.MethodGet, baseURL+"/api/books/"+id, nil)
	committed := call(t, client, http.MethodPost, baseURL+"/api/uploads/"+id+"/commit", nil)
	again := upload(t, client, baseURL+"/api/uploads", path)
	list := call(t, client, http.MethodGet, baseURL+"/api/books?q=дон", nil)
	file, err := client.Get(baseURL + "/api/books/1/file")
	require.NoError(t, err)
	defer func() { _ = file.Body.Close() }()
	content, _ := io.ReadAll(file.Body)

	assert.Equal(t, http.StatusNotFound, hidden.status)
	assert.Equal(t, http.StatusOK, committed.status)
	assert.Equal(t, http.StatusConflict, again.status)
	assert.Equal(t, "BOOK_001", again.data()["code"])
	assert.Equal(t, float64(1), list.body["metadata"].(map[string]any)["total"])
	assert.Contains(t, file.Header.Get("Content-Disposition"), "attachment")
	assert.Contains(t, string(content), "FictionBook")
}
