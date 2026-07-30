package domain

import "testing"

func TestRanker_Rank_OrdersByScoreThenLength(t *testing.T) {
	paths := []ReferralPath{
		{Score: 0.5, Length: 3},
		{Score: 0.9, Length: 4},
		{Score: 0.9, Length: 2},
		{Score: 0.1, Length: 1},
	}

	ranked := NewRanker().Rank(paths)

	want := []struct {
		score  float64
		length int
	}{
		{0.9, 2}, // higher score wins; tie broken by shorter length
		{0.9, 4},
		{0.5, 3},
		{0.1, 1},
	}
	if len(ranked) != len(want) {
		t.Fatalf("Rank() returned %d paths, want %d", len(ranked), len(want))
	}
	for i, w := range want {
		if ranked[i].Score != w.score || ranked[i].Length != w.length {
			t.Errorf("ranked[%d] = {score:%v len:%d}, want {score:%v len:%d}",
				i, ranked[i].Score, ranked[i].Length, w.score, w.length)
		}
	}
}

func TestRanker_Rank_DoesNotMutateInput(t *testing.T) {
	paths := []ReferralPath{{Score: 0.1}, {Score: 0.9}}
	_ = NewRanker().Rank(paths)
	if paths[0].Score != 0.1 || paths[1].Score != 0.9 {
		t.Errorf("Rank() mutated input slice: %+v", paths)
	}
}

func TestRanker_TopN(t *testing.T) {
	paths := []ReferralPath{
		{Score: 0.1}, {Score: 0.5}, {Score: 0.9}, {Score: 0.3},
	}

	top := NewRanker().TopN(paths, 2)
	if len(top) != 2 {
		t.Fatalf("TopN(2) returned %d paths, want 2", len(top))
	}
	if top[0].Score != 0.9 || top[1].Score != 0.5 {
		t.Errorf("TopN(2) = %+v, want scores [0.9, 0.5]", top)
	}
}

func TestRanker_TopN_NLargerThanInput(t *testing.T) {
	paths := []ReferralPath{{Score: 0.1}, {Score: 0.9}}
	top := NewRanker().TopN(paths, 10)
	if len(top) != 2 {
		t.Fatalf("TopN(10) returned %d paths, want 2 (all of them)", len(top))
	}
}

func TestRanker_TopN_Empty(t *testing.T) {
	top := NewRanker().TopN(nil, 5)
	if len(top) != 0 {
		t.Errorf("TopN(nil, 5) = %+v, want empty", top)
	}
}
