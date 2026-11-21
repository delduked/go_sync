FROM golang:1.25 AS build
WORKDIR /src
COPY go.mod go.sum ./
RUN go mod download
COPY . .
RUN CGO_ENABLED=0 go build -trimpath -o /gosync ./cmd/server
FROM scratch
COPY --from=build /gosync /gosync
USER 65532:65532
EXPOSE 50051/tcp 5353/udp
ENTRYPOINT ["/gosync"]
