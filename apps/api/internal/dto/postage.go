package dto

type PostAgeBucketState string

const (
	PostAgeStateObserved     PostAgeBucketState = "observed"
	PostAgeStatePrior        PostAgeBucketState = "prior"
	PostAgeStateInsufficient PostAgeBucketState = "insufficient"
)

type PostAgeBucketDto struct {
	Bucket    string             `json:"bucket"`
	N         int32              `json:"n"`
	Responses int32              `json:"responses"`
	Rate      *float64           `json:"rate"`
	State     PostAgeBucketState `json:"state"`
}

type PostAgeResponseDto struct {
	Buckets      []PostAgeBucketDto `json:"buckets"`
	TotalApps    int32              `json:"totalApps"`
	GlobalState  PostAgeBucketState `json:"globalState"`
	PriorRate    float64            `json:"priorRate"`
	PriorLabel   string             `json:"priorLabel"`
	ThresholdMsg *string            `json:"thresholdMsg,omitempty"`
}
