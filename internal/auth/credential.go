package auth

import (
	"bufio"
	"crypto/subtle"
	"errors"
	"fmt"
	"io"
	"strings"
)

const maxCredentialLine = 16 * 1024

type CredentialHandler struct {
	Store   *Store
	Account Account
}

func (h CredentialHandler) Run(operation string, input io.Reader, output io.Writer) error {
	values, err := readCredentialInput(input)
	if err != nil {
		return err
	}
	if values["protocol"] != "https" ||
		(values["host"] != "github.com" && values["host"] != "github.com:443") {
		return errors.New("credential request is not for GitHub HTTPS")
	}
	if h.Store == nil || h.Account.ID == 0 || h.Account.Login == "" {
		return errors.New("GitHub account is not configured")
	}

	switch operation {
	case "get":
		token, err := h.Store.Token(h.Account.ID)
		if errors.Is(err, ErrNotFound) {
			_, writeErr := io.WriteString(output, "quit=1\n\n")
			return writeErr
		}
		if err != nil {
			return err
		}
		_, err = fmt.Fprintf(output, "username=%s\npassword=%s\n\n", h.Account.Login, token)
		return err
	case "store":
		return nil
	case "erase":
		supplied := values["password"]
		if supplied == "" {
			return nil
		}
		token, err := h.Store.Token(h.Account.ID)
		if errors.Is(err, ErrNotFound) {
			return nil
		}
		if err != nil {
			return err
		}
		if subtle.ConstantTimeCompare([]byte(supplied), []byte(token)) == 1 {
			return h.Store.Delete(h.Account.ID)
		}
		return nil
	default:
		return fmt.Errorf("unsupported Git credential operation %q", operation)
	}
}

func readCredentialInput(input io.Reader) (map[string]string, error) {
	scanner := bufio.NewScanner(input)
	scanner.Buffer(make([]byte, 1024), maxCredentialLine)
	values := make(map[string]string)
	for scanner.Scan() {
		line := scanner.Text()
		if line == "" {
			break
		}
		if strings.ContainsAny(line, "\r\x00") {
			return nil, errors.New("invalid control character in credential input")
		}
		key, value, ok := strings.Cut(line, "=")
		if !ok || key == "" {
			return nil, errors.New("invalid Git credential input")
		}
		if _, exists := values[key]; exists {
			return nil, fmt.Errorf("duplicate Git credential field %q", key)
		}
		values[key] = value
	}
	if err := scanner.Err(); err != nil {
		return nil, fmt.Errorf("read Git credential input: %w", err)
	}
	return values, nil
}
