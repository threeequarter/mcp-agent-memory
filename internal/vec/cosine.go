package vec

import (
	"encoding/binary"
	"math"
	"sort"
)

func Pack(v []float32) []byte {
	b := make([]byte, len(v)*4)
	for i, f := range v {
		binary.LittleEndian.PutUint32(b[i*4:], math.Float32bits(f))
	}
	return b
}

func Unpack(b []byte) []float32 {
	v := make([]float32, len(b)/4)
	for i := range v {
		bits := binary.LittleEndian.Uint32(b[i*4:])
		v[i] = math.Float32frombits(bits)
	}
	return v
}

func Cosine(a, b []float32) float32 {
	var dot, normA, normB float64
	for i := range a {
		ai, bi := float64(a[i]), float64(b[i])
		dot += ai * bi
		normA += ai * ai
		normB += bi * bi
	}
	if normA == 0 || normB == 0 {
		return 0
	}
	return float32(dot / (math.Sqrt(normA) * math.Sqrt(normB)))
}

type Result struct {
	ID        int64
	Score     float32
	Embedding []byte
}

func TopK(query []float32, candidates []Result, k int) []Result {
	for i := range candidates {
		candidates[i].Score = Cosine(query, Unpack(candidates[i].Embedding))
	}
	sort.Slice(candidates, func(i, j int) bool {
		return candidates[i].Score > candidates[j].Score
	})
	if k > len(candidates) {
		k = len(candidates)
	}
	return candidates[:k]
}
