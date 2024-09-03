# File Sync Service

## Overview

This Go application provides a real-time file synchronization service between two remote systems using gRPC. It listens to a specified local folder and syncs any newly created or modified files to a corresponding folder on a remote system.

### Key Features:
- **Real-Time File Sync:** Watches a local folder for changes and immediately syncs files to a remote folder.
- **gRPC Communication:** Utilizes gRPC to handle file transfer between the two systems.
- **Concurrent Transfers:** Manages multiple file transfers simultaneously without interference.

## Setup Instructions

### Prerequisites
- **Go**: Ensure you have Go installed on your system. You can download it from [golang.org](https://golang.org/dl/).
- **gRPC and Protocol Buffers**: Install the necessary gRPC tools and dependencies.

### Installation
1. Clone the repository:
   ```sh
   git clone https://github.com/yourusername/go_sync.git
   cd go_sync
   ```

2. Install the required Go packages:
   ```sh
   go get google.golang.org/grpc
   go get github.com/fsnotify/fsnotify
   ```

3. Compile the application:
   ```sh
   go build -o filesync
   ```

### Usage

#### 1. Run the gRPC Server
On the first system (Server A), start the gRPC server. This server will listen for file sync requests and update its local folder with incoming files.

```sh
./filesync -local /path/to/local/folder -port 50051
```

- `-local`: Path to the folder on Server A that you want to sync.
- `-port`: Port number on which the gRPC server will run (default is 50051).

#### 2. Run the File Watcher and Client
On the second system (Client B), start the file watcher. This client will monitor the local folder for changes and sync any new or modified files to Server A.

```sh
./filesync -local /path/to/local/folder -remoteAddr serverA_IP:50051
```

- `-local`: Path to the folder on Client B that you want to monitor.
- `-remoteAddr`: The IP address and port of Server A's gRPC server.

### Example Workflow

1. **Start the Server on Server A:**
   ```sh
   ./filesync -local /home/user/server_folder -port 50051
   ```
   This command starts a gRPC server that listens on port 50051 and syncs files to the `/home/user/server_folder` directory.

2. **Start the Watcher on Client B:**
   ```sh
   ./filesync -local /home/user/client_folder -remoteAddr 192.168.1.100:50051
   ```
   This command starts the file watcher on Client B, monitoring the `/home/user/client_folder` directory. It will sync any new or modified files to Server A's folder at `192.168.1.100:50051`.

3. **File Syncing:**
   - When you create or modify a file in the `/home/user/client_folder` on Client B, the file will be immediately synced to `/home/user/server_folder` on Server A.
   - The server logs connections and transfers, providing feedback on successful and unsuccessful file syncs.

## Detailed Explanation of the Code

### 1. **Main Function**
The main function initializes the server and the file watcher based on the command-line arguments provided.

### 2. **Server (gRPC)**
The `startServer` function initializes a gRPC server that listens for incoming file streams. It uses the `SyncFile` method to handle file chunks received from the client.

- **Connection Logging:** Logs when a client connects or disconnects.
- **File Writing:** Writes received file chunks to the specified local folder.

### 3. **Client**
The `startClient` function initiates a connection to the server and begins streaming a file in real-time using gRPC.

- **File Streaming:** Reads the file in chunks and sends it to the server.
- **Acknowledgment:** Receives acknowledgment from the server for each chunk sent.

### 4. **Folder Watcher**
The `watchFolderForRealTimeSync` function monitors the specified local folder for any file changes using `fsnotify`. When a new or modified file is detected, it starts the client to sync that file to the server.

### 5. **Concurrency and Safety**
- **Mutex Locking:** Ensures that the file transfer map is accessed safely across multiple goroutines, preventing race conditions.
- **Active Transfers Map:** Keeps track of currently active file transfers to avoid duplicate sync attempts.

## Conclusion

This file sync service provides a robust, real-time solution for synchronizing files between two remote systems. It is ideal for environments where maintaining identical directories across different systems is crucial. By leveraging gRPC for communication, it ensures efficient and reliable file transfers with minimal latency.