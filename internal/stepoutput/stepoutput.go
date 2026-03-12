package stepoutput

import (
	"bufio"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"

	"github.com/BurntSushi/toml"
	"gopkg.in/yaml.v3"
)

type OutputRef struct {
	Step string
	Key  string
}

type Patch map[string]*string

func ParseRef(input string) (OutputRef, error) {
	input = strings.TrimSpace(input)
	switch {
	case input == "":
		return OutputRef{}, errors.New("empty output reference")
	case strings.HasPrefix(input, "steps.") && strings.Contains(input, ".outputs."):
		parts := strings.SplitN(strings.TrimPrefix(input, "steps."), ".outputs.", 2)
		if len(parts) != 2 || parts[0] == "" || parts[1] == "" {
			return OutputRef{}, fmt.Errorf("invalid from_output reference: %q", input)
		}
		return OutputRef{Step: parts[0], Key: parts[1]}, ValidateKey(parts[1], true)
	case strings.Contains(input, "step=") || strings.Contains(input, "key="):
		var ref OutputRef
		for _, part := range strings.Split(input, ",") {
			part = strings.TrimSpace(part)
			switch {
			case strings.HasPrefix(part, "step="):
				ref.Step = strings.TrimSpace(strings.TrimPrefix(part, "step="))
			case strings.HasPrefix(part, "key="):
				ref.Key = strings.TrimSpace(strings.TrimPrefix(part, "key="))
			default:
				return OutputRef{}, fmt.Errorf("invalid from_output reference: %q", input)
			}
		}
		if ref.Step == "" || ref.Key == "" {
			return OutputRef{}, fmt.Errorf("invalid from_output reference: %q", input)
		}
		return ref, ValidateKey(ref.Key, true)
	case strings.Contains(input, "/"):
		parts := strings.SplitN(input, "/", 2)
		if len(parts) != 2 || parts[0] == "" || parts[1] == "" {
			return OutputRef{}, fmt.Errorf("invalid from_output reference: %q", input)
		}
		return OutputRef{Step: parts[0], Key: parts[1]}, ValidateKey(parts[1], true)
	default:
		parts := strings.SplitN(input, ".", 2)
		if len(parts) != 2 || parts[0] == "" || parts[1] == "" {
			return OutputRef{}, fmt.Errorf("invalid from_output reference: %q", input)
		}
		return OutputRef{Step: parts[0], Key: parts[1]}, ValidateKey(parts[1], true)
	}
}

func ValidateKey(key string, allowNegativeFinal bool) error {
	if strings.TrimSpace(key) == "" {
		return errors.New("empty output key")
	}
	if strings.HasPrefix(key, ".") || strings.HasSuffix(key, ".") || strings.Contains(key, "..") {
		return fmt.Errorf("invalid output key: %q", key)
	}
	parts := strings.Split(key, ".")
	for i, part := range parts {
		if part == "" {
			return fmt.Errorf("invalid output key: %q", key)
		}
		if allowNegativeFinal && i == len(parts)-1 && isNegativeIndex(part) {
			continue
		}
		for _, r := range part {
			if !(r == '_' || r == '-' || (r >= 'a' && r <= 'z') || (r >= 'A' && r <= 'Z') || (r >= '0' && r <= '9')) {
				return fmt.Errorf("invalid output key: %q", key)
			}
		}
	}
	return nil
}

func ParseSetValue(raw, format string) (any, bool, error) {
	if raw == "" {
		return "", false, nil
	}
	if format == "" {
		trimmed := strings.TrimSpace(raw)
		if strings.HasPrefix(trimmed, "{") || strings.HasPrefix(trimmed, "[") {
			var decoded any
			if err := json.Unmarshal([]byte(trimmed), &decoded); err != nil {
				return nil, false, err
			}
			return decoded, true, nil
		}
		return raw, false, nil
	}
	if isTrivialValue(raw) {
		return raw, false, nil
	}
	return parseBytes([]byte(raw), format)
}

