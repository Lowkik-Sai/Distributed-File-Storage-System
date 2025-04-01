# Distributed File Storage System

This project implements a Distributed File Storage System inspired by Google File System (GFS). It consists of a **Tracker Node (Master)** managing metadata and **Data Nodes (Slaves)** storing file chunks. The system ensures scalability, fault tolerance, and replication.

## Features
- **File Chunking**: Files are split into smaller chunks for distributed storage.
- **Fault Tolerance**: Lost chunks are detected and reallocated.
- **Replication**: Ensures redundancy for data safety.
- **Scalability**: New data nodes can be registered dynamically.

## Prerequisites
Ensure you have the following installed:
- [Go](https://go.dev/doc/install) (1.18 or later)
- Git

## Installation & Setup
### 1. Clone the repository
```sh
git clone https://github.com/yourusername/distributed-file-storage.git
cd distributed-file-storage
```

### 2. Install dependencies
```sh
go mod tidy
```

### 3. Run the Tracker Node (Master)
```sh
go run tracker.go
```
The tracker node starts on `http://localhost:6000`.

### 4. Run a Data Node (Slave)
```sh
go run datanode.go --port=6001 -tracker=http://<IP_ADDR>/:6000
go run datanode.go --port=6002 -tracker=http://<IP_ADDR>/:6000
go run datanode.go --port=6003 -tracker=http://<IP_ADDR>/:6000
...
```
Replace `6000` with any available port.

## API Endpoints
### Tracker Node
| Method | Endpoint               | Description |
|--------|------------------------|-------------|
| `GET`  | `/register?port=6000`  | Registers a Data Node |
| `POST` | `/upload`              | Uploads a file (multipart form) |
| `GET`  | `/download/:filename`  | Downloads a file |
| `GET`  | `/nodes`               | Lists active Data Nodes |

### Data Node
| Method | Endpoint               | Description |
|--------|------------------------|-------------|
| `POST` | `/upload-chunk`        | Stores a file chunk |
| `GET`  | `/download-chunk`      | Retrieves a file chunk |

## Example Usage
### Upload a File
Use cURL or Postman to upload a file:
```sh
curl -X POST -F "file=@example.txt" http://localhost:6000/upload
```

### Download a File
```sh
curl -X GET http://localhost:6000/download/example.txt -o example.txt
```


