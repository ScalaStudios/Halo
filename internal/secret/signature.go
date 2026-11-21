package secret

import (
	"encoding/hex"
	"slices"
	"strconv"
	"strings"
	"time"
)

const SignatureTolerance = 5 * time.Minute

func VerifySignature(key []byte, header string, body []byte, now time.Time) bool {
	var stamp string
	var signatures [][]byte
	for part := range strings.SplitSeq(header, ",") {
		k, v, _ := strings.Cut(strings.TrimSpace(part), "=")
		switch k {
		case "t":
			stamp = v
		case "v1":
			if mac, err := hex.DecodeString(v); err == nil {
				signatures = append(signatures, mac)
			}
		}
	}
	at, err := strconv.ParseInt(stamp, 10, 64)
	if err != nil || now.Sub(time.Unix(at, 0)).Abs() > SignatureTolerance {
		return false
	}
	want := MAC(key, stamp+"."+string(body))
	return slices.ContainsFunc(signatures, func(mac []byte) bool { return Equal(mac, want) })
}
