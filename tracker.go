package main

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"github.com/gin-contrib/cors"
	"github.com/gin-gonic/gin"
	"io"
	"log"
	"mime/multipart"
	"net/http"
	"strings"
	"sync"
	"time"
)

type DataNode struct {
	IP       string
	LastSeen time.Time
}

type FileChunk struct {
	ChunkID   string
	NodeIP    string
	ChunkSize int64
}

type FileMetadata struct {
	Filename    string
	TotalSize   int64
	TotalChunks int
	Chunks      []FileChunk
}

var (
	dataNodes   = make(map[string]DataNode)
	fileStorage = make(map[string]FileMetadata)
	lock        sync.RWMutex
)

func generateChunkID(data []byte) string {
	hash := sha256.Sum256(data)
	return hex.EncodeToString(hash[:])
}

func registerDataNode(c *gin.Context) {
	ip := c.ClientIP()
	port := c.Query("port")

	if port == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Port is required"})
		return
	}

	// Normalize IP addresses
	if ip == "::1" {
		ip = "127.0.0.1"
	}

	fullAddress := ip + ":" + port
	fullAddress = strings.Replace(fullAddress, "::1", "127.0.0.1", 1)

	lock.Lock()
	dataNodes[fullAddress] = DataNode{IP: fullAddress, LastSeen: time.Now()}
	lock.Unlock()

	log.Println("Registered node:", fullAddress)
	c.JSON(http.StatusOK, gin.H{"status": "registered", "node": fullAddress})
}

func uploadFile(c *gin.Context) {
	file, header, err := c.Request.FormFile("file")
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Failed to get file"})
		return
	}
	defer file.Close()

	// Get active data nodes
	lock.RLock()
	activeNodes := make([]string, 0, len(dataNodes))
	for ip := range dataNodes {
		activeNodes = append(activeNodes, ip)
	}
	lock.RUnlock()

	if len(activeNodes) == 0 {
		c.JSON(http.StatusServiceUnavailable, gin.H{"error": "No active data nodes"})
		return
	}

	// Chunking file and distributing to data nodes
	fileMetadata := FileMetadata{
		Filename:  header.Filename,
		TotalSize: header.Size,
		Chunks:    []FileChunk{},
	}

	// chunkSize := int64(10 * 1024 * 1024) // 10MB chunks

	// For kilobytes (KB)
	// chunkSize := int64(64 * 1024) // 64KB chunks

	// For bytes (B)
	chunkSize := int64(30) // 1024 bytes (1KB) chunks
	buffer := make([]byte, chunkSize)
	chunkNum := 0

	for {
		bytesRead, readErr := file.Read(buffer)
		if bytesRead > 0 {
			chunkData := buffer[:bytesRead]
			chunkID := generateChunkID(chunkData)

			// Select a data node for this chunk (round-robin or random selection)
			targetNode := activeNodes[chunkNum%len(activeNodes)]

			// Send chunk to selected data node
			_, uploadErr := uploadChunkToDataNode(targetNode, header.Filename, chunkID, chunkData)
			if uploadErr != nil {
				log.Printf("Failed to upload chunk to node %s: %v", targetNode, uploadErr)
				continue
			}

			fileMetadata.Chunks = append(fileMetadata.Chunks, FileChunk{
				ChunkID:   chunkID,
				NodeIP:    targetNode,
				ChunkSize: int64(bytesRead),
			})
			fileMetadata.TotalChunks++
			chunkNum++
		}

		if readErr == io.EOF {
			break
		}
		if readErr != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": "Error reading file"})
			return
		}
	}

	// Store file metadata
	lock.Lock()
	fileStorage[header.Filename] = fileMetadata
	lock.Unlock()

	c.JSON(http.StatusOK, gin.H{
		"status":   "File uploaded successfully",
		"filename": header.Filename,
		"chunks":   fileMetadata.TotalChunks,
	})
}

