package handler

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/pingo/hub/internal/filestore"
)

func TestDownloadRequiresFileMessageMembership(t *testing.T) {
	gin.SetMode(gin.TestMode)
	files, err := filestore.NewLocalFileStore(t.TempDir(), "50MB")
	if err != nil {
		t.Fatal(err)
	}
	key, _, err := files.Save("report.txt", http.NoBody)
	if err != nil {
		t.Fatal(err)
	}
	handler := NewFileHandler(files, 50*1024*1024)
	response := httptest.NewRecorder()
	context, _ := gin.CreateTestContext(response)
	context.Params = gin.Params{{Key: "key", Value: key}}
	context.Set("agent_id", "other-agent")
	context.Request = httptest.NewRequest(http.MethodGet, "/api/files/"+key, nil)
	handler.Download(context)
	if response.Code != http.StatusForbidden {
		t.Fatalf("download returned %d without membership", response.Code)
	}
}
