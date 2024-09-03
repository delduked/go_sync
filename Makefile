# Define the paths
PROTO_DIR=./filesync
PROTO_FILE=$(PROTO_DIR)/filesync.proto
OUT_DIR=$(PROTO_DIR)

# Define the protoc command
PROTOC=protoc

# Define the Go plugin paths
PROTOC_GEN_GO=$(shell go env GOPATH)/bin/protoc-gen-go
PROTOC_GEN_GO_GRPC=$(shell go env GOPATH)/bin/protoc-gen-go-grpc

# Ensure the Go plugins are installed
install:
	go install google.golang.org/protobuf/cmd/protoc-gen-go@latest
	go install google.golang.org/grpc/cmd/protoc-gen-go-grpc@latest

# Generate the Go code from the proto file
generate_proto: $(PROTO_FILE) install
	$(PROTOC) --go_out=$(OUT_DIR) --go-grpc_out=$(OUT_DIR) $(PROTO_FILE)

# Clean generated files
clean:
	rm -f $(OUT_DIR)/*.pb.go

# Run the Go application
run: generate_proto
	go run main.go

# Default target
all: generate_proto

test:
	rm ./A/main
	rm ./B/main
	go build main.go
	chmod +x main
	cp main ./A/
	cp main ./B/
