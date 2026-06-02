package fuzzy

import (
	"fmt"
	"hash/fnv"
	"io"
	"math/bits"
	"os"
	"path/filepath"
)

const shingleSize = 4

// SignatureFromFile computes a 64-bit simhash signature from file content.
// At most maxReadBytes are sampled from the file prefix.
func SignatureFromFile(path string, maxReadBytes int64) (uint64, error) {
	if path == "" {
		return 0, fmt.Errorf("empty path")
	}
	if maxReadBytes <= 0 {
		maxReadBytes = DefaultMaxReadBytes
	}

	cleanPath := filepath.Clean(path)
	dir := filepath.Dir(cleanPath)
	fileName := filepath.Base(cleanPath)
	if fileName == "" || fileName == "." {
		return 0, fmt.Errorf("invalid file path: %s", path)
	}

	root, err := os.OpenRoot(dir)
	if err != nil {
		return 0, err
	}
	defer root.Close()

	f, err := root.Open(fileName)
	if err != nil {
		return 0, err
	}
	defer f.Close()

	buf, err := io.ReadAll(io.LimitReader(f, maxReadBytes))
	if err != nil {
		return 0, err
	}
	if len(buf) == 0 {
		return 0, fmt.Errorf("empty file")
	}

	return signatureFromBytes(buf), nil
}

func signatureFromBytes(data []byte) uint64 {
	var weights [64]int

	if len(data) < shingleSize {
		for _, b := range data {
			hv := hashToken([]byte{b})
			accumulateWeights(&weights, hv)
		}
		return flattenWeights(&weights)
	}

	for i := 0; i <= len(data)-shingleSize; i++ {
		hv := hashToken(data[i : i+shingleSize])
		accumulateWeights(&weights, hv)
	}

	return flattenWeights(&weights)
}

func hashToken(token []byte) uint64 {
	h := fnv.New64a()
	_, _ = h.Write(token)
	return h.Sum64()
}

func accumulateWeights(weights *[64]int, hv uint64) {
	for bit := 0; bit < 64; bit++ {
		mask := uint64(1) << bit
		if hv&mask != 0 {
			weights[bit]++
			continue
		}
		weights[bit]--
	}
}

func flattenWeights(weights *[64]int) uint64 {
	var out uint64
	for bit := 0; bit < 64; bit++ {
		if weights[bit] > 0 {
			out |= uint64(1) << bit
		}
	}
	return out
}

// HammingDistance returns the bitwise Hamming distance between two signatures.
func HammingDistance(a, b uint64) int {
	return bits.OnesCount64(a ^ b)
}

// SimilarityFromDistance converts a 64-bit Hamming distance to a percentage score.
func SimilarityFromDistance(distance int) int {
	if distance <= 0 {
		return 100
	}
	if distance >= 64 {
		return 0
	}
	remaining := 64 - distance
	return int(float64(remaining) / 64.0 * 100.0)
}
