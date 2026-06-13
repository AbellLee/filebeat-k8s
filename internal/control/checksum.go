package control

import (
	"crypto/sha256"
	"encoding/hex"
	"sort"
)

func ConfigSetChecksum(files []ConfigFile) string {
	sorted := append([]ConfigFile(nil), files...)
	sort.Slice(sorted, func(i, j int) bool {
		return sorted[i].Filename < sorted[j].Filename
	})
	h := sha256.New()
	for _, f := range sorted {
		_, _ = h.Write([]byte(f.Filename))
		_, _ = h.Write([]byte{0})
		_, _ = h.Write([]byte(f.Content))
		_, _ = h.Write([]byte{0})
	}
	return "sha256:" + hex.EncodeToString(h.Sum(nil))
}

func ContentChecksum(content string) string {
	sum := sha256.Sum256([]byte(content))
	return "sha256:" + hex.EncodeToString(sum[:])
}
