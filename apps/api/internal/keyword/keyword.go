package keyword

import (
	"github.com/job-finder/api/internal/keyword/application"
	"github.com/job-finder/api/internal/keyword/domain"
	"github.com/job-finder/api/internal/keyword/infrastructure/rephraseadapter"
)

type (
	DiffService            = application.DiffService
	Suggester              = application.Suggester
	CachedRephraser        = application.CachedRephraser
	RephraseSuggestion     = application.RephraseSuggestion
	RephraseModel          = application.RephraseModel
	Rephraser              = application.Rephrase
	BulletsProvider        = application.BulletsProvider
	DiffReader             = application.DiffReader
	LexicalEvidenceFinder  = application.LexicalEvidenceFinder
	EvidenceFinder         = application.EvidenceFinder
	ProviderRephraseModel  = rephraseadapter.ProviderRephraseModel

	Proximity       = domain.Proximity
	DiffResult      = domain.DiffResult
	DiffTerm        = domain.DiffTerm
	DiffMetadata    = domain.DiffMetadata
	Differ          = domain.Differ
	Polarity        = domain.Polarity
	ExtractedTerm   = domain.ExtractedTerm
	ExtractResult   = domain.ExtractResult
	Extractor       = domain.Extractor
	RegexExtractor  = domain.RegexExtractor
	ClassifiedTerm  = domain.ClassifiedTerm
	StarStory       = domain.StarStory
	StoryMapping    = domain.StoryMapping
)

var (
	NewDiffService           = application.NewDiffService
	NewSuggester             = application.NewSuggester
	NewCachedRephraser       = application.NewCachedRephraser
	NewProviderRephraseModel = rephraseadapter.NewProviderRephraseModel
	ErrDiffNotFound          = application.ErrDiffNotFound
	ReasonNoHonestRephrase   = application.ReasonNoHonestRephrase
	DefaultRephraseCacheTTL  = application.DefaultRephraseCacheTTL

	ProximityClose    = domain.ProximityClose
	ProximityModerate = domain.ProximityModerate
	ProximityDistant  = domain.ProximityDistant
	Adjacent          = domain.Adjacent
	NewDiffer         = domain.NewDiffer
	PolarityRequired  = domain.PolarityRequired
	PolarityPreferred = domain.PolarityPreferred
	NewExtractor      = domain.NewExtractor
	ClassMustHave     = domain.ClassMustHave
	DeriveQuestions   = domain.DeriveQuestions
	SelectStories     = domain.SelectStories
	TokenRe           = domain.TokenRe
	LowerASCII        = domain.LowerASCII
	FilterStopwords   = domain.FilterStopwords
	Stem              = domain.Stem
	ExtractResumeTerms = domain.ExtractResumeTerms
)
