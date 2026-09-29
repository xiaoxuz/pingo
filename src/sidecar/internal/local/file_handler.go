package local

import (
	"bytes"
	"io"
	"mime/multipart"
	"net/http"
	"strconv"

	"github.com/gin-gonic/gin"
)

// handleFileUpload 接收本地文件上传，转发到 Hub
func (s *APIServer) handleFileUpload(c *gin.Context) {
	agentID := getAgentID(c)

	file, header, err := c.Request.FormFile("file")
	if err != nil {
		errResponse(c, http.StatusBadRequest, "no file uploaded")
		return
	}
	defer file.Close()

	// 读取文件内容
	fileBytes, err := io.ReadAll(file)
	if err != nil {
		errResponse(c, http.StatusInternalServerError, "read file failed: "+err.Error())
		return
	}

	// 构造 multipart 请求转发到 Hub
	var buf bytes.Buffer
	writer := multipart.NewWriter(&buf)
	part, err := writer.CreateFormFile("file", header.Filename)
	if err != nil {
		errResponse(c, http.StatusInternalServerError, "create form file failed: "+err.Error())
		return
	}
	part.Write(fileBytes)
	writer.Close()

	hubHTTP := s.config.Hub.HTTPEndpoint()
	req, err := http.NewRequest("POST", hubHTTP+"/api/files/upload", &buf)
	if err != nil {
		errResponse(c, http.StatusInternalServerError, "create request failed: "+err.Error())
		return
	}
	req.Header.Set("Content-Type", writer.FormDataContentType())

	// 找 agent 的 token
	token := s.getAgentToken(agentID)
	if token != "" {
		req.Header.Set("Authorization", "Bearer "+token)
	}

	client := &http.Client{}
	resp, err := client.Do(req)
	if err != nil {
		errResponse(c, http.StatusBadGateway, "hub upload failed: "+err.Error())
		return
	}
	defer resp.Body.Close()

	body, _ := io.ReadAll(resp.Body)
	c.Data(resp.StatusCode, "application/json", body)
}

// handleFileDownload 代理文件下载
func (s *APIServer) handleFileDownload(c *gin.Context) {
	agentID := getAgentID(c)
	key := c.Param("key")
	if key == "" {
		errResponse(c, http.StatusBadRequest, "missing file key")
		return
	}

	hubHTTP := s.config.Hub.HTTPEndpoint()
	req, err := http.NewRequest("GET", hubHTTP+"/api/files/"+key, nil)
	if err != nil {
		errResponse(c, http.StatusInternalServerError, "create request failed: "+err.Error())
		return
	}

	token := s.getAgentToken(agentID)
	if token != "" {
		req.Header.Set("Authorization", "Bearer "+token)
	}

	client := &http.Client{}
	resp, err := client.Do(req)
	if err != nil {
		errResponse(c, http.StatusBadGateway, "hub download failed: "+err.Error())
		return
	}
	defer resp.Body.Close()

	// 复制 header
	for k, v := range resp.Header {
		if k == "Content-Disposition" || k == "Content-Length" || k == "Content-Type" {
			c.Header(k, v[0])
		}
	}
	c.Status(resp.StatusCode)
	io.Copy(c.Writer, resp.Body)
}

// getAgentToken 获取 agent 的 token
func (s *APIServer) getAgentToken(agentID string) string {
	for _, a := range s.config.Agents {
		if a.ID == agentID {
			return a.Token
		}
	}
	return ""
}

// 确保使用 strconv（防未使用警告）
var _ = strconv.Itoa
