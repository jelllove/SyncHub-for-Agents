package cli

import (
	"errors"
	"io"
	"os"
	"path/filepath"

	"github.com/qinqingxu/acsync/internal/auth"
)

func AuthMetadataPath(home string) string {
	return filepath.Join(home, "auth.json")
}

func RunCredential(operation string, input io.Reader, output io.Writer) error {
	home, err := Home()
	if err != nil {
		return err
	}
	return RunCredentialWith(home, operation, input, output, auth.NewSystemStore())
}

func RunCredentialWith(
	home string,
	operation string,
	input io.Reader,
	output io.Writer,
	store *auth.Store,
) error {
	metadata, err := auth.LoadMetadata(AuthMetadataPath(home))
	if err != nil {
		if operation == "get" && (os.IsNotExist(err) || errors.Is(err, auth.ErrNotFound)) {
			_, writeErr := io.WriteString(output, "quit=1\n\n")
			return writeErr
		}
		return err
	}
	handler := auth.CredentialHandler{
		Store:   store,
		Account: metadata.Active,
	}
	return handler.Run(operation, input, output)
}
