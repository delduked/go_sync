package servers

import (
	"context"
	"fmt"
	"strings"

	"github.com/dgraph-io/badger/v3"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/metadata"
	"google.golang.org/grpc/status"
)

const folderHeader = "x-gosync-folder"

func WithFolderID(id string) Option {
	return func(e *Engine) error {
		if len(id) > 128 || strings.IndexFunc(id, func(r rune) bool {
			return !(r >= 'a' && r <= 'z' || r >= 'A' && r <= 'Z' || r >= '0' && r <= '9' || r == '-' || r == '_')
		}) >= 0 {
			return fmt.Errorf("folder ID must contain at most 128 letters, digits, dashes or underscores")
		}
		e.folderID = id
		return nil
	}
}
func (e *Engine) loadFolderID() error {
	return e.db.Update(func(tx *badger.Txn) error {
		item, err := tx.Get([]byte("folder-id"))
		if err == nil {
			var stored string
			if err = item.Value(func(b []byte) error { stored = string(b); return nil }); err != nil {
				return err
			}
			if e.folderID != "" && e.folderID != stored {
				return fmt.Errorf("state belongs to folder %q, not %q", stored, e.folderID)
			}
			e.folderID = stored
			return nil
		}
		if err != badger.ErrKeyNotFound {
			return err
		}
		if e.folderID == "" {
			return nil
		} // Legacy unnamed state may be explicitly named later.
		return tx.Set([]byte("folder-id"), []byte(e.folderID))
	})
}
func (e *Engine) FolderID() string { return e.folderID }
func folderContext(ctx context.Context, id string) context.Context {
	return metadata.AppendToOutgoingContext(ctx, folderHeader, id)
}
func checkFolder(ctx context.Context, id string) error {
	meta, _ := metadata.FromIncomingContext(ctx)
	values := meta.Get(folderHeader)
	if len(values) == 0 && id == "" || len(values) == 1 && values[0] == id {
		return nil
	}
	return status.Error(codes.PermissionDenied, "shared folder identity does not match")
}
func FolderServerOptions(id string) []grpc.ServerOption {
	return []grpc.ServerOption{
		grpc.UnaryInterceptor(func(ctx context.Context, req any, info *grpc.UnaryServerInfo, next grpc.UnaryHandler) (any, error) {
			if err := checkFolder(ctx, id); err != nil {
				return nil, err
			}
			return next(ctx, req)
		}),
		grpc.StreamInterceptor(func(srv any, stream grpc.ServerStream, info *grpc.StreamServerInfo, next grpc.StreamHandler) error {
			if err := checkFolder(stream.Context(), id); err != nil {
				return err
			}
			return next(srv, stream)
		}),
	}
}
