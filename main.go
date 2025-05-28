package main

import (
	"bufio"
	"fmt"
	"io"
	"log"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"github.com/gorilla/websocket"
)

type CompileRequest struct {
	Latex string `json:"latex"`
}

var upgrader = websocket.Upgrader{
	CheckOrigin: func(r *http.Request) bool { return true },
}

func main() {
	r := gin.Default()
	r.Static("/static", "./static")
	r.GET("/", func(c *gin.Context) {
		c.Redirect(http.StatusFound, "/static/index.html")
	})
	r.GET("/ws/compile", wsCompileHandler)
	r.GET("/pdf/:filename", servePDF)

	r.POST("/compile", compileHandler)

	_ = os.MkdirAll("tmp", 0755)
	r.Run(":8080")
}

func servePDF(c *gin.Context) {
	filename := c.Param("filename")
	filePath := filepath.Join("tmp", filename)
	defer os.Remove(filePath)
	c.File(filePath)
}

func compileHandler(c *gin.Context) {
	var req CompileRequest
	if err := c.BindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid request"})
		return
	}

	id := uuid.New().String()
	tmpDir := "tmp"

	texPath, err := writeTempLatexFile(tmpDir, id, req.Latex)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	defer os.Remove(texPath)

	pdfPath, err := compilePDF(tmpDir, texPath, id)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	defer os.Remove(pdfPath)

	c.FileAttachment(pdfPath, id+".pdf")
}

func wsCompileHandler(c *gin.Context) {
	conn, err := upgrader.Upgrade(c.Writer, c.Request, nil)
	if err != nil {
		log.Println("WebSocket upgrade error:", err)
		return
	}
	defer conn.Close()

	_, latex, err := conn.ReadMessage()
	if err != nil {
		conn.WriteMessage(websocket.TextMessage, []byte("Failed to read message: "+err.Error()))
		return
	}

	id := uuid.New().String()
	tmpDir := "tmp"

	texPath, err := writeTempLatexFile(tmpDir, id, string(latex))
	if err != nil {
		conn.WriteMessage(websocket.TextMessage, []byte("Failed to write file: "+err.Error()))
		return
	}
	defer os.Remove(texPath)

	cmd := exec.Command("pdflatex", "-interaction=nonstopmode", "-output-directory", tmpDir, texPath)
	stdout, _ := cmd.StdoutPipe()
	stderr, _ := cmd.StderrPipe()
	_ = cmd.Start()

	go streamLogs(stdout, conn)
	go streamLogs(stderr, conn)

	_ = cmd.Wait()

	pdfPath := filepath.Join(tmpDir, id+".pdf")
	if _, err := os.Stat(pdfPath); err == nil {
		conn.WriteMessage(websocket.TextMessage, []byte("PDF:/pdf/"+id+".pdf"))
	} else {
		conn.WriteMessage(websocket.TextMessage, []byte("Compilation failed"))
	}
}

func streamLogs(r io.Reader, conn *websocket.Conn) {
	scanner := bufio.NewScanner(r)
	for scanner.Scan() {
		conn.WriteMessage(websocket.TextMessage, scanner.Bytes())
	}
}

func writeTempLatexFile(dir, id, content string) (string, error) {
	texPath := filepath.Join(dir, id+".tex")
	if err := os.WriteFile(texPath, []byte(content), 0644); err != nil {
		return "", fmt.Errorf("failed to write LaTeX file: %w", err)
	}
	return texPath, nil
}

func compilePDF(tmpDir, texPath, id string) (string, error) {
	pdfPath := filepath.Join(tmpDir, id+".pdf")
	cmd := exec.Command("pdflatex", "-interaction=nonstopmode", "-output-directory", tmpDir, texPath)
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr

	if err := cmd.Run(); err != nil {
		return "", fmt.Errorf("pdflatex failed: %w", err)
	}

	if _, err := os.Stat(pdfPath); os.IsNotExist(err) {
		return "", fmt.Errorf("PDF was not generated")
	}

	return pdfPath, nil
}
