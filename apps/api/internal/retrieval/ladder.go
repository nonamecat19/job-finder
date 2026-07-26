package retrieval

type RetrievalMethod struct {
	Key   string
	Order int
}

var (
	RungDirect       = RetrievalMethod{Key: "direct", Order: 0}
	RungBrowser      = RetrievalMethod{Key: "browser", Order: 1}
	RungFlareSolverr = RetrievalMethod{Key: "flaresolverr", Order: 2}
)

var AllRungs = []RetrievalMethod{RungDirect, RungBrowser, RungFlareSolverr}

func RungForKey(key string) (RetrievalMethod, bool) {
	for _, r := range AllRungs {
		if r.Key == key {
			return r, true
		}
	}
	return RetrievalMethod{}, false
}

func (r RetrievalMethod) Next() (RetrievalMethod, bool) {
	nextOrder := r.Order + 1
	for _, candidate := range AllRungs {
		if candidate.Order == nextOrder {
			return candidate, true
		}
	}
	return RetrievalMethod{}, false
}

func (r RetrievalMethod) Available() bool {
	return !r.IsEmpty()
}

func (r RetrievalMethod) IsEmpty() bool {
	return r.Key == "" && r.Order == 0
}

func MaxRungForAccount(usesUserAccount bool) RetrievalMethod {
	if usesUserAccount {
		return RungDirect
	}
	return RungFlareSolverr
}
