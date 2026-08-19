package portablemerge

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"reflect"
	"strings"

	"github.com/qinqingxu/synchub-for-agents/internal/portableconfig"
	"github.com/qinqingxu/synchub-for-agents/internal/processattr"
)

type Result struct {
	Data     []byte
	Conflict bool
}

type TextMerger interface {
	Merge(base, local, remote []byte) (Result, error)
}

func Structured(base, local, remote []byte) (Result, error) {
	baseDocument, err := parseObject(base)
	if err != nil {
		return Result{}, fmt.Errorf("parse structured base: %w", err)
	}

	localDocument, err := parseObject(local)
	if err != nil {
		return Result{}, fmt.Errorf("parse structured local: %w", err)
	}
	remoteDocument, err := parseObject(remote)
	if err != nil {
		return Result{}, fmt.Errorf("parse structured remote: %w", err)
	}
	merged, conflict := mergeMaps(baseDocument, localDocument, remoteDocument)
	if conflict {
		return Result{Conflict: true}, nil
	}
	var buffer bytes.Buffer
	encoder := json.NewEncoder(&buffer)
	encoder.SetIndent("", "  ")
	if err := encoder.Encode(merged); err != nil {
		return Result{}, fmt.Errorf("marshal structured merge: %w", err)
	}
	return Result{Data: buffer.Bytes()}, nil
}

func StructuredDocument(rel string, base, local, remote []byte) (Result, error) {
	format, baseDocument, err := portableconfig.Parse(rel, base)
	if err != nil {
		return Result{}, fmt.Errorf("parse structured base: %w", err)
	}
	localFormat, localDocument, err := portableconfig.Parse(rel, local)
	if err != nil {
		return Result{}, fmt.Errorf("parse structured local: %w", err)
	}
	remoteFormat, remoteDocument, err := portableconfig.Parse(rel, remote)
	if err != nil {
		return Result{}, fmt.Errorf("parse structured remote: %w", err)
	}
	if localFormat != format || remoteFormat != format {
		return Result{}, fmt.Errorf("structured merge formats do not match")
	}
	merged, conflict := mergeMaps(baseDocument, localDocument, remoteDocument)
	if conflict {
		return Result{Conflict: true}, nil
	}
	data, err := portableconfig.Marshal(format, merged)
	if err != nil {
		return Result{}, fmt.Errorf("marshal structured merge: %w", err)
	}
	return Result{Data: data}, nil
}

func Binary(base, local, remote []byte) Result {
	switch {
	case bytes.Equal(local, remote):
		return Result{Data: bytes.Clone(local)}
	case bytes.Equal(local, base):
		return Result{Data: bytes.Clone(remote)}
	case bytes.Equal(remote, base):
		return Result{Data: bytes.Clone(local)}
	default:
		return Result{Conflict: true}
	}
}

type GitTextMerger struct {
	GitPath    string
	TempParent string
}

func (m GitTextMerger) Merge(base, local, remote []byte) (Result, error) {
	tempDir, err := os.MkdirTemp(m.TempParent, "synchub-merge-*")
	if err != nil {
		return Result{}, fmt.Errorf("create text merge directory: %w", err)
	}
	defer os.RemoveAll(tempDir)

	basePath := filepath.Join(tempDir, "base")
	localPath := filepath.Join(tempDir, "local")
	remotePath := filepath.Join(tempDir, "remote")
	for filename, data := range map[string][]byte{
		basePath:   base,
		localPath:  local,
		remotePath: remote,
	} {
		if err := os.WriteFile(filename, data, 0o600); err != nil {
			return Result{}, fmt.Errorf("write text merge input: %w", err)
		}
	}

	gitPath := m.GitPath
	if gitPath == "" {
		gitPath = "git"
	}
	command := exec.Command(gitPath, "merge-file", "--stdout", localPath, basePath, remotePath)
	processattr.HideWindow(command)
	var stdout, stderr bytes.Buffer
	command.Stdout = &stdout
	command.Stderr = &stderr
	err = command.Run()
	if err == nil {
		return Result{Data: stdout.Bytes()}, nil
	}
	var exitError *exec.ExitError
	if errors.As(err, &exitError) && exitError.ExitCode() == 1 {
		return Result{Conflict: true}, nil
	}
	message := strings.TrimSpace(stderr.String())
	if message == "" {
		message = err.Error()
	}
	return Result{}, fmt.Errorf("git merge-file: %w: %s", err, message)
}

type node struct {
	value   any
	present bool
}

func parseObject(data []byte) (map[string]any, error) {
	decoder := json.NewDecoder(bytes.NewReader(data))
	decoder.UseNumber()
	var value any
	if err := decoder.Decode(&value); err != nil {
		return nil, err
	}
	var extra any
	if err := decoder.Decode(&extra); err == nil {
		return nil, fmt.Errorf("multiple JSON documents")
	} else if err != io.EOF {
		return nil, err
	}
	document, ok := value.(map[string]any)
	if !ok {
		return nil, fmt.Errorf("document root must be an object")
	}
	return document, nil
}

func mergeMaps(base, local, remote map[string]any) (map[string]any, bool) {
	keys := map[string]struct{}{}
	for key := range base {
		keys[key] = struct{}{}
	}
	for key := range local {
		keys[key] = struct{}{}
	}
	for key := range remote {
		keys[key] = struct{}{}
	}
	out := make(map[string]any, len(keys))
	for key := range keys {
		baseValue, inBase := base[key]
		localValue, inLocal := local[key]
		remoteValue, inRemote := remote[key]
		merged, conflict := mergeNode(
			node{value: baseValue, present: inBase},
			node{value: localValue, present: inLocal},
			node{value: remoteValue, present: inRemote},
		)
		if conflict {
			return nil, true
		}
		if merged.present {
			out[key] = merged.value
		}
	}
	return out, false
}

func mergeNode(base, local, remote node) (node, bool) {
	switch {
	case nodesEqual(local, remote):
		return cloneNode(local), false
	case nodesEqual(base, local):
		return cloneNode(remote), false
	case nodesEqual(base, remote):
		return cloneNode(local), false
	}

	localMap, localIsMap := local.value.(map[string]any)
	remoteMap, remoteIsMap := remote.value.(map[string]any)
	baseMap, baseIsMap := base.value.(map[string]any)
	if local.present && remote.present && localIsMap && remoteIsMap &&
		(!base.present || baseIsMap) {
		if !base.present {
			baseMap = map[string]any{}
		}
		merged, conflict := mergeMaps(baseMap, localMap, remoteMap)
		return node{value: merged, present: true}, conflict
	}
	return node{}, true
}

func nodesEqual(left, right node) bool {
	return left.present == right.present &&
		(!left.present || reflect.DeepEqual(left.value, right.value))
}

func cloneNode(source node) node {
	if !source.present {
		return node{}
	}
	return node{value: cloneValue(source.value), present: true}
}

func cloneValue(value any) any {
	switch item := value.(type) {
	case map[string]any:
		out := make(map[string]any, len(item))
		for key, child := range item {
			out[key] = cloneValue(child)
		}
		return out
	case []any:
		out := make([]any, len(item))
		for index, child := range item {
			out[index] = cloneValue(child)
		}
		return out
	default:
		return value
	}
}
