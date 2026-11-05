package eval

import "math"

// RetrievalScores are the ranking metrics for one query. Relevance is binary: a retrieved chunk
// is relevant when it contains one of the case's evidence strings.
type RetrievalScores struct {
	RecallAtK    float64 `json:"recall_at_k"`    // share of evidence strings found in the top k
	PrecisionAtK float64 `json:"precision_at_k"` // share of the top k that is relevant
	MRR          float64 `json:"mrr"`            // 1 / rank of the first relevant chunk, 0 if none
	NDCGAtK      float64 `json:"ndcg_at_k"`      // rank-discounted gain against an ideal ordering
	FirstHitRank int     `json:"first_hit_rank"` // 1-based; 0 when nothing relevant came back
}

// ScoreRetrieval scores ranked chunk texts (best first) against the evidence, looking at the top k.
func ScoreRetrieval(chunks []string, evidence []string, k int) RetrievalScores {
	if k <= 0 || len(evidence) == 0 {
		return RetrievalScores{}
	}
	if len(chunks) > k {
		chunks = chunks[:k]
	}

	var s RetrievalScores
	found := make([]bool, len(evidence))
	relevant := 0
	dcg := 0.0
	for i, chunk := range chunks {
		hit := false
		for j, e := range evidence {
			if Contains(chunk, e) {
				found[j] = true
				hit = true
			}
		}
		if !hit {
			continue
		}
		relevant++
		dcg += 1 / math.Log2(float64(i)+2)
		if s.FirstHitRank == 0 {
			s.FirstHitRank = i + 1
			s.MRR = 1 / float64(i+1)
		}
	}

	nFound := 0
	for _, f := range found {
		if f {
			nFound++
		}
	}
	s.RecallAtK = float64(nFound) / float64(len(evidence))
	s.PrecisionAtK = float64(relevant) / float64(k)

	// The ideal ranking puts one relevant chunk per evidence string at the top.
	idcg := 0.0
	for i := 0; i < min(len(evidence), k); i++ {
		idcg += 1 / math.Log2(float64(i)+2)
	}
	s.NDCGAtK = math.Min(1, dcg/idcg)
	return s
}

// Mean averages values; it returns 0 for an empty slice.
func Mean(values []float64) float64 {
	if len(values) == 0 {
		return 0
	}
	sum := 0.0
	for _, v := range values {
		sum += v
	}
	return sum / float64(len(values))
}
