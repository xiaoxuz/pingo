package handler

import (
	"io"
	"net/http"
	"path/filepath"
	"strconv"

	"github.com/gin-gonic/gin"
	"github.com/pingo/hub/internal/auth"
	"github.com/pingo/hub/internal/filestore"
	"github.com/pingo/hub/internal/store"
)

type FileHandler struct {
	store    filestore.FileStore
	maxSize  int64
	messages *store.MessageStore
}

func NewFileHandler(store filestore.FileStore, maxSize int64) *FileHandler {
	return &FileHandler{
		store:   store,
		maxSize: maxSize,
	}
}

func (h *FileHandler) RequireMembership(messages *store.MessageStore) *FileHandler {
	h.messages = messages
	return h
}

// Upload 上传文件
func (h *FileHandler) Upload(c *gin.Context) {
	agentID := auth.GetAgentID(c)
	_ = agentID

	file, header, err := c.Request.FormFile("file")
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"ok": false, "error": "no file uploaded"})
		return
	}
	defer file.Close()

	filename := header.Filename
	size := header.Size

	if size > h.maxSize {
		c.JSON(http.StatusRequestEntityTooLarge, gin.H{
			"ok":    false,
			"error": "file too large, max size: " + strconv.FormatInt(h.maxSize/1024/1024, 10) + "MB",
		})
		return
	}

	storageKey, savedSize, err := h.store.Save(filename, file)
	if err != nil {
		if err == filestore.ErrFileTooLarge {
			c.JSON(http.StatusRequestEntityTooLarge, gin.H{"ok": false, "error": "file too large"})
			return
		}
		c.JSON(http.StatusInternalServerError, gin.H{"ok": false, "error": err.Error()})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"ok": true,
		"data": gin.H{
			"storage_key": storageKey,
			"filename":    filename,
			"size":        savedSize,
		},
	})
}

// Download 下载文件
func (h *FileHandler) Download(c *gin.Context) {
	agentID := auth.GetAgentID(c)

	key := c.Param("key")
	if key == "" {
		c.JSON(http.StatusBadRequest, gin.H{"ok": false, "error": "missing file key"})
		return
	}

	storageKey := key
	if len(key) > 6 && key[:6] != "files/" {
		storageKey = "files/" + key
	}
	if h.messages == nil {
		c.JSON(http.StatusForbidden, gin.H{"ok": false, "error": "file access unavailable"})
		return
	}
	member, err := h.messages.CanDownloadFile(c.Request.Context(), storageKey, agentID)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"ok": false, "error": "file lookup failed"})
		return
	}
	if !member {
		c.JSON(http.StatusForbidden, gin.H{"ok": false, "error": "not a conversation member"})
		return
	}

	reader, size, filename, err := h.store.Get(storageKey)
	if err != nil {
		if err == filestore.ErrFileNotFound {
			c.JSON(http.StatusNotFound, gin.H{"ok": false, "error": "file not found"})
			return
		}
		c.JSON(http.StatusInternalServerError, gin.H{"ok": false, "error": err.Error()})
		return
	}
	defer reader.Close()

	c.Header("Content-Disposition", "attachment; filename=\""+filepath.Base(filename)+"\"")
	c.Header("Content-Length", strconv.FormatInt(size, 10))
	c.Header("Content-Type", "application/octet-stream")
	c.Status(http.StatusOK)
	io.Copy(c.Writer, reader)
}
