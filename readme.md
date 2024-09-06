# Go File Synchronization Service

This project implements a **real-time file synchronization system** using Go, `gRPC`, and `fsnotify`. The primary purpose is to monitor a local folder for changes (new files, modifications, or deletions) and synchronize those changes with connected peers in real-time over the network using `gRPC`.

## Features

- **Real-Time File Synchronization**: As files are created, modified, or deleted in the local folder, they are immediately synced with other peers on the network.
- **Peer Discovery**: Peers are automatically discovered on the same network using `zeroconf` (multicast DNS), removing the need for manual configuration.
- **Concurrent Transfer**: The file transfer starts as soon as a peer detects a file change, and it can handle multiple transfers concurrently.

## Key Behavior

- **File Transfer During Reception**: While the original peer is receiving a file from another system, it can also start sending the file to its connected peers before the file is fully downloaded. This ensures efficient file distribution across the network.
- **Real-Time Monitoring**: Using `fsnotify`, the system watches the designated folder and triggers file sync or deletion operations whenever changes are detected.
  
## How It Works

1. **gRPC Server**: Each peer runs a gRPC server, which listens for file sync requests.
2. **File Watching**: The system monitors a local folder for changes (file creations, modifications, and deletions).
3. **File Transfer**: When a new file is detected, the system starts streaming the file in chunks to other connected peers. As chunks are received by a peer, the peer can start forwarding the file to its own peers, allowing simultaneous transfer.
4. **Peer Discovery**: Peers on the network are discovered automatically using `zeroconf`. Once discovered, they are connected through a persistent `gRPC` connection, enabling efficient file sharing.
5. **File Deletion**: If a file is deleted locally, it is also deleted on all connected peers.

## Prerequisites

Before running this project, make sure you have the following installed:

- **Go**: [Download Go](https://golang.org/dl/)
- **gRPC**: gRPC is automatically included as a dependency in Go modules.
- **fsnotify**: A Go package to monitor file system changes (included as a dependency).
- **zeroconf**: Used for peer discovery (included as a dependency).

## Setup

1. **Clone the repository**:

   ```bash
   git clone https://github.com/yourusername/go_sync
   cd go_sync
   ```

2. **Install dependencies**:

   Make sure all necessary Go modules are installed:

   ```bash
   go mod tidy
   ```

3. **Run the service**:

   To start the synchronization service, run the following command with your folder path:

   ```bash
   go run main.go -local="/path/to/your/folder" -port=50051
   ```

   - Replace `/path/to/your/folder` with the folder you want to monitor.
   - You can change the port (default is 50051) if needed.

## Usage

1. **Start the service**: Run the above command on each machine that should participate in file synchronization.
2. **File Sync**: Any file added or modified in the specified folder will be synchronized with the connected peers in real-time.
3. **File Deletion Sync**: If a file is deleted, the deletion will be propagated to all peers, removing the file from their folders as well.

### Example

```bash
go run main.go -local="/home/user/myfolder" -port=50051
```

In this example:
- The program monitors `/home/user/myfolder` for changes.
- It listens on port 50051 for incoming sync requests.

## How to Stop the Service

To stop the service gracefully, press `Ctrl+C` in the terminal where the service is running. This will close all active connections and shut down the service properly.

## Technical Details

### File Transfer During Reception

- **Original Peer**: When the original peer (the system where the file was first added or modified) starts receiving a file, it immediately writes it in chunks to the local folder. As each chunk is received, it can simultaneously forward those chunks to any connected peers. This behavior ensures that file synchronization can happen quickly, even while the file is still being downloaded by the original peer.
- **Peers**: Once a peer starts receiving a file from the original peer, it saves the file in its local folder. Like the original peer, it can start forwarding the file to its own peers while still receiving chunks of the file.

### Peer Discovery

- Peers are discovered automatically using `zeroconf`. Once a peer is found, a persistent connection is established using `gRPC`. This allows for seamless file synchronization between multiple systems on the same network without the need for manual configuration.

### File Watching and Syncing

- The program uses the `fsnotify` library to monitor the specified folder. When a file is created or modified, the system begins streaming the file in real-time to other connected peers. If a file is deleted or moved, the system notifies the peers to remove the corresponding file from their folders.

## Future Improvements

- Implement **Cascading Synchronization**: In the current version, each peer syncs directly with its connected peers. A future update will allow a more sophisticated cascading sync, where each peer syncs with a few peers, and those peers further propagate the file to others, achieving more efficient distribution.