func ParsePutFile(path, format string) (any, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	if format == "" {
		switch strings.ToLower(filepath.Ext(path)) {
		case ".env":
			format = "env"
		case ".json":
			format = "json"
		case ".yaml", ".yml":
			format = "yaml"
		case ".toml":
			format = "toml"
		default:
			return nil, fmt.Errorf("cannot infer output format from path: %s", path)
		}
	}
	value, _, err := parseBytes(data, format)
	return value, err
}

func FlattenUnder(prefix string, value any) (Patch, error) {
	if err := ValidateKey(prefix, false); err != nil {
		return nil, err
	}
	patch := Patch{}
	if err := flattenInto(patch, prefix, value); err != nil {
		return nil, err
	}
	return patch, nil
}

func FlattenRoot(value any) (Patch, error) {
	patch := Patch{}
	switch typed := value.(type) {
	case map[string]any:
		for key, child := range typed {
			if err := ValidateKey(key, false); err != nil {
				return nil, err
			}
			if err := flattenInto(patch, key, child); err != nil {
				return nil, err
			}
		}
		return patch, nil
	case map[any]any:
		converted := map[string]any{}
		for key, child := range typed {
			str, ok := key.(string)
			if !ok {
				return nil, fmt.Errorf("invalid non-string map key %T", key)
			}
			converted[str] = child
		}
		return FlattenRoot(converted)
	default:
		return nil, errors.New("root output data must be a map")
	}
}

func ApplyPatch(dir string, patch Patch) error {
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return err
	}
	keys := make([]string, 0, len(patch))
	for key := range patch {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	for _, key := range keys {
		value := patch[key]
		if value == nil {
			if err := Unset(dir, key); err != nil {
				return err
			}
			continue
		}
		path := filepath.Join(dir, key)
		tmp := path + ".tmp"
		if err := os.WriteFile(tmp, []byte(*value), 0o644); err != nil {
			return err
		}
		if err := os.Rename(tmp, path); err != nil {
			return err
		}
	}
	return nil
}

func Unset(dir, key string) error {
	if err := ValidateKey(key, false); err != nil {
		return err
	}
	entries, err := os.ReadDir(dir)
	if errors.Is(err, os.ErrNotExist) {
		return nil
	}
	if err != nil {
		return err
	}
	prefix := key + "."
	for _, entry := range entries {
		name := entry.Name()
		if name == key || strings.HasPrefix(name, prefix) {
			if err := os.Remove(filepath.Join(dir, name)); err != nil && !errors.Is(err, os.ErrNotExist) {
				return err
			}
		}
	}
	return nil
}

func Load(dir string) (map[string]string, error) {
	entries, err := os.ReadDir(dir)
	if errors.Is(err, os.ErrNotExist) {
		return map[string]string{}, nil
	}
	if err != nil {
		return nil, err
	}
	out := map[string]string{}
	for _, entry := range entries {
		if entry.IsDir() {
			continue
		}
		name := entry.Name()
		if err := ValidateKey(name, false); err != nil {
			return nil, err
		}
		data, err := os.ReadFile(filepath.Join(dir, name))
		if err != nil {
			return nil, err
		}
		out[name] = string(data)
	}
	return out, nil
}

func ResolveValue(values map[string]string, key string) (string, bool, error) {
	if _, ok := values[key]; ok {
		return values[key], true, nil
	}
	if err := ValidateKey(key, true); err != nil {
		return "", false, err
	}
	parts := strings.Split(key, ".")
	last := parts[len(parts)-1]
	if !isNegativeIndex(last) {
		return "", false, nil
	}
	prefix := strings.Join(parts[:len(parts)-1], ".")
	indices := make([]int, 0)
	for candidate := range values {
		if !strings.HasPrefix(candidate, prefix+".") {
			continue
		}
		tail := strings.TrimPrefix(candidate, prefix+".")
		if strings.Contains(tail, ".") {
			continue
		}
		index, err := strconv.Atoi(tail)
		if err == nil && index >= 0 {
			indices = append(indices, index)
		}
	}
	if len(indices) == 0 {
		return "", false, nil
	}
	sort.Ints(indices)
	offset, _ := strconv.Atoi(last)
	indexPos := len(indices) + offset
	if indexPos < 0 || indexPos >= len(indices) {
		return "", false, fmt.Errorf("output index %s out of range for %s", last, prefix)
	}
	resolved := prefix + "." + strconv.Itoa(indices[indexPos])
	value, ok := values[resolved]
	return value, ok, nil
}

