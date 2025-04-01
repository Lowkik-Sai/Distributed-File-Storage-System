package main

import (
	"flag"
	"fmt"
	"github.com/gin-gonic/gin"
	"log"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"sync"
	"time"
)

var (
	trackerIP    string
	dataNodeIP   = GetLocalIP()
	dataNodePort string
	storagePath  = "storage/"
	lock         sync.Mutex
)

func sendHeartbeat() {
	for {
		time.Sleep(1 * time.Second)
		_, err := http.Get(trackerIP + "/register?port=" + dataNodePort)
		if err != nil {
			log.Println("Error sending heartbeat:", err)
		}
	}
}

func uploadChunk(c *gin.Context) {
	filename := c.PostForm("filename")
	chunkID := c.PostForm("chunkid")

	file, err := c.FormFile("chunk")
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Failed to get chunk"})
		return
	}

	// Ensure storage directory exists
	chunkStoragePath := filepath.Join(storagePath, filename)
	os.MkdirAll(chunkStoragePath, os.ModePerm)

	// Save chunk with unique chunk ID
	chunkFilename := filepath.Join(chunkStoragePath, chunkID)
	if err := c.SaveUploadedFile(file, chunkFilename); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to save chunk"})
		return
	}

	c.JSON(http.StatusOK, gin.H{"status": "Chunk uploaded successfully"})
}

func downloadChunk(c *gin.Context) {
	filename := c.Query("filename")
	chunkID := c.Query("chunkid")

	chunkPath := filepath.Join(storagePath, filename, chunkID)
	if _, err := os.Stat(chunkPath); os.IsNotExist(err) {
		c.JSON(http.StatusNotFound, gin.H{"error": "Chunk not found"})
		return
	}

	c.File(chunkPath)
}

func listChunks(c *gin.Context) {
	files, err := os.ReadDir(storagePath)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Unable to list files"})
		return
	}

	fileList := []string{}
	for _, file := range files {
		if file.IsDir() {
			fileList = append(fileList, file.Name())
		}
	}
	c.JSON(http.StatusOK, fileList)
}

func GetLocalIP() string {
	addrs, err := net.InterfaceAddrs()
	if err != nil {
		return "127.0.0.1"
	}
	for _, address := range addrs {
		if ipNet, ok := address.(*net.IPNet); ok && !ipNet.IP.IsLoopback() {
			if ipNet.IP.To4() != nil {
				return ipNet.IP.String()
			}
		}
	}
	return "127.0.0.1"
}

func main() {
	port := flag.String("port", "6000", "Port number for the Data Node")
	tracker := flag.String("tracker", "http://192.168.1.100:5000", "Tracker node address")

	flag.Parse()

	dataNodePort = *port
	trackerIP = *tracker

	// Ensure storage directory exists
	if _, err := os.Stat(storagePath); os.IsNotExist(err) {
		os.Mkdir(storagePath, os.ModePerm)
	}

	router := gin.Default()
	router.POST("/upload-chunk", uploadChunk)
	router.GET("/download-chunk", downloadChunk)
	router.GET("/list", listChunks)

	go sendHeartbeat()

	fmt.Println("Data Node running on", dataNodeIP+":"+dataNodePort)
	router.Run(":" + dataNodePort)
}