func uploadChunkToDataNode(nodeAddress, filename, chunkID string, chunkData []byte) (bool, error) {
	url := fmt.Sprintf("http://%s/upload-chunk", nodeAddress)

	body := &bytes.Buffer{}
	writer := multipart.NewWriter(body)

	writer.WriteField("filename", filename)
	writer.WriteField("chunkid", chunkID)

	part, err := writer.CreateFormFile("chunk", filename+"."+chunkID)
	if err != nil {
		return false, err
	}
	part.Write(chunkData)

	writer.Close()

	req, err := http.NewRequest("POST", url, body)
	if err != nil {
		return false, err
	}
	req.Header.Set("Content-Type", writer.FormDataContentType())

	client := &http.Client{}
	resp, err := client.Do(req)
	if err != nil {
		return false, err
	}
	defer resp.Body.Close()

	return resp.StatusCode == http.StatusOK, nil
}

func downloadFile(c *gin.Context) {
	filename := c.Param("filename")

	lock.RLock()
	metadata, exists := fileStorage[filename]
	lock.RUnlock()

	if !exists {
		c.JSON(http.StatusNotFound, gin.H{"error": "File not found"})
		return
	}

	// Retrieve chunks from respective data nodes
	chunks := make([][]byte, metadata.TotalChunks)
	for i, chunk := range metadata.Chunks {
		chunkData, err := downloadChunkFromDataNode(chunk.NodeIP, filename, chunk.ChunkID)
		if err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to retrieve chunk"})
			return
		}
		chunks[i] = chunkData
	}

	// Reassemble file
	fullFile := bytes.Join(chunks, nil)
	c.Data(http.StatusOK, "application/octet-stream", fullFile)
}

func downloadChunkFromDataNode(nodeAddress, filename, chunkID string) ([]byte, error) {
	url := fmt.Sprintf("http://%s/download-chunk?filename=%s&chunkid=%s",
		nodeAddress, filename, chunkID)

	resp, err := http.Get(url)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	return io.ReadAll(resp.Body)
}

func heartbeatCheck() {
	for {
		time.Sleep(2 * time.Second)
		lock.Lock()
		for ip, node := range dataNodes {
			if time.Since(node.LastSeen) > 5*time.Second {
				log.Printf("Data Node %s is inactive", ip)
				delete(dataNodes, ip)
				redistributeFiles(ip)
			}
		}
		lock.Unlock()
	}
}

func redistributeFiles(deadNode string) {
	for filename, metadata := range fileStorage {
		newChunks := []FileChunk{}
		for _, chunk := range metadata.Chunks {
			if chunk.NodeIP != deadNode {
				newChunks = append(newChunks, chunk)
			}
		}

		if len(newChunks) == 0 && len(dataNodes) > 0 {
			log.Printf("Need to re-replicate file: %s", filename)
		}

		metadata.Chunks = newChunks
		fileStorage[filename] = metadata
	}
}

func getActiveNodes(c *gin.Context) {
	lock.RLock()
	nodes := make([]string, 0, len(dataNodes))
	for ip := range dataNodes {
		nodes = append(nodes, ip)
	}
	lock.RUnlock()
	c.JSON(http.StatusOK, nodes)
}

func main() {
	router := gin.Default()
	// Configure CORS middleware
	router.Use(cors.New(cors.Config{
		AllowOrigins:     []string{"*"},
		AllowMethods:     []string{"GET", "POST", "PUT", "DELETE", "OPTIONS"},
		AllowHeaders:     []string{"Origin", "Content-Type"},
		ExposeHeaders:    []string{"Content-Length"},
		AllowCredentials: true,
		MaxAge:           12 * time.Hour,
	}))
	router.POST("/upload", uploadFile)
	router.GET("/download/:filename", downloadFile)
	router.GET("/register", registerDataNode)
	router.GET("/nodes", getActiveNodes)

	go heartbeatCheck()

	fmt.Println("Tracker Node running on port 5000...")
	router.Run("0.0.0.0:5000")
}