func parseBytes(data []byte, format string) (any, bool, error) {
	switch strings.ToLower(format) {
	case "env":
		return parseEnvFile(data)
	case "json":
		var decoded any
		if err := json.Unmarshal(data, &decoded); err != nil {
			return nil, false, err
		}
		return decoded, true, nil
	case "yaml", "yml":
		var decoded any
		if err := yaml.Unmarshal(data, &decoded); err != nil {
			return nil, false, err
		}
		return normalize(decoded), true, nil
	case "toml":
		decoded := map[string]any{}
		if err := toml.Unmarshal(data, &decoded); err != nil {
			return nil, false, err
		}
		return decoded, true, nil
	default:
		return nil, false, fmt.Errorf("unsupported output format: %s", format)
	}
}

func parseEnvFile(data []byte) (any, bool, error) {
	values := map[string]any{}
	scanner := bufio.NewScanner(strings.NewReader(string(data)))
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		key, value, ok := strings.Cut(line, "=")
		if !ok {
			return nil, false, fmt.Errorf("invalid env line: %q", line)
		}
		values[strings.TrimSpace(key)] = value
	}
	return values, true, scanner.Err()
}

func flattenInto(patch Patch, prefix string, value any) error {
	switch typed := normalize(value).(type) {
	case nil:
		patch[prefix] = nil
	case string:
		copy := typed
		patch[prefix] = &copy
	case bool:
		copy := strconv.FormatBool(typed)
		patch[prefix] = &copy
	case int:
		copy := strconv.Itoa(typed)
		patch[prefix] = &copy
	case int64:
		copy := strconv.FormatInt(typed, 10)
		patch[prefix] = &copy
	case float64:
		copy := strconv.FormatFloat(typed, 'f', -1, 64)
		patch[prefix] = &copy
	case []any:
		for i, child := range typed {
			if err := flattenInto(patch, fmt.Sprintf("%s.%d", prefix, i), child); err != nil {
				return err
			}
		}
	case map[string]any:
		for key, child := range typed {
			if err := ValidateKey(key, false); err != nil {
				return err
			}
			if err := flattenInto(patch, prefix+"."+key, child); err != nil {
				return err
			}
		}
	default:
		return fmt.Errorf("unsupported output value type %T", value)
	}
	return nil
}

func normalize(value any) any {
	switch typed := value.(type) {
	case map[any]any:
		normalized := map[string]any{}
		for key, child := range typed {
			if str, ok := key.(string); ok {
				normalized[str] = normalize(child)
			}
		}
		return normalized
	case map[string]any:
		normalized := map[string]any{}
		for key, child := range typed {
			normalized[key] = normalize(child)
		}
		return normalized
	case []any:
		normalized := make([]any, 0, len(typed))
		for _, child := range typed {
			normalized = append(normalized, normalize(child))
		}
		return normalized
	case []string:
		normalized := make([]any, 0, len(typed))
		for _, child := range typed {
			normalized = append(normalized, child)
		}
		return normalized
	default:
		return typed
	}
}

func isNegativeIndex(part string) bool {
	if !strings.HasPrefix(part, "-") || len(part) == 1 {
		return false
	}
	_, err := strconv.Atoi(part)
	return err == nil
}

func isTrivialValue(value string) bool {
	trimmed := strings.TrimSpace(value)
	if trimmed == "" {
		return true
	}
	switch trimmed[0] {
	case '{', '[':
		return false
	default:
		return true
	}
}
