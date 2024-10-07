# GoSync: A Peer-to-Peer File Synchronization Application

GoSync is a peer-to-peer file synchronization application written in Go. It allows multiple nodes on the same local network to automatically synchronize files in a specified folder. GoSync is designed to detect changes in files and propagate those changes to other peers in real-time, ensuring that all nodes have the most up-to-date files.

## Table of Contents

- [Introduction](#introduction)
- [Features](#features)
- [Getting Started](#getting-started)
  - [Prerequisites](#prerequisites)
  - [Installation](#installation)
  - [Running the Application](#running-the-application)
- [Application Architecture](#application-architecture)
  - [Overview](#overview)
  - [Services](#services)
    - [mDNS Service](#mdns-service)
    - [Connection Service](#connection-service)
    - [File Service](#file-service)
    - [Metadata Service](#metadata-service)
    - [gRPC Service](#grpc-service)
  - [Communication Flow](#communication-flow)
- [Detailed Components Explanation](#detailed-components-explanation)
  - [mDNS Service](#mdns-service-detailed)
  - [Connection Service](#connection-service-detailed)
  - [File Service](#file-service-detailed)
  - [Metadata Service](#metadata-service-detailed)
  - [gRPC Methods](#grpc-methods-detailed)
- [Configuration](#configuration)
- [Contributing](#contributing)
- [License](#license)

---

## Introduction

GoSync is designed to keep a specified folder in sync across multiple machines on the same local network. It detects file changes using filesystem notifications and synchronizes those changes to other connected peers using gRPC streaming. The application uses mDNS (Multicast DNS) for peer discovery, allowing nodes to find each other without manual configuration.

## Features

- **Automatic Peer Discovery**: Uses mDNS to discover other GoSync instances on the local network.
- **Real-Time File Synchronization**: Monitors file changes and synchronizes them across peers in real-time.
- **Efficient Data Transfer**: Uses chunking and hashing to minimize data transfer by only sending changed portions of files.
- **Robust Connection Management**: Maintains stable connections with peers and handles reconnections seamlessly.
- **Cross-Platform Compatibility**: Written in Go, making it easy to run on various operating systems.

## Getting Started

### Prerequisites

- **Go Programming Language**: Go version 1.16 or higher.
- **Git**: For cloning the repository.
- **Protobuf Compiler**: For generating gRPC code (only if modifying `.proto` files).

### Installation

1. **Clone the Repository**

   ```bash
   git clone https://github.com/TypeTerrors/go_sync.git
   cd go_sync
   ```

2. **Install Dependencies**

   Ensure all Go dependencies are installed:

   ```bash
   go mod tidy
   ```

3. **Compile Protobuf Files (If Necessary)**

   If you modify the `.proto` files, you need to regenerate the Go code:

   ```bash
   protoc --go_out=./proto --go-grpc_out=./proto proto/filesync.proto
   ```

### Running the Application

1. **Build the Application**

   ```bash
   go build -o gosync ./cmd/server/main.go
   ```

2. **Run the Application**

   ```bash
   ./gosync --sync-folder=<path_to_sync_folder> [options]
   ```

   **Example:**

   ```bash
   ./gosync --sync-folder=./sync_folder --chunk-size=64 --sync-interval=1m --port=50051
   ```

   **Available Flags:**

   - `--sync-folder`: **(Required)** Path to the folder to synchronize.
   - `--chunk-size`: **(Optional)** Chunk size in kilobytes (default: 64).
   - `--sync-interval`: **(Optional)** Synchronization interval (default: 1 minute).
   - `--port`: **(Optional)** Port number for the gRPC server (default: 50051).

## Application Architecture

### Overview

GoSync is structured into several components, each responsible for a specific aspect of the application's functionality:

- **mDNS Service**: Handles peer discovery using Multicast DNS.
- **Connection Service**: Manages connections to peers and message dispatching.
- **File Service**: Monitors filesystem changes and handles file events.
- **Metadata Service**: Manages file metadata, including chunk hashes and synchronization status.
- **gRPC Service**: Provides the RPC interface for communication between peers.

### Services

#### mDNS Service

- Discovers other GoSync instances on the local network.
- Registers the local instance as a service that others can discover.
- Maintains a list of peers and notifies the Connection Service of new peers.

#### Connection Service

- Manages gRPC connections to peers.
- Dispatches messages to peers based on the message type.
- Maintains channels for different types of messages (file sync, health check, metadata exchange, etc.).

#### File Service

- Watches the synchronization folder for file changes using filesystem notifications.
- Handles file creation, modification, and deletion events.
- Initiates synchronization processes when files change.

#### Metadata Service

- Calculates and stores metadata for files and their chunks.
- Uses strong and weak hashes to detect changes in file chunks.
- Determines which chunks need to be synchronized with peers.

#### gRPC Service

- Implements the gRPC server interface.
- Handles incoming RPC calls from peers.
- Manages streaming RPCs for efficient data transfer.

### Communication Flow

1. **Peer Discovery**: Instances discover each other using mDNS.
2. **Connection Establishment**: A gRPC connection is established between peers.
3. **File Monitoring**: The File Service detects changes in the sync folder.
4. **Metadata Calculation**: The Metadata Service calculates hashes for changed files/chunks.
5. **Message Dispatching**: The Connection Service sends messages to peers about file changes.
6. **Data Synchronization**: Peers exchange file data using gRPC streaming methods.
7. **Metadata Exchange**: Peers compare metadata to determine which chunks need synchronization.

---

## Detailed Components Explanation

### mDNS Service Detailed

#### Purpose

- Discovers other GoSync instances on the local network without manual configuration.
- Registers the local GoSync instance as a discoverable service.

#### How It Works

- **Service Registration**: The local instance registers itself using `zeroconf`, advertising its IP and port.
- **Service Discovery**: Continuously browses for services matching the GoSync service type (`_myapp_filesync._tcp`).
- **Peer Filtering**:
  - Skips its own service instance.
  - Checks if discovered services are on the same subnet.
  - Validates the service using TXT records.
- **Peer Management**: Notifies the Connection Service to add or remove peers based on discovery results.

#### Key Functions

- `Start()`: Starts the mDNS service discovery.
- `Ping()`: Periodically sends health check messages to peers.
- `LocalIp()`: Returns the local IP address.
- `SetConn()`: Sets the connection interface for communication.

### Connection Service Detailed

#### Purpose

- Manages gRPC connections and communication with peers.
- Dispatches messages to peers and handles incoming messages.

#### How It Works

- **Peer Management**:
  - Maintains a map of peers (`peers`).
  - Adds or removes peers based on mDNS discoveries.
- **Connection Handling**:
  - Establishes gRPC connections with peers.
  - Monitors connection states and handles reconnections.
- **Message Dispatching**:
  - Uses channels to send different types of messages (file sync, health checks, etc.).
  - Implements message senders and receivers for each gRPC stream.
- **Synchronization Logic**:
  - Receives file lists and determines missing files.
  - Handles file chunk transfers and metadata synchronization.

#### Key Structures

- `Conn`: Main structure managing peers and message dispatching.
- `Peer`: Represents a peer connection, including channels and streams.

#### Key Functions

- `AddPeer()`: Adds a new peer and starts managing it.
- `RemovePeer()`: Removes a peer and cleans up resources.
- `Start()`: Starts the dispatch of messages to peers.
- `SendMessage()`: Enqueues a message to be sent to all peers.
- `managePeer()`: Handles the connection and communication with a single peer.
- `dispatchMessages()`: Distributes messages from the central send channel to all peers.

### File Service Detailed

#### Purpose

- Monitors the synchronization folder for file changes.
- Handles file creation, modification, and deletion events.
- Initiates synchronization processes when files change.

#### How It Works

- **Filesystem Monitoring**:
  - Uses `fsnotify` to watch for file system events.
  - Detects file creation, modification, and deletion.
- **Event Handling**:
  - Debounces events to prevent rapid, repeated processing.
  - Handles debounced events by updating metadata and notifying peers.
- **File Synchronization**:
  - Marks files as "in progress" during synchronization to prevent conflicts.
  - Updates metadata for new or modified files.
  - Notifies the Connection Service to send updates to peers.

#### Key Functions

- `Start()`: Starts watching the directory for changes.
- `Scan()`: Periodically checks if each peer has the same list of files.
- `HandleFileCreation()`, `HandleFileModification()`, `HandleFileDeletion()`: Handles respective file events.
- `markFileAsInProgress()`, `markFileAsComplete()`: Manages file synchronization state.
- `BuildLocalFileList()`: Builds a list of local files for comparison with peers.
- `CompareFileLists()`: Compares the local file list with that of a peer to identify missing files.

### Metadata Service Detailed

#### Purpose

- Manages file metadata, including chunk hashes.
- Determines which chunks need to be synchronized.
- Stores metadata in memory and persists it using BadgerDB.

#### How It Works

- **Metadata Calculation**:
  - Splits files into chunks based on the configured chunk size.
  - Calculates strong and weak hashes for each chunk.
  - Uses a rolling checksum for efficient weak hash calculation.
- **Change Detection**:
  - Compares current chunk hashes with previous ones to detect changes.
  - Identifies new, modified, or deleted chunks.
- **Metadata Storage**:
  - Stores metadata in an in-memory map for quick access.
  - Persists metadata to BadgerDB for durability.
- **Synchronization Assistance**:
  - Provides methods to check if all chunks of a file have been received.
  - Helps the File Service and Connection Service in determining synchronization actions.

#### Key Structures

- `Meta`: Main structure holding file metadata.
- `FileMetaData`: Holds metadata for a specific file.
- `Hash`: Represents the strong and weak hashes of a chunk.

#### Key Functions

- `CreateFileMetaData()`: Creates or updates metadata for a file.
- `SaveMetaData()`: Saves metadata to both memory and database.
- `GetMetaData()`: Retrieves metadata for a specific chunk.
- `DeleteMetaData()`: Deletes metadata for a chunk.
- `hashChunk()`: Calculates the strong hash for a chunk.

### gRPC Methods Detailed

#### Purpose

- Provides the RPC interface for peer-to-peer communication.
- Implements methods for file synchronization, health checks, metadata exchange, and more.

#### Key Methods

1. **SyncFile** (`rpc SyncFile(stream FileSyncRequest) returns (stream FileSyncResponse);`)

   - **Purpose**: Handles file synchronization requests and responses.
   - **Usage**: Transfers file chunks, deletion notices, and truncation commands between peers.
   - **Messages**:
     - `FileChunk`: Contains file chunk data, offset, and metadata.
     - `FileDelete`: Instructs a peer to delete a file or specific chunk.
     - `FileTruncate`: Instructs a peer to truncate a file to a specific size.

2. **HealthCheck** (`rpc HealthCheck(stream Ping) returns (stream Pong);`)

   - **Purpose**: Performs health checks between peers to ensure connectivity.
   - **Usage**: Exchanges ping and pong messages to monitor peer availability.

3. **ExchangeMetadata** (`rpc ExchangeMetadata(stream MetadataRequest) returns (stream MetadataResponse);`)

   - **Purpose**: Exchanges file metadata between peers.
   - **Usage**: Allows peers to compare metadata to determine which chunks need synchronization.

4. **RequestChunks** (`rpc RequestChunks(stream ChunkRequest) returns (stream ChunkResponse);`)

   - **Purpose**: Requests specific chunks from a peer.
   - **Usage**: Used when a peer needs to synchronize specific chunks of a file.

5. **GetMissingFiles** (`rpc GetMissingFiles(stream FileList) returns (stream FileChunk);`)

   - **Purpose**: Synchronizes missing files between peers.
   - **Usage**: Peers send their file lists to each other, and missing files are transferred accordingly.

#### Message Structures

- **FileSyncRequest**: Oneof message containing `FileChunk`, `FileDelete`, or `FileTruncate`.
- **FileChunk**: Contains file name, chunk data, offset, total chunks, and total size.
- **FileDelete**: Contains file name and offset (if deleting a specific chunk).
- **MetadataRequest/Response**: Contains file name and a list of chunk metadata.
- **ChunkRequest/Response**: Contains file name, offsets, and chunk data.
- **FileList**: Contains a list of `FileEntry` structures, each representing a file.

---

## Configuration

The application uses a `Config` struct defined in `./conf/conf.go` to hold runtime configurations:

```go
type Config struct {
    SyncFolder   string
    ChunkSize    int64
    SyncInterval time.Duration
    Port         string
}

var AppConfig Config
```

**Configuration Parameters:**

- **SyncFolder**: The folder to synchronize across peers.
- **ChunkSize**: The size of file chunks in bytes.
- **SyncInterval**: The interval at which the application checks for synchronization.
- **Port**: The port on which the gRPC server listens.

**Setting Configurations:**

Configurations are set using command-line flags when starting the application. For example:

```bash
./gosync --sync-folder=./sync_folder --chunk-size=64 --sync-interval=1m --port=50051
```

---

## Contributing

Contributions are welcome! If you'd like to contribute to GoSync, please follow these steps:

1. **Fork the Repository**: Create a fork of the repository on GitHub.
2. **Create a Feature Branch**: Work on your feature or bugfix in a separate branch.
3. **Commit Your Changes**: Make clear and concise commit messages.
4. **Create a Pull Request**: Submit your pull request for review.

---

## License

This project is licensed under the MIT License - see the [LICENSE](LICENSE) file for details.

---

**Note**: This application is intended for use on local networks and may not be secure for use over the internet. Always ensure you understand the security implications before deploying applications that synchronize files across networks.

# Acknowledgments

- **Go Programming Language**: For providing an excellent platform for building network applications.
- **gRPC**: For facilitating efficient communication between peers.
- **fsnotify**: For providing filesystem notifications.
- **zeroconf**: For enabling mDNS service discovery.

---

# Additional Details

## How Each Service Works

### mDNS Service

- **Discovery**: Uses Multicast DNS to broadcast and discover services.
- **Registration**: Registers the local GoSync instance with a unique service name that includes the local IP.
- **Filtering**: Ensures that only GoSync services are discovered by checking TXT records.
- **Peer Updates**: Notifies the Connection Service to add or remove peers based on discovery events.

### Connection Service

- **Peer Management**: Adds peers discovered by the mDNS Service and manages their connections.
- **Message Channels**: Uses separate channels for different message types to organize communication.
- **gRPC Streams**: Establishes persistent gRPC streams for continuous communication.
- **Error Handling**: Monitors connection states and handles reconnections when necessary.

### File Service

- **Event Debouncing**: Debounces file system events to avoid processing rapid, successive events.
- **Synchronization States**: Marks files as "in progress" or "complete" to manage synchronization flow.
- **Open File Management**: Keeps track of open file handles to manage resources efficiently.
- **Conflict Handling**: Checks if a file is already being synchronized to prevent conflicts.

### Metadata Service

- **Chunk Hashing**: Uses XXH3 hashing for strong hashes and a rolling checksum for weak hashes.
- **Metadata Comparison**: Compares previous and current metadata to detect changes.
- **Data Structures**: Maintains in-memory maps and persists data using BadgerDB.
- **Chunk Management**: Identifies new, modified, or deleted chunks for synchronization.

### gRPC Methods

- **Streaming RPCs**: Utilizes streaming RPCs for efficient and continuous data transfer.
- **Message Handling**: Processes different types of messages based on the request type.
- **Error Handling**: Handles errors gracefully and attempts reconnections when necessary.
- **Data Transfer**: Manages the sending and receiving of file chunks and metadata.

## How Synchronization Works

1. **Initial Scan**: On startup, GoSync scans the synchronization folder and builds metadata for existing files.
2. **Peer Discovery**: The mDNS Service discovers peers and establishes connections.
3. **File Monitoring**: The File Service watches for file events and updates metadata accordingly.
4. **Metadata Exchange**: Peers exchange metadata to determine differences in files and chunks.
5. **Chunk Comparison**: The Metadata Service identifies which chunks need to be sent or requested.
6. **Data Transfer**: Chunks are sent between peers using the appropriate gRPC methods.
7. **File Assembly**: Received chunks are written to files, and metadata is updated.
8. **Completion**: Once all chunks are received, files are marked as complete.

## Running Multiple Instances

To synchronize files between multiple machines:

1. **Ensure All Machines Are on the Same Network**: GoSync uses mDNS for discovery, which works on local networks.
2. **Start GoSync on Each Machine**: Use the same `--sync-folder` path on each machine (the folder must exist).
3. **Wait for Discovery**: The mDNS Service will discover peers automatically.
4. **Monitor Logs**: Check the logs to ensure peers are discovered and files are synchronizing.

## Logs and Debugging

- GoSync uses `github.com/charmbracelet/log` for logging.
- Logs include information about:
  - Peer discovery and connection states.
  - File events and synchronization status.
  - Metadata calculations and comparisons.
- To adjust log levels or formats, modify the logging configuration in the code.
