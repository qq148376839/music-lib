package qq

import (
	"crypto/md5"
	"encoding/base64"
	"fmt"
	"strings"
)

// zzb sign algorithm constants.
var (
	zzbHeadIdx   = [8]int{21, 4, 9, 26, 16, 20, 27, 30}
	zzbTailIdx   = [8]int{18, 11, 3, 2, 1, 7, 6, 25}
	zzbMiddleXOR = [16]byte{212, 45, 80, 68, 195, 163, 163, 203, 157, 220, 254, 91, 204, 79, 104, 6}
)

// zzbSign computes the zzb request signature for QQ Music's musicu.fcg API.
// Input: raw JSON request body. Output: signature string like "zzb...".
func zzbSign(data string) string {
	hash := md5.Sum([]byte(data))
	hex := fmt.Sprintf("%X", hash) // 32-char uppercase hex

	h := extractBytes([]byte(hex), zzbHeadIdx[:])
	t := extractBytes([]byte(hex), zzbTailIdx[:])
	m := middle([]byte(hex))

	b64 := base64.StdEncoding.EncodeToString(m)

	var sb strings.Builder
	sb.WriteString("zzb")
	for _, b := range h {
		sb.WriteByte(b)
	}
	sb.WriteString(b64)
	for _, b := range t {
		sb.WriteByte(b)
	}

	result := strings.ToLower(sb.String())
	result = strings.ReplaceAll(result, "/", "")
	result = strings.ReplaceAll(result, "+", "")
	result = strings.ReplaceAll(result, "=", "")
	return result
}

// extractBytes picks bytes at the given positions from b.
func extractBytes(b []byte, positions []int) []byte {
	out := make([]byte, len(positions))
	for i, pos := range positions {
		out[i] = b[pos]
	}
	return out
}

// hexVal returns the numeric value of a hex character (0-9, A-F).
func hexVal(c byte) byte {
	switch {
	case c >= '0' && c <= '9':
		return c - '0'
	case c >= 'A' && c <= 'F':
		return c - 'A' + 10
	default:
		return 0
	}
}

// middle processes the MD5 hex string: pairs of hex chars → byte value → XOR with fixed table.
func middle(hex []byte) []byte {
	out := make([]byte, len(zzbMiddleXOR))
	for i := 0; i < len(hex); i += 2 {
		hi := hexVal(hex[i])
		lo := hexVal(hex[i+1])
		out[i/2] = (hi*16 ^ lo) ^ zzbMiddleXOR[i/2]
	}
	return out
}
