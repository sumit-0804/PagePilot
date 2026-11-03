package embeddings

import (
	"context"
	"math"
	"testing"
)

func TestBuiltinEmbedder(t *testing.T) {
	emb := NewBuiltinEmbedder(128)
	if emb.Dimension() != 128 {
		t.Fatalf("expected dimension 128, got %d", emb.Dimension())
	}

	texts := []string{
		"Gradient descent optimizes loss functions in deep learning.",
		"Convolutional neural networks excel at computer vision tasks.",
		"Gradient descent finds local minima.",
	}

	vecs, err := emb.EmbedDocuments(context.Background(), texts)
	if err != nil {
		t.Fatalf("embedding failed: %v", err)
	}

	if len(vecs) != 3 {
		t.Fatalf("expected 3 vectors, got %d", len(vecs))
	}

	// Verify L2 normalization
	for i, v := range vecs {
		if len(v) != 128 {
			t.Errorf("vector %d: expected length 128, got %d", i, len(v))
		}
		var norm float64
		for _, val := range v {
			norm += float64(val * val)
		}
		if math.Abs(norm-1.0) > 1e-4 {
			t.Errorf("vector %d: expected L2 norm ~1.0, got %f", i, norm)
		}
	}
}
