// Package cursor implements opaque pagination cursors for MCP tools.
// A cursor encodes an offset into a result stream plus a query hash so stale
// cursors from different queries fail cleanly.
//
// Encoding: base64url(json({"offset":N,"query_hash":"hex"}))
// The query_hash is a hex-encoded sha256 over the deterministic query params.
package cursor

import (
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"fmt"
	"sort"
	"strings"

	json "github.com/goccy/go-json"
)

// payload is the JSON structure inside a cursor.
type payload struct {
	Offset    int    `json:"offset"`
	QueryHash string `json:"query_hash"`
}

// Encode creates an opaque cursor string from an offset and query hash.
func Encode(offset int, queryHash string) string {
	p := payload{Offset: offset, QueryHash: queryHash}
	b, _ := json.Marshal(p)
	return base64.RawURLEncoding.EncodeToString(b)
}

// Decode parses a cursor string. Returns offset, queryHash, error.
func Decode(cursor string) (int, string, error) {
	b, err := base64.RawURLEncoding.DecodeString(cursor)
	if err != nil {
		return 0, "", fmt.Errorf("invalid cursor encoding: %w", err)
	}
	var p payload
	if err := json.Unmarshal(b, &p); err != nil {
		return 0, "", fmt.Errorf("invalid cursor payload: %w", err)
	}
	return p.Offset, p.QueryHash, nil
}

// HashParams computes a deterministic query hash from a set of key=value params.
// Keys are sorted before hashing so order doesn't matter.
func HashParams(params map[string]string) string {
	keys := make([]string, 0, len(params))
	for k := range params {
		keys = append(keys, k)
	}
	sort.Strings(keys)

	var sb strings.Builder
	for _, k := range keys {
		sb.WriteString(k)
		sb.WriteByte('=')
		sb.WriteString(params[k])
		sb.WriteByte('\n')
	}

	sum := sha256.Sum256([]byte(sb.String()))
	return hex.EncodeToString(sum[:])
}
