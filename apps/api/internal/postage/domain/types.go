package domain

const (
	// GlobalColdStartThreshold is the minimum total applications (rows with an
	// `applied` event) before the feature shows any personalised observed rate.
	// Below it the feature shows the documented prior.
	GlobalColdStartThreshold = 30

	// PerBucketMinSample is the minimum applications in a bucket before that
	// bucket shows its own observed rate. Below it the bucket shows
	// "not enough data".
	PerBucketMinSample = 8

	// DocumentedPriorRate is a conservative industry-baseline job-application
	// response rate used only for the overall view during global cold-start.
	// Source: placeholder — cited source TBD per spec el-37xa.
	DocumentedPriorRate = 0.20

	// DocumentedPriorLabel is the label rendered alongside the prior rate so
	// the user knows it is a baseline, not their personal rate.
	DocumentedPriorLabel = "Typical baseline — not yet your data"
)
