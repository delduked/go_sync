# Define the proto files
PROTO_FILES = ./proto/filesync.proto


# Define the Go plugin paths
PROTOC_GEN_GO=$(shell go env GOPATH)/bin/protoc-gen-go
PROTOC_GEN_GO_GRPC=$(shell go env GOPATH)/bin/protoc-gen-go-grpc

# Ensure the Go plugins are installed
install:
	go install google.golang.org/protobuf/cmd/protoc-gen-go@latest
	go install google.golang.org/grpc/cmd/protoc-gen-go-grpc@latest

# Define the generate_proto target
generate_proto:
	@echo "Generating proto files..."
	protoc --proto_path=. --go_out=. --go-grpc_out=. $(PROTO_FILES)

# Define a clean target to remove generated files
clean:
	@echo "Cleaning generated files..."
	rm -rf ./proto/*.pb.go

# Run the Go application
run: generate_proto
	go run main.go

# Default target
all: generate_proto